package constants

import (
	"log"
	"strings"

	"github.com/M0Rf30/yap/pkg/set"
)

const (
	ColorYellow  = "\033[33m"
	ColorBlue    = "\033[34m"
	ColorWhite   = "\033[37m"
	DockerOrg    = "m0rf30/yap-"
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

// init initializes the package.
//
// It iterates over the Releases slice and adds each release to the ReleasesSet set.
// It also extracts the distribution name from each release and adds it to the Distros slice.
// The function then iterates over the Distros slice and assigns the corresponding package manager
// to each distribution in the DistroPackageManager map.
// If a distribution does not have a supported package manager, the function prints an error message
// and exits the program.
// Finally, it adds each package manager to the PackagersSet set.
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
			log.Fatalf("%s❌ :: %sfailed to find supported package manager for %s\n",
				string(ColorBlue),
				string(ColorYellow), distro)
		}

		DistroPackageManager[distro] = packageManager
	}

	for _, packageManager := range PackageManagers {
		PackagersSet.Add(packageManager)
	}
}
