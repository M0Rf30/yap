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
		"alma":   "redhat",
		"alpine": "alpine",
		"amazon": "redhat",
		"arch":   "pacman",
		"centos": "redhat",
		"debian": "debian",
		"fedora": "redhat",
		"oracle": "redhat",
		"rhel":   "redhat",
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
