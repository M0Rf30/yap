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
	installArg = "install"
)

// Package names shared by the per-format build dependency lists. Declared
// as constants so the same tool is spelled identically everywhere it is
// guaranteed by the toolchain contract in GetBuildDeps.
const (
	pkgAutoconf  = "autoconf"
	pkgAutomake  = "automake"
	pkgBzip2     = "bzip2"
	pkgCcache    = "ccache"
	pkgDiffutils = "diffutils"
	pkgFindutils = "findutils"
	pkgGzip      = CompressionGzip
	pkgLibtool   = "libtool"
	pkgM4        = "m4"
	pkgPatch     = "patch"
	pkgPerl      = "perl"
	pkgTar       = "tar"
	pkgWhich     = "which"
	pkgXz        = CompressionXz
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
//
// Every format declares the SAME core toolchain contract, spelled with that
// distro's package names:
//
//	C/C++ compiler + linker, make, patch, m4,
//	autoconf, automake, libtool, pkg-config,
//	bzip2, xz, gzip, tar,
//	diffutils, findutils, which, perl, ccache
//
// Anything a PKGBUILD needs beyond that belongs in makedepends. Previously
// each format carried an ad-hoc list, so what a build could rely on depended
// on which distro's transitive closure happened to drag a tool in: DEB got
// bzip2/xz only because dpkg-dev depends on them and had no autotools at
// all, while RPM had autotools but no bzip2/xz. Identical PKGBUILDs
// succeeded on one distro and failed on the other.
//
// fakeroot is deliberately absent: pkg/shell/fakeroot.go implements it
// in-process via CLONE_NEWUSER and never execs the binary.
func GetBuildDeps() *BuildEnvironmentDeps {
	return &BuildEnvironmentDeps{
		// alpine-sdk pulls build-base (gcc/g++/make/patch/libc-dev) plus
		// abuild and git.
		APK: []string{
			"alpine-sdk",
			pkgAutoconf,
			pkgAutomake,
			pkgBzip2,
			pkgCcache,
			pkgDiffutils,
			pkgFindutils,
			pkgGzip,
			pkgLibtool,
			pkgM4,
			pkgPerl,
			"pkgconf",
			pkgTar,
			pkgWhich,
			pkgXz,
		},
		// build-essential pulls gcc/g++/make/libc6-dev/dpkg-dev. `which`
		// lives in debianutils, which is Essential: yes.
		DEB: []string{
			pkgAutoconf,
			pkgAutomake,
			"build-essential",
			pkgBzip2,
			pkgCcache,
			pkgDiffutils,
			pkgFindutils,
			pkgGzip,
			pkgLibtool,
			pkgM4,
			pkgPatch,
			pkgPerl,
			"pkg-config",
			pkgTar,
			"xz-utils",
		},
		// libtool here is the binary (libtoolize); the previous
		// libtool-ltdl/libtool-ltdl-devel entries are the libltdl runtime
		// library and its headers, which ship no executable.
		RPM: []string{
			pkgAutoconf,
			pkgAutomake,
			pkgBzip2,
			pkgCcache,
			pkgDiffutils,
			pkgFindutils,
			"gcc",
			"gcc-c++",
			pkgGzip,
			pkgLibtool,
			pkgM4,
			"make",
			pkgPatch,
			pkgPerl,
			"pkgconf-pkg-config",
			pkgTar,
			pkgWhich,
			pkgXz,
		},
		// base-devel is a meta package covering gcc/make/patch/autotools/
		// pkgconf/m4; the rest are in base but named explicitly so the
		// contract does not depend on the image's base group.
		Pacman: []string{
			"base-devel",
			pkgBzip2,
			pkgCcache,
			pkgDiffutils,
			pkgFindutils,
			pkgGzip,
			pkgPerl,
			pkgTar,
			pkgWhich,
			pkgXz,
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
