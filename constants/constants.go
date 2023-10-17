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
	GoArchiveURL = "https://go.dev/dl/go1.21.3.linux-amd64.tar.gz"
)

var (
	Releases = [...]string{
		"alpine",
		"amazon",
		"arch",
		"centos",
		"debian",
		"fedora",
		"rocky",
		"ubuntu",
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
		distro := strings.Split(release, "_")[0]
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
