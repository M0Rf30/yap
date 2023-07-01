package debian

import (
	"bytes"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"text/template"

	"github.com/M0Rf30/yap/builder"
	"github.com/M0Rf30/yap/constants"
	"github.com/M0Rf30/yap/options"
	"github.com/M0Rf30/yap/pkgbuild"
	"github.com/M0Rf30/yap/utils"
	"github.com/otiai10/copy"
)

type Debian struct {
	debDir    string
	debOutput string
	PKGBUILD  *pkgbuild.PKGBUILD
	// sums          string
}

func (d *Debian) getArch() {
	for index, arch := range d.PKGBUILD.Arch {
		d.PKGBUILD.Arch[index] = ArchToDebian[arch]
	}
}

func (d *Debian) createConfFiles() error {
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

	return err
}

func (d *Debian) createDebconfTemplate() error {
	var err error
	if len(d.PKGBUILD.DebTemplate) == 0 {
		return err
	}

	template := filepath.Join(d.PKGBUILD.Home, d.PKGBUILD.DebTemplate)
	path := filepath.Join(d.debDir, "templates")

	err = copy.Copy(template, path)
	if err != nil {
		return err
	}

	return err
}

func (d *Debian) createDebconfConfig() error {
	var err error
	if len(d.PKGBUILD.DebConfig) == 0 {
		return err
	}

	config := filepath.Join(d.PKGBUILD.Home, d.PKGBUILD.DebConfig)
	path := filepath.Join(d.debDir, "config")

	err = copy.Copy(config, path)
	if err != nil {
		return err
	}

	return err
}

func (d *Debian) createScripts() error {
	var err error

	scripts := map[string]string{
		"preinst":  d.PKGBUILD.PreInst,
		"postinst": d.PKGBUILD.PostInst,
		"prerm":    d.PKGBUILD.PreRm,
		"postrm":   d.PKGBUILD.PostRm,
	}

	for name, script := range scripts {
		if len(script) == 0 {
			continue
		}

		data := script + "\n"
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

	return err
}

func (d *Debian) dpkgDeb() (string, error) {
	var newPath string

	err := utils.Exec("", "dpkg-deb", "-b", d.PKGBUILD.PackageDir)

	if err != nil {
		return "", err
	}

	_, dir := filepath.Split(filepath.Clean(d.PKGBUILD.PackageDir))
	path := filepath.Join(d.PKGBUILD.StartDir, dir+".deb")

	for _, arch := range d.PKGBUILD.Arch {
		newPath = filepath.Join(d.PKGBUILD.Home,
			fmt.Sprintf("%s_%s-%s%s_%s.deb",
				d.PKGBUILD.PkgName, d.PKGBUILD.PkgVer, d.PKGBUILD.PkgRel, d.PKGBUILD.CodeName,
				arch))
	}

	os.Remove(newPath)

	err = copy.Copy(path, newPath)
	if err != nil {
		return "", err
	}

	return newPath, nil
}

func (d *Debian) Prepare(makeDepends []string) error {
	args := []string{
		"--assume-yes",
		"install",
	}

	err := d.PKGBUILD.GetDepends("apt-get", args, makeDepends)
	if err != nil {
		return err
	}

	return err
}

func (d *Debian) Strip() error {
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

	return err
}

func (d *Debian) Update() error {
	err := d.PKGBUILD.GetUpdates("apt-get", "update")
	if err != nil {
		return err
	}

	return err
}

func (d *Debian) createDebResources() error {
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

	// err = d.createMd5Sums()
	// if err != nil {
	// 	return err
	// }

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

	return err
}

func (d *Debian) Build() ([]string, error) {
	var err error

	d.getArch()

	err = utils.RemoveAll(d.debDir)
	if err != nil {
		return nil, err
	}

	err = d.createDebResources()
	if err != nil {
		return nil, err
	}

	err = d.Strip()
	if err != nil {
		return nil, err
	}

	dpkgDeb, err := d.dpkgDeb()
	if err != nil {
		return nil, err
	}

	d.debOutput = dpkgDeb

	err = utils.RemoveAll(d.PKGBUILD.PackageDir)
	if err != nil {
		return nil, err
	}

	return []string{dpkgDeb}, nil
}

func (d *Debian) Install() error {
	absPath, err := filepath.Abs(d.debOutput)
	if err != nil {
		return err
	}

	return utils.Exec("", "apt-get", "install", "-y", absPath)
}

func (d *Debian) PrepareEnvironment(golang bool) error {
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

	return err
}
