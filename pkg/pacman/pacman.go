package pacman

import (
	"path/filepath"

	"github.com/M0Rf30/yap/pkg/pkgbuild"
	"github.com/M0Rf30/yap/pkg/utils"
)

// Pacman represents a package manager for the Pacman distribution.
//
// It contains methods for building, installing, and updating packages.
type Pacman struct {
	PKGBUILD  *pkgbuild.PKGBUILD
	pacmanDir string
}

// Build builds the Pacman package.
//
// It takes the artifactsPath as a parameter and returns an error if any.
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

// Install installs the package using the given artifacts path.
//
// artifactsPath: the path where the package artifacts are located.
// error: an error if the installation fails.
func (p *Pacman) Install(artifactsPath string) error {
	for _, arch := range p.PKGBUILD.Arch {
		pkgName := p.PKGBUILD.PkgName + "-" +
			p.PKGBUILD.PkgVer +
			"-" +
			p.PKGBUILD.PkgRel +
			"-" +
			arch +
			".pkg.tar.zst"

		pkgFilePath := filepath.Join(artifactsPath, pkgName)

		if err := utils.Exec("",
			"pacman",
			"-U",
			"--noconfirm",
			pkgFilePath); err != nil {
			return err
		}
	}

	return nil
}

// Prepare prepares the Pacman package by getting the dependencies using the PKGBUILD.
//
// makeDepends is a slice of strings representing the dependencies to be included.
// It returns an error if there is any issue getting the dependencies.
func (p *Pacman) Prepare(makeDepends []string) error {
	args := []string{
		"-S",
		"--noconfirm",
	}

	err := p.PKGBUILD.GetDepends("pacman", args, makeDepends)
	if err != nil {
		return err
	}

	return nil
}

// PrepareEnvironment prepares the environment for the Pacman.
//
// It takes a boolean parameter `golang` which indicates whether the environment should be prepared for Golang.
// It returns an error if there is any issue in preparing the environment.
func (p *Pacman) PrepareEnvironment(golang bool) error {
	args := []string{
		"-S",
		"--noconfirm",
	}
	args = append(args, buildEnvironmentDeps...)

	if golang {
		utils.CheckGO()

		args = append(args, "go")
	}

	return utils.Exec("", "pacman", args...)
}

// Update updates the Pacman package manager.
//
// It retrieves the updates using the GetUpdates method of the PKGBUILD struct.
// It returns an error if there is any issue during the update process.
func (p *Pacman) Update() error {
	return p.PKGBUILD.GetUpdates("pacman", "-Sy")
}

// pacmanBuild builds the package using makepkg command.
//
// It executes the makepkg command in the pacman directory and returns an error if any.
// The error is returned as is.
// Returns:
// - error: An error if any occurred during the execution of the makepkg command.
func (p *Pacman) pacmanBuild() error {
	return utils.Exec(p.pacmanDir, "makepkg", "-f")
}
