package deb

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/blakesmith/ar"
	"github.com/otiai10/copy"

	"github.com/M0Rf30/yap/v2/pkg/constants"
	"github.com/M0Rf30/yap/v2/pkg/options"
	"github.com/M0Rf30/yap/v2/pkg/osutils"
	"github.com/M0Rf30/yap/v2/pkg/pkgbuild"
)

// Package represents a Deb package.
//
// It contains the directory path of the package and the PKGBUILD struct, which
// contains the metadata and build instructions for the package.
type Package struct {
	debDir   string
	PKGBUILD *pkgbuild.PKGBUILD
}

// NewPackage creates a new Debian package manager.
func NewPackage(pkgBuild *pkgbuild.PKGBUILD) *Package {
	return &Package{
		PKGBUILD: pkgBuild,
	}
}

// BuildPackage builds the Debian package and cleans up afterward.
// It takes artifactsPath to specify where to store the package.
// The method calls dpkgDeb to create the package and removes the
// package directory, returning an error if any step fails.
func (d *Package) BuildPackage(artifactsPath string) error {
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

// Install installs a Debian package from the specified artifacts path. It
// constructs the package file path using details from the PKGBUILD and
// executes the `apt-get install` command. Returns an error if the
// installation fails.
func (d *Package) Install(artifactsPath string) error {
	artifactFilePath := filepath.Join(artifactsPath,
		fmt.Sprintf("%s_%s-%s_%s.deb",
			d.PKGBUILD.PkgName, d.PKGBUILD.PkgVer, d.PKGBUILD.PkgRel,
			d.PKGBUILD.ArchComputed))

	// Use centralized install arguments
	installArgs := constants.GetInstallArgs(constants.FormatDEB)
	installArgs = append(installArgs, artifactFilePath)

	err := osutils.Exec(false, "", "apt-get", installArgs...)
	if err != nil {
		return err
	}

	return nil
}

// Prepare prepares the Deb package by installing its dependencies using apt-get.
//
// makeDepends: a slice of strings representing the dependencies to be installed.
// Returns an error if there was a problem installing the dependencies.
func (d *Package) Prepare(makeDepends []string) error {
	// Use centralized install arguments
	installArgs := constants.GetInstallArgs(constants.FormatDEB)
	return d.PKGBUILD.GetDepends("apt-get", installArgs, makeDepends)
}

// PrepareEnvironment prepares the environment for the Deb package.
//
// It takes a boolean parameter `golang` which indicates whether or not to set up Go.
// It returns an error if there was a problem during the environment preparation.
func (d *Package) PrepareEnvironment(golang bool) error {
	// Use centralized build dependencies and install arguments
	buildDeps := constants.GetBuildDeps()
	installArgs := constants.GetInstallArgs(constants.FormatDEB)
	installArgs = append(installArgs, buildDeps.DEB...)

	err := osutils.Exec(false, "", "apt-get", installArgs...)
	if err != nil {
		return err
	}

	if golang {
		err := osutils.GOSetup()
		if err != nil {
			return err
		}
	}

	return nil
}

// PrepareFakeroot sets up the environment for building a Debian package in a fakeroot context.
// It retrieves architecture and release information, cleans up the debDir, creates necessary
// resources, and strips binaries. The method returns an error if any step fails.
func (d *Package) PrepareFakeroot(_ string) error {
	d.getRelease()

	// Use centralized architecture mapping
	archMapping := constants.GetArchMapping()
	d.PKGBUILD.ArchComputed = archMapping.TranslateArch(constants.FormatDEB, d.PKGBUILD.ArchComputed)

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
//
// It calls the GetUpdates method of the PKGBUILD field of the Deb struct
// to retrieve any updates using the "apt-get" command and the "update" argument.
// If an error occurs during the update, it is returned.
//
// Returns:
// - error: An error if the update fails.
func (d *Package) Update() error {
	return d.PKGBUILD.GetUpdates("apt-get", "update")
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

	return osutils.CreateWrite(path, data)
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
			osutils.Logger.Warn("failed to close debian package file", osutils.Logger.Args("error", err))
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

	modtime := getModTime()

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

	pkgLogger := osutils.WithComponent(d.PKGBUILD.PkgName)
	pkgLogger.Info("package artifact created", osutils.Logger.Args("pkgver", d.PKGBUILD.PkgVer,
		"pkgrel", d.PKGBUILD.PkgRel,
		"artifact", artifactFilePath))

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

	err := osutils.ExistsMakeDir(d.debDir)
	if err != nil {
		return err
	}

	err = d.createConfFiles()
	if err != nil {
		return err
	}

	size, _ := osutils.GetDirSize(d.PKGBUILD.PackageDir)
	d.PKGBUILD.InstalledSize = size / 1024
	d.PKGBUILD.Depends = d.processDepends(d.PKGBUILD.Depends)
	d.PKGBUILD.MakeDepends = d.processDepends(d.PKGBUILD.MakeDepends)
	d.PKGBUILD.OptDepends = d.processDepends(d.PKGBUILD.OptDepends)

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

// getModTime returns the current local time. It uses the time.Now() function
// from the time package to retrieve the current time.
func getModTime() time.Time {
	return time.Now()
}

func (d *Package) getRelease() {
	if d.PKGBUILD.Codename != "" {
		d.PKGBUILD.PkgRel += d.PKGBUILD.Codename
	} else {
		d.PKGBUILD.PkgRel += d.PKGBUILD.Distro
	}
}

// processDepends takes a slice of strings and processes each string in order to
// modify it and return a new slice of strings for deb syntax.
//
// It splits each string into three parts: name, operator, and version. If the
// string is split successfully, it combines the three parts into a new format
// and replaces the original string in the slice.
//
// Parameters:
//   - depends: a slice of strings to be processed.
//
// Returns:
//   - a new slice of strings with modified elements for deb syntax.
func (d *Package) processDepends(depends []string) []string {
	pattern := `(?m)(<|<=|>=|=|>|<)`
	regex := regexp.MustCompile(pattern)

	for index, depend := range depends {
		result := regex.Split(depend, -1)
		if len(result) == 2 {
			name := result[0]
			operator := strings.Trim(depend, result[0]+result[1])
			version := result[1]
			depends[index] = name + " (" + operator + " " + version + ")"
		}
	}

	return depends
}
