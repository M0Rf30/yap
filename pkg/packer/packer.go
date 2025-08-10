// Package packer provides unified package building interface for multiple formats.
package packer

import (
	"github.com/M0Rf30/yap/v2/pkg/abuild"
	"github.com/M0Rf30/yap/v2/pkg/constants"
	"github.com/M0Rf30/yap/v2/pkg/dpkg"
	"github.com/M0Rf30/yap/v2/pkg/makepkg"
	"github.com/M0Rf30/yap/v2/pkg/osutils"
	"github.com/M0Rf30/yap/v2/pkg/pkgbuild"
	"github.com/M0Rf30/yap/v2/pkg/rpm"
)

// Packer is the common interface implemented by all package managers.
type Packer interface {
	// BuildPackage starts the package building process and writes the final artifact
	// to the specified output path. It returns an error if any issues occur during the build.
	BuildPackage(output string) error
	// Install reads the path where the final artifact will be written. It returns
	// any error if encountered.
	Install(output string) error
	// Prepare appends the dependencies required to build all the projects. It
	// returns any error if encountered.
	Prepare(depends []string) error
	// PrepareEnvironment reads a flag to install golang tools on request, on the
	// build machine. It returns any error if encountered.
	PrepareEnvironment(flag bool) error
	// PrepareFakeroot sets up the environment for building the final artifact in a fakeroot context.
	// It takes an output path where the artifact will be written and returns an error if any issues
	// occur.
	PrepareFakeroot(output string) error
	// Update performs a package manager update operation. It returns any error if
	// encountered.
	Update() error
}

// PackageManagerConfigs holds all package manager configurations.
var PackageManagerConfigs = map[string]*PackageManagerConfig{
	"apk": {
		Name:        "apk",
		InstallCmd:  "apk",
		InstallArgs: []string{"add", "--allow-untrusted"},
		UpdateArgs:  []string{"update"},
		ArchMap: map[string]string{
			"x86_64":  "x86_64",
			"i686":    "x86",
			"aarch64": "aarch64",
			"arm":     "armhf",
			"armv6h":  "armhf",
			"armv7h":  "armv7",
		},
		BuildEnvDeps: []string{"bash", "build-base", "fakeroot"},
	},
	"apt": {
		Name:        "apt",
		InstallCmd:  "apt-get",
		InstallArgs: []string{"--allow-downgrades", "--assume-yes", "install"},
		UpdateArgs:  []string{"update"},
		ArchMap: map[string]string{
			"x86_64":  "amd64",
			"i686":    "i386",
			"aarch64": "arm64",
			"arm":     "armel",
			"armv6h":  "armel",
			"armv7h":  "armhf",
		},
		BuildEnvDeps: []string{"build-essential", "fakeroot"},
	},
	"pacman": {
		Name:        "pacman",
		InstallCmd:  "pacman",
		InstallArgs: []string{"-U", "--noconfirm"},
		UpdateArgs:  []string{"-Sy"},
		ArchMap: map[string]string{
			"x86_64":  "x86_64",
			"i686":    "i686",
			"aarch64": "aarch64",
			"arm":     "arm",
			"armv6h":  "armv6h",
			"armv7h":  "armv7h",
		},
		BuildEnvDeps: []string{"base-devel", "fakeroot"},
	},
	"yum": {
		Name:        "dnf",
		InstallCmd:  "dnf",
		InstallArgs: []string{"-y", "install"},
		UpdateArgs:  []string{}, // RPM doesn't need explicit update
		ArchMap: map[string]string{
			"x86_64":  "x86_64",
			"i686":    "i686",
			"aarch64": "aarch64",
			"arm":     "arm",
			"armv6h":  "armv6hl",
			"armv7h":  "armv7hl",
		},
		GroupMap: map[string]string{
			"admin": "System Environment/Base",
			"base":  "System Environment/Base",
			"devel": "Development/Tools",
			"libs":  "System Environment/Libraries",
			"utils": "Applications/System",
		},
		BuildEnvDeps: []string{"rpm-build", "fakeroot"},
	},
	"zypper": {
		Name:        "dnf",
		InstallCmd:  "dnf",
		InstallArgs: []string{"-y", "install"},
		UpdateArgs:  []string{},
		ArchMap: map[string]string{
			"x86_64":  "x86_64",
			"i686":    "i686",
			"aarch64": "aarch64",
		},
		GroupMap: map[string]string{
			"admin": "System Environment/Base",
			"devel": "Development/Tools",
			"libs":  "System Environment/Libraries",
		},
		BuildEnvDeps: []string{"rpm-build", "fakeroot"},
	},
}

// GetPackageManager returns a Packer interface based on the given package build and distribution.
//
// pkgBuild: A pointer to a pkgbuild.PKGBUILD struct.
// distro: A string representing the distribution.
// Returns a Packer interface.
func GetPackageManager(pkgBuild *pkgbuild.PKGBUILD, distro string) Packer {
	pkgManager := constants.DistroToPackageManager[distro]

	// Get configuration for the package manager
	config := PackageManagerConfigs[pkgManager]
	if config == nil {
		osutils.Logger.Fatal("unsupported package manager", osutils.Logger.Args("manager", pkgManager))
		return nil
	}

	switch pkgManager {
	case "apk":
		return &abuild.Apk{
			PKGBUILD: pkgBuild,
		}
	case "apt":
		return &dpkg.Deb{
			PKGBUILD: pkgBuild,
		}
	case "pacman":
		return &makepkg.Pkg{
			PKGBUILD: pkgBuild,
		}
	case "yum":
		return &rpm.RPM{
			PKGBUILD: pkgBuild,
		}
	case "zypper":
		return &rpm.RPM{
			PKGBUILD: pkgBuild,
		}
	default:
		osutils.Logger.Fatal("unsupported linux distro",
			osutils.Logger.Args("distro", distro))
	}

	return nil
}
