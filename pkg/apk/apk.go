package apk

import (
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

// Build builds the APK package.
//
// It takes the artifactsPath as a parameter and returns an error.
func (a *Apk) Build(artifactsPath string) error {
	a.apkDir = filepath.Join(a.PKGBUILD.StartDir, "apk")

	err := utils.RemoveAll(a.apkDir)
	if err != nil {
		return err
	}

	err = a.makePackerDir()
	if err != nil {
		return err
	}

	err = a.PKGBUILD.CreateSpec(filepath.Join(a.apkDir, "APKBUILD"), specFile)
	if err != nil {
		return err
	}

	err = a.PKGBUILD.CreateSpec(filepath.Join(a.apkDir, a.PKGBUILD.PkgName+".install"), postInstall)
	if err != nil {
		return err
	}

	err = a.apkBuild(artifactsPath)
	if err != nil {
		return err
	}

	return nil
}

// Install installs the APK package to the specified artifacts path.
//
// It takes a string parameter `artifactsPath` which specifies the path where the artifacts are located.
// It returns an error if there was an error during the installation process.
func (a *Apk) Install(artifactsPath string) error {
	for _, arch := range a.PKGBUILD.Arch {
		pkgName := a.PKGBUILD.PkgName + "-" +
			a.PKGBUILD.PkgVer +
			"-" +
			"r" + a.PKGBUILD.PkgRel +
			"-" +
			arch +
			".apk"

		pkgFilePath := filepath.Join(artifactsPath, a.PKGBUILD.PkgName, arch, pkgName)

		if err := utils.Exec("",
			"apk",
			"add",
			"--allow-untrusted",
			pkgFilePath); err != nil {
			return err
		}
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

	err := a.PKGBUILD.GetDepends("apk", args, makeDepends)
	if err != nil {
		return err
	}

	return nil
}

// PrepareEnvironment prepares the build environment for APK packaging.
// It installs requested Go tools if 'golang' is true.
// It returns an error if any step fails.
func (a *Apk) PrepareEnvironment(golang bool) error {
	var err error

	args := []string{
		"add",
	}
	args = append(args, buildEnvironmentDeps...)

	if golang {
		utils.CheckGO()

		args = append(args, "go")
	}

	err = utils.Exec("", "apk", args...)
	if err != nil {
		return err
	}

	return nil
}

// Update updates the APK package manager's package database.
// It returns an error if the update process fails.
func (a *Apk) Update() error {
	err := a.PKGBUILD.GetUpdates("apk", "update")
	if err != nil {
		return err
	}

	return nil
}

// apkBuild compiles the APK package using 'abuild-keygen' and 'abuild'.
// It returns an error if any compilation step fails.
func (a *Apk) apkBuild(artifactsPath string) error {
	err := utils.Exec(a.apkDir,
		"abuild-keygen",
		"-n",
		"-a")
	if err != nil {
		return err
	}

	err = utils.Exec(a.apkDir,
		"abuild",
		"-F",
		"-K",
		"-P",
		artifactsPath)
	if err != nil {
		return err
	}

	return nil
}

// makePackerDir creates the necessary directories for the Apk struct.
//
// It does not take any parameters.
// It returns an error if any of the directory operations fail.
func (a *Apk) makePackerDir() error {
	err := utils.ExistsMakeDir(a.apkDir)
	if err != nil {
		return err
	}

	err = utils.ExistsMakeDir(a.apkDir +
		"/pkg/" +
		a.PKGBUILD.PkgName)
	if err != nil {
		return err
	}

	return nil
}
