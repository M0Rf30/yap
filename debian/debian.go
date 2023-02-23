package debian

import (
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"text/template"

	"github.com/M0Rf30/yap/pkgbuild"
	"github.com/M0Rf30/yap/utils"
)

type Debian struct {
	PKGBUILD      *pkgbuild.PKGBUILD
	debDir        string
	InstalledSize int
	sums          string
	debOutput     string
}

func (d *Debian) getArch() {
	for index, arch := range d.PKGBUILD.Arch {
		d.PKGBUILD.Arch[index] = ArchToDebian[arch]
	}
}

func (d *Debian) getDepends() error {
	var err error
	if len(d.PKGBUILD.MakeDepends) == 0 {
		return err
	}

	args := []string{
		"--assume-yes",
		"install",
	}
	args = append(args, d.PKGBUILD.MakeDepends...)

	err = utils.Exec("", "apt-get", args...)
	if err != nil {
		return err
	}

	return err
}

func (d *Debian) getUpdates() error {
	err := utils.Exec("", "apt-get", "--assume-yes", "update")
	if err != nil {
		return err
	}

	return err
}

func (d *Debian) getSums() error {
	output, err := utils.ExecOutput(d.PKGBUILD.PackageDir, "find", ".",
		"-type", "f", "-exec", "md5sum", "{}", ";")
	if err != nil {
		return err
	}

	d.sums = ""
	for _, line := range strings.Split(output, "\n") {
		d.sums += strings.Replace(line, "./", "", 1) + "\n"
	}

	return err
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

func (d *Debian) createControl() error {
	path := filepath.Join(d.debDir, "control")
	file, err := os.Create(path)

	if err != nil {
		log.Fatal(err)
	}
	// remember to close the file
	defer file.Close()
	// create new buffer
	writer := io.Writer(file)

	tmpl := template.New("control")
	tmpl.Funcs(template.FuncMap{
		"join": func(strs []string) string {
			return strings.Trim(strings.Join(strs, ", "), " ")
		},
		"multiline": func(strs string) string {
			ret := strings.ReplaceAll(strs, "\n", "\n ")

			return strings.Trim(ret, " \n")
		},
	})

	template.Must(tmpl.Parse(specFile))

	if err != nil {
		log.Fatal(err)
	}

	if pkgbuild.Verbose {
		err = tmpl.Execute(os.Stdout, d)
		if err != nil {
			log.Fatal(err)
		}
	}

	err = tmpl.Execute(writer, d)
	if err != nil {
		log.Fatal(err)
	}

	return err
}

func (d *Debian) createMd5Sums() error {
	path := filepath.Join(d.debDir, "md5sums")

	err := utils.CreateWrite(path, d.sums)
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

	err = utils.CopyFile("", template, path, false)
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

	err = utils.CopyFile("", config, path, false)
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
	path := filepath.Join(d.PKGBUILD.Root, dir+".deb")

	for _, arch := range d.PKGBUILD.Arch {
		newPath = filepath.Join(d.PKGBUILD.Home,
			fmt.Sprintf("%s_%s-%s%s_%s.deb",
				d.PKGBUILD.PkgName, d.PKGBUILD.PkgVer, d.PKGBUILD.PkgRel, d.PKGBUILD.CodeName,
				arch))
	}

	os.Remove(newPath)

	err = utils.CopyFile("", path, newPath, false)
	if err != nil {
		return "", err
	}

	return newPath, nil
}

func (d *Debian) Prep() error {
	err := d.getDepends()
	if err != nil {
		return err
	}

	return err
}

func (d *Debian) Update() error {
	err := d.getUpdates()
	if err != nil {
		return err
	}

	return err
}

func (d *Debian) createDebResources() error {
	err := d.getSums()
	if err != nil {
		return err
	}

	d.debDir = filepath.Join(d.PKGBUILD.PackageDir, "DEBIAN")
	err = utils.ExistsMakeDir(d.debDir)

	if err != nil {
		return err
	}

	err = d.createConfFiles()
	if err != nil {
		return err
	}

	err = d.createControl()
	if err != nil {
		return err
	}

	err = d.createMd5Sums()
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

func (d *Debian) Build() ([]string, error) {
	var err error

	d.InstalledSize, err = utils.GetDirSize(d.PKGBUILD.PackageDir)

	if err != nil {
		return nil, err
	}

	d.getArch()

	err = utils.RemoveAll(d.debDir)
	if err != nil {
		return nil, err
	}

	err = d.createDebResources()
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
