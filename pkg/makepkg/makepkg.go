package makepkg

import (
	"path/filepath"

	"github.com/M0Rf30/yap/pkg/pkgbuild"
	"github.com/M0Rf30/yap/pkg/utils"
)

// Makepkg represents a package manager for the Makepkg distribution.
//
// It contains methods for building, installing, and updating packages.
type Makepkg struct {
	PKGBUILD  *pkgbuild.PKGBUILD
	pacmanDir string
}

// BuildPackage initiates the package building process for the Makepkg instance.
//
// It takes a single parameter:
// - artifactsPath: a string representing the path where the build artifacts will be stored.
//
// The method calls the internal pacmanBuild function to perform the actual build process.
// It returns an error if the build process encounters any issues.
func (m *Makepkg) BuildPackage(_ string) error {
	return m.pacmanBuild()
}

// PrepareFakeroot sets um the environment for building a package in a fakeroot context.
//
// It takes an artifactsPath parameter, which specifies where to store build artifacts.
// The method initializes the pacmanDir, resolves the package destination, and creates
// the PKGBUILD and post-installation script files if necessary. It returns an error
// if any stem fails.
func (m *Makepkg) PrepareFakeroot(artifactsPath string) error {
	m.pacmanDir = m.PKGBUILD.StartDir

	m.PKGBUILD.PkgDest, _ = filepath.Abs(artifactsPath)

	tmpl := m.PKGBUILD.RenderSpec(specFile)

	if m.PKGBUILD.Home != m.PKGBUILD.StartDir {
		err := m.PKGBUILD.CreateSpec(filepath.Join(m.pacmanDir,
			"PKGBUILD"), tmpl)
		if err != nil {
			return err
		}
	}

	tmpl = m.PKGBUILD.RenderSpec(postInstall)
	err := m.PKGBUILD.CreateSpec(filepath.Join(m.pacmanDir,
		m.PKGBUILD.PkgName+".install"), tmpl)

	if err != nil {
		return err
	}

	return nil
}

// Install installs the package using the given artifacts path.
//
// artifactsPath: the path where the package artifacts are located.
// error: an error if the installation fails.
func (m *Makepkg) Install(artifactsPath string) error {
	pkgName := m.PKGBUILD.PkgName + "-" +
		m.PKGBUILD.PkgVer +
		"-" +
		m.PKGBUILD.PkgRel +
		"-" +
		m.PKGBUILD.ArchComputed +
		".pkg.tar.zst"

	pkgFilePath := filepath.Join(artifactsPath, pkgName)

	if err := utils.Exec(false, "",
		"pacman",
		"-U",
		"--noconfirm",
		pkgFilePath); err != nil {
		return err
	}

	return nil
}

// Prepare prepares the Makepkg package by getting the dependencies using the PKGBUILD.
//
// makeDepends is a slice of strings representing the dependencies to be included.
// It returns an error if there is any issue getting the dependencies.
func (m *Makepkg) Prepare(makeDepends []string) error {
	args := []string{
		"-S",
		"--noconfirm",
	}

	return m.PKGBUILD.GetDepends("pacman", args, makeDepends)
}

// PrepareEnvironment prepares the environment for the Makepkg.
//
// It takes a boolean parameter `golang` which indicates whether the environment should be prepared for Golang.
// It returns an error if there is any issue in preparing the environment.
func (m *Makepkg) PrepareEnvironment(golang bool) error {
	args := []string{
		"-S",
		"--noconfirm",
	}
	args = append(args, buildEnvironmentDeps...)

	if golang {
		utils.CheckGO()

		args = append(args, "go")
	}

	return utils.Exec(false, "", "pacman", args...)
}

// Update updates the Makepkg package manager.
//
// It retrieves the updates using the GetUpdates method of the PKGBUILD struct.
// It returns an error if there is any issue during the update process.
func (m *Makepkg) Update() error {
	return m.PKGBUILD.GetUpdates("pacman", "-Sy")
}

// pacmanBuild builds the package using makepkg command.
//
// It executes the makepkg command in the pacman directory and returns an error if any.
// The error is returned as is.
// Returns:
// - error: An error if any occurred during the execution of the makepkg command.
func (m *Makepkg) pacmanBuild() error {
	return utils.Exec(true, m.pacmanDir, "makepkg", "-ef")
}
