package packer

import (
	"fmt"
	"os"

	"github.com/packagefoundation/yap/apk"
	"github.com/packagefoundation/yap/constants"
	"github.com/packagefoundation/yap/debian"
	"github.com/packagefoundation/yap/pack"
	"github.com/packagefoundation/yap/pacman"
	"github.com/packagefoundation/yap/redhat"
)

type Packer interface {
	Prep() error
	Build() ([]string, error)
	Install() error
	Update() error
}

func GetPacker(pac *pack.Pack, distro, release string) Packer {
	var pcker Packer

	switch constants.DistroPack[distro] {
	case "alpine":
		pcker = &apk.Apk{
			Pack: pac,
		}
	case "pacman":
		pcker = &pacman.Pacman{
			Pack: pac,
		}
	case "debian":
		pcker = &debian.Debian{
			Pack: pac,
		}
	case "redhat":
		pcker = &redhat.Redhat{
			Pack: pac,
		}
	default:
		system := distro
		if release != "" {
			system += "-" + release
		}

		fmt.Printf("packer: Unknown system %s\n", system)
		os.Exit(1)
	}

	return pcker
}
