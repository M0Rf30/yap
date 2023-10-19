package packer

import (
	"fmt"
	"os"

	"github.com/M0Rf30/yap/apk"
	"github.com/M0Rf30/yap/constants"
	"github.com/M0Rf30/yap/debian"
	"github.com/M0Rf30/yap/pacman"
	"github.com/M0Rf30/yap/pkgbuild"
	"github.com/M0Rf30/yap/redhat"
)

type Packer interface {
	Prepare([]string) error
	Build(string) error
	Install(string) error
	PrepareEnvironment(bool) error
	Update() error
}

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
