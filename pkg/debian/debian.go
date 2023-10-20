package debian

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

type Debian struct {
	debDir   string
	PKGBUILD *pkgbuild.PKGBUILD
}

func (d *Debian) getArch() {
	for index, arch := range d.PKGBUILD.Arch {
		d.PKGBUILD.Arch[index] = DebianArchs[arch]
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
	if d.PKGBUILD.DebTemplate == "" {
		return err
	}

	debconfTemplate := filepath.Join(d.PKGBUILD.Home, d.PKGBUILD.DebTemplate)
	path := filepath.Join(d.debDir, "templates")

	err = copy.Copy(debconfTemplate, path)
	if err != nil {
		return err
	}

	return err
}

func (d *Debian) createDebconfConfig() error {
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

	return err
}

func (d *Debian) dpkgDeb(artifactPath string) error {
	var err error

	for _, arch := range d.PKGBUILD.Arch {
		artifactFilePath := filepath.Join(artifactPath,
			fmt.Sprintf("%s_%s-%s_%s.deb",
				d.PKGBUILD.PkgName, d.PKGBUILD.PkgVer, d.PKGBUILD.PkgRel,
				arch))

		err = utils.Exec("", "dpkg-deb", "-b", d.PKGBUILD.PackageDir, artifactFilePath)

		if err != nil {
			return err
		}
	}

	return err
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

func (d *Debian) Build(artifactsPath string) error {
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

	return err
}

func (d *Debian) Install(artifactsPath string) error {
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

	return err
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

func (d *Debian) getRelease() {
	if d.PKGBUILD.Codename != "" {
		d.PKGBUILD.PkgRel += d.PKGBUILD.Codename
	} else {
		d.PKGBUILD.PkgRel += d.PKGBUILD.Distro
	}
}
