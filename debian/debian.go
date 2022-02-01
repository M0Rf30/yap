package debian

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/packagefoundation/yap/constants"
	"github.com/packagefoundation/yap/pack"
	"github.com/packagefoundation/yap/utils"
)

type Debian struct {
	Pack        *pack.Pack
	debDir      string
	installSize int
	sums        string
	debOutput   string
}

func (d *Debian) getDepends() (err error) {
	if len(d.Pack.MakeDepends) == 0 {
		return
	}

	args := []string{
		"--assume-yes",
		"install",
	}
	args = append(args, d.Pack.MakeDepends...)

	err = utils.Exec("", "apt-get", args...)
	if err != nil {
		return
	}

	return
}

func (d *Debian) getUpdates() (err error) {
	err = utils.Exec("", "apt-get", "--assume-yes", "update")
	if err != nil {
		return
	}

	return
}

func (d *Debian) getSums() (err error) {
	output, err := utils.ExecOutput(d.Pack.PackageDir, "find", ".",
		"-type", "f", "-exec", "md5sum", "{}", ";")
	if err != nil {
		return
	}

	d.sums = ""
	for _, line := range strings.Split(output, "\n") {
		d.sums += strings.Replace(line, "./", "", 1) + "\n"
	}

	return
}

func (d *Debian) createConfFiles() (err error) {
	if len(d.Pack.Backup) == 0 {
		return
	}

	path := filepath.Join(d.debDir, "conffiles")

	data := ""

	for _, name := range d.Pack.Backup {
		if !strings.HasPrefix(name, "/") {
			name = "/" + name
		}

		data += name + "\n"
	}

	err = utils.CreateWrite(path, data)
	if err != nil {
		return
	}

	return
}

func (d *Debian) createControl() (err error) {
	path := filepath.Join(d.debDir, "control")

	data := ""

	data += fmt.Sprintf("Package: %s\n", d.Pack.PkgName)
	data += fmt.Sprintf("Version: %s-%s%s1~%s\n",
		d.Pack.PkgVer, d.Pack.PkgRel, d.Pack.Distro, d.Pack.Release)
	data += fmt.Sprintf("Architecture: %s\n", d.Pack.Arch)
	data += fmt.Sprintf("Maintainer: %s\n", d.Pack.Maintainer)
	data += fmt.Sprintf("Installed-Size: %d\n", d.installSize)

	if len(d.Pack.Depends) > 0 {
		data += fmt.Sprintf("Depends: %s\n",
			strings.Join(d.Pack.Depends, ", "))
	}

	if len(d.Pack.Conflicts) > 0 {
		data += fmt.Sprintf("Conflicts: %s\n",
			strings.Join(d.Pack.Conflicts, ", "))
	}

	if len(d.Pack.OptDepends) > 0 {
		data += fmt.Sprintf("Recommends: %s\n",
			strings.Join(d.Pack.OptDepends, ", "))
	}

	if len(d.Pack.Provides) > 0 {
		data += fmt.Sprintf("Provides: %s\n",
			strings.Join(d.Pack.Provides, ", "))
	}

	data += fmt.Sprintf("Section: %s\n", d.Pack.Section)
	data += fmt.Sprintf("Priority: %s\n", d.Pack.Priority)
	data += fmt.Sprintf("Homepage: %s\n", d.Pack.URL)
	data += fmt.Sprintf("Description: %s\n", d.Pack.PkgDesc)

	for _, line := range d.Pack.PkgDescLong {
		if line == "" {
			line = "."
		}

		data += fmt.Sprintf("  %s\n", line)
	}

	err = utils.CreateWrite(path, data)
	if err != nil {
		return
	}

	return err
}

func (d *Debian) createMd5Sums() (err error) {
	path := filepath.Join(d.debDir, "md5sums")

	err = utils.CreateWrite(path, d.sums)
	if err != nil {
		return
	}

	return
}

func (d *Debian) createDebconfTemplate() (err error) {
	if len(d.Pack.DebTemplate) == 0 {
		return
	}

	template := filepath.Join(d.Pack.Home, d.Pack.DebTemplate)
	path := filepath.Join(d.debDir, "templates")

	err = utils.CopyFile("", template, path, false)
	if err != nil {
		return
	}

	return
}

func (d *Debian) createDebconfConfig() (err error) {
	if len(d.Pack.DebConfig) == 0 {
		return
	}

	config := filepath.Join(d.Pack.Home, d.Pack.DebConfig)
	path := filepath.Join(d.debDir, "config")

	err = utils.CopyFile("", config, path, false)
	if err != nil {
		return
	}

	return
}

func (d *Debian) createScripts() (err error) {
	scripts := map[string][]string{
		"preinst":  d.Pack.PreInst,
		"postinst": d.Pack.PostInst,
		"prerm":    d.Pack.PreRm,
		"postrm":   d.Pack.PostRm,
	}

	for name, script := range scripts {
		if len(script) == 0 {
			continue
		}

		data := strings.Join(script, "\n")
		if name == "prerm" || name == "postrm" {
			data = removeHeader + data
		}

		path := filepath.Join(d.debDir, name)

		err = utils.CreateWrite(path, data)
		if err != nil {
			return
		}

		err = utils.Chmod(path, 0o755)
		if err != nil {
			return
		}
	}

	return err
}

func (d *Debian) clean() (err error) {
	if !constants.CleanPrevious {
		return
	}

	pkgPaths, err := utils.FindExt(d.Pack.Home, ".deb")
	if err != nil {
		return
	}

	match, ok := constants.ReleasesMatch[d.Pack.FullRelease]
	if !ok {
		fmt.Printf("debian: Failed to find match for '%s'\n",
			d.Pack.FullRelease)
	}

	for _, pkgPath := range pkgPaths {
		if strings.Contains(filepath.Base(pkgPath), match) {
			_ = utils.Remove(pkgPath)
		}
	}

	return
}

func (d *Debian) dpkgDeb() (string, error) {
	err := utils.Exec("", "dpkg-deb", "-b", d.Pack.PackageDir)
	if err != nil {
		return "", err
	}

	_, dir := filepath.Split(filepath.Clean(d.Pack.PackageDir))
	path := filepath.Join(d.Pack.Root, dir+".deb")
	newPath := filepath.Join(d.Pack.Home,
		fmt.Sprintf("%s_%s-%s%s_%s.deb",
			d.Pack.PkgName, d.Pack.PkgVer, d.Pack.PkgRel, d.Pack.Release,
			d.Pack.Arch))

	os.Remove(newPath)

	err = utils.CopyFile("", path, newPath, false)
	if err != nil {
		return "", err
	}

	return newPath, nil
}

func (d *Debian) Prep() (err error) {
	err = d.getDepends()
	if err != nil {
		return
	}

	return
}

func (d *Debian) Update() (err error) {
	err = d.getUpdates()
	if err != nil {
		return
	}

	return
}

func (d *Debian) Build() ([]string, error) {
	var err error
	d.installSize, err = utils.GetDirSize(d.Pack.PackageDir)

	if err != nil {
		return nil, err
	}

	err = d.getSums()
	if err != nil {
		return nil, err
	}

	d.debDir = filepath.Join(d.Pack.PackageDir, "DEBIAN")
	err = utils.ExistsMakeDir(d.debDir)

	if err != nil {
		return nil, err
	}

	defer os.RemoveAll(d.debDir)

	err = d.createConfFiles()
	if err != nil {
		return nil, err
	}

	err = d.createControl()
	if err != nil {
		return nil, err
	}

	err = d.createMd5Sums()
	if err != nil {
		return nil, err
	}

	err = d.createScripts()
	if err != nil {
		return nil, err
	}

	err = d.createDebconfTemplate()
	if err != nil {
		return nil, err
	}

	err = d.createDebconfConfig()
	if err != nil {
		return nil, err
	}

	err = d.clean()
	if err != nil {
		return nil, err
	}

	dpkgDeb, err := d.dpkgDeb()
	if err != nil {
		return nil, err
	}

	d.debOutput = dpkgDeb

	return []string{dpkgDeb}, nil
}

func (d *Debian) Install() error {
	absPath, err := filepath.Abs(d.debOutput)
	if err != nil {
		return err
	}

	return utils.Exec("", "apt-get", "install", "-y", absPath)
}
