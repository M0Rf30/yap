// Package rpm provides functionality for building RPM packages from PKGBUILD specifications.
package rpm

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/M0Rf30/yap/v2/pkg/builders/common"
	"github.com/M0Rf30/yap/v2/pkg/errors"
	"github.com/M0Rf30/yap/v2/pkg/files"
	"github.com/M0Rf30/yap/v2/pkg/i18n"
	"github.com/M0Rf30/yap/v2/pkg/logger"
	"github.com/M0Rf30/yap/v2/pkg/pkgbuild"

	rpmpack "github.com/M0Rf30/rpmpack"
)

const rootOwner = "root"

// RPM represents a RPM package.
//
// It contains the directory path of the package and the PKGBUILD struct, which
// contains the metadata and build instructions for the package.
type RPM struct {
	*common.BaseBuilder
	compression string
}

// NewBuilder creates a new RPM package builder with optional compression setting.
// If compression is empty, defaults to "zstd".
func NewBuilder(pkgBuild *pkgbuild.PKGBUILD, compression string) *RPM {
	if compression == "" {
		compression = "zstd"
	}

	return &RPM{
		BaseBuilder: common.NewBaseBuilder(pkgBuild, "rpm"),
		compression: compression,
	}
}

// BuildPackage creates an RPM package based on the provided PKGBUILD information.
// Returns the path to the created RPM file.
func (r *RPM) BuildPackage(ctx context.Context, artifactsPath string, targetArch string) (string, error) {
	r.SetTargetArchitecture(targetArch)

	pkgName := fmt.Sprintf("%s-%s-%s.%s.rpm",
		r.PKGBUILD.PkgName,
		r.PKGBUILD.PkgVer,
		r.PKGBUILD.PkgRel,
		r.PKGBUILD.ArchComputed)

	epoch, _ := strconv.ParseUint(r.PKGBUILD.Epoch, 10, 32)
	if epoch == 0 {
		epoch = uint64(rpmpack.NoEpoch)
	}

	license := strings.Join(r.PKGBUILD.License, " ")

	pkgFilePath := filepath.Join(artifactsPath, pkgName)

	// Pre-compute all dependency relations before creating RPM metadata
	rel, err := r.buildDependencyRelations()
	if err != nil {
		return "", err
	}

	rpm, err := rpmpack.NewRPM(rpmpack.RPMMetaData{
		Name:        r.PKGBUILD.PkgName,
		Summary:     r.PKGBUILD.PkgDesc,
		Description: r.PKGBUILD.PkgDesc,
		Epoch:       uint32(epoch),
		Version:     r.PKGBUILD.PkgVer,
		Release:     r.PKGBUILD.PkgRel,
		Arch:        r.PKGBUILD.ArchComputed,
		URL:         r.PKGBUILD.URL,
		Packager:    r.PKGBUILD.Maintainer,
		Group:       r.PKGBUILD.Section,
		Compressor:  r.compression,
		Licence:     license,
		// BuildHost is fixed for reproducibility — rpmpack would otherwise
		// embed the build machine's hostname, leaking build-infra detail
		// and breaking byte-identical rebuilds.
		BuildHost: "yap",
		// Distribution is the target distro flavour the package was built for.
		// FullDistroName looks like "rocky_9" / "ubuntu_jammy"; close enough
		// to what `rpm -qi`'s Distribution tag is meant to convey.
		Distribution: r.PKGBUILD.FullDistroName,
		// BugURL maps directly from the `bugs=` PKGBUILD scalar (also wired
		// to deb Bugs:).
		BugURL:      r.PKGBUILD.Bugs,
		Obsoletes:   rel.obsoletes,
		Provides:    rel.provides,
		Requires:    rel.requires,
		Conflicts:   rel.conflicts,
		Recommends:  rel.recommends,
		Suggests:    rel.suggests,
		Enhances:    rel.enhances,
		Supplements: rel.supplements,
		BuildTime:   files.SourceDateEpochFromEnv(),
	})
	if err != nil {
		return "", errors.Wrap(err, errors.ErrTypePackaging, "failed to create RPM metadata").
			WithOperation("BuildPackage")
	}

	err = r.createFilesInsideRPM(rpm)
	if err != nil {
		return "", err
	}

	r.addScriptlets(rpm)

	r.addChangelog(rpm)

	cleanFilePath := filepath.Clean(pkgFilePath)

	rpmFile, err := os.Create(cleanFilePath)
	if err != nil {
		return "", err
	}

	defer func() {
		err := rpmFile.Close()
		if err != nil {
			logger.Warn(i18n.T("logger.rpm.warn.failed_to_close_rpm"), "path", cleanFilePath,
				"error", err)
		}
	}()

	err = rpm.Write(rpmFile)
	if err != nil {
		return "", err
	}

	// Log package creation using common functionality
	r.LogPackageCreated(cleanFilePath)

	return cleanFilePath, nil
}

// PrepareFakeroot sets up the environment for building an RPM package in a fakeroot context.
// It retrieves architecture, group, and release information, sets the package destination,
// cleans up the RPM directory, creates necessary directories, and gathers files.
// It also processes package dependencies and creates the RPM spec file, returning
// an error if any step fails.
func (r *RPM) PrepareFakeroot(ctx context.Context, _ string, targetArch string) error {
	r.getGroup()
	r.getRelease()
	r.LogCrossCompilation(targetArch)
	r.SetTargetArchitecture(targetArch)

	return r.ApplyOptionsWithEnv(r.CrossStripEnvMap(targetArch))
}

// createFilesInsideRPM prepares and adds files to the specified RPM object.
// It retrieves backup files, walks through the package directory, and adds the contents to the RPM.
func (r *RPM) createFilesInsideRPM(rpm *rpmpack.RPM) error {
	backupFiles := r.PrepareBackupFilePaths()

	originalBackup := r.PKGBUILD.Backup
	r.PKGBUILD.Backup = backupFiles

	defer func() {
		r.PKGBUILD.Backup = originalBackup
	}()

	walker := r.CreateFileWalker()

	entries, err := walker.Walk()
	if err != nil {
		return err
	}

	var contents []*files.Entry

	contents = append(contents, entries...)

	return addContentsToRPM(contents, rpm)
}

// addContentsToRPM adds a slice of Entry objects to the specified RPM object.
// It creates RPMFile objects from the Entry and adds them to the RPM.
// Intermediate directories (those that are ancestors of other entries) are
// skipped to avoid polluting the RPM file list with implicit path components.
func addContentsToRPM(contents []*files.Entry, rpm *rpmpack.RPM) error {
	// Build a set of all non-directory destinations for fast lookup.
	// A directory is "intermediate" if any other entry's destination
	// starts with that directory path + "/".
	dirHasChildren := make(map[string]bool)

	for _, content := range contents {
		if content.Type != files.TypeDir {
			// Walk up the path and mark every parent as having children.
			dir := filepath.Dir(filepath.Clean(content.Destination))
			for dir != "/" && dir != "." {
				dirHasChildren[dir] = true
				dir = filepath.Dir(dir)
			}
		}
	}

	for _, content := range contents {
		// Skip intermediate directories — only include empty dirs.
		if content.Type == files.TypeDir {
			dest := filepath.Clean(content.Destination)
			if dirHasChildren[dest] {
				continue
			}
		}

		file, err := createRPMFile(content)
		if err != nil {
			return err
		}

		file.Name = filepath.Clean(file.Name)
		rpm.AddFile(*file)
	}

	return nil
}

// addScriptlets adds pre-install, post-install, pre-remove and post-remove
// scripts from the PKGBUILD to the RPM package if they are defined.
//
// It takes a pointer to the rpmpack.RPM instance as a parameter.
func (r *RPM) addScriptlets(rpm *rpmpack.RPM) {
	// This string is appended to preun and postun directives
	// to have a similar behaviour between deb and rpm.
	onlyOnUninstall := "if [ $1 -ne 0 ]; then exit 0; fi\n"

	if r.PKGBUILD.PreTrans != "" {
		rpm.AddPretrans(r.PrepareScriptletWithHelpers(r.PKGBUILD.PreTrans))
	}

	if r.PKGBUILD.PreInst != "" {
		rpm.AddPrein(r.PrepareScriptletWithHelpers(r.PKGBUILD.PreInst))
	}

	if r.PKGBUILD.PostInst != "" {
		rpm.AddPostin(r.PrepareScriptletWithHelpers(r.PKGBUILD.PostInst))
	}

	if r.PKGBUILD.PreRm != "" {
		rpm.AddPreun(r.PrepareScriptletWithHelpers(onlyOnUninstall + r.PKGBUILD.PreRm))
	}

	if r.PKGBUILD.PostRm != "" {
		rpm.AddPostun(r.PrepareScriptletWithHelpers(onlyOnUninstall + r.PKGBUILD.PostRm))
	}

	if r.PKGBUILD.PostTrans != "" {
		rpm.AddPosttrans(r.PrepareScriptletWithHelpers(r.PKGBUILD.PostTrans))
	}
}

// addChangelog adds changelog entries to the RPM package if a changelog is
// specified in the PKGBUILD. It reads the changelog file, parses it into
// ChangelogEntry objects, and sets them on the RPM metadata.
func (r *RPM) addChangelog(rpm *rpmpack.RPM) {
	changelogData, err := r.PKGBUILD.ReadChangelog()
	if err != nil {
		logger.Warn(i18n.T("logger.rpm.warn.failed_read_changelog_rpm"), "error", err)

		return
	}

	if changelogData == nil {
		return
	}

	entries := parseRPMChangelog(changelogData)
	rpm.Changelog = entries
}

// asRPMDirectory creates an RPMFile object for a directory based on the provided Entry.
// It retrieves the directory's modification time and sets the appropriate fields in the RPMFile.
func asRPMDirectory(entry *files.Entry) (*rpmpack.RPMFile, error) {
	// Get file information for the directory specified in the entry.
	fileInfo, err := os.Stat(filepath.Clean(entry.Source))
	if err != nil {
		return nil, errors.Wrap(err, errors.ErrTypeFileSystem, fmt.Sprintf("stat directory %q", entry.Source)).
			WithOperation("asRPMDirectory").
			WithContext("path", entry.Source)
	}

	// Retrieve the modification time of the directory.
	mTime, err := extractFileModTimeUint32(fileInfo)
	if err != nil {
		return nil, err
	}

	// Create and return an RPMFile object for the directory.
	return &rpmpack.RPMFile{
		Name: entry.Destination, // Set the destination name.
		// Set the mode to indicate it's a directory.
		Mode:  uint(fileInfo.Mode()) | files.TagDirectory,
		MTime: mTime,     // Set the modification time.
		Owner: rootOwner, // Set the owner to "root".
		Group: rootOwner, // Set the group to "root".
	}, nil
}

// asRPMFile creates an RPMFile object for a regular file based on the provided Entry.
// It reads the file's data and retrieves its modification time.
func asRPMFile(
	entry *files.Entry,
	fileType rpmpack.FileType) (*rpmpack.RPMFile, error) {
	// Read the file data from the source path.
	data, err := os.ReadFile(entry.Source)
	if err != nil {
		return nil, err // Return nil and the error if reading the file fails.
	}

	cleanFilePath := filepath.Clean(entry.Source)

	fileInfo, err := os.Stat(cleanFilePath)
	if err != nil {
		return nil, errors.Wrap(err, errors.ErrTypeFileSystem, fmt.Sprintf("stat file %q", entry.Source)).
			WithOperation("asRPMFile").
			WithContext("path", entry.Source)
	}

	// Retrieve the modification time of the file.
	mTime, err := extractFileModTimeUint32(fileInfo)
	if err != nil {
		return nil, err
	}

	// Create and return an RPMFile object for the regular file.
	return &rpmpack.RPMFile{
		Name:  entry.Destination,     // Set the destination name.
		Body:  data,                  // Set the file data.
		Mode:  uint(fileInfo.Mode()), // Set the file mode.
		MTime: mTime,                 // Set the modification time.
		Owner: rootOwner,             // Set the owner to "root".
		Group: rootOwner,             // Set the group to "root".
		Type:  fileType,              // Set the file type.
	}, nil
}

// asRPMSymlink creates an RPMFile object for a symbolic link based on the provided Entry.
// It retrieves the link's target and modification time.
func asRPMSymlink(entry *files.Entry) (*rpmpack.RPMFile, error) {
	cleanFilePath := filepath.Clean(entry.Source)

	fileInfo, err := os.Lstat(cleanFilePath) // Use Lstat to get information about the symlink.
	if err != nil {
		return nil, errors.Wrap(err, errors.ErrTypeFileSystem, fmt.Sprintf("lstat symlink %q", entry.Source)).
			WithOperation("asRPMSymlink").
			WithContext("path", entry.Source)
	}

	body, err := os.Readlink(cleanFilePath) // Read the target of the symlink.
	if err != nil {
		return nil, errors.Wrap(err, errors.ErrTypeFileSystem, fmt.Sprintf("readlink %q", entry.Source)).
			WithOperation("asRPMSymlink").
			WithContext("path", entry.Source)
	}

	// Retrieve the modification time of the symlink.
	mTime, err := extractFileModTimeUint32(fileInfo)
	if err != nil {
		return nil, err
	}

	// Create and return an RPMFile object for the symlink.
	return &rpmpack.RPMFile{
		Name:  entry.Destination,   // Set the destination name.
		Body:  []byte(body),        // Set the target of the symlink as the body.
		Mode:  uint(files.TagLink), // Set the mode to indicate it's a symlink.
		MTime: mTime,               // Set the modification time.
		Owner: rootOwner,           // Set the owner to "root".
		Group: rootOwner,           // Set the group to "root".
	}, nil
}

// createRPMFile converts an Entry object into an RPMFile object based on its type.
// It returns the created RPMFile and any error encountered during the conversion.
func createRPMFile(entry *files.Entry) (*rpmpack.RPMFile, error) {
	var file *rpmpack.RPMFile

	var err error

	switch entry.Type {
	case files.TypeConfigNoReplace:
		file, err = asRPMFile(entry, rpmpack.ConfigFile|rpmpack.NoReplaceFile)
	case files.TypeSymlink:
		file, err = asRPMSymlink(entry)
	case files.TypeDir:
		file, err = asRPMDirectory(entry)
	case files.TypeFile:
		file, err = asRPMFile(entry, rpmpack.GenericFile)
	}

	return file, err
}

// getGroup updates the section of the RPM struct with the corresponding
// value from the RPMGroups map.
//
// No parameters.
// No return types.
func (r *RPM) getGroup() {
	r.PKGBUILD.Section = RPMGroups[r.PKGBUILD.Section]
}

// extractFileModTimeUint32 retrieves the modification time of a file and converts it to uint32.
// It checks for overflow and returns an error if the time is out of range for uint32.
func extractFileModTimeUint32(fileInfo os.FileInfo) (uint32, error) {
	mTime := fileInfo.ModTime().Unix()
	if mTime < 0 || mTime > int64(^uint32(0)) {
		return 0, errors.New(errors.ErrTypePackaging,
			i18n.T("errors.rpm.modification_time_out_of_range")).
			WithOperation("extractFileModTimeUint32").
			WithContext("time", mTime)
	}

	return uint32(mTime), nil //nolint:gosec // range validated above
}

// getRelease updates the release information with RPM distribution suffix.
// It uses the common FormatRelease method with RPM-specific distro mappings.
func (r *RPM) getRelease() {
	r.FormatRelease(RPMDistros)
}

// rpmRelations bundles all dependency-relation flavors consumed by rpmpack.NewRPM.
// Grouping them avoids 7 sequential local variables in BuildPackage and makes the
// build-up phase trivially testable in isolation.
type rpmRelations struct {
	obsoletes, provides, requires, conflicts    rpmpack.Relations
	recommends, suggests, enhances, supplements rpmpack.Relations
}

// buildDependencyRelations computes every dependency relation needed by the RPM
// metadata in one pass, short-circuiting on the first parse error.
func (r *RPM) buildDependencyRelations() (rpmRelations, error) {
	var rel rpmRelations

	specs := []struct {
		dst  *rpmpack.Relations
		deps []string
	}{
		{&rel.obsoletes, r.PKGBUILD.Replaces},
		{&rel.provides, r.PKGBUILD.Provides},
		{&rel.requires, r.PKGBUILD.Depends},
		{&rel.conflicts, r.PKGBUILD.Conflicts},
		{&rel.recommends, r.PKGBUILD.OptDepends},
		{&rel.suggests, r.PKGBUILD.Suggests},
		{&rel.enhances, r.PKGBUILD.Enhances},
		{&rel.supplements, r.PKGBUILD.Supplements},
	}

	for _, s := range specs {
		rels, err := r.processDepends(s.deps)
		if err != nil {
			return rpmRelations{}, err
		}

		*s.dst = rels
	}

	return rel, nil
}

// processDepends converts dependency strings to rpmpack.Relations format.
// It uses the common BaseBuilder.ProcessDependencies for consistent version
// operator handling, then converts the result to RPM-specific Relations type.
// Returns an error if any dependency string is invalid.
func (r *RPM) processDepends(depends []string) (rpmpack.Relations, error) {
	processed := r.ProcessDependencies(depends)
	relations := make(rpmpack.Relations, 0)

	for _, dep := range processed {
		if err := relations.Set(dep); err != nil {
			return nil, errors.Wrap(err, errors.ErrTypePackaging,
				"invalid dependency string").
				WithOperation("processDepends").
				WithContext("dependency", dep)
		}
	}

	return relations, nil
}

// parseRPMChangelog parses raw changelog data in RPM format into ChangelogEntry objects.
// The expected format is:
//
//   - Wed Jan 01 2025 Author Name <email@example.com> - 1.0-1
//
//   - Change description
//
//   - Another change
//
//   - Mon Dec 01 2024 Author Name <email@example.com> - 0.9-1
//
//   - Previous change
//
// Each entry block starts with "* " followed by a date, author, and version.
// Subsequent lines starting with "- " are change descriptions.
// If date parsing fails, time.Now() is used as fallback.
func parseRPMChangelog(data []byte) []rpmpack.ChangelogEntry {
	var entries []rpmpack.ChangelogEntry

	scanner := bufio.NewScanner(bytes.NewReader(data))

	var currentEntry *rpmpack.ChangelogEntry

	var textLines []string

	for scanner.Scan() {
		line := scanner.Text()

		// Check if this is a header line (starts with "* ")
		if strings.HasPrefix(line, "* ") {
			// Save the previous entry if it exists
			if currentEntry != nil {
				currentEntry.Text = strings.Join(textLines, "\n")
				entries = append(entries, *currentEntry)
				textLines = nil
			}

			// Parse the new header line
			currentEntry = parseChangelogHeader(line)
		} else if currentEntry != nil && strings.HasPrefix(line, "- ") {
			// This is a change description line
			textLines = append(textLines, line)
		}
	}

	// Don't forget the last entry
	if currentEntry != nil {
		currentEntry.Text = strings.Join(textLines, "\n")
		entries = append(entries, *currentEntry)
	}

	return entries
}

// parseChangelogHeader parses a changelog header line in the format:
// * Wed Jan 01 2025 Author Name <email@example.com> - 1.0-1
// Returns a ChangelogEntry with the parsed date and author.
// If date parsing fails, uses time.Now() as fallback.
func parseChangelogHeader(line string) *rpmpack.ChangelogEntry {
	// Remove the leading "* "
	line = strings.TrimPrefix(line, "* ")

	// Find the " - " separator that precedes the version
	versionSepIdx := strings.LastIndex(line, " - ")
	if versionSepIdx == -1 {
		// No version separator found, treat entire line as date + author
		versionSepIdx = len(line)
	}

	dateAndAuthor := line[:versionSepIdx]

	// Try to parse the date from the beginning of dateAndAuthor
	// RPM changelog format: "Mon Jan 02 2006 Author Name <email>"
	// We need to extract the first 4 tokens as the date
	parts := strings.Fields(dateAndAuthor)
	if len(parts) < 4 {
		// Not enough parts for a valid date, use current time
		return &rpmpack.ChangelogEntry{
			Time:   time.Now(),
			Author: dateAndAuthor,
			Text:   "",
		}
	}

	// Try to parse date with the first 4 parts: "Mon Jan 02 2006"
	dateStr := strings.Join(parts[:4], " ")
	author := strings.Join(parts[4:], " ")

	// Try parsing with standard format
	parsedTime, err := time.Parse("Mon Jan 02 2006", dateStr)
	if err != nil {
		// Try alternative format with single-digit day (Mon Jan  2 2006)
		parsedTime, err = time.Parse("Mon Jan  2 2006", dateStr)
		if err != nil {
			// If both formats fail, use current time
			parsedTime = time.Now()
		}
	}

	return &rpmpack.ChangelogEntry{
		Time:   parsedTime,
		Author: author,
		Text:   "",
	}
}
