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
	GoArchiveURL = "https://go.dev/dl/go1.25.6.linux-amd64.tar.gz"
	YAPVersion   = "v1.46"
)

var (
	// Releases contains the supported distribution IDs from /etc/os-release.
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
		"almalinux":     "yum",
		"alpine":        "apk",
		"amzn":          "yum",
		"arch":          "pacman",
		"centos":        "yum",
		"debian":        "apt",
		"fedora":        "yum",
		"linuxmint":     "apt",
		"ol":            "yum",
		"opensuse-leap": "zypper",
		"pop":           "apt",
		"rhel":          "yum",
		"rocky":         "yum",
		"ubuntu":        "apt",
	}

	Packers = [...]string{
		"apk",
		"apt",
		"pacman",
		"yum",
		"zypper",
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
