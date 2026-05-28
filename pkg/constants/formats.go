// Package constants provides centralized constants and mappings for all package formats.
package constants

import (
	"slices"
	"strings"
)

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

// Package compression algorithm constants accepted by the DEB and RPM builders.
const (
	CompressionZstd = "zstd"
	CompressionGzip = "gzip"
	CompressionXz   = "xz"
)

// SupportedCompressions is the canonical set of package compression
// algorithms accepted by the DEB and RPM builders. It is the single source
// of truth shared by the CLI (cmd/yap/command) and the MCP server (pkg/mcp).
var SupportedCompressions = []string{CompressionZstd, CompressionGzip, CompressionXz}

// IsSupportedCompression reports whether algo is one of the supported package
// compression algorithms. The empty string is treated as supported (the
// builder default is applied downstream).
func IsSupportedCompression(algo string) bool {
	if algo == "" {
		return true
	}

	return slices.Contains(SupportedCompressions, algo)
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

// SudoAllowedCommands is a set of package manager commands allowed to run with sudo.
// This is a security allowlist to prevent arbitrary command execution.
var SudoAllowedCommands = map[string]bool{
	PMPacman:  true,
	PMDnf:     true,
	PMYum:     true,
	"apt-get": true,
	PMApt:     true,
	FormatAPK: true,
	"dpkg":    true,
	FormatRPM: true,
	"makepkg": true,
	PMZypper:  true,
}

// GetInstallArgs returns the package manager install arguments.
func GetInstallArgs(format string) []string {
	switch format {
	case FormatAPK:
		return []string{"add", "--allow-untrusted"}
	case FormatDEB:
		// --allow-unauthenticated lets apt install packages from repos whose
		// Release file lacks a valid signature against the trust set (e.g.
		// --repo extras added at runtime). Without it, apt aborts with
		// "E: There were unauthenticated packages and -y was used without
		// --allow-unauthenticated" the moment a single dep comes from an
		// unsigned source.
		return []string{
			"--allow-downgrades",
			"--allow-unauthenticated",
			"--assume-yes",
			"--no-install-recommends",
			installArg,
		}
	case FormatRPM:
		return []string{"-y", installArg}
	case FormatPacman:
		return []string{"-S", "--noconfirm", "--needed"}
	default:
		return []string{}
	}
}
