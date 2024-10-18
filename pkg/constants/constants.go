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
	Releases = [...]string{
		"alma",
		"alpine",
		"amazon",
		"arch",
		"centos",
		"debian",
		"fedora",
		"rhel",
		"rocky",
		"ubuntu",
	}

	DistroToPackageManager = map[string]string{
		"alma":   "rpm",
		"alpine": "apk",
		"amazon": "rpm",
		"arch":   "pacman",
		"centos": "rpm",
		"debian": "dpkg",
		"fedora": "rpm",
		"oracle": "rpm",
		"rhel":   "rpm",
		"rocky":  "rpm",
		"ubuntu": "dpkg",
	}

	ReleasesSet          = set.NewSet()
	Distros              = []string{}
	DistrosSet           = set.NewSet()
	DistroPackageManager = map[string]string{}
	PackagersSet         = set.NewSet()
	CleanPrevious        = false
)
