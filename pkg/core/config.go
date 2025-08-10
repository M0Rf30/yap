// Package core provides package manager configurations and metadata.
package core

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
	"apk": {
		Name:        "apk",
		InstallCmd:  "apk",
		InstallArgs: []string{"add", "--allow-untrusted"},
		UpdateArgs:  []string{"update"},
		ArchMap: map[string]string{
			"x86_64":  "x86_64",
			"i686":    "x86",
			"aarch64": "aarch64",
			"arm":     "armhf",
			"armv6h":  "armhf",
			"armv7h":  "armv7",
		},
		BuildEnvDeps: []string{"bash", "build-base", "fakeroot"},
	},
	"apt": {
		Name:        "apt",
		InstallCmd:  "apt-get",
		InstallArgs: []string{"--allow-downgrades", "--assume-yes", "install"},
		UpdateArgs:  []string{"update"},
		ArchMap: map[string]string{
			"x86_64":  "amd64",
			"i686":    "i386",
			"aarch64": "arm64",
			"arm":     "armel",
			"armv6h":  "armel",
			"armv7h":  "armhf",
		},
		BuildEnvDeps: []string{"build-essential", "fakeroot"},
	},
	"pacman": {
		Name:        "pacman",
		InstallCmd:  "pacman",
		InstallArgs: []string{"-U", "--noconfirm"},
		UpdateArgs:  []string{"-Sy"},
		ArchMap: map[string]string{
			"x86_64":  "x86_64",
			"i686":    "i686",
			"aarch64": "aarch64",
			"arm":     "arm",
			"armv6h":  "armv6h",
			"armv7h":  "armv7h",
		},
		BuildEnvDeps: []string{"base-devel", "fakeroot"},
	},
	"yum": {
		Name:        "dnf",
		InstallCmd:  "dnf",
		InstallArgs: []string{"-y", "install"},
		UpdateArgs:  []string{}, // RPM doesn't need explicit update
		ArchMap: map[string]string{
			"x86_64":  "x86_64",
			"i686":    "i686",
			"aarch64": "aarch64",
			"arm":     "arm",
			"armv6h":  "armv6hl",
			"armv7h":  "armv7hl",
		},
		GroupMap: map[string]string{
			"admin": "System Environment/Base",
			"base":  "System Environment/Base",
			"devel": "Development/Tools",
			"libs":  "System Environment/Libraries",
			"utils": "Applications/System",
		},
		BuildEnvDeps: []string{"rpm-build", "fakeroot"},
	},
	"zypper": {
		Name:        "dnf",
		InstallCmd:  "dnf",
		InstallArgs: []string{"-y", "install"},
		UpdateArgs:  []string{},
		ArchMap: map[string]string{
			"x86_64":  "x86_64",
			"i686":    "i686",
			"aarch64": "aarch64",
		},
		GroupMap: map[string]string{
			"admin": "System Environment/Base",
			"devel": "Development/Tools",
			"libs":  "System Environment/Libraries",
		},
		BuildEnvDeps: []string{"rpm-build", "fakeroot"},
	},
}

// GetConfig returns the configuration for a given package manager.
func GetConfig(packageManager string) *Config {
	return PackageManagerConfigs[packageManager]
}
