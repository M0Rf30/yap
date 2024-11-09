package constants

import (
	"strings"

	"github.com/M0Rf30/yap/pkg/set"
)

const (
	ColorYellow  = "\033[33m"
	ColorBlue    = "\033[34m"
	ColorWhite   = "\033[37m"
	DockerOrg    = "docker.io/m0rf30/yap-"
	Git          = "git"
	GoArchiveURL = "https://go.dev/dl/go1.23.1.linux-amd64.tar.gz"
)

var (
	// These values are not invented,
	// but refer to /etc/os-release ID field values.
	Releases = [...]string{
		"almalinux",
		"alpine",
		"amzn",
		"arch",
		"centos",
		"debian",
		"fedora",
		"linuxmint",
		"opensuse-leap",
		"ol",
		"pop",
		"rhel",
		"rocky",
		"ubuntu",
	}

	DistroToPackageManager = map[string]string{
		"almalinux":     "rpm",
		"alpine":        "apk",
		"amzn":          "rpm",
		"arch":          "makepkg",
		"centos":        "rpm",
		"debian":        "dpkg",
		"fedora":        "rpm",
		"linuxmint":     "dpkg",
		"ol":            "rpm",
		"opensuse-leap": "rpm",
		"pop":           "dpkg",
		"rhel":          "rpm",
		"rocky":         "rpm",
		"ubuntu":        "dpkg",
	}

	Packers = [...]string{
		"apk",
		"dpkg",
		"makepkg",
		"rpm",
	}

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
	for _, release := range Releases {
		distro := strings.Split(release, "_")[0]
		Distros = append(Distros, distro)
		DistrosSet.Add(distro)
	}

	for _, distro := range Distros {
		DistroPackageManager[distro] = DistroToPackageManager[distro]
	}

	for _, packageManager := range Packers {
		PackagersSet.Add(packageManager)
	}
}
