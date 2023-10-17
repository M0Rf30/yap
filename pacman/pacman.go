package pacman

import (
	"path/filepath"

	"github.com/M0Rf30/yap/pkgbuild"
	"github.com/M0Rf30/yap/utils"
)

type Pacman struct {
	PKGBUILD  *pkgbuild.PKGBUILD
	pacmanDir string
}

func (p *Pacman) Build(artifactsPath string) error {
	p.pacmanDir = p.PKGBUILD.StartDir

	p.PKGBUILD.PkgDest, _ = filepath.Abs(artifactsPath)

	err := p.PKGBUILD.CreateSpec(filepath.Join(p.pacmanDir,
		"PKGBUILD"), specFile)
	if err != nil {
		return err
	}

	err = p.PKGBUILD.CreateSpec(filepath.Join(p.pacmanDir,
		p.PKGBUILD.PkgName+".install"), postInstall)
	if err != nil {
		return err
	}

	err = p.pacmanBuild()
	if err != nil {
		return err
	}

	return nil
}

func (p *Pacman) Install(artifactsPath string) error {
	var err error

	for _, arch := range p.PKGBUILD.Arch {
		pkgName := p.PKGBUILD.PkgName + "-" +
			p.PKGBUILD.PkgVer +
			"-" +
			p.PKGBUILD.PkgRel +
			"-" +
			arch +
			".pkg.tar.zst"

		pkgFilePath := filepath.Join(artifactsPath, pkgName)

		if err := utils.Exec("", "pacman", "-U", "--noconfirm", pkgFilePath); err != nil {
			return err
		}
	}

	return err
}

func (p *Pacman) Prepare(makeDepends []string) error {
	args := []string{
		"-S",
		"--noconfirm",
	}

	err := p.PKGBUILD.GetDepends("pacman", args, makeDepends)
	if err != nil {
		return err
	}

	return err
}

func (p *Pacman) PrepareEnvironment(golang bool) error {
	var err error

	args := []string{
		"-S",
		"--noconfirm",
	}
	args = append(args, buildEnvironmentDeps...)

	if golang {
		utils.CheckGO()

		args = append(args, "go")
	}

	err = utils.Exec("", "pacman", args...)
	if err != nil {
		return err
	}

	return err
}

func (p *Pacman) Update() error {
	err := p.PKGBUILD.GetUpdates("pacman", "-Sy")
	if err != nil {
		return err
	}

	return err
}

func (p *Pacman) pacmanBuild() error {
	err := utils.Exec(p.pacmanDir, "makepkg", "-f")
	if err != nil {
		return err
	}

	return err
}
