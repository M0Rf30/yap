package constants

import (
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
		"arch":          "pacman",
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

	ReleasesSet          = set.NewSet()
	Distros              = []string{}
	DistrosSet           = set.NewSet()
	DistroPackageManager = map[string]string{}
	PackagersSet         = set.NewSet()
	CleanPrevious        = false
)
