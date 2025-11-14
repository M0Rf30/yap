// Package common provides shared functionality for cross-compilation toolchains.
package common

import "fmt"

// CrossToolchain represents a cross-compilation toolchain for a specific target architecture.
type CrossToolchain struct {
	// GCCPackage is the name of the GCC cross-compiler package
	GCCPackage string
	// GPlusPlusPackage is the name of the G++ cross-compiler package
	GPlusPlusPackage string
	// BinutilsPackage is the name of the binutils package for the target architecture
	BinutilsPackage string
	// AdditionalPackages are any additional packages needed for the toolchain
	AdditionalPackages []string
}

// CrossToolchainMap maps target architectures to toolchain packages for each distribution.
var CrossToolchainMap = func() map[string]map[string]CrossToolchain {
	// Define base patterns for each distribution
	basePatterns := map[string]map[string]string{
		"debian": {
			"gcc":        "gcc-%s-linux-gnu",
			"g++":        "g++-%s-linux-gnu",
			"binutils":   "binutils-%s-linux-gnu",
			"additional": "libc6-dev-%s-cross",
		},
		"ubuntu": {
			"gcc":        "gcc-%s-linux-gnu",
			"g++":        "g++-%s-linux-gnu",
			"binutils":   "binutils-%s-linux-gnu",
			"additional": "libc6-dev-%s-cross",
		},
		"fedora": {
			"gcc":        "gcc-%s-linux-gnu",
			"g++":        "gcc-c++-%s-linux-gnu",
			"binutils":   "binutils-%s-linux-gnu",
			"additional": "",
		},
		"alpine": {
			"gcc":        "gcc-%s",
			"g++":        "g++-%s",
			"binutils":   "binutils-%s",
			"additional": "musl-dev",
		},
		"arch": {
			"gcc":        "%s-gcc",
			"g++":        "%s-g++",
			"binutils":   "%s-binutils",
			"additional": "",
		},
	}

	// Define architecture-specific mappings for special cases
	archSpecific := map[string]map[string]map[string]string{
		"i686": {
			"arch": {
				"gcc":      "gcc-multilib",
				"g++":      "gcc-c++-multilib",
				"binutils": "binutils-multilib",
			},
		},
		"x86_64": {
			"fedora": {
				"gcc":      "gcc-x86_64-linux-gnu",
				"g++":      "gcc-c++-x86_64-linux-gnu",
				"binutils": "binutils-x86_64-linux-gnu",
			},
		},
		"aarch64": {
			"arch": {
				"gcc":      "aarch64-linux-gnu-gcc",
				"g++":      "aarch64-linux-gnu-g++",
				"binutils": "aarch64-linux-gnu-binutils",
			},
		},
		"armv6": {
			"alpine": {
				"gcc":      "gcc-armhf",
				"g++":      "g++-armhf",
				"binutils": "binutils-armhf",
			},
		},
		"armv7": {
			"alpine": {
				"gcc":      "gcc-armv7",
				"g++":      "g++-armv7",
				"binutils": "binutils-armv7",
			},
		},
		"s390x": {
			"fedora": {
				"gcc":      "gcc-s390x-redhat-linux",
				"g++":      "gcc-c++-s390x-redhat-linux",
				"binutils": "binutils-s390x-redhat-linux",
			},
		},
	}

	// Define architecture-specific additional packages
	archAdditional := map[string]map[string][]string{
		"aarch64": {
			"alpine":  {"musl-dev"},
			"default": {"libc6-dev-arm64-cross"},
		},
		"armv7": {
			"alpine":  {"musl-dev"},
			"default": {"libc6-dev-armhf-cross"},
		},
		"armv6": {
			"alpine":  {"musl-dev"},
			"default": {"libc6-dev-armhf-cross"},
		},
		"i686": {
			"alpine":  {"musl-dev"},
			"default": {"libc6-dev-i386-cross"},
		},
		"x86_64": {
			"alpine":  {"musl-dev"},
			"default": {"libc6-dev-amd64-cross"},
		},
		"ppc64le": {
			"alpine":  {"musl-dev"},
			"default": {"libc6-dev-ppc64el-cross"},
		},
		"s390x": {
			"alpine":  {"musl-dev"},
			"default": {"libc6-dev-s390x-cross"},
		},
		"riscv64": {
			"alpine":  {"musl-dev"},
			"default": {"libc6-dev-riscv64-cross"},
		},
	}

	// Define architecture-specific patterns that need special handling
	archPatterns := map[string]string{
		"aarch64": "aarch64-linux-gnu",
		"armv7":   "arm-linux-gnueabihf",
		"armv6":   "arm-linux-gnueabihf",
		"i686":    "i686-linux-gnu",
		"x86_64":  "x86-64-linux-gnu",
		"ppc64le": "powerpc64le-linux-gnu",
		"s390x":   "s390x-linux-gnu",
		"riscv64": "riscv64-linux-gnu",
	}

	result := make(map[string]map[string]CrossToolchain)

	// Architectures to process
	architectures := []string{
		"aarch64", "armv7", "armv6", "i686",
		"x86_64", "ppc64le", "s390x", "riscv64",
	}
	distributions := []string{"debian", "ubuntu", "fedora", "alpine", "arch"}

	for _, arch := range architectures {
		result[arch] = make(map[string]CrossToolchain)

		for _, distro := range distributions {
			// Get base patterns for this distribution
			patterns, exists := basePatterns[distro]
			if !exists {
				continue
			}

			// For Arch Linux, use the specialized archPatterns
			// For other distributions, use the simple architecture name
			archPattern := arch
			if distro == "arch" {
				if pattern, exists := archPatterns[arch]; exists {
					archPattern = pattern
				}
			}

			// Start with base values using the appropriate pattern
			gcc := fmt.Sprintf(patterns["gcc"], archPattern)
			gpp := fmt.Sprintf(patterns["g++"], archPattern)
			binutils := fmt.Sprintf(patterns["binutils"], archPattern)

			// Apply architecture-specific overrides if they exist
			if archOverrides, exists := archSpecific[arch]; exists {
				if distroOverrides, exists := archOverrides[distro]; exists {
					if override, exists := distroOverrides["gcc"]; exists {
						gcc = override
					}
					if override, exists := distroOverrides["g++"]; exists {
						gpp = override
					}
					if override, exists := distroOverrides["binutils"]; exists {
						binutils = override
					}
				}
			}

			// Determine additional packages
			var additional []string
			if archAdd, exists := archAdditional[arch]; exists {
				if add, exists := archAdd[distro]; exists {
					additional = add
				} else if add, exists := archAdd["default"]; exists {
					additional = add
				}
			}

			result[arch][distro] = CrossToolchain{
				GCCPackage:         gcc,
				GPlusPlusPackage:   gpp,
				BinutilsPackage:    binutils,
				AdditionalPackages: additional,
			}
		}
	}

	return result
}()
