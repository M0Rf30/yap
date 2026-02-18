package rpm

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/M0Rf30/yap/v2/pkg/builders/common"
	"github.com/M0Rf30/yap/v2/pkg/files"
	"github.com/M0Rf30/yap/v2/pkg/i18n"
	"github.com/M0Rf30/yap/v2/pkg/logger"
	"github.com/M0Rf30/yap/v2/pkg/options"
	"github.com/M0Rf30/yap/v2/pkg/pkgbuild"

	rpmpack "github.com/google/rpmpack"
)

// RPM represents a RPM package.
//
// It contains the directory path of the package and the PKGBUILD struct, which
// contains the metadata and build instructions for the package.
type RPM struct {
	*common.BaseBuilder
}

// NewBuilder creates a new RPM package builder.
func NewBuilder(pkgBuild *pkgbuild.PKGBUILD) *RPM {
	return &RPM{
		BaseBuilder: common.NewBaseBuilder(pkgBuild, "rpm"),
	}
}

// BuildPackage creates an RPM package based on the provided PKGBUILD information.
func (r *RPM) BuildPackage(artifactsPath string, targetArch string) error {
	r.LogCrossCompilation(targetArch)

	// Use target architecture for cross-compilation if specified
	arch := r.PKGBUILD.ArchComputed
	if targetArch != "" {
		arch = targetArch
	}

	pkgName := fmt.Sprintf("%s-%s-%s.%s.rpm",
		r.PKGBUILD.PkgName,
		r.PKGBUILD.PkgVer,
		r.PKGBUILD.PkgRel,
		arch)

	epoch, _ := strconv.ParseUint(r.PKGBUILD.Epoch, 10, 32)
	if epoch == 0 {
		epoch = uint64(rpmpack.NoEpoch)
	}

	copyright := strings.Join(r.PKGBUILD.Copyright, "; ")
	copyright = strings.TrimSuffix(copyright, " ")
	license := strings.Join(r.PKGBUILD.License, " ")

	pkgFilePath := filepath.Join(artifactsPath, pkgName)
	rpm, _ := rpmpack.NewRPM(rpmpack.RPMMetaData{
		Name:        r.PKGBUILD.PkgName,
		Summary:     r.PKGBUILD.PkgDesc,
		Description: r.PKGBUILD.PkgDesc,
		Epoch:       uint32(epoch),
		Version:     r.PKGBUILD.PkgVer,
		Release:     r.PKGBUILD.PkgRel,
		Arch:        arch,
		Vendor:      copyright,
		URL:         r.PKGBUILD.URL,
		Packager:    r.PKGBUILD.Maintainer,
		Group:       r.PKGBUILD.Section,
		Compressor:  "zstd",
		Licence:     license,
		Obsoletes:   r.processDepends(r.PKGBUILD.Replaces),
		Provides:    r.processDepends(r.PKGBUILD.Provides),
		Requires:    r.processDepends(r.PKGBUILD.Depends),
		Conflicts:   r.processDepends(r.PKGBUILD.Conflicts),
		Recommends:  r.processDepends(r.PKGBUILD.OptDepends),
		BuildTime:   time.Now(),
	})

	err := r.createFilesInsideRPM(rpm)
	if err != nil {
		return err
	}

	r.addScriptlets(rpm)

	cleanFilePath := filepath.Clean(pkgFilePath)

	rpmFile, err := os.Create(cleanFilePath)
	if err != nil {
		return err
	}

	defer func() {
		err := rpmFile.Close()
		if err != nil {
			logger.Warn(i18n.T("logger.unknown.warn.failed_to_close_rpm_1"), "path", cleanFilePath,
				"error", err)
		}
	}()

	err = rpm.Write(rpmFile)
	if err != nil {
		return err
	}

	// Log package creation using common functionality
	r.LogPackageCreated(cleanFilePath)

	return nil
}

// PrepareFakeroot sets up the environment for building an RPM package in a fakeroot context.
// It retrieves architecture, group, and release information, sets the package destination,
// cleans up the RPM directory, creates necessary directories, and gathers files.
// It also processes package dependencies and creates the RPM spec file, returning
// an error if any step fails.
func (r *RPM) PrepareFakeroot(_ string, targetArch string) error {
	r.getGroup()
	r.getRelease()
	r.SetTargetArchitecture(targetArch)

	if r.PKGBUILD.StripEnabled {
		return options.Strip(r.PKGBUILD.PackageDir)
	}

	return nil
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
func addContentsToRPM(contents []*files.Entry, rpm *rpmpack.RPM) error {
	// Iterate over each Entry in the provided slice.
	for _, content := range contents {
		// Create an RPMFile from the Entry.
		file, err := createRPMFile(content)
		if err != nil {
			return err // Return the error if creating the RPMFile fails.
		}

		// Clean the file name to ensure it has a proper format.
		file.Name = filepath.Clean(file.Name)
		// Add the created RPMFile to the RPM object.
		rpm.AddFile(*file)
	}

	// Return nil indicating that all contents were added successfully.
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
		rpm.AddPretrans(r.PKGBUILD.PreTrans)
	}

	if r.PKGBUILD.PreInst != "" {
		rpm.AddPrein(r.PKGBUILD.PreInst)
	}

	if r.PKGBUILD.PostInst != "" {
		rpm.AddPostin(r.PKGBUILD.PostInst)
	}

	if r.PKGBUILD.PreRm != "" {
		rpm.AddPreun(onlyOnUninstall + r.PKGBUILD.PreRm)
	}

	if r.PKGBUILD.PostRm != "" {
		rpm.AddPostun(onlyOnUninstall + r.PKGBUILD.PostRm)
	}

	if r.PKGBUILD.PostTrans != "" {
		rpm.AddPosttrans(r.PKGBUILD.PostTrans)
	}
}

// asRPMDirectory creates an RPMFile object for a directory based on the provided Entry.
// It retrieves the directory's modification time and sets the appropriate fields in the RPMFile.
func asRPMDirectory(entry *files.Entry) *rpmpack.RPMFile {
	// Get file information for the directory specified in the entry.
	fileInfo, _ := os.Stat(filepath.Clean(entry.Source))

	// Retrieve the modification time of the directory.
	mTime := extractFileModTimeUint32(fileInfo)

	// Create and return an RPMFile object for the directory.
	return &rpmpack.RPMFile{
		Name: entry.Destination, // Set the destination name.
		// Set the mode to indicate it's a directory.
		Mode:  uint(fileInfo.Mode()) | files.TagDirectory,
		MTime: mTime,  // Set the modification time.
		Owner: "root", // Set the owner to "root".
		Group: "root", // Set the group to "root".
	}
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
	fileInfo, _ := os.Stat(cleanFilePath)

	// Retrieve the modification time of the file.
	mTime := extractFileModTimeUint32(fileInfo)

	// Create and return an RPMFile object for the regular file.
	return &rpmpack.RPMFile{
		Name:  entry.Destination,     // Set the destination name.
		Body:  data,                  // Set the file data.
		Mode:  uint(fileInfo.Mode()), // Set the file mode.
		MTime: mTime,                 // Set the modification time.
		Owner: "root",                // Set the owner to "root".
		Group: "root",                // Set the group to "root".
		Type:  fileType,              // Set the file type.
	}, nil
}

// asRPMSymlink creates an RPMFile object for a symbolic link based on the provided Entry.
// It retrieves the link's target and modification time.
func asRPMSymlink(entry *files.Entry) *rpmpack.RPMFile {
	cleanFilePath := filepath.Clean(entry.Source)
	fileInfo, _ := os.Lstat(cleanFilePath) // Use Lstat to get information about the symlink.
	body, _ := os.Readlink(cleanFilePath)  // Read the target of the symlink.

	// Retrieve the modification time of the symlink.
	mTime := extractFileModTimeUint32(fileInfo)

	// Create and return an RPMFile object for the symlink.
	return &rpmpack.RPMFile{
		Name:  entry.Destination,   // Set the destination name.
		Body:  []byte(body),        // Set the target of the symlink as the body.
		Mode:  uint(files.TagLink), // Set the mode to indicate it's a symlink.
		MTime: mTime,               // Set the modification time.
		Owner: "root",              // Set the owner to "root".
		Group: "root",              // Set the group to "root".
	}
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
		file = asRPMSymlink(entry)
	case files.TypeDir:
		file = asRPMDirectory(entry)
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
// It checks for overflow and fatally logs if the time is out of range for uint32.
func extractFileModTimeUint32(fileInfo os.FileInfo) uint32 {
	mTime := fileInfo.ModTime().Unix()
	if mTime < 0 || mTime > int64(^uint32(0)) {
		logger.Fatal(i18n.T("errors.rpm.modification_time_out_of_range"),
			"time", mTime)
	}

	return uint32(mTime) //nolint:gosec // range already validated above with fatal log
}

// getRelease updates the release information with RPM distribution suffix.
// It uses the common FormatRelease method with RPM-specific distro mappings.
func (r *RPM) getRelease() {
	r.FormatRelease(RPMDistros)
}

// processDepends converts dependency strings to rpmpack.Relations format.
// It uses the common BaseBuilder.ProcessDependencies for consistent version
// operator handling, then converts the result to RPM-specific Relations type.
func (r *RPM) processDepends(depends []string) rpmpack.Relations {
	processed := r.ProcessDependencies(depends)
	relations := make(rpmpack.Relations, 0)

	for _, dep := range processed {
		err := relations.Set(dep)
		if err != nil {
			return nil
		}
	}

	return relations
}
