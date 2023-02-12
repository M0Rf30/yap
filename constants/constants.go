package constants

import (
	"strings"

	"github.com/M0Rf30/yap/set"
)

const (
	DockerOrg   = "m0rf30/yap-"
	ColorYellow = "\033[33m"
	ColorBlue   = "\033[34m"
	ColorWhite  = "\033[37m"
)

var (
	ArchToDebian = map[string]string{
		"any":     "all",
		"x86_64":  "amd64",
		"i686":    "386",
		"aarch64": "arm64",
		"armv7h":  "arm7",
		"armv6h":  "arm6",
		"arm":     "arm5",
	}

	ArchToRPM = map[string]string{
		"any": "noarch",
	}

	Releases = [...]string{
		"alpine",
		"arch",
		"astra",
		"amazon-1",
		"amazon-2",
		"fedora-35",
		"debian-jessie",
		"debian-stretch",
		"debian-buster",
		"oracle-8",
		"rocky-8",
		"rocky-9",
		"ubuntu-bionic",
		"ubuntu-focal",
		"ubuntu-jammy",
	}

	ReleasesMatch = map[string]string{
		"alpine":         "",
		"arch":           "",
		"astra":          "astra",
		"amazo-1":        ".amzn1.",
		"amazon-2":       ".amzn2.",
		"fedora-35":      ".fc35.",
		"debian-jessie":  "jessie",
		"debian-stretch": "stretch",
		"debian-buster":  "buster",
		"oracle-8":       ".ol8.",
		"rocky-8":        ".el8.",
		"rocky-9":        ".el9.",
		"ubuntu-bionic":  "bionic",
		"ubuntu-focal":   "focal",
		"ubuntu-jammy":   "jammy",
	}

	DistroToPackageManager = map[string]string{
		"alpine": "alpine",
		"arch":   "pacman",
		"astra":  "debian",
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
			panic("Failed to find packageManager for distro")
		}

		DistroPackageManager[distro] = packageManager
	}

	for _, packageManager := range PackageManagers {
		PackagersSet.Add(packageManager)
	}
}
