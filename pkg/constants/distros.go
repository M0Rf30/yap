// Package constants provides centralized constants and mappings for all package formats.
package constants

// Canonical distribution name constants.
//
// These are the lowercase ID values from /etc/os-release for the distributions
// supported by YAP. Use these constants throughout the codebase rather than
// repeating string literals.
const (
	// DistroAlmalinux is the os-release ID for AlmaLinux.
	DistroAlmalinux = "almalinux"
	// DistroAlpine is the os-release ID for Alpine Linux.
	DistroAlpine = "alpine"
	// DistroAmzn is the os-release ID for Amazon Linux.
	DistroAmzn = "amzn"
	// DistroArch is the os-release ID for Arch Linux.
	DistroArch = "arch"
	// DistroCentos is the os-release ID for CentOS.
	DistroCentos = "centos"
	// DistroDebian is the os-release ID for Debian.
	DistroDebian = "debian"
	// DistroFedora is the os-release ID for Fedora.
	DistroFedora = "fedora"
	// DistroLinuxmint is the os-release ID for Linux Mint.
	DistroLinuxmint = "linuxmint"
	// DistroOpenSUSELeap is the os-release ID for openSUSE Leap.
	DistroOpenSUSELeap = "opensuse-leap"
	// DistroOpenSUSETumbleweed is the os-release ID for openSUSE Tumbleweed.
	DistroOpenSUSETumbleweed = "opensuse-tumbleweed"
	// DistroOl is the os-release ID for Oracle Linux.
	DistroOl = "ol"
	// DistroPop is the os-release ID for Pop!_OS.
	DistroPop = "pop"
	// DistroRhel is the os-release ID for Red Hat Enterprise Linux.
	DistroRhel = "rhel"
	// DistroRocky is the os-release ID for Rocky Linux.
	DistroRocky = "rocky"
	// DistroUbuntu is the os-release ID for Ubuntu.
	DistroUbuntu = "ubuntu"
)

// Package manager command name constants.
//
// These are the canonical names of the package manager binaries used by the
// supported distributions.
const (
	// PMApk is the Alpine package manager.
	PMApk = "apk"
	// PMApt is the Debian/Ubuntu package manager.
	PMApt = "apt"
	// PMPacman is the Arch Linux package manager.
	PMPacman = "pacman"
	// PMYum is the legacy RPM-based package manager.
	PMYum = "yum"
	// PMZypper is the openSUSE package manager.
	PMZypper = "zypper"
)
