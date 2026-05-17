// Package packer provides unified package building interface for multiple formats.
package packer

import (
	"github.com/M0Rf30/yap/v2/pkg/builders/apk"
	"github.com/M0Rf30/yap/v2/pkg/builders/deb"
	"github.com/M0Rf30/yap/v2/pkg/builders/pacman"
	"github.com/M0Rf30/yap/v2/pkg/builders/rpm"
	"github.com/M0Rf30/yap/v2/pkg/constants"
	"github.com/M0Rf30/yap/v2/pkg/core"
	"github.com/M0Rf30/yap/v2/pkg/errors"
	"github.com/M0Rf30/yap/v2/pkg/i18n"
	"github.com/M0Rf30/yap/v2/pkg/pkgbuild"
)

// InstallOrExtractor is implemented by package builders that extract built
// artifacts to the root filesystem (/) so that dependent packages can
// resolve headers and libraries.
type InstallOrExtractor interface {
	InstallOrExtract(artifactsPath, buildDir, targetArch string) error
}

// Packer is the common interface implemented by all package managers.
type Packer interface {
	// BuildPackage starts the package building process and writes the final artifact
	// to the specified output path. It returns the path to the built artifact and an error
	// if any issues occur during the build.
	BuildPackage(output string, targetArch string) (string, error)
	// Prepare appends the dependencies required to build all the projects. It
	// returns any error if encountered.
	Prepare(depends []string, targetArch string) error
	// PrepareEnvironment reads a flag to install golang tools on request, on the
	// build machine. It returns any error if encountered.
	PrepareEnvironment(flag bool, targetArch string) error
	// PrepareFakeroot sets up the environment for building the final artifact in a fakeroot context.
	// It takes an output path where the artifact will be written and returns an error if any issues
	// occur.
	PrepareFakeroot(output string, targetArch string) error
	// Update performs a package manager update operation. It returns any error if
	// encountered.
	Update() error
}

// CrossDepsExtractor is implemented by builders that support downloading and
// extracting cross-build runtime dependencies without registering them in the
// package database. This avoids circular dependency conflicts between arch-all
// meta-packages and their arch-specific transitive dependencies.
type CrossDepsExtractor interface {
	DownloadAndExtractCrossDeps(deps []string, targetArch string) error
}

// GetPackageManager returns a Packer interface based on the given package build and distribution.
//
// pkgBuild: A pointer to a pkgbuild.PKGBUILD struct.
// distro: A string representing the distribution.
// compressionDeb: Compression algorithm for DEB packages (empty string uses default).
// compressionRpm: Compression algorithm for RPM packages (empty string uses default).
// Returns a Packer interface and an error if any issues occur.
func GetPackageManager(
	pkgBuild *pkgbuild.PKGBUILD,
	distro string,
	compressionDeb string,
	compressionRpm string,
) (Packer, error) {
	pkgManager := constants.DistroToPackageManager[distro]

	// Get configuration for the package manager
	config := core.GetConfig(pkgManager)
	if config == nil {
		return nil, errors.New(errors.ErrTypeConfiguration,
			i18n.T("errors.packer.unsupported_package_manager")).
			WithOperation("GetPackageManager").
			WithContext("distro", distro)
	}

	switch pkgManager {
	case "apk":
		return apk.NewBuilder(pkgBuild), nil
	case "apt":
		return deb.NewBuilder(pkgBuild, compressionDeb), nil
	case "pacman":
		return pacman.NewBuilder(pkgBuild), nil
	case "yum", "zypper":
		return rpm.NewBuilder(pkgBuild, compressionRpm), nil
	default:
		return nil, errors.New(errors.ErrTypeConfiguration,
			i18n.T("errors.packer.unsupported_linux_distro")).
			WithOperation("GetPackageManager").
			WithContext("distro", distro)
	}
}
