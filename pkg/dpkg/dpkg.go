package dpkg

import (
	"bytes"
	"fmt"
	"log"
	"path/filepath"
	"strings"
	"text/template"

	"github.com/M0Rf30/yap/pkg/builder"
	"github.com/M0Rf30/yap/pkg/constants"
	"github.com/M0Rf30/yap/pkg/options"
	"github.com/M0Rf30/yap/pkg/pkgbuild"
	"github.com/M0Rf30/yap/pkg/utils"
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

// getArch updates the architecture field in the Deb struct.
//
// It iterates over the architecture values in the PKGBUILD field of the Deb struct
// and replaces them with the corresponding values from the DebArchs map.
func (d *Deb) getArch() {
	for index, arch := range d.PKGBUILD.Arch {
		d.PKGBUILD.Arch[index] = DebArchs[arch]
	}
}

// createConfFiles creates the conffiles file in the Deb package.
//
// It iterates over the Backup field of the PKGBUILD struct and adds each name
// to the data string. The data string is then written to the conffiles file
// located at the debDir path.
//
// Returns an error if there was a problem creating or writing to the file.
func (d *Deb) createConfFiles() error {
	var err error
	if len(d.PKGBUILD.Backup) == 0 {
		return err
	}

	path := filepath.Join(d.debDir, "conffiles")

	data := ""

	for _, name := range d.PKGBUILD.Backup {
		if !strings.HasPrefix(name, "/") {
			name = "/" + name
		}

		data += name + "\n"
	}

	err = utils.CreateWrite(path, data)
	if err != nil {
		return err
	}

	return nil
}

func (d *Deb) createCopyrightFile() error {
	if len(d.PKGBUILD.License) == 0 {
		return nil
	}

	copyrightFilePath := filepath.Join(d.debDir, "copyright")

	return d.PKGBUILD.CreateSpec(copyrightFilePath, copyrightFile)
}

// createDebconfTemplate creates a Deb package configuration file template.
//
// It does not take any parameters.
// It returns an error if there was an issue creating the template.
func (d *Deb) createDebconfTemplate() error {
	var err error
	if d.PKGBUILD.DebTemplate == "" {
		return err
	}

	debconfTemplate := filepath.Join(d.PKGBUILD.Home, d.PKGBUILD.DebTemplate)
	path := filepath.Join(d.debDir, "templates")

	err = copy.Copy(debconfTemplate, path)
	if err != nil {
		return err
	}

	return nil
}

// createDebconfConfig creates a Deb configuration file.
//
// It takes no parameters and returns an error.
func (d *Deb) createDebconfConfig() error {
	var err error
	if d.PKGBUILD.DebConfig == "" {
		return err
	}

	config := filepath.Join(d.PKGBUILD.Home, d.PKGBUILD.DebConfig)
	path := filepath.Join(d.debDir, "config")

	err = copy.Copy(config, path)
	if err != nil {
		return err
	}

	return nil
}

// createScripts generates and writes the scripts for the Deb package.
//
// It takes no parameters.
// It returns an error if there was an issue generating or writing the scripts.
func (d *Deb) createScripts() error {
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

		data := script
		if name == "prerm" || name == "postrm" {
			data = removeHeader + data
		}

		path := filepath.Join(d.debDir, name)

		err := utils.CreateWrite(path, data)
		if err != nil {
			return err
		}

		err = utils.Chmod(path, 0o755)
		if err != nil {
			return err
		}
	}

	return nil
}

// dpkgDeb generates Deb package files from the given artifact path.
//
// It takes a string parameter `artifactPath` which represents the path where the
// Deb package files will be generated.
//
// The function returns an error if there was an issue generating the Deb package
// files.
func (d *Deb) dpkgDeb(artifactPath string) error {
	var err error

	for _, arch := range d.PKGBUILD.Arch {
		artifactFilePath := filepath.Join(artifactPath,
			fmt.Sprintf("%s_%s-%s_%s.deb",
				d.PKGBUILD.PkgName, d.PKGBUILD.PkgVer, d.PKGBUILD.PkgRel,
				arch))

		err = utils.Exec("",
			"dpkg-deb",
			"-b",
			"-Zzstd",
			d.PKGBUILD.PackageDir, artifactFilePath)

		if err != nil {
			return err
		}
	}

	return nil
}

// Prepare prepares the Deb package by installing its dependencies using apt-get.
//
// makeDepends: a slice of strings representing the dependencies to be installed.
// Returns an error if there was a problem installing the dependencies.
func (d *Deb) Prepare(makeDepends []string) error {
	args := []string{
		"--assume-yes",
		"install",
	}

	err := d.PKGBUILD.GetDepends("apt-get", args, makeDepends)
	if err != nil {
		return err
	}

	return nil
}

// Strip strips binaries from the Deb package.
//
// It does not take any parameters.
// It returns an error if there is any issue during stripping.
func (d *Deb) Strip() error {
	var err error

	var tmplBytesBuffer bytes.Buffer

	fmt.Printf("%s🧹 :: %sStripping binaries ...%s\n",
		string(constants.ColorBlue),
		string(constants.ColorYellow),
		string(constants.ColorWhite))

	tmpl := template.New("strip")

	template.Must(tmpl.Parse(options.StripScript))

	if pkgbuild.Verbose {
		err = tmpl.Execute(&tmplBytesBuffer, d.PKGBUILD)
		if err != nil {
			log.Fatal(err)
		}
	}

	err = builder.RunScript(tmplBytesBuffer.String())
	if err != nil {
		return err
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
func (d *Deb) Update() error {
	err := d.PKGBUILD.GetUpdates("apt-get", "update")
	if err != nil {
		return err
	}

	return nil
}

// createDebResources creates the Deb package resources.
//
// It creates the necessary directories and files for the Deb package.
// It also sets the installed size of the package based on the size of the package directory.
// It generates the control file for the package.
// It creates the scripts for the package.
// It creates the debconf template file.
// It creates the debconf config file.
// It returns an error if any of the operations fail.
func (d *Deb) createDebResources() error {
	d.debDir = filepath.Join(d.PKGBUILD.PackageDir, "DEBIAN")
	err := utils.ExistsMakeDir(d.debDir)

	if err != nil {
		return err
	}

	err = d.createConfFiles()
	if err != nil {
		return err
	}

	d.PKGBUILD.InstalledSize, _ = utils.GetDirSize(d.PKGBUILD.PackageDir)

	err = d.PKGBUILD.CreateSpec(filepath.Join(d.debDir, "control"), specFile)
	if err != nil {
		return err
	}

	err = d.createCopyrightFile()
	if err != nil {
		return err
	}

	err = d.createScripts()
	if err != nil {
		return err
	}

	err = d.createDebconfTemplate()
	if err != nil {
		return err
	}

	err = d.createDebconfConfig()
	if err != nil {
		return err
	}

	return nil
}

// Build builds the Deb package.
//
// It takes the artifactsPath as a parameter and returns an error if any.
func (d *Deb) Build(artifactsPath string) error {
	var err error

	d.getArch()
	d.getRelease()

	err = utils.RemoveAll(d.debDir)
	if err != nil {
		return err
	}

	err = d.createDebResources()
	if err != nil {
		return err
	}

	err = d.Strip()
	if err != nil {
		return err
	}

	err = d.dpkgDeb(artifactsPath)
	if err != nil {
		return err
	}

	err = utils.RemoveAll(d.PKGBUILD.PackageDir)
	if err != nil {
		return err
	}

	return nil
}

func (d *Deb) Install(artifactsPath string) error {
	var err error

	for _, arch := range d.PKGBUILD.Arch {
		artifactFilePath := filepath.Join(artifactsPath,
			fmt.Sprintf("%s_%s-%s_%s.deb",
				d.PKGBUILD.PkgName, d.PKGBUILD.PkgVer, d.PKGBUILD.PkgRel,
				arch))

		err = utils.Exec("", "apt-get", "install", "-y", artifactFilePath)

		if err != nil {
			return err
		}
	}

	return nil
}

// PrepareEnvironment prepares the environment for the Deb package.
//
// It takes a boolean parameter `golang` which indicates whether or not to set up Go.
// It returns an error if there was a problem during the environment preparation.
func (d *Deb) PrepareEnvironment(golang bool) error {
	var err error

	args := []string{
		"--assume-yes",
		"install",
	}
	args = append(args, buildEnvironmentDeps...)

	err = utils.Exec("", "apt-get", args...)
	if err != nil {
		return err
	}

	if golang {
		utils.GOSetup()
	}

	return nil
}

func (d *Deb) getRelease() {
	if d.PKGBUILD.Codename != "" {
		d.PKGBUILD.PkgRel += d.PKGBUILD.Codename
	} else {
		d.PKGBUILD.PkgRel += d.PKGBUILD.Distro
	}
}