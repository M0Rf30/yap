// Package constants provides centralized constants and mappings for all package formats.
package constants

import "strings"

// Package format constants
const (
	FormatAPK    = "apk"
	FormatDEB    = "deb"
	FormatRPM    = "rpm"
	FormatPacman = "pacman"
)

// RPMGroups maps common group names to RPM group categories.
// This consolidates the group mapping logic from the RPM package.
var RPMGroups = map[string]string{
	"admin":        "Applications/System",
	"any":          "noarch",
	"comm":         "Applications/Communications",
	"database":     "Applications/Databases",
	"debug":        "Development/Debuggers",
	"devel":        "Development/Tools",
	"doc":          "Documentation",
	"editors":      "Applications/Editors",
	"electronics":  "Applications/Engineering",
	"embedded":     "Applications/Engineering",
	"fonts":        "Interface/Desktops",
	"games":        "Amusements/Games",
	"graphics":     "Applications/Multimedia",
	"httpd":        "Applications/Internet",
	"interpreters": "Development/Tools",
	"kernel":       "System Environment/Kernel",
	"libdevel":     "Development/Libraries",
	"libs":         "System Environment/Libraries",
	"localization": "Development/Languages",
	"mail":         "Applications/Communications",
	"math":         "Applications/Productivity",
	"misc":         "Applications/System",
	"net":          "Applications/Internet",
	"news":         "Applications/Publishing",
	"science":      "Applications/Engineering",
	"shells":       "System Environment/Shells",
	"sound":        "Applications/Multimedia",
	"text":         "Applications/Text",
	"vcs":          "Development/Tools",
	"video":        "Applications/Multimedia",
	"web":          "Applications/Internet",
	"x11":          "User Interface/X",
}

// RPMDistros maps distribution names to their RPM suffix.
var RPMDistros = map[string]string{
	"almalinux": ".el",
	"amzn":      ".amzn",
	"fedora":    ".fc",
	"ol":        ".ol",
	"rhel":      ".el",
	"rocky":     ".el",
}

// BuildEnvironmentDeps provides build environment dependencies for each package manager.
type BuildEnvironmentDeps struct {
	APK    []string
	DEB    []string
	RPM    []string
	Pacman []string
}

// GetBuildDeps returns the build environment dependencies for all package formats.
func GetBuildDeps() *BuildEnvironmentDeps {
	return &BuildEnvironmentDeps{
		APK: []string{
			"alpine-sdk",
			"ccache",
		},
		DEB: []string{
			"build-essential",
			"ccache",
			"fakeroot",
		},
		RPM: []string{
			"autoconf",
			"automake",
			"ccache",
			"diffutils",
			"expect",
			"gcc",
			"gcc-c++",
			"libtool-ltdl",
			"libtool-ltdl-devel",
			"make",
			"openssl",
			"patch",
			"pkgconf",
			"which",
		},
		Pacman: []string{
			"base-devel",
			"ccache",
		},
	}
}

// distroFormatMap maps distribution names (lowercase) to their package format.
// Legacy aliases (alma, opensuse, suse) are kept for backward compatibility.
var distroFormatMap = map[string]string{
	"almalinux":           FormatRPM,
	"alpine":              FormatAPK,
	"amzn":                FormatRPM,
	"arch":                FormatPacman,
	"centos":              FormatRPM,
	"debian":              FormatDEB,
	"fedora":              FormatRPM,
	"linuxmint":           FormatDEB,
	"ol":                  FormatRPM,
	"opensuse-leap":       FormatRPM, // zypper-based; format is still RPM
	"opensuse-tumbleweed": FormatRPM, // zypper-based; format is still RPM
	"pop":                 FormatDEB,
	"rhel":                FormatRPM,
	"rocky":               FormatRPM,
	"ubuntu":              FormatDEB,
	// Legacy aliases kept for backward compatibility
	"alma":     FormatRPM,
	"opensuse": FormatRPM,
	"suse":     FormatRPM,
}

// DistroFormat returns the package format for a given distribution name.
// Returns an empty string if the distribution is not recognized.
func DistroFormat(distro string) string {
	return distroFormatMap[strings.ToLower(distro)]
}

// GetInstallArgs returns the package manager install arguments.
func GetInstallArgs(format string) []string {
	switch format {
	case FormatAPK:
		return []string{"add", "--allow-untrusted"}
	case FormatDEB:
		return []string{"--allow-downgrades", "--assume-yes", "install"}
	case FormatRPM:
		return []string{"-y", "install"}
	case FormatPacman:
		return []string{"-S", "--noconfirm", "--needed"}
	default:
		return []string{}
	}
}
