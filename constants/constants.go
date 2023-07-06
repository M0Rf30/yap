package constants

import (
	"fmt"
	"os"
	"strings"

	"github.com/M0Rf30/yap/set"
)

const (
	DockerOrg    = "m0rf30/yap-"
	ColorYellow  = "\033[33m"
	ColorBlue    = "\033[34m"
	ColorWhite   = "\033[37m"
	GoArchiveURL = "https://dl.google.com/go/go1.20.linux-amd64.tar.gz"
)

var (
	Releases = [...]string{
		"alpine",
		"arch",
		"amazon-1",
		"amazon-2",
		"fedora-38",
		"debian-jessie",
		"debian-stretch",
		"debian-buster",
		"rocky-8",
		"rocky-9",
		"ubuntu-bionic",
		"ubuntu-focal",
		"ubuntu-jammy",
	}

	ReleasesMatch = map[string]string{
		"alpine":        "",
		"arch":          "",
		"amazo-1":       ".amzn1.",
		"amazon-2":      ".amzn2.",
		"fedora-38":     ".fc38.",
		"debian-jessie": "jessie",
		"A-stretch":     "stretch",
		"debian-buster": "buster",
		"rocky-8":       ".el8.",
		"rocky-9":       ".el9.",
		"ubuntu-bionic": "bionic",
		"ubuntu-focal":  "focal",
		"ubuntu-jammy":  "jammy",
	}

	DistroToPackageManager = map[string]string{
		"alpine": "alpine",
		"arch":   "pacman",
		"amazon": "redhat",
		"fedora": "redhat",
		"centos": "redhat",
		"debian": "debian",
		"oracle": "redhat",
		"rocky":  "redhat",
		"ubuntu": "debian",
	}
	PackageManagers = [...]string{
		"apk",
		"apt",
		"pacman",
		"yum",
	}

	ReleasesSet          = set.NewSet()
	Distros              = []string{}
	DistrosSet           = set.NewSet()
	DistroPackageManager = map[string]string{}
	PackagersSet         = set.NewSet()
	CleanPrevious        = false
)

func init() {
	var packageManager string

	for _, release := range Releases {
		ReleasesSet.Add(release)
		distro := strings.Split(release, "-")[0]
		Distros = append(Distros, distro)
		DistrosSet.Add(distro)
	}

	for _, distro := range Distros {
		switch DistroToPackageManager[distro] {
		case "alpine":
			packageManager = "apk"
		case "debian":
			packageManager = "apt"
		case "pacman":
			packageManager = "pacman"
		case "redhat":
			packageManager = "yum"
		default:
			fmt.Printf("%s❌ :: %sfailed to find supported package manager for %s\n",
				string(ColorBlue),
				string(ColorYellow), distro)

			os.Exit(1)
		}

		DistroPackageManager[distro] = packageManager
	}

	for _, packageManager := range PackageManagers {
		PackagersSet.Add(packageManager)
	}
}
