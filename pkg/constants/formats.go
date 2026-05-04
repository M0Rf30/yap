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

const (
	pkgCcache  = "ccache"
	installArg = "install"
)

// Package file extension constants.
const (
	// ExtAPK is the Alpine package file extension.
	ExtAPK = ".apk"
	// ExtDEB is the Debian package file extension.
	ExtDEB = ".deb"
	// ExtRPM is the RPM package file extension.
	ExtRPM = ".rpm"
	// ExtPacmanZst is the Arch Linux zstd-compressed package file extension.
	ExtPacmanZst = ".pkg.tar.zst"
)

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
			pkgCcache,
		},
		DEB: []string{
			"build-essential",
			pkgCcache,
			"fakeroot",
		},
		RPM: []string{
			"autoconf",
			"automake",
			pkgCcache,
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
			pkgCcache,
		},
	}
}

// distroFormatMap maps distribution names (lowercase) to their package format.
// Legacy aliases (alma, opensuse, suse) are kept for backward compatibility.
var distroFormatMap = map[string]string{
	DistroAlmalinux:          FormatRPM,
	DistroAlpine:             FormatAPK,
	DistroAmzn:               FormatRPM,
	DistroArch:               FormatPacman,
	DistroCentos:             FormatRPM,
	DistroDebian:             FormatDEB,
	DistroFedora:             FormatRPM,
	DistroLinuxmint:          FormatDEB,
	DistroOl:                 FormatRPM,
	DistroOpenSUSELeap:       FormatRPM, // zypper-based; format is still RPM
	DistroOpenSUSETumbleweed: FormatRPM, // zypper-based; format is still RPM
	DistroPop:                FormatDEB,
	DistroRhel:               FormatRPM,
	DistroRocky:              FormatRPM,
	DistroUbuntu:             FormatDEB,
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
		return []string{"--allow-downgrades", "--assume-yes", installArg}
	case FormatRPM:
		return []string{"-y", installArg}
	case FormatPacman:
		return []string{"-S", "--noconfirm", "--needed"}
	default:
		return []string{}
	}
}
