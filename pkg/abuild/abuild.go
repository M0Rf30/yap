package abuild

import (
	"os"
	"path/filepath"

	"github.com/M0Rf30/yap/pkg/pkgbuild"
	"github.com/M0Rf30/yap/pkg/utils"
)

// Apk represents the APK package manager.
//
// It contains the directory path of the package and the PKGBUILD struct, which
// contains the metadata and build instructions for the package.
type Apk struct {
	// PKGBUILD is a pointer to the pkgbuild.PKGBUILD struct, which contains information about the package being built.
	PKGBUILD *pkgbuild.PKGBUILD

	// apkDir is a string representing the directory where the APK package files are stored.
	apkDir string
}

// BuildPackage initiates the package building process for the Apk instance.
// It takes artifactsPath to specify where to store the build artifacts
// and calls the internal apkBuild method, returning any errors encountered.
func (a *Apk) BuildPackage(artifactsPath string) error {
	return a.apkBuild(artifactsPath)
}

// PrepareFakeroot sets up the environment for building an APK package in a fakeroot context.
// It initializes the apkDir, cleans up any existing directory, creates the necessary packer directory,
// and generates the APKBUILD and post-installation script files. The method returns an error if any step fails.
func (a *Apk) PrepareFakeroot(_ string) error {
	a.PKGBUILD.ArchComputed = APKArchs[a.PKGBUILD.ArchComputed]
	a.apkDir = filepath.Join(a.PKGBUILD.StartDir, "apk")

	if err := os.RemoveAll(a.apkDir); err != nil {
		return err
	}

	if err := a.makePackerDir(); err != nil {
		return err
	}

	tmpl := a.PKGBUILD.RenderSpec(specFile)

	if err := a.PKGBUILD.CreateSpec(filepath.Join(a.apkDir,
		"APKBUILD"), tmpl); err != nil {
		return err
	}

	tmpl = a.PKGBUILD.RenderSpec(postInstall)

	if err := a.PKGBUILD.CreateSpec(filepath.Join(a.apkDir,
		a.PKGBUILD.PkgName+".install"), tmpl); err != nil {
		return err
	}

	return nil
}

// Install installs the APK package to the specified artifacts path.
//
// It takes a string parameter `artifactsPath` which specifies the path where the artifacts are located.
// It returns an error if there was an error during the installation process.
func (a *Apk) Install(artifactsPath string) error {
	pkgName := a.PKGBUILD.PkgName + "-" +
		a.PKGBUILD.PkgVer +
		"-" +
		"r" + a.PKGBUILD.PkgRel +
		"-" +
		a.PKGBUILD.ArchComputed +
		".apk"

	pkgFilePath := filepath.Join(artifactsPath, a.PKGBUILD.PkgName, a.PKGBUILD.ArchComputed, pkgName)

	if err := utils.Exec(true,
		"apk",
		"add",
		"--allow-untrusted",
		pkgFilePath); err != nil {
		return err
	}

	return nil
}

// Prepare prepares the Apk by adding dependencies to the PKGBUILD file.
//
// makeDepends is a slice of strings representing the dependencies to be added.
// It returns an error if there is any issue with adding the dependencies.
func (a *Apk) Prepare(makeDepends []string) error {
	args := []string{
		"add",
	}

	return a.PKGBUILD.GetDepends("apk", args, makeDepends)
}

// PrepareEnvironment prepares the build environment for APK packaging.
// It installs requested Go tools if 'golang' is true.
// It returns an error if any step fails.
func (a *Apk) PrepareEnvironment(golang bool) error {
	args := []string{
		"add",
	}
	args = append(args, buildEnvironmentDeps...)

	if golang {
		utils.CheckGO()

		args = append(args, "go")
	}

	return utils.Exec(true, "", "apk", args...)
}

// Update updates the APK package manager's package database.
// It returns an error if the update process fails.
func (a *Apk) Update() error {
	return a.PKGBUILD.GetUpdates("apk", "update")
}

// apkBuild compiles the APK package using 'abuild-keygen' and 'abuild'.
// It returns an error if any compilation step fails.
func (a *Apk) apkBuild(artifactsPath string) error {
	if err := utils.Exec(true, a.apkDir,
		"abuild-keygen",
		"-n",
		"-a"); err != nil {
		return err
	}

	if err := utils.Exec(true, a.apkDir,
		"abuild",
		"-F",
		"-K",
		"-P",
		artifactsPath); err != nil {
		return err
	}

	return nil
}

// makePackerDir creates the necessary directories for the Apk struct.
//
// It does not take any parameters.
// It returns an error if any of the directory operations fail.
func (a *Apk) makePackerDir() error {
	if err := utils.ExistsMakeDir(a.apkDir); err != nil {
		return err
	}

	if err := utils.ExistsMakeDir(a.apkDir +
		"/pkg/" +
		a.PKGBUILD.PkgName); err != nil {
		return err
	}

	return nil
}
