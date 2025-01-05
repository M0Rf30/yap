package dpkg

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/M0Rf30/yap/pkg/options"
	"github.com/M0Rf30/yap/pkg/pkgbuild"
	"github.com/M0Rf30/yap/pkg/utils"
	"github.com/blakesmith/ar"
	"github.com/mholt/archives"
	"github.com/otiai10/copy"
)

// Deb represents a Deb package.
//
// It contains the directory path of the package and the PKGBUILD struct, which
// contains the metadata and build instructions for the package.
type Deb struct {
	debDir   string
	PKGBUILD *pkgbuild.PKGBUILD
}

// createTarZst creates a compressed tar.zst archive from the specified source directory.
// It takes the source directory and the output file path as arguments and returns an error if any occurs.
func createTarZst(sourceDir, outputFile string) error {
	ctx := context.TODO()

	// Retrieve the list of files from the source directory on disk.
	// The map specifies that the files should be read from the sourceDir
	// and the output path in the archive should be empty.
	files, err := archives.FilesFromDisk(ctx, nil, map[string]string{
		sourceDir + string(os.PathSeparator): "",
	})

	if err != nil {
		return err
	}

	cleanFilePath := filepath.Clean(outputFile)

	out, err := os.Create(cleanFilePath)
	if err != nil {
		return err
	}
	defer out.Close()

	format := archives.CompressedArchive{
		Compression: archives.Zstd{},
		Archival:    archives.Tar{},
	}

	return format.Archive(ctx, out, files)
}

// createConfFiles creates the configuration files for the Debian package.
// It generates a file located at the debDir path containing the backup
// files specified in the PKGBUILD. Returns an error if there was a
// problem creating or writing to the file.
func (d *Deb) createConfFiles() error {
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

	return utils.CreateWrite(path, data)
}

// createCopyrightFile generates a copyright file for the Debian package.
// It checks if there is a license specified in the PKGBUILD and creates
// the copyright file accordingly. Returns an error if there was an
// issue creating the file.
func (d *Deb) createCopyrightFile() error {
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
func (d *Deb) createDebconfFile(name, variable string) error {
	if variable == "" {
		return nil
	}

	assetPath := filepath.Join(d.PKGBUILD.Home, variable)
	destPath := filepath.Join(d.debDir, name)

	return copy.Copy(assetPath, destPath)
}

// addScriptlets generates and writes the scripts for the Deb package.
// It takes no parameters and returns an error if there was an issue
// generating or writing the scripts.
func (d *Deb) addScriptlets() error {
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

		if err := utils.CreateWrite(path, script); err != nil {
			return err
		}

		if err := utils.Chmod(path, 0o755); err != nil {
			return err
		}
	}

	return nil
}

// createDeb generates Deb package files from the given artifact path.
// It takes a string parameter `artifactPath` which represents the path
// where the Deb package files will be generated. The function returns
// an error if there was an issue generating the Deb package files.
func (d *Deb) createDeb(artifactPath, control, data string) error {
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
	defer debPackage.Close()

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
	if err := writer.WriteGlobalHeader(); err != nil {
		return err
	}

	if err := addArFile(writer, binaryFilename, debianBinary); err != nil {
		return err
	}

	if err := addArFile(writer, controlFilename, controlArchive); err != nil {
		return err
	}

	if err := addArFile(writer, dataFilename, dataArchive); err != nil {
		return err
	}

	return nil
}

// Prepare prepares the Deb package by installing its dependencies using apt-get.
//
// makeDepends: a slice of strings representing the dependencies to be installed.
// Returns an error if there was a problem installing the dependencies.
func (d *Deb) Prepare(makeDepends []string) error {
	args := []string{
		"--allow-downgrades",
		"--assume-yes",
		"install",
	}

	return d.PKGBUILD.GetDepends("apt-get", args, makeDepends)
}

// Update updates the Deb package list.
//
// It calls the GetUpdates method of the PKGBUILD field of the Deb struct
// to retrieve any updates using the "apt-get" command and the "update" argument.
// If an error occurs during the update, it is returned.
//
// Returns:
// - error: An error if the update fails.
func (d *Deb) Update() error {
	return d.PKGBUILD.GetUpdates("apt-get", "update")
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
func (d *Deb) createDebResources() error {
	d.debDir = filepath.Join(d.PKGBUILD.PackageDir, "DEBIAN")
	if err := utils.ExistsMakeDir(d.debDir); err != nil {
		return err
	}

	if err := d.createConfFiles(); err != nil {
		return err
	}

	d.PKGBUILD.InstalledSize, _ = utils.GetDirSize(d.PKGBUILD.PackageDir)
	d.PKGBUILD.Depends = d.processDepends(d.PKGBUILD.Depends)
	d.PKGBUILD.MakeDepends = d.processDepends(d.PKGBUILD.MakeDepends)
	d.PKGBUILD.OptDepends = d.processDepends(d.PKGBUILD.OptDepends)

	tmpl := d.PKGBUILD.RenderSpec(specFile)
	if err := d.PKGBUILD.CreateSpec(filepath.Join(d.debDir,
		"control"), tmpl); err != nil {
		return err
	}

	if err := d.createCopyrightFile(); err != nil {
		return err
	}

	if err := d.addScriptlets(); err != nil {
		return err
	}

	if err := d.createDebconfFile("config",
		d.PKGBUILD.DebConfig); err != nil {
		return err
	}

	if err := d.createDebconfFile("templates",
		d.PKGBUILD.DebTemplate); err != nil {
		return err
	}

	return nil
}

// BuildPackage builds the Debian package and cleans up afterward.
// It takes artifactsPath to specify where to store the package.
// The method calls dpkgDeb to create the package and removes the
// package directory, returning an error if any step fails.
func (d *Deb) BuildPackage(artifactsPath string) error {
	debTemp, err := os.MkdirTemp(d.PKGBUILD.SourceDir, "tmp")
	if err != nil {
		return err
	}
	defer os.RemoveAll(debTemp)

	controlArchive := filepath.Join(debTemp, controlFilename)
	dataArchive := filepath.Join(debTemp, dataFilename)

	// Create control archive
	if err := createTarZst(d.debDir, controlArchive); err != nil {
		return err
	}

	if err := os.RemoveAll(d.debDir); err != nil {
		return err
	}

	// Create data archive
	if err := createTarZst(d.PKGBUILD.PackageDir, dataArchive); err != nil {
		return err
	}

	if err := d.createDeb(artifactsPath, controlArchive, dataArchive); err != nil {
		return err
	}

	if err := os.RemoveAll(d.PKGBUILD.PackageDir); err != nil {
		return err
	}

	return nil
}

// PrepareFakeroot sets up the environment for building a Debian package in a fakeroot context.
// It retrieves architecture and release information, cleans up the debDir, creates necessary
// resources, and strips binaries. The method returns an error if any step fails.
func (d *Deb) PrepareFakeroot(_ string) error {
	d.getRelease()
	d.PKGBUILD.ArchComputed = DebArchs[d.PKGBUILD.ArchComputed]

	if err := os.RemoveAll(d.debDir); err != nil {
		return err
	}

	if err := d.createDebResources(); err != nil {
		return err
	}

	if d.PKGBUILD.StripEnabled {
		return options.Strip(d.PKGBUILD.PackageDir)
	}

	return nil
}

func (d *Deb) Install(artifactsPath string) error {
	artifactFilePath := filepath.Join(artifactsPath,
		fmt.Sprintf("%s_%s-%s_%s.deb",
			d.PKGBUILD.PkgName, d.PKGBUILD.PkgVer, d.PKGBUILD.PkgRel,
			d.PKGBUILD.ArchComputed))

	if err := utils.Exec(false, "", "apt-get", "install", "-y", artifactFilePath); err != nil {
		return err
	}

	return nil
}

// PrepareEnvironment prepares the environment for the Deb package.
//
// It takes a boolean parameter `golang` which indicates whether or not to set up Go.
// It returns an error if there was a problem during the environment preparation.
func (d *Deb) PrepareEnvironment(golang bool) error {
	args := []string{
		"--assume-yes",
		"install",
	}
	args = append(args, buildEnvironmentDeps...)

	if err := utils.Exec(false, "", "apt-get", args...); err != nil {
		return err
	}

	if golang {
		if err := utils.GOSetup(); err != nil {
			return err
		}
	}

	return nil
}

func addArFile(writer *ar.Writer, name string, body []byte) error {
	header := ar.Header{
		Name: name,
		Size: int64(len(body)),
		Mode: 0o644,
	}

	if err := writer.WriteHeader(&header); err != nil {
		return err
	}

	_, err := writer.Write(body)

	return err
}

func (d *Deb) getRelease() {
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
func (d *Deb) processDepends(depends []string) []string {
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
