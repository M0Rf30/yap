// Package common provides shared functionality for cross-compilation toolchains.
package common

import (
	"fmt"
	"os/exec"
	"strings"

	"github.com/M0Rf30/yap/v2/pkg/i18n"
)

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
	// Triple is the GNU triplet for the target architecture
	Triple string
	// InstallCommands provides distribution-specific installation commands
	InstallCommands map[string]string
}

// Validate checks if the required toolchain executables are available in PATH.
func (ct *CrossToolchain) Validate() ([]string, error) {
	var missing []string

	// Check GCC
	if ct.GCCPackage != "" {
		if _, err := exec.LookPath(ct.GCCPackage); err != nil {
			missing = append(missing, ct.GCCPackage)
		}
	}

	// Check G++
	if ct.GPlusPlusPackage != "" {
		if _, err := exec.LookPath(ct.GPlusPlusPackage); err != nil {
			missing = append(missing, ct.GPlusPlusPackage)
		}
	}

	// Check Binutils
	if ct.BinutilsPackage != "" {
		if _, err := exec.LookPath(ct.BinutilsPackage); err != nil {
			missing = append(missing, ct.BinutilsPackage)
		}
	}

	// Check for common binutils tools
	binutilsTools := []string{"ar", "ld", "strip", "objdump", "nm"}
	for _, tool := range binutilsTools {
		if _, err := exec.LookPath(tool); err != nil {
			missing = append(missing, tool)
		}
	}

	if len(missing) > 0 {
		return missing, fmt.Errorf("missing required toolchain executables: %v", missing)
	}

	return nil, nil
}

// GetAllPackages returns all packages needed for this toolchain.
func (ct *CrossToolchain) GetAllPackages() []string {
	var packages []string

	if ct.GCCPackage != "" {
		packages = append(packages, ct.GCCPackage)
	}

	if ct.GPlusPlusPackage != "" {
		packages = append(packages, ct.GPlusPlusPackage)
	}

	if ct.BinutilsPackage != "" {
		packages = append(packages, ct.BinutilsPackage)
	}

	packages = append(packages, ct.AdditionalPackages...)

	return packages
}

// GetPackagesByType returns packages categorized by type.
func (ct *CrossToolchain) GetPackagesByType() map[string][]string {
	result := make(map[string][]string)

	if ct.GCCPackage != "" {
		result["compiler"] = append(result["compiler"], ct.GCCPackage)
	}

	if ct.GPlusPlusPackage != "" {
		result["compiler"] = append(result["compiler"], ct.GPlusPlusPackage)
	}

	if ct.BinutilsPackage != "" {
		result["binutils"] = append(result["binutils"], ct.BinutilsPackage)
	}

	if len(ct.AdditionalPackages) > 0 {
		result["libraries"] = ct.AdditionalPackages
	}

	return result
}

// CrossToolchainMap maps target architectures to toolchain packages for each distribution.
var CrossToolchainMap = func() map[string]map[string]CrossToolchain {
	// Define base patterns for each distribution
	basePatterns := map[string]map[string]string{
		"debian": {
			"gcc":        "gcc-%s",
			"g++":        "g++-%s",
			"binutils":   "binutils-%s",
			"additional": "libc6-dev-%s-cross",
		},
		"ubuntu": {
			"gcc":        "gcc-%s",
			"g++":        "g++-%s",
			"binutils":   "binutils-%s",
			"additional": "libc6-dev-%s-cross",
		},
		"fedora": {
			"gcc":        "gcc-%s",
			"g++":        "gcc-c++-%s",
			"binutils":   "binutils-%s",
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
			"arch":    {"lib32-gcc-libs"}, // Arch multilib needs lib32 libraries
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

			// Determine which pattern to use for this architecture
			// For Alpine, use simple arch name (armv7, aarch64, etc.)
			// For Debian/Ubuntu/Fedora/Arch, use GNU triplet from archPatterns
			archPattern := arch
			if distro == "alpine" {
				// Alpine uses simplified names
				archPattern = arch
			} else {
				// Debian, Ubuntu, Fedora, Arch use GNU triplet patterns
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

			// Get the GNU triplet for this architecture
			triple := archPatterns[arch]

			// Build install commands for each distribution
			installCommands := make(map[string]string)
			switch distro {
			case distroDebian, distroUbuntu:
				installCommands[distro] = fmt.Sprintf("sudo apt-get install %s %s", gcc, gpp)
			case distroFedora:
				installCommands[distro] = fmt.Sprintf("sudo dnf install %s %s", gcc, gpp)
			case distroArch:
				installCommands[distro] = fmt.Sprintf("sudo pacman -S %s %s", gcc, gpp)
			case distroAlpine:
				installCommands[distro] = fmt.Sprintf("sudo apk add %s %s", gcc, gpp)
			}

			result[arch][distro] = CrossToolchain{
				GCCPackage:         gcc,
				GPlusPlusPackage:   gpp,
				BinutilsPackage:    binutils,
				AdditionalPackages: additional,
				Triple:             triple,
				InstallCommands:    installCommands,
			}
		}
	}

	return result
}()

// GetExecutableName converts a package name to its executable name.
// Different distributions use different naming conventions:
//   - Debian/Ubuntu/Fedora: gcc-aarch64-linux-gnu -> aarch64-linux-gnu-gcc
//   - Arch Linux: aarch64-linux-gnu-gcc -> aarch64-linux-gnu-gcc (already correct)
//   - Alpine: gcc-aarch64 -> aarch64-alpine-linux-musl-gcc (needs special handling)
func (ct *CrossToolchain) GetExecutableName(packageName string) string {
	// Handle Fedora's gcc-c++ pattern (G++)
	if after, ok := strings.CutPrefix(packageName, "gcc-c++-"); ok {
		suffix := after
		// Fedora: gcc-c++-aarch64-linux-gnu -> aarch64-linux-gnu-g++
		return suffix + "-g++"
	}

	// Handle g++ pattern
	if after, ok := strings.CutPrefix(packageName, "g++-"); ok {
		suffix := after
		// Debian/Ubuntu: g++-aarch64-linux-gnu -> aarch64-linux-gnu-g++
		return suffix + "-g++"
	}

	// Handle gcc pattern
	if after, ok := strings.CutPrefix(packageName, "gcc-"); ok {
		suffix := after

		// Alpine special case: gcc-aarch64 needs to become aarch64-alpine-linux-musl-gcc
		// However, we can't reliably determine this without knowing the distribution
		// So we'll just do the basic transformation and let Alpine handle it
		// Alpine: gcc-aarch64 -> aarch64-gcc (which may need further handling at runtime)
		return suffix + "-gcc"
	}

	// Otherwise, return as-is (already in correct format for Arch, etc.)
	// Arch: aarch64-linux-gnu-gcc -> aarch64-linux-gnu-gcc
	return packageName
}

// GetCrossToolchain retrieves the toolchain for a given architecture and distribution.
func GetCrossToolchain(arch, distro string) (CrossToolchain, error) {
	toolchains := CrossToolchainMap

	// Try exact match first
	if distroChains, exists := toolchains[arch]; exists {
		if chain, exists := distroChains[distro]; exists {
			return chain, nil
		}
	}

	// Try fallback to debian for ubuntu/debian family
	if distro == "ubuntu" || distro == "debian" {
		if distroChains, exists := toolchains[arch]; exists {
			if chain, exists := distroChains["debian"]; exists {
				return chain, nil
			}
		}
	}

	return CrossToolchain{}, fmt.Errorf(
		"unsupported cross-compilation toolchain: arch=%s, distro=%s",
		arch,
		distro,
	)
}

// ValidateToolchain checks if the required cross-compilation toolchain is available
// for the given target architecture and package format. It returns a detailed error
// message with installation instructions if the toolchain is missing.
func ValidateToolchain(targetArch, format string) error {
	// Map package format to distribution for toolchain lookup
	formatToDistro := map[string]string{
		"deb":    "ubuntu",
		"rpm":    "fedora",
		"apk":    "alpine",
		"pacman": "arch",
	}

	distro, exists := formatToDistro[format]
	if !exists {
		return fmt.Errorf(i18n.T("errors.cross_compilation.unsupported_format")+" %s", format)
	}

	// Get the toolchain configuration
	toolchain, err := GetCrossToolchain(targetArch, distro)
	if err != nil {
		return fmt.Errorf(i18n.T("errors.cross_compilation.failed_to_get_toolchain")+" %s/%s: %w", targetArch, distro, err)
	}

	// Validate the toolchain
	missing, err := (&toolchain).Validate()
	if err == nil {
		// All required tools are available
		return nil
	}

	// Build detailed error message with missing executables and installation instructions
	var msg strings.Builder

	msg.WriteString(i18n.T("errors.cross_compilation.toolchain_validation_failed") + "\n")
	archFormatMsg := i18n.T("errors.cross_compilation.target_architecture_format")
	msg.WriteString(fmt.Sprintf("%s: %s (%s)\n", archFormatMsg, targetArch, format))
	msg.WriteString("\n" + i18n.T("errors.cross_compilation.missing_executables") + ":\n")

	for _, exe := range missing {
		msg.WriteString(fmt.Sprintf("  - %s\n", exe))
	}

	// Get all required packages
	packages := (&toolchain).GetAllPackages()
	if len(packages) > 0 {
		msg.WriteString("\n" + i18n.T("errors.cross_compilation.required_packages") + ":\n")

		for _, pkg := range packages {
			msg.WriteString(fmt.Sprintf("  - %s\n", pkg))
		}
	}

	// Add installation command if available
	if installCmd, exists := toolchain.InstallCommands[distro]; exists {
		msg.WriteString("\n" + i18n.T("errors.cross_compilation.installation_command") + ":\n")
		msg.WriteString(fmt.Sprintf("  %s", installCmd))

		// Add additional packages if present
		if len(toolchain.AdditionalPackages) > 0 {
			for _, pkg := range toolchain.AdditionalPackages {
				msg.WriteString(fmt.Sprintf(" %s", pkg))
			}
		}

		if toolchain.BinutilsPackage != "" {
			msg.WriteString(fmt.Sprintf(" %s", toolchain.BinutilsPackage))
		}

		msg.WriteString("\n")
	}

	// Add additional distribution-specific guidance
	msg.WriteString("\n" + i18n.T("errors.cross_compilation.path_note") + "\n")

	switch distro {
	case "alpine":
		msg.WriteString(i18n.T("errors.cross_compilation.alpine_note") + "\n")
	case "arch":
		if targetArch == "i686" {
			msg.WriteString(i18n.T("errors.cross_compilation.arch_multilib_note") + "\n")
		} else {
			msg.WriteString(i18n.T("errors.cross_compilation.arch_prefix_note") + "\n")
		}
	}

	// Add tip about skipping validation
	msg.WriteString("\n" + i18n.T("errors.cross_compilation.skip_validation_tip") + "\n")

	return fmt.Errorf("%s", msg.String())
}
