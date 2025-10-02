// Package apk provides functionality for building Alpine Linux APK packages.
package apk

import (
	"path/filepath"
	"time"

	"github.com/M0Rf30/yap/v2/pkg/builders/common"
	"github.com/M0Rf30/yap/v2/pkg/constants"
	"github.com/M0Rf30/yap/v2/pkg/files"
	"github.com/M0Rf30/yap/v2/pkg/pkgbuild"
	"github.com/M0Rf30/yap/v2/pkg/shell"
)

// Apk represents the APK package builder.
// It embeds the common.BaseBuilder to inherit shared functionality.
type Apk struct {
	*common.BaseBuilder
}

// NewBuilder creates a new APK package builder.
func NewBuilder(pkgBuild *pkgbuild.PKGBUILD) *Apk {
	return &Apk{
		BaseBuilder: common.NewBaseBuilder(pkgBuild, "apk"),
	}
}

// BuildPackage creates an APK package in pure Go without external dependencies.
//
// NOTE: APK builder is currently incomplete. This is a placeholder implementation
// that needs to be finished. The APK format requires implementing:
// - APK package structure (APKINDEX, .PKGINFO, install scripts)
// - Tar archive creation with proper compression
// - Signature generation for package verification
func (a *Apk) BuildPackage(artifactsPath string) error {
	a.TranslateArchitecture()

	pkgName := a.BuildPackageName(".apk")
	pkgFilePath := filepath.Join(artifactsPath, pkgName)

	err := a.createAPKPackage(pkgFilePath, artifactsPath)
	if err != nil {
		return err
	}

	a.LogPackageCreated(pkgFilePath)

	return nil
}

// PrepareFakeroot sets up the APK package metadata.
//
// NOTE: APK builder is currently incomplete. This method prepares metadata
// but the actual package building logic (createPkgInfo, createInstallScript)
// needs to be implemented.
func (a *Apk) PrepareFakeroot(artifactsPath string) error {
	a.TranslateArchitecture()

	a.PKGBUILD.InstalledSize, _ = files.GetDirSize(a.PKGBUILD.PackageDir)
	a.PKGBUILD.BuildDate = time.Now().Unix()
	a.PKGBUILD.PkgDest = artifactsPath
	a.PKGBUILD.YAPVersion = constants.YAPVersion

	err := a.createPkgInfo()
	if err != nil {
		return err
	}

	if a.PKGBUILD.PreInst != "" || a.PKGBUILD.PostInst != "" ||
		a.PKGBUILD.PreRm != "" || a.PKGBUILD.PostRm != "" {
		err = a.createInstallScript()
		if err != nil {
			return err
		}
	}

	return nil
}

// Install installs the APK package.
func (a *Apk) Install(artifactsPath string) error {
	pkgName := a.BuildPackageName(".apk")
	pkgFilePath := filepath.Join(artifactsPath, pkgName)
	installArgs := constants.GetInstallArgs("apk")
	installArgs = append(installArgs, pkgFilePath)

	return shell.ExecWithSudo(true, "", "apk", installArgs...)
}

// Prepare prepares the build environment.
func (a *Apk) Prepare(makeDepends []string) error {
	installArgs := constants.GetInstallArgs("apk")
	return a.PKGBUILD.GetDepends("apk", installArgs, makeDepends)
}

// PrepareEnvironment prepares the build environment.
func (a *Apk) PrepareEnvironment(golang bool) error {
	allArgs := a.SetupEnvironmentDependencies(golang)
	return shell.ExecWithSudo(true, "", "apk", allArgs...)
}

// Update updates the APK package database.
func (a *Apk) Update() error {
	return a.PKGBUILD.GetUpdates("apk", "update")
}

// TODO: Implement APK package building
//
// The following methods are placeholders that need proper implementation:
//
// createAPKPackage should:
//   - Create APK tar.gz archive with proper structure
//   - Include .PKGINFO metadata file
//   - Add all package files with correct permissions
//   - Generate and sign package checksums
//
// createPkgInfo should:
//   - Generate .PKGINFO file with package metadata
//   - Include package name, version, architecture
//   - Add dependencies, conflicts, provides information
//   - Write to package directory for inclusion in APK
//
// createInstallScript should:
//   - Create install/upgrade/removal scripts
//   - Handle pre-install, post-install, pre-remove, post-remove hooks
//   - Follow APK script conventions
//
// Reference: https://wiki.alpinelinux.org/wiki/Apk_spec

func (a *Apk) createAPKPackage(pkgFilePath, artifactsPath string) error {
	return nil
}

func (a *Apk) createPkgInfo() error {
	return nil
}

func (a *Apk) createInstallScript() error {
	return nil
}
