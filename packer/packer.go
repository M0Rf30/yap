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
	Prep() error
	Build() ([]string, error)
	Install() error
	Update() error
}

func GetPackageManager(pkgbuild *pkgbuild.PKGBUILD, distro, codeName string) Packer {
	var packageManager Packer

	switch constants.DistroToPackageManager[distro] {
	case "alpine":
		packageManager = &apk.Apk{
			PKGBUILD: pkgbuild,
		}
	case "pacman":
		packageManager = &pacman.Pacman{
			PKGBUILD: pkgbuild,
		}
	case "debian":
		packageManager = &debian.Debian{
			PKGBUILD: pkgbuild,
		}
	case "redhat":
		packageManager = &redhat.Redhat{
			PKGBUILD: pkgbuild,
		}
	default:
		system := distro
		if codeName != "" {
			system += "-" + codeName
		}

		fmt.Printf("packer: Unknown system %s\n", system)
		os.Exit(1)
	}

	return packageManager
}
