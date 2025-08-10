// Package deb provides Debian package building functionality.
package deb

import (
	"os"
	"path/filepath"
	"time"

	"github.com/blakesmith/ar"
	"github.com/otiai10/copy"

	"github.com/M0Rf30/yap/v2/pkg/core"
	"github.com/M0Rf30/yap/v2/pkg/dependencies"
	"github.com/M0Rf30/yap/v2/pkg/options"
	"github.com/M0Rf30/yap/v2/pkg/osutils"
	"github.com/M0Rf30/yap/v2/pkg/pkgbuild"
)

// Package represents a Debian package manager.
type Package struct {
	*core.BasePackageManager
	debDir              string
	dependencyProcessor *dependencies.Processor
}

// NewPackage creates a new Debian package manager.
func NewPackage(pkgBuild *pkgbuild.PKGBUILD) *Package {
	config := core.GetConfig("apt")
	return &Package{
		BasePackageManager:  core.NewBasePackageManager(pkgBuild, config),
		dependencyProcessor: dependencies.NewProcessor(),
	}
}

// BuildPackage builds the Debian package and cleans up afterward.
func (d *Package) BuildPackage(artifactsPath string) error {
	if err := d.ValidateArtifactsPath(artifactsPath); err != nil {
		return err
	}

	debTemp, err := os.MkdirTemp(d.PKGBUILD.SourceDir, "tmp")
	if err != nil {
		return err
	}

	defer func() {
		err := os.RemoveAll(debTemp)
		if err != nil {
			osutils.Logger.Warn("failed to remove temporary directory",
				osutils.Logger.Args("path", debTemp, "error", err))
		}
	}()

	controlArchive := filepath.Join(debTemp, controlFilename)
	dataArchive := filepath.Join(debTemp, dataFilename)

	// Create control archive
	err = osutils.CreateTarZst(d.debDir, controlArchive, true)
	if err != nil {
		return err
	}

	err = os.RemoveAll(d.debDir)
	if err != nil {
		return err
	}

	// Create data archive
	err = osutils.CreateTarZst(d.PKGBUILD.PackageDir, dataArchive, true)
	if err != nil {
		return err
	}

	packageName := d.BuildPackageName("deb")
	packagePath := filepath.Join(artifactsPath, packageName)

	err = d.createDeb(packagePath, controlArchive, dataArchive)
	if err != nil {
		return err
	}

	err = os.RemoveAll(d.PKGBUILD.PackageDir)
	if err != nil {
		return err
	}

	d.LogPackageCreated(packagePath)
	return nil
}

// Install installs a Debian package from the specified artifacts path.
func (d *Package) Install(artifactsPath string) error {
	packageName := d.BuildPackageName("deb")
	return d.InstallCommon(artifactsPath, packageName)
}

// Prepare prepares the Deb package by installing its dependencies.
func (d *Package) Prepare(makeDepends []string) error {
	return d.PrepareCommon(makeDepends)
}

// PrepareEnvironment prepares the environment for the Deb package.
func (d *Package) PrepareEnvironment(golang bool) error {
	return d.PrepareEnvironmentCommon(golang)
}

// PrepareFakeroot sets up the environment for building a Debian package in a fakeroot context.
func (d *Package) PrepareFakeroot(_ string) error {
	d.getRelease()
	d.SetComputedFields()

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

// Update updates the Deb package list.
func (d *Package) Update() error {
	return d.UpdateCommon()
}

// createDebResources creates the Deb package resources.
func (d *Package) createDebResources() error {
	d.debDir = filepath.Join(d.PKGBUILD.PackageDir, "DEBIAN")

	err := osutils.ExistsMakeDir(d.debDir)
	if err != nil {
		return err
	}

	err = d.createConfFiles()
	if err != nil {
		return err
	}

	err = d.SetInstalledSize()
	if err != nil {
		return err
	}

	// Process dependencies using the common processor
	d.PKGBUILD.Depends = d.dependencyProcessor.FormatForDeb(d.PKGBUILD.Depends)
	d.PKGBUILD.MakeDepends = d.dependencyProcessor.FormatForDeb(d.PKGBUILD.MakeDepends)
	d.PKGBUILD.OptDepends = d.dependencyProcessor.FormatForDeb(d.PKGBUILD.OptDepends)

	tmpl := d.PKGBUILD.RenderSpec(specFile)

	err = d.PKGBUILD.CreateSpec(filepath.Join(d.debDir, "control"), tmpl)
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

	err = d.createDebconfFile("config", d.PKGBUILD.DebConfig)
	if err != nil {
		return err
	}

	err = d.createDebconfFile("templates", d.PKGBUILD.DebTemplate)
	if err != nil {
		return err
	}

	return nil
}

// createDeb generates Deb package files from the given artifact path.
func (d *Package) createDeb(artifactPath, control, data string) error {
	cleanFilePath := filepath.Clean(artifactPath)
	debianBinary := []byte(binaryContent)

	debPackage, err := os.Create(cleanFilePath)
	if err != nil {
		return err
	}

	defer func() {
		err := debPackage.Close()
		if err != nil {
			osutils.Logger.Warn("failed to close debian package file", osutils.Logger.Args("error", err))
		}
	}()

	controlArchive, err := os.ReadFile(filepath.Clean(control))
	if err != nil {
		return err
	}

	dataArchive, err := os.ReadFile(filepath.Clean(data))
	if err != nil {
		return err
	}

	writer := ar.NewWriter(debPackage)

	err = writer.WriteGlobalHeader()
	if err != nil {
		return err
	}

	modtime := getModTime()

	err = addArFile(writer, binaryFilename, debianBinary, modtime)
	if err != nil {
		return err
	}

	err = addArFile(writer, controlFilename, controlArchive, modtime)
	if err != nil {
		return err
	}

	err = addArFile(writer, dataFilename, dataArchive, modtime)
	if err != nil {
		return err
	}

	return nil
}

// getRelease updates the release information.
func (d *Package) getRelease() {
	if d.PKGBUILD.Codename != "" {
		d.PKGBUILD.PkgRel += d.PKGBUILD.Codename
	} else {
		d.PKGBUILD.PkgRel += d.PKGBUILD.Distro
	}
}

// Remaining methods (createConfFiles, createCopyrightFile, etc.) remain the same
// but can be simplified using the common utilities...

// getModTime returns the current local time.
func getModTime() time.Time {
	return time.Now()
}

// addArFile adds a file to an archive writer.
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

// Rest of the constants and helper methods...
const (
	binaryContent   = "2.0\n"
	binaryFilename  = "debian-binary"
	controlFilename = "control.tar.zst"
	dataFilename    = "data.tar.zst"
	removeHeader    = "if [ \"$1\" = \"remove\" ]; then\n"
)

// Placeholder for template constants
const (
	specFile      = "" // Template content would go here
	copyrightFile = "" // Template content would go here
)

// createConfFiles creates the configuration files for the Debian package.
func (d *Package) createConfFiles() error {
	if len(d.PKGBUILD.Backup) == 0 {
		return nil
	}

	path := filepath.Join(d.debDir, "conffiles")
	data := ""

	normalizedBackup := dependencies.NormalizeBackupFiles(d.PKGBUILD.Backup)
	for _, name := range normalizedBackup {
		data += name + "\n"
	}

	return osutils.CreateWrite(path, data)
}

// createCopyrightFile generates a copyright file for the Debian package.
func (d *Package) createCopyrightFile() error {
	if len(d.PKGBUILD.License) == 0 {
		return nil
	}

	copyrightFilePath := filepath.Join(d.debDir, "copyright")
	tmpl := d.PKGBUILD.RenderSpec(copyrightFile)

	return d.PKGBUILD.CreateSpec(copyrightFilePath, tmpl)
}

// createDebconfFile creates a debconf file with the given variable and name.
func (d *Package) createDebconfFile(name, variable string) error {
	if variable == "" {
		return nil
	}

	assetPath := filepath.Join(d.PKGBUILD.Home, variable)
	destPath := filepath.Join(d.debDir, name)

	return copy.Copy(assetPath, destPath)
}

// addScriptlets generates and writes the scripts for the Deb package.
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

		err := osutils.CreateWrite(path, script)
		if err != nil {
			return err
		}

		err = osutils.Chmod(path, 0o755)
		if err != nil {
			return err
		}
	}

	return nil
}
