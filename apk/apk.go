package apk

import (
	"path/filepath"

	"github.com/M0Rf30/yap/pkgbuild"
	"github.com/M0Rf30/yap/utils"
)

type Apk struct {
	PKGBUILD *pkgbuild.PKGBUILD
	apkDir   string
}

func (a *Apk) apkBuild(artifactsPath string) error {
	err := utils.Exec(a.apkDir, "abuild-keygen", "-n", "-a")
	if err != nil {
		return err
	}

	err = utils.Exec(a.apkDir, "abuild", "-F", "-K", "-P", artifactsPath)
	if err != nil {
		return err
	}

	return err
}

func (a *Apk) Prepare(makeDepends []string) error {
	args := []string{
		"add",
	}

	err := a.PKGBUILD.GetDepends("apk", args, makeDepends)
	if err != nil {
		return err
	}

	return err
}

func (a *Apk) Update() error {
	err := a.PKGBUILD.GetUpdates("apk", "update")
	if err != nil {
		return err
	}

	return err
}

func (a *Apk) makePackerDir() error {
	err := utils.ExistsMakeDir(a.apkDir)
	if err != nil {
		return err
	}

	err = utils.ExistsMakeDir(a.apkDir + "/pkg/" + a.PKGBUILD.PkgName)
	if err != nil {
		return err
	}

	return err
}

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

	return err
}

func (a *Apk) Install(artifactsPath string) error {
	pkgs, err := utils.FindExt(artifactsPath, ".apk")
	if err != nil {
		return err
	}

	for _, pkg := range pkgs {
		if err := utils.Exec("", "apk", "add", "--allow-untrusted", pkg); err != nil {
			return err
		}
	}

	return nil
}

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

	return err
}
