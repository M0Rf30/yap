package dpkg

import (
	"bytes"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
	"text/template"

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

func (d *Deb) createCopyrightFile() error {
	if len(d.PKGBUILD.License) == 0 {
		return nil
	}

	copyrightFilePath := filepath.Join(d.debDir, "copyright")
	tmpl := d.PKGBUILD.RenderSpec(copyrightFile)

	return d.PKGBUILD.CreateSpec(copyrightFilePath, tmpl)
}

// createDebconfFile creates a debconf file with the given variable and name.
//
// Parameters:
// - variable: the variable used to create the debconf asset.
// - name: the name of the debconf asset.
//
// Return type: error.
func (d *Deb) createDebconfFile(name, variable string) error {
	if variable == "" {
		return nil
	}

	assetPath := filepath.Join(d.PKGBUILD.Home, variable)
	destPath := filepath.Join(d.debDir, name)

	return copy.Copy(assetPath, destPath)
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

// dpkgDeb generates Deb package files from the given artifact path.
//
// It takes a string parameter `artifactPath` which represents the path where the
// Deb package files will be generated.
//
// The function returns an error if there was an issue generating the Deb package
// files.
func (d *Deb) dpkgDeb(artifactPath string) error {
	for _, arch := range d.PKGBUILD.Arch {
		artifactFilePath := filepath.Join(artifactPath,
			fmt.Sprintf("%s_%s-%s_%s.deb",
				d.PKGBUILD.PkgName, d.PKGBUILD.PkgVer, d.PKGBUILD.PkgRel,
				arch))

		if err := utils.Exec(true, "",
			"dpkg-deb",
			"-b",
			"-Zzstd",
			d.PKGBUILD.PackageDir, artifactFilePath); err != nil {
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

	return d.PKGBUILD.GetDepends("apt-get", args, makeDepends)
}

// Strip strips binaries from the Deb package.
//
// It does not take any parameters.
// It returns an error if there is any issue during stripping.
func (d *Deb) Strip() error {
	var tmplBytesBuffer bytes.Buffer

	utils.Logger.Info("stripping binaries")

	tmpl := template.New("strip")

	template.Must(tmpl.Parse(options.StripScript))

	if pkgbuild.Verbose {
		return tmpl.Execute(&tmplBytesBuffer, d.PKGBUILD)
	}

	return utils.RunScript(tmplBytesBuffer.String())
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

	if err := d.createScripts(); err != nil {
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

// Build builds the Deb package.
//
// It takes the artifactsPath as a parameter and returns an error if any.
func (d *Deb) Build(artifactsPath string) error {
	d.getArch()
	d.getRelease()

	if err := utils.RemoveAll(d.debDir); err != nil {
		return err
	}

	if err := d.createDebResources(); err != nil {
		return err
	}

	if err := d.Strip(); err != nil {
		return err
	}

	if err := d.dpkgDeb(artifactsPath); err != nil {
		return err
	}

	if err := utils.RemoveAll(d.PKGBUILD.PackageDir); err != nil {
		return err
	}

	return nil
}

func (d *Deb) Install(artifactsPath string) error {
	for _, arch := range d.PKGBUILD.Arch {
		artifactFilePath := filepath.Join(artifactsPath,
			fmt.Sprintf("%s_%s-%s_%s.deb",
				d.PKGBUILD.PkgName, d.PKGBUILD.PkgVer, d.PKGBUILD.PkgRel,
				arch))

		if err := utils.Exec(false, "", "apt-get", "install", "-y", artifactFilePath); err != nil {
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
