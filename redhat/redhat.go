package redhat

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/M0Rf30/yap/pkgbuild"
	"github.com/M0Rf30/yap/set"
	"github.com/M0Rf30/yap/utils"
	"github.com/otiai10/copy"
)

type Redhat struct {
	PKGBUILD     *pkgbuild.PKGBUILD
	redhatDir    string
	buildDir     string
	buildRootDir string
	rpmsDir      string
	sourcesDir   string
	specsDir     string
	srpmsDir     string
}

func (r *Redhat) Build(artifactsPath string) error {
	r.getRPMArch()
	r.getRPMGroup()
	r.getRPMRelease()

	r.PKGBUILD.PkgDest, _ = filepath.Abs(artifactsPath)

	err := utils.RemoveAll(r.redhatDir)
	if err != nil {
		return err
	}

	err = r.makeDirs()
	if err != nil {
		return err
	}

	err = r.getFiles()
	if err != nil {
		return err
	}

	buildRootPackageDir := fmt.Sprintf("%s/%s-%s-%s.%s",
		r.buildRootDir,
		r.PKGBUILD.PkgName,
		r.PKGBUILD.PkgVer,
		r.PKGBUILD.PkgRel,
		r.PKGBUILD.Arch[0])

	err = copy.Copy(r.PKGBUILD.PackageDir, buildRootPackageDir)
	if err != nil {
		return err
	}

	err = r.PKGBUILD.CreateSpec(filepath.Join(r.specsDir, r.PKGBUILD.PkgName+".spec"), specFile)
	if err != nil {
		return err
	}

	err = r.rpmBuild()
	if err != nil {
		return err
	}

	return err
}

func (r *Redhat) Install(artifactsPath string) error {
	var err error

	for _, arch := range r.PKGBUILD.Arch {
		pkgName := r.PKGBUILD.PkgName + "-" +
			r.PKGBUILD.PkgVer +
			"-" +
			r.PKGBUILD.PkgRel +
			"." +
			RPMArchs[arch] +
			".rpm"

		pkgFilePath := filepath.Join(artifactsPath, RPMArchs[arch], pkgName)

		if err := utils.Exec("", "yum", "install", "-y", pkgFilePath); err != nil {
			return err
		}
	}

	return err
}

func (r *Redhat) PrepareEnvironment(golang bool) error {
	var err error

	args := []string{
		"-y",
		"install",
	}
	args = append(args, buildEnvironmentDeps...)

	err = utils.Exec("", "yum", args...)

	if err != nil {
		return err
	}

	if golang {
		utils.GOSetup()
	}

	return err
}

func (r *Redhat) Prepare(makeDepends []string) error {
	args := []string{
		"-y",
		"install",
	}

	err := r.PKGBUILD.GetDepends("dnf", args, makeDepends)
	if err != nil {
		return err
	}

	return err
}

func (r *Redhat) Update() error {
	var err error

	return err
}

func (r *Redhat) getFiles() error {
	backup := set.NewSet()
	paths := set.NewSet()

	for _, path := range r.PKGBUILD.Backup {
		if !strings.HasPrefix(path, "/") {
			path = "/" + path
		}

		backup.Add(path)
	}

	var files []string

	err := filepath.Walk(r.PKGBUILD.PackageDir, func(path string, info os.FileInfo, err error) error {
		if !info.IsDir() {
			files = append(files, path)
		}

		return err
	})

	if err != nil {
		return err
	}

	for _, filePath := range files {
		if len(filePath) < 1 || strings.Contains(filePath, ".build-id") {
			continue
		}

		paths.Remove(filepath.Dir(filePath))
		paths.Add(strings.TrimPrefix(filePath, r.PKGBUILD.PackageDir))
	}

	for pathInf := range paths.Iter() {
		path := pathInf.(string)

		if !strings.HasPrefix(path, "/") {
			path = "/" + path
		}

		if backup.Contains(path) {
			path = `%config "` + path + `"`
		} else {
			path = `"` + path + `"`
		}

		r.PKGBUILD.Files = append(r.PKGBUILD.Files, path)
	}

	return err
}

func (r *Redhat) getRPMArch() {
	for index, arch := range r.PKGBUILD.Arch {
		r.PKGBUILD.Arch[index] = RPMArchs[arch]
	}
}

func (r *Redhat) getRPMGroup() {
	r.PKGBUILD.Section = RPMGroups[r.PKGBUILD.Section]
}

func (r *Redhat) getRPMRelease() {
	if r.PKGBUILD.Codename != "" {
		r.PKGBUILD.PkgRel = r.PKGBUILD.PkgRel + RPMDistros[r.PKGBUILD.Distro] + r.PKGBUILD.Codename
	}
}

func (r *Redhat) makeDirs() error {
	var err error

	r.redhatDir = filepath.Join(r.PKGBUILD.StartDir, "redhat")
	r.buildDir = filepath.Join(r.redhatDir, "BUILD")
	r.buildRootDir = filepath.Join(r.redhatDir, "BUILDROOT")
	r.rpmsDir = filepath.Join(r.redhatDir, "RPMS")
	r.sourcesDir = filepath.Join(r.redhatDir, "SOURCES")
	r.specsDir = filepath.Join(r.redhatDir, "SPECS")
	r.srpmsDir = filepath.Join(r.redhatDir, "SRPMS")

	for _, path := range []string{
		r.redhatDir,
		r.buildDir,
		r.buildRootDir,
		r.rpmsDir,
		r.sourcesDir,
		r.specsDir,
		r.srpmsDir,
	} {
		err = utils.ExistsMakeDir(path)
		if err != nil {
			return err
		}
	}

	return err
}

func (r *Redhat) rpmBuild() error {
	err := utils.Exec(r.specsDir, "rpmbuild", "--define",
		"_topdir "+r.redhatDir, "-bb", r.PKGBUILD.PkgName+".spec")
	if err != nil {
		return err
	}

	return err
}
