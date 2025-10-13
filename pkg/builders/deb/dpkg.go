package deb

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/blakesmith/ar"
	"github.com/otiai10/copy"

	"github.com/M0Rf30/yap/v2/pkg/archive"
	"github.com/M0Rf30/yap/v2/pkg/builders/common"
	"github.com/M0Rf30/yap/v2/pkg/files"
	"github.com/M0Rf30/yap/v2/pkg/i18n"
	"github.com/M0Rf30/yap/v2/pkg/logger"
	"github.com/M0Rf30/yap/v2/pkg/options"
	"github.com/M0Rf30/yap/v2/pkg/pkgbuild"
)

// Package represents a Deb package.
//
// It contains the directory path of the package and the PKGBUILD struct, which
// contains the metadata and build instructions for the package.
type Package struct {
	*common.BaseBuilder
	debDir string
}

// NewBuilder creates a new Debian package manager.
func NewBuilder(pkgBuild *pkgbuild.PKGBUILD) *Package {
	return &Package{
		BaseBuilder: common.NewBaseBuilder(pkgBuild, "deb"),
		debDir:      "",
	}
}

// BuildPackage builds the Debian package and cleans up afterward.
// It takes artifactsPath to specify where to store the package.
// The method calls dpkgDeb to create the package and removes the
// package directory, returning an error if any step fails.
func (d *Package) BuildPackage(artifactsPath string, targetArch string) error {
	// Log cross-compilation build initiation
	if targetArch != "" && targetArch != d.PKGBUILD.ArchComputed {
		logger.Info(i18n.T("logger.cross_compilation.starting_cross_compilation_build"),
			"package", d.PKGBUILD.PkgName,
			"target_arch", targetArch,
			"build_arch", d.PKGBUILD.ArchComputed)
	}

	debTemp, err := os.MkdirTemp(d.PKGBUILD.SourceDir, "tmp")
	if err != nil {
		return err
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
	err = archive.CreateTarZst(d.debDir, controlArchive, true)
	if err != nil {
		return err
	}

	err = os.RemoveAll(d.debDir)
	if err != nil {
		return err
	}

	// Create data archive
	err = archive.CreateTarZst(d.PKGBUILD.PackageDir, dataArchive, true)
	if err != nil {
		return err
	}

	err = d.createDeb(artifactsPath, controlArchive, dataArchive)
	if err != nil {
		return err
	}

	err = os.RemoveAll(d.PKGBUILD.PackageDir)
	if err != nil {
		return err
	}

	return nil
}

// PrepareFakeroot sets up the environment for building a Debian package in a fakeroot context.
// It retrieves architecture and release information, cleans up the debDir, creates necessary
// resources, and strips binaries. The method returns an error if any step fails.
func (d *Package) PrepareFakeroot(_ string, targetArch string) error {
	d.getRelease()

	// Use centralized architecture mapping from BaseBuilder
	// If target architecture is specified for cross-compilation, use it
	if targetArch != "" {
		d.PKGBUILD.ArchComputed = targetArch
	}

	d.TranslateArchitecture()

	err := os.RemoveAll(d.debDir)
	if err != nil {
		return err
	}

	err = d.createDebResources()
	if err != nil {
		return err
	}

	if d.PKGBUILD.StripEnabled {
		return options.Strip(d.PKGBUILD.PackageDir)
	}

	return nil
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

// addScriptlets generates and writes the scripts for the Deb package.
// It takes no parameters and returns an error if there was an issue
// generating or writing the scripts.
func (d *Package) addScriptlets() error {
	scripts := map[string]string{
		"preinst":  d.PKGBUILD.PreInst,
		"postinst": d.PKGBUILD.PostInst,
		"prerm":    d.PKGBUILD.PreRm,
		"postrm":   d.PKGBUILD.PostRm,
	}

	for name, script := range scripts {
		if script == "" {
			continue
		}

		if name == "prerm" || name == "postrm" {
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

	data := ""

	for _, name := range d.PKGBUILD.Backup {
		if !strings.HasPrefix(name, "/") {
			name = "/" + name
		}

		data += name + "\n"
	}

	return files.CreateWrite(path, data)
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

// createDeb generates Deb package files from the given artifact path.
// It takes a string parameter `artifactPath` which represents the path
// where the Deb package files will be generated. The function returns
// an error if there was an issue generating the Deb package files.
func (d *Package) createDeb(artifactPath, control, data string) error {
	// Create the .deb package
	artifactFilePath := filepath.Join(artifactPath,
		fmt.Sprintf("%s_%s-%s_%s.deb",
			d.PKGBUILD.PkgName, d.PKGBUILD.PkgVer, d.PKGBUILD.PkgRel,
			d.PKGBUILD.ArchComputed))

	cleanFilePath := filepath.Clean(artifactFilePath)
	debianBinary := []byte(binaryContent)

	debPackage, err := os.Create(cleanFilePath)
	if err != nil {
		return err
	}

	defer func() {
		err := debPackage.Close()
		if err != nil {
			logger.Warn(i18n.T("logger.createdeb.warn.failed_to_close_debian_1"), "error", err)
		}
	}()

	cleanFilePath = filepath.Clean(control)

	controlArchive, err := os.ReadFile(cleanFilePath)
	if err != nil {
		return err
	}

	cleanFilePath = filepath.Clean(data)

	dataArchive, err := os.ReadFile(cleanFilePath)
	if err != nil {
		return err
	}

	writer := ar.NewWriter(debPackage)

	err = writer.WriteGlobalHeader()
	if err != nil {
		return err
	}

	modtime := getCurrentBuildTime()

	err = addArFile(writer,
		binaryFilename,
		debianBinary,
		modtime)
	if err != nil {
		return err
	}

	err = addArFile(writer,
		controlFilename,
		controlArchive,
		modtime)
	if err != nil {
		return err
	}

	err = addArFile(writer,
		dataFilename,
		dataArchive,
		modtime)
	if err != nil {
		return err
	}

	// Log package creation using common functionality
	d.LogPackageCreated(artifactFilePath)

	return nil
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

	size, _ := files.GetDirSize(d.PKGBUILD.PackageDir)
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

	return nil
}

// getCurrentBuildTime returns the current local time for package timestamping.
// It uses the time.Now() function from the time package to retrieve the current time.
func getCurrentBuildTime() time.Time {
	return time.Now()
}

// getRelease updates the package release with distribution-specific suffix.
// This delegates to the common FormatRelease method.
func (d *Package) getRelease() {
	if d.PKGBUILD.Codename != "" {
		d.PKGBUILD.PkgRel += d.PKGBUILD.Codename
	} else {
		d.PKGBUILD.PkgRel += d.PKGBUILD.Distro
	}
}
