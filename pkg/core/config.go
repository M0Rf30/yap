// Package core provides package manager configurations and metadata.
package core

import "github.com/M0Rf30/yap/v2/pkg/constants"

const (
	armKey                = "arm"
	armv6hKey             = "armv6h"
	armv7hKey             = "armv7h"
	armhfArch             = "armhf"
	armv7Arch             = "armv7"
	x86Arch               = "x86"
	amd64Arch             = "amd64"
	i386Arch              = "i386"
	arm64Arch             = "arm64"
	armelArch             = "armel"
	armhfDebArch          = "armhf"
	armv6hlArch           = "armv6hl"
	armv7hlArch           = "armv7hl"
	fakeroot              = "fakeroot"
	installCmd            = "install"
	dnfCmd                = "dnf"
	systemEnvironmentBase = "System Environment/Base"
	systemEnvironmentLibs = "System Environment/Libraries"
	developmentTools      = "Development/Tools"
	applicationsSystem    = "Applications/System"
	devel                 = "devel"
	libs                  = "libs"
	groupAdmin            = "admin"
)

// Config holds common configuration for all package managers.
type Config struct {
	Name         string
	InstallCmd   string
	InstallArgs  []string
	UpdateArgs   []string
	ArchMap      map[string]string
	DistroMap    map[string]string
	GroupMap     map[string]string
	BuildEnvDeps []string
}

// PackageManagerConfigs holds all package manager configurations.
var PackageManagerConfigs = map[string]*Config{
	constants.FormatAPK: {
		Name:        constants.FormatAPK,
		InstallCmd:  constants.FormatAPK,
		InstallArgs: []string{"add", "--allow-untrusted"},
		UpdateArgs:  []string{"update"},
		ArchMap: map[string]string{
			constants.ArchX86_64:  constants.ArchX86_64,
			constants.ArchI686:    x86Arch,
			constants.ArchAarch64: constants.ArchAarch64,
			armKey:                armhfArch,
			armv6hKey:             armhfArch,
			armv7hKey:             armv7Arch,
		},
		BuildEnvDeps: []string{"bash", "build-base", fakeroot},
	},
	constants.PMApt: {
		Name:        constants.PMApt,
		InstallCmd:  "apt-get",
		InstallArgs: []string{"--allow-downgrades", "--assume-yes", installCmd},
		UpdateArgs:  []string{"update"},
		ArchMap: map[string]string{
			constants.ArchX86_64:  amd64Arch,
			constants.ArchI686:    i386Arch,
			constants.ArchAarch64: arm64Arch,
			armKey:                armelArch,
			armv6hKey:             armelArch,
			armv7hKey:             armhfDebArch,
		},
		BuildEnvDeps: []string{"build-essential", fakeroot},
	},
	constants.FormatPacman: {
		Name:        constants.FormatPacman,
		InstallCmd:  constants.FormatPacman,
		InstallArgs: []string{"-U", "--noconfirm"},
		UpdateArgs:  []string{"-Sy"},
		ArchMap: map[string]string{
			constants.ArchX86_64:  constants.ArchX86_64,
			constants.ArchI686:    constants.ArchI686,
			constants.ArchAarch64: constants.ArchAarch64,
			armKey:                armKey,
			armv6hKey:             armv6hKey,
			armv7hKey:             armv7hKey,
		},
		BuildEnvDeps: []string{"base-devel", fakeroot},
	},
	constants.PMYum: {
		Name:        dnfCmd,
		InstallCmd:  dnfCmd,
		InstallArgs: []string{"-y", installCmd},
		UpdateArgs:  []string{}, // RPM doesn't need explicit update
		ArchMap: map[string]string{
			constants.ArchX86_64:  constants.ArchX86_64,
			constants.ArchI686:    constants.ArchI686,
			constants.ArchAarch64: constants.ArchAarch64,
			armKey:                armKey,
			armv6hKey:             armv6hlArch,
			armv7hKey:             armv7hlArch,
		},
		GroupMap: map[string]string{
			groupAdmin: systemEnvironmentBase,
			"base":     systemEnvironmentBase,
			devel:      developmentTools,
			libs:       systemEnvironmentLibs,
			"utils":    applicationsSystem,
		},
		BuildEnvDeps: []string{"rpm-build", fakeroot},
	},
	constants.PMZypper: {
		Name:        dnfCmd,
		InstallCmd:  dnfCmd,
		InstallArgs: []string{"-y", installCmd},
		UpdateArgs:  []string{},
		ArchMap: map[string]string{
			constants.ArchX86_64:  constants.ArchX86_64,
			constants.ArchI686:    constants.ArchI686,
			constants.ArchAarch64: constants.ArchAarch64,
		},
		GroupMap: map[string]string{
			groupAdmin: systemEnvironmentBase,
			devel:      developmentTools,
			libs:       systemEnvironmentLibs,
		},
		BuildEnvDeps: []string{"rpm-build", fakeroot},
	},
}

// GetConfig returns the configuration for a given package manager.
func GetConfig(packageManager string) *Config {
	return PackageManagerConfigs[packageManager]
}

// SigningConfig holds signing configuration for a project.
type SigningConfig struct {
	// Enabled indicates whether signing is active for this project.
	Enabled bool `json:"enabled,omitempty"`
	// KeyPath is the path to the private key file (PEM for RSA, ASCII-armored for GPG).
	KeyPath string `json:"keyPath,omitempty"`
	// Passphrase is the passphrase for the private key (discouraged in config; prefer env vars).
	Passphrase string `json:"passphrase,omitempty"`
	// KeyName is optional, used for APK key naming (e.g., "mykey").
	KeyName string `json:"keyName,omitempty"`
}
