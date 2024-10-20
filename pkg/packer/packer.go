package packer

import (
	"github.com/M0Rf30/yap/pkg/apk"
	"github.com/M0Rf30/yap/pkg/constants"
	"github.com/M0Rf30/yap/pkg/dpkg"
	"github.com/M0Rf30/yap/pkg/pacman"
	"github.com/M0Rf30/yap/pkg/pkgbuild"
	"github.com/M0Rf30/yap/pkg/rpm"
	"github.com/M0Rf30/yap/pkg/utils"
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
	// It takes an output path where the artifact will be written and returns an error if any issues occur.
	PrepareFakeroot(output string) error
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
	pkgManager := constants.DistroToPackageManager[distro]
	switch pkgManager {
	case "apk":
		return &apk.Apk{
			PKGBUILD: pkgBuild,
		}
	case "pacman":
		return &pacman.Pacman{
			PKGBUILD: pkgBuild,
		}
	case "dpkg":
		return &dpkg.Deb{
			PKGBUILD: pkgBuild,
		}
	case "rpm":
		return &rpm.RPM{
			PKGBUILD: pkgBuild,
		}
	default:
		utils.Logger.Fatal("unsupported linux target",
			utils.Logger.Args("distro", distro))
	}

	return nil
}
