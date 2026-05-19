// Package deb provides functionality for building Debian (.deb) packages from PKGBUILD specifications.
package deb

import (
	"compress/gzip"
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/blakesmith/ar"
	"github.com/otiai10/copy"

	"github.com/M0Rf30/yap/v2/pkg/archive"
	"github.com/M0Rf30/yap/v2/pkg/builders/common"
	"github.com/M0Rf30/yap/v2/pkg/constants"
	"github.com/M0Rf30/yap/v2/pkg/errors"
	"github.com/M0Rf30/yap/v2/pkg/files"
	"github.com/M0Rf30/yap/v2/pkg/i18n"
	"github.com/M0Rf30/yap/v2/pkg/logger"
	"github.com/M0Rf30/yap/v2/pkg/pkgbuild"
)

// Package represents a Deb package.
//
// It contains the directory path of the package and the PKGBUILD struct, which
// contains the metadata and build instructions for the package.
type Package struct {
	*common.BaseBuilder
	debDir      string
	compression string
}

// NewBuilder creates a new Debian package manager with optional compression setting.
// If compression is empty, defaults to "zstd".
func NewBuilder(pkgBuild *pkgbuild.PKGBUILD, compression string) *Package {
	if compression == "" {
		compression = "zstd"
	}

	return &Package{
		BaseBuilder: common.NewBaseBuilder(pkgBuild, "deb"),
		debDir:      "",
		compression: compression,
	}
}

// BuildPackage builds the Debian package and cleans up afterward.
// It takes artifactsPath to specify where to store the package.
// The method calls dpkgDeb to create the package and removes the
// package directory, returning the path to the created DEB file and an error if any step fails.
func (d *Package) BuildPackage(artifactsPath string, targetArch string) (string, error) {
	debTemp, err := os.MkdirTemp(d.PKGBUILD.SourceDir, "tmp")
	if err != nil {
		return "", err
	}

	defer func() {
		err := os.RemoveAll(debTemp)
		if err != nil {
			logger.Warn(i18n.T("logger.buildpackage.warn.failed_to_remove_temporary_1"),
				"path", debTemp, "error", err)
		}
	}()

	controlArchive := filepath.Join(debTemp, controlFilename)
	dataArchive := filepath.Join(debTemp, dataFilename)

	// Create control archive
	err = archive.CreateTarCompressed(context.Background(), d.debDir, controlArchive,
		d.compression, true)
	if err != nil {
		return "", err
	}

	err = os.RemoveAll(d.debDir)
	if err != nil {
		return "", err
	}

	// Create data archive
	err = archive.CreateTarCompressed(context.Background(), d.PKGBUILD.PackageDir,
		dataArchive, d.compression, true)
	if err != nil {
		return "", err
	}

	debPath, err := d.createDeb(artifactsPath, controlArchive, dataArchive)
	if err != nil {
		return "", err
	}

	err = os.RemoveAll(d.PKGBUILD.PackageDir)
	if err != nil {
		return "", err
	}

	return debPath, nil
}

// PrepareFakeroot sets up the environment for building a Debian package in a fakeroot context.
// It retrieves architecture and release information, cleans up the debDir, creates necessary
// resources, and strips binaries. The method returns an error if any step fails.
func (d *Package) PrepareFakeroot(_ string, targetArch string) error {
	d.getRelease()
	d.LogCrossCompilation(targetArch)
	d.SetTargetArchitecture(targetArch)

	err := os.RemoveAll(d.debDir)
	if err != nil {
		return err
	}

	err = d.createDebResources()
	if err != nil {
		return err
	}

	return d.ApplyOptions()
}

// addArFile adds a file to an archive writer with the specified name, body,
// and modification date. It constructs an ar.Header with the provided
// parameters and writes it to the archive. If the header write fails,
// the function returns the error. After writing the header, it writes
// the file body to the archive and returns any error encountered.
func addArFile(writer *ar.Writer, name string, body []byte, date time.Time) error {
	header := ar.Header{
		Name:    name,
		Size:    int64(len(body)),
		Mode:    0o644,
		ModTime: date,
	}

	err := writer.WriteHeader(&header)
	if err != nil {
		return err
	}

	_, err = writer.Write(body)

	return err
}

// arWriterCompat wraps ar.Writer to fix its non-standard Write contract.
// ar.Writer.Write returns n+1 for odd-sized payloads (padding byte), which
// causes io.Copy to return errInvalidWrite. This wrapper caps nw to len(b).
type arWriterCompat struct {
	w *ar.Writer
}

func (a arWriterCompat) Write(b []byte) (int, error) {
	nw, err := a.w.Write(b)
	if nw > len(b) {
		nw = len(b)
	}

	return nw, err
}

// addArFileFromPath streams a file from disk into the ar archive without
// reading the entire file into memory.
func addArFileFromPath(writer *ar.Writer, name string, filePath string, modtime time.Time) error {
	f, err := os.Open(filepath.Clean(filePath))
	if err != nil {
		return err
	}

	defer func() {
		if cerr := f.Close(); cerr != nil {
			logger.Warn("failed to close file", "path", filePath, "error", cerr)
		}
	}()

	info, err := f.Stat()
	if err != nil {
		return err
	}

	hdr := &ar.Header{
		Name:    name,
		ModTime: modtime,
		Mode:    0o644,
		Size:    info.Size(),
	}

	if err := writer.WriteHeader(hdr); err != nil {
		return err
	}

	_, err = io.Copy(arWriterCompat{w: writer}, f)

	return err
}

// addScriptlets generates and writes the scripts for the Deb package.
// It takes no parameters and returns an error if there was an issue
// generating or writing the scripts.
func (d *Package) addScriptlets() error {
	const (
		prermScript  = "prerm"
		postrmScript = "postrm"
	)

	scripts := map[string]string{
		"preinst":    d.PKGBUILD.PreInst,
		"postinst":   d.PKGBUILD.PostInst,
		prermScript:  d.PKGBUILD.PreRm,
		postrmScript: d.PKGBUILD.PostRm,
	}

	for name, script := range scripts {
		if script == "" {
			continue
		}

		// Prepend only the helper function definitions that are actually
		// called within this scriptlet, so that helpers like _postinst() or
		// _postinst_legacy() are available at install time without injecting
		// unrelated build-time helpers (_package, _package_systemd, etc.).
		script = d.PrepareScriptletWithHelpers(script)

		if name == prermScript || name == postrmScript {
			script = removeHeader + script
		}

		path := filepath.Join(d.debDir, name)

		err := files.CreateWrite(path, script)
		if err != nil {
			return err
		}

		err = files.Chmod(path, 0o755)
		if err != nil {
			return err
		}
	}

	return nil
}

// createConfFiles creates the configuration files for the Debian package.
// It generates a file located at the debDir path containing the backup
// files specified in the PKGBUILD. Returns an error if there was a
// problem creating or writing to the file.
func (d *Package) createConfFiles() error {
	if len(d.PKGBUILD.Backup) == 0 {
		return nil
	}

	path := filepath.Join(d.debDir, "conffiles")

	var data strings.Builder

	for _, name := range d.PKGBUILD.Backup {
		if !strings.HasPrefix(name, "/") {
			name = "/" + name
		}

		data.WriteString(name + "\n")
	}

	return files.CreateWrite(path, data.String())
}

// createCopyrightFile generates a copyright file for the Debian package.
// It checks if there is a license specified in the PKGBUILD and creates
// the copyright file accordingly. Returns an error if there was an
// issue creating the file.
func (d *Package) createCopyrightFile() error {
	if len(d.PKGBUILD.License) == 0 {
		return nil
	}

	copyrightFilePath := filepath.Join(d.debDir, "copyright")
	tmpl := d.PKGBUILD.RenderSpec(copyrightFile)

	return d.PKGBUILD.CreateSpec(copyrightFilePath, tmpl)
}

// createDebconfFile creates a debconf file with the given variable and
// name. It takes parameters for the variable used to create the debconf
// asset and the name of the debconf asset. Returns an error if there
// was an issue during the creation.
func (d *Package) createDebconfFile(name, variable string) error {
	if variable == "" {
		return nil
	}

	assetPath := filepath.Join(d.PKGBUILD.Home, variable)
	destPath := filepath.Join(d.debDir, name)

	return copy.Copy(assetPath, destPath)
}

// createChangelogFile creates the changelog file for the Debian package.
// It reads the changelog from PKGBUILD, gzip-compresses it, and writes it to
// usr/share/doc/<pkgname>/changelog.Debian.gz in the package directory.
// Returns nil if no changelog is specified.
func (d *Package) createChangelogFile() error {
	changelogData, err := d.PKGBUILD.ReadChangelog()
	if err != nil {
		return err
	}

	if changelogData == nil {
		return nil
	}

	// Create the doc directory structure
	docDir := filepath.Join(d.PKGBUILD.PackageDir, "usr", "share", "doc",
		d.PKGBUILD.PkgName)

	err = files.ExistsMakeDir(docDir)
	if err != nil {
		return err
	}

	// Create and write the gzip-compressed changelog
	changelogPath := filepath.Join(docDir, "changelog.Debian.gz")

	changelogFile, err := os.Create(filepath.Clean(changelogPath))
	if err != nil {
		return err
	}

	defer func() {
		if cerr := changelogFile.Close(); cerr != nil {
			logger.Warn("failed to close changelog file", "path", changelogPath,
				"error", cerr)
		}
	}()

	gzWriter := gzip.NewWriter(changelogFile)
	defer func() {
		if cerr := gzWriter.Close(); cerr != nil {
			logger.Warn("failed to close gzip writer", "path", changelogPath,
				"error", cerr)
		}
	}()

	// Set the modification time for reproducibility
	gzWriter.ModTime = files.SourceDateEpochFromEnv()

	_, err = gzWriter.Write(changelogData)
	if err != nil {
		return err
	}

	return nil
}

// createDeb generates Deb package files from the given artifact path.
// It takes a string parameter `artifactPath` which represents the path
// where the Deb package files will be generated. The function returns
// the path to the created DEB file and an error if there was an issue generating the Deb package files.
func (d *Package) createDeb(artifactPath, control, data string) (string, error) {
	// Create the .deb package
	artifactFilePath := filepath.Join(artifactPath, d.BuildPackageName(constants.ExtDEB))

	cleanFilePath := filepath.Clean(artifactFilePath)
	debianBinary := []byte(binaryContent)

	debPackage, err := os.Create(cleanFilePath)
	if err != nil {
		return "", err
	}

	defer func() {
		err := debPackage.Close()
		if err != nil {
			logger.Warn(i18n.T("logger.createdeb.warn.failed_to_close_debian_1"), "error", err)
		}
	}()

	writer := ar.NewWriter(debPackage)

	err = writer.WriteGlobalHeader()
	if err != nil {
		return "", err
	}

	modtime := files.SourceDateEpochFromEnv()

	err = addArFile(writer,
		binaryFilename,
		debianBinary,
		modtime)
	if err != nil {
		return "", err
	}

	err = addArFileFromPath(writer, controlFilename, control, modtime)
	if err != nil {
		return "", err
	}

	err = addArFileFromPath(writer, dataFilename, data, modtime)
	if err != nil {
		return "", err
	}

	// Log package creation using common functionality
	d.LogPackageCreated(artifactFilePath)

	return cleanFilePath, nil
}

// createDebResources creates the Deb package resources.
//
// It creates the necessary directories and files for the Deb package.
// It also sets the installed size of the package based on the size of the package directory.
// It generates the control file for the package.
// It creates the scripts for the package.
// It creates the debconf templates file.
// It creates the debconf config file.
// It returns an error if any of the operations fail.
func (d *Package) createDebResources() error {
	d.debDir = filepath.Join(d.PKGBUILD.PackageDir, "DEBIAN")

	err := files.ExistsMakeDir(d.debDir)
	if err != nil {
		return err
	}

	err = d.createConfFiles()
	if err != nil {
		return err
	}

	size, err := files.GetDirSize(d.PKGBUILD.PackageDir)
	if err != nil {
		return errors.Wrap(err, errors.ErrTypeFileSystem, "failed to get package dir size").
			WithOperation("createDebResources")
	}

	d.PKGBUILD.InstalledSize = size / 1024
	d.PKGBUILD.Depends = d.ProcessDependencies(d.PKGBUILD.Depends)
	d.PKGBUILD.MakeDepends = d.ProcessDependencies(d.PKGBUILD.MakeDepends)
	d.PKGBUILD.OptDepends = d.ProcessDependencies(d.PKGBUILD.OptDepends)

	tmpl := d.PKGBUILD.RenderSpec(specFile)

	err = d.PKGBUILD.CreateSpec(filepath.Join(d.debDir,
		"control"), tmpl)
	if err != nil {
		return err
	}

	err = d.createCopyrightFile()
	if err != nil {
		return err
	}

	err = d.addScriptlets()
	if err != nil {
		return err
	}

	err = d.createDebconfFile("config",
		d.PKGBUILD.DebConfig)
	if err != nil {
		return err
	}

	err = d.createDebconfFile("templates",
		d.PKGBUILD.DebTemplate)
	if err != nil {
		return err
	}

	err = d.createChangelogFile()
	if err != nil {
		return err
	}

	return nil
}

// getRelease updates the package release with distribution-specific suffix.
// Unlike FormatRelease in BaseBuilder, this method always appends a distro suffix
// (either codename or distro name) to ensure proper Debian repository targeting.
// This divergence is intentional: DEB requires a distro suffix for repo selection,
// while RPM only appends when codename is explicitly set.
func (d *Package) getRelease() {
	var suffix string
	if d.PKGBUILD.Codename != "" {
		suffix = d.PKGBUILD.Codename
	} else {
		suffix = d.PKGBUILD.Distro
	}

	// Guard against double-append when called multiple times for split packages.
	if !strings.HasSuffix(d.PKGBUILD.PkgRel, suffix) {
		d.PKGBUILD.PkgRel += suffix
	}
}
