package packer

import (
	"fmt"
	"os"

	"github.com/M0Rf30/yap/pkg/apk"
	"github.com/M0Rf30/yap/pkg/constants"
	"github.com/M0Rf30/yap/pkg/debian"
	"github.com/M0Rf30/yap/pkg/pacman"
	"github.com/M0Rf30/yap/pkg/pkgbuild"
	"github.com/M0Rf30/yap/pkg/redhat"
)

// Packer is the common interface implemented by all package managers.
type Packer interface {
	// Prepare appends the dependencies required to build all the projects. It
	// returns any error if encountered.
	Prepare(depends []string) error
	// Build reads the path where the final artifact will be written. It returns any
	// error if encountered.
	Build(output string) error
	// Install reads the path where the final artifact will be written. It returns
	// any error if encountered.
	Install(output string) error
	// PrepareEnvironment reads a flag to install golang tools on request, on the
	// build machine. It returns any error if encountered.
	PrepareEnvironment(flag bool) error
	// Update performs a package manager update operation. It returns any error if
	// encountered.
	Update() error
}

// GetPackageManager returns a Packer interface based on the given package build and distribution.
//
// pkgBuild: A pointer to a pkgbuild.PKGBUILD struct.
// distro: A string representing the distribution.
// Returns a Packer interface.
func GetPackageManager(pkgBuild *pkgbuild.PKGBUILD, distro string) Packer {
	var packageManager Packer

	distroFamily := constants.DistroToPackageManager[distro]
	switch distroFamily {
	case "alpine":
		packageManager = &apk.Apk{
			PKGBUILD: pkgBuild,
		}
	case "pacman":
		packageManager = &pacman.Pacman{
			PKGBUILD: pkgBuild,
		}
	case "debian":
		packageManager = &debian.Debian{
			PKGBUILD: pkgBuild,
		}
	case "redhat":
		packageManager = &redhat.Redhat{
			PKGBUILD: pkgBuild,
		}
	default:
		fmt.Printf("%s%s ❌ :: unknown unsupported system.%s\n",
			string(constants.ColorBlue),
			string(constants.ColorYellow),
			string(constants.ColorWhite))
		os.Exit(1)
	}

	return packageManager
}
