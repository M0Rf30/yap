// Package constants defines global constants and configuration values used throughout YAP.
package constants

import (
	"strings"

	"github.com/M0Rf30/yap/v2/pkg/set"
)

const (
	// ColorYellow defines the ANSI color code for yellow text.
	ColorYellow = "\033[33m"
	// ColorBlue defines the ANSI color code for blue text.
	ColorBlue = "\033[34m"
	// ColorWhite defines the ANSI color code for white text.
	ColorWhite = "\033[37m"
	// DockerOrg defines the Docker organization prefix for YAP containers.
	DockerOrg = "docker.io/m0rf30/yap-"
	// Git defines the git command name.
	Git = "git"
	// GoArchiveURL defines the URL for the Go programming language archive.
	GoArchiveURL = "https://go.dev/dl/go1.24.5.linux-amd64.tar.gz"
	// YAPVersion defines the current version of YAP.
	YAPVersion = "v2.0.0"
)

var (
	// Releases contains the supported Linux distribution IDs as defined in /etc/os-release.
	// These values are not invented, but refer to /etc/os-release ID field values.
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

	// DistroToPackageManager maps distribution names to their package managers.
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

	// Packers defines the supported package managers for different distributions.
	Packers = [...]string{
		"apk",
		"apt",
		"pacman",
		"yum",
		"zypper",
	}

	// Distros contains the list of supported distribution names.
	Distros = []string{}
	// DistrosSet contains the set of supported distributions.
	DistrosSet = set.NewSet()
	// DistroPackageManager maps distributions to their package managers.
	DistroPackageManager = map[string]string{}
	// PackagersSet contains the set of supported package managers.
	PackagersSet = set.NewSet()
	// CleanPrevious controls whether to clean previous builds.
	CleanPrevious = false
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
//
//nolint:gochecknoinits // Required for initialization of package constants and data structures
func init() {
	for _, release := range &Releases {
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
