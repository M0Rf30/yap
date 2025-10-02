// Package apk provides functionality for building Alpine Linux APK packages.
package apk

import (
	"fmt"
	"path/filepath"
	"time"

	"github.com/M0Rf30/yap/v2/pkg/builders/common"
	"github.com/M0Rf30/yap/v2/pkg/constants"
	"github.com/M0Rf30/yap/v2/pkg/files"
	"github.com/M0Rf30/yap/v2/pkg/pkgbuild"
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

func (a *Apk) createAPKPackage(_, _ string) error {
	return fmt.Errorf(
		"APK package building is not yet implemented - " +
			"see TODO above for required implementation details",
	)
}

func (a *Apk) createPkgInfo() error {
	return fmt.Errorf(
		"APK .PKGINFO generation is not yet implemented - " +
			"see TODO above for required implementation details",
	)
}

func (a *Apk) createInstallScript() error {
	return fmt.Errorf(
		"APK install script generation is not yet implemented - " +
			"see TODO above for required implementation details",
	)
}
