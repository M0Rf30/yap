// Package common provides shared functionality for cross-compilation toolchains.
package common

import (
	"fmt"
	"os/exec"
	"strings"

	"github.com/M0Rf30/yap/v2/pkg/constants"
	"github.com/M0Rf30/yap/v2/pkg/errors"
	"github.com/M0Rf30/yap/v2/pkg/i18n"
)

const (
	gccKey           = "gcc"
	gppKey           = "g++"
	binutilsKey      = "binutils"
	additionalKey    = "additional"
	defaultKey       = "default"
	muslDevPkg       = "musl-dev"
	gccMultilibPkg   = "gcc-multilib"
	binutilsMultilib = "binutils-multilib"
	gppArmv7Pkg      = "g++-armv7"
	lib32GccLibs     = "lib32-gcc-libs"
	libc6DevI386     = "libc6-dev-i386-cross"
	gccFmtPattern    = "gcc-%s"
	gppFmtPattern    = "g++-%s"
	binutilsFmtPat   = "binutils-%s"
	gppCxxFmtPat     = "gcc-c++-%s"
	archGccFmtPat    = "%s-gcc"
	archGppFmtPat    = "%s-g++"
	archBinFmtPat    = "%s-binutils"
	libc6DevFmtPat   = "libc6-dev-%s-cross"
	gccCxxMultilib   = "gcc-c++-multilib"
	gccX86FmtPat     = "gcc-x86_64-linux-gnu"
	gppX86FmtPat     = "gcc-c++-x86_64-linux-gnu"
	binX86FmtPat     = "binutils-x86_64-linux-gnu"
	gccArmhf         = "gcc-armhf"
	gppArmhf         = "g++-armhf"
	binArmhf         = "binutils-armhf"
	gccS390xRH       = "gcc-s390x-redhat-linux"
	gppS390xRH       = "gcc-c++-s390x-redhat-linux"
	binS390xRH       = "binutils-s390x-redhat-linux"
	aarch64ArchGcc   = "aarch64-linux-gnu-gcc"
	aarch64ArchGpp   = "aarch64-linux-gnu-g++"
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
// It resolves package names to their actual executable names before checking.
func (ct *CrossToolchain) Validate() ([]string, error) {
	var missing []string

	// Check GCC — resolve package name to executable name first.
	if ct.GCCPackage != "" {
		exe := ct.GetExecutableName(ct.GCCPackage)
		if _, err := exec.LookPath(exe); err != nil {
			missing = append(missing, exe)
		}
	}

	// Check G++ — resolve package name to executable name first.
	if ct.GPlusPlusPackage != "" {
		exe := ct.GetExecutableName(ct.GPlusPlusPackage)
		if _, err := exec.LookPath(exe); err != nil {
			missing = append(missing, exe)
		}
	}

	// Check cross-prefixed binutils tools (ar, strip, etc.) derived from the
	// binutils package name.  We skip the host-native tools (ar, ld, …) because
	// those are always present and checking them adds no signal for cross builds.
	if ct.BinutilsPackage != "" {
		prefix := ct.binutilsPrefix()
		if prefix != "" {
			for _, tool := range []string{"ar", "strip", "nm", "objdump", "objcopy"} {
				exe := prefix + "-" + tool
				if _, err := exec.LookPath(exe); err != nil {
					missing = append(missing, exe)
				}
			}
		}
	}

	if len(missing) > 0 {
		return missing, errors.New(errors.ErrTypeBuild, "missing required toolchain executables").
			WithOperation("Validate").
			WithContext("missing", fmt.Sprintf("%v", missing))
	}

	return nil, nil
}

// binutilsPrefix derives the cross-tool prefix from the BinutilsPackage name.
// Examples:
//   - "binutils-aarch64-linux-gnu"  → "aarch64-linux-gnu"   (Debian/Ubuntu/Fedora)
//   - "aarch64-linux-gnu-binutils"  → "aarch64-linux-gnu"   (Arch)
//   - "binutils-armv7"              → "armv7"               (Alpine)
func (ct *CrossToolchain) binutilsPrefix() string {
	pkg := ct.BinutilsPackage

	// "binutils-<prefix>" style (Debian, Ubuntu, Fedora, Alpine)
	if after, ok := strings.CutPrefix(pkg, "binutils-"); ok {
		return after
	}

	// "<prefix>-binutils" style (Arch)
	if before, ok := strings.CutSuffix(pkg, "-binutils"); ok {
		return before
	}

	return ""
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
		constants.DistroDebian: {
			gccKey:        gccFmtPattern,
			gppKey:        gppFmtPattern,
			binutilsKey:   binutilsFmtPat,
			additionalKey: libc6DevFmtPat,
		},
		constants.DistroUbuntu: {
			gccKey:        gccFmtPattern,
			gppKey:        gppFmtPattern,
			binutilsKey:   binutilsFmtPat,
			additionalKey: libc6DevFmtPat,
		},
		constants.DistroFedora: {
			gccKey:        gccFmtPattern,
			gppKey:        gppCxxFmtPat,
			binutilsKey:   binutilsFmtPat,
			additionalKey: "",
		},
		// Alpine Linux does not ship Linux-userspace cross-compiler packages
		// (gcc-aarch64, etc.) in its repos.  Cross-compilation for APK targets
		// requires bootstrap.sh or a native Alpine container/QEMU environment.
		// Alpine is intentionally omitted from CrossToolchainMap.
		constants.DistroArch: {
			gccKey:        archGccFmtPat,
			gppKey:        archGppFmtPat,
			binutilsKey:   archBinFmtPat,
			additionalKey: "",
		},
	}

	// Define architecture-specific mappings for special cases
	archSpecific := map[string]map[string]map[string]string{
		constants.ArchI686: {
			constants.DistroArch: {
				gccKey:      gccMultilibPkg,
				gppKey:      gccCxxMultilib,
				binutilsKey: binutilsMultilib,
			},
		},
		constants.ArchX86_64: {
			constants.DistroFedora: {
				gccKey:      gccX86FmtPat,
				gppKey:      gppX86FmtPat,
				binutilsKey: binX86FmtPat,
			},
		},
		constants.ArchAarch64: {
			constants.DistroArch: {
				gccKey:      aarch64ArchGcc,
				gppKey:      aarch64ArchGpp,
				binutilsKey: "aarch64-linux-gnu-binutils",
			},
		},

		constants.ArchS390x: {
			constants.DistroFedora: {
				gccKey:      gccS390xRH,
				gppKey:      gppS390xRH,
				binutilsKey: binS390xRH,
			},
		},
	}

	// Define architecture-specific additional packages
	archAdditional := map[string]map[string][]string{
		constants.ArchAarch64: {
			defaultKey: {"libc6-dev-arm64-cross"},
		},
		constants.ArchArmv7: {
			defaultKey: {"libc6-dev-armhf-cross"},
		},
		constants.ArchArmv6: {
			defaultKey: {"libc6-dev-armhf-cross"},
		},
		constants.ArchI686: {
			constants.DistroArch: {lib32GccLibs}, // Arch multilib needs lib32 libraries
			defaultKey:           {libc6DevI386},
		},
		constants.ArchX86_64: {
			defaultKey: {"libc6-dev-amd64-cross"},
		},
		constants.ArchPpc64le: {
			defaultKey: {"libc6-dev-ppc64el-cross"},
		},
		constants.ArchS390x: {
			defaultKey: {"libc6-dev-s390x-cross"},
		},
		constants.ArchRiscv64: {
			defaultKey: {"libc6-dev-riscv64-cross"},
		},
	}

	// Define architecture-specific patterns that need special handling
	archPatterns := map[string]string{
		constants.ArchAarch64: constants.TripletAarch64Linux,
		constants.ArchArmv7:   constants.TripletArmLinuxHf,
		constants.ArchArmv6:   constants.TripletArmLinuxHf,
		constants.ArchI686:    constants.TripletI686Linux,
		constants.ArchX86_64:  "x86-64-linux-gnu",
		constants.ArchPpc64le: constants.TripletPpc64leLinux,
		constants.ArchS390x:   constants.TripletS390xLinux,
		constants.ArchRiscv64: constants.TripletRiscv64Linux,
	}

	result := make(map[string]map[string]CrossToolchain)

	// Architectures to process
	architectures := []string{
		constants.ArchAarch64, constants.ArchArmv7, constants.ArchArmv6, constants.ArchI686,
		constants.ArchX86_64, constants.ArchPpc64le, constants.ArchS390x, constants.ArchRiscv64,
	}
	// Alpine is excluded: it has no Linux-userspace cross-compiler packages.
	distributions := []string{
		constants.DistroDebian, constants.DistroUbuntu, constants.DistroFedora,
		constants.DistroArch,
	}

	for _, arch := range architectures {
		result[arch] = make(map[string]CrossToolchain)

		for _, distro := range distributions {
			// Get base patterns for this distribution
			patterns, exists := basePatterns[distro]
			if !exists {
				continue
			}

			// All supported distros use GNU triplet patterns.
			archPattern := arch
			if pattern, exists := archPatterns[arch]; exists {
				archPattern = pattern
			}

			// Start with base values using the appropriate pattern
			gcc := fmt.Sprintf(patterns[gccKey], archPattern)
			gpp := fmt.Sprintf(patterns[gppKey], archPattern)
			binutils := fmt.Sprintf(patterns[binutilsKey], archPattern)

			// Apply architecture-specific overrides if they exist
			if archOverrides, exists := archSpecific[arch]; exists {
				if distroOverrides, exists := archOverrides[distro]; exists {
					if override, exists := distroOverrides[gccKey]; exists {
						gcc = override
					}

					if override, exists := distroOverrides[gppKey]; exists {
						gpp = override
					}

					if override, exists := distroOverrides[binutilsKey]; exists {
						binutils = override
					}
				}
			}

			// Determine additional packages
			var additional []string

			if archAdd, exists := archAdditional[arch]; exists {
				if add, exists := archAdd[distro]; exists {
					additional = add
				} else if add, exists := archAdd[defaultKey]; exists {
					additional = add
				}
			}

			// Get the GNU triplet for this architecture
			triple := archPatterns[arch]

			// Build install commands for each distribution
			installCommands := make(map[string]string)

			switch distro {
			case constants.DistroDebian, constants.DistroUbuntu:
				installCommands[distro] = fmt.Sprintf("sudo apt-get install %s %s", gcc, gpp)
			case constants.DistroFedora:
				installCommands[distro] = fmt.Sprintf("sudo dnf install %s %s", gcc, gpp)
			case constants.DistroArch:
				installCommands[distro] = fmt.Sprintf("sudo pacman -S %s %s", gcc, gpp)
			case constants.DistroAlpine:
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

// alpineMuslTriplets maps Alpine arch names (as used in package names like
// "gcc-aarch64") to their full musl cross-compiler triplet prefixes.
var alpineMuslTriplets = map[string]string{
	constants.ArchAarch64: "aarch64-alpine-linux-musl",
	constants.ArchArmv7:   "armv7-alpine-linux-musleabihf",
	"armhf":               "arm-alpine-linux-musleabihf",
	"x86":                 "i586-alpine-linux-musl",
	constants.ArchX86_64:  "x86_64-alpine-linux-musl",
	constants.ArchPpc64le: "powerpc64le-alpine-linux-musl",
	constants.ArchS390x:   "s390x-alpine-linux-musl",
	constants.ArchRiscv64: "riscv64-alpine-linux-musl",
}

// GetExecutableName converts a package name to its actual cross-compiler
// executable name.  Naming conventions differ by distribution:
//
//   - Debian/Ubuntu: gcc-aarch64-linux-gnu  → aarch64-linux-gnu-gcc
//   - Fedora:        gcc-c++-aarch64-linux-gnu → aarch64-linux-gnu-g++
//   - Arch:          aarch64-linux-gnu-gcc  → aarch64-linux-gnu-gcc (already correct)
//   - Alpine:        gcc-aarch64            → aarch64-alpine-linux-musl-gcc
func (ct *CrossToolchain) GetExecutableName(packageName string) string {
	// Fedora G++: "gcc-c++-<triplet>" → "<triplet>-g++"
	if after, ok := strings.CutPrefix(packageName, "gcc-c++-"); ok {
		return after + "-g++"
	}

	// Debian/Ubuntu G++: "g++-<triplet>" → "<triplet>-g++"
	if after, ok := strings.CutPrefix(packageName, "g++-"); ok {
		return after + "-g++"
	}

	// GCC: "gcc-<suffix>" → resolve suffix to executable.
	if after, ok := strings.CutPrefix(packageName, "gcc-"); ok {
		// Alpine uses short arch names (e.g. "gcc-aarch64").  Map them to the
		// full musl triplet so the executable name is correct.
		if triplet, ok := alpineMuslTriplets[after]; ok {
			return triplet + "-gcc"
		}

		// Debian/Ubuntu/Fedora: "gcc-aarch64-linux-gnu" → "aarch64-linux-gnu-gcc"
		return after + "-gcc"
	}

	// Arch (and any already-correct form): return as-is.
	// e.g. "aarch64-linux-gnu-gcc" stays "aarch64-linux-gnu-gcc".
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
	if distro == constants.DistroUbuntu || distro == constants.DistroDebian {
		if distroChains, exists := toolchains[arch]; exists {
			if chain, exists := distroChains[constants.DistroDebian]; exists {
				return chain, nil
			}
		}
	}

	return CrossToolchain{}, errors.New(errors.ErrTypeBuild,
		fmt.Sprintf("unsupported cross-compilation toolchain: arch=%s, distro=%s", arch, distro)).
		WithOperation("GetCrossToolchain").
		WithContext("targetArch", arch).
		WithContext("distro", distro)
}

// ValidateToolchain checks if the required cross-compilation toolchain is available
// for the given target architecture and package format. It returns a detailed error
// message with installation instructions if the toolchain is missing.
//
// APK (Alpine) format always returns an error: Alpine does not ship Linux-userspace
// cross-compiler packages.  Use a native Alpine container or QEMU instead.
func ValidateToolchain(targetArch, format string) error {
	// Alpine cross-compilation via host packages is not supported.
	if format == constants.FormatAPK {
		return errors.New(errors.ErrTypeBuild,
			"cross-compilation for APK (Alpine) is not supported via host toolchains: "+
				"Alpine does not ship Linux-userspace cross-compiler packages. "+
				"Use a native Alpine container or QEMU binfmt_misc instead.").
			WithOperation("ValidateToolchain").
			WithContext("targetArch", targetArch).
			WithContext("format", format)
	}

	// Map package format to distribution for toolchain lookup
	formatToDistro := map[string]string{
		constants.FormatDEB:    constants.DistroUbuntu,
		constants.FormatRPM:    constants.DistroFedora,
		constants.FormatPacman: constants.DistroArch,
	}

	distro, exists := formatToDistro[format]
	if !exists {
		msg := fmt.Sprintf("%s %s", i18n.T("errors.cross_compilation.unsupported_format"), format)

		return errors.New(errors.ErrTypeBuild, msg).
			WithOperation("ValidateToolchain").
			WithContext("format", format)
	}

	// Get the toolchain configuration
	toolchain, err := GetCrossToolchain(targetArch, distro)
	if err != nil {
		return errors.Wrap(err, errors.ErrTypeBuild, i18n.T("errors.cross_compilation.failed_to_get_toolchain")).
			WithOperation("ValidateToolchain").
			WithContext("targetArch", targetArch).
			WithContext("distro", distro)
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
	fmt.Fprintf(&msg, "%s: %s (%s)\n", archFormatMsg, targetArch, format)
	msg.WriteString("\n" + i18n.T("errors.cross_compilation.missing_executables") + ":\n")

	for _, exe := range missing {
		fmt.Fprintf(&msg, "  - %s\n", exe)
	}

	// Get all required packages
	packages := (&toolchain).GetAllPackages()
	if len(packages) > 0 {
		msg.WriteString("\n" + i18n.T("errors.cross_compilation.required_packages") + ":\n")

		for _, pkg := range packages {
			fmt.Fprintf(&msg, "  - %s\n", pkg)
		}
	}

	// Add installation command if available
	if installCmd, exists := toolchain.InstallCommands[distro]; exists {
		msg.WriteString("\n" + i18n.T("errors.cross_compilation.installation_command") + ":\n")
		fmt.Fprintf(&msg, "  %s", installCmd)

		// Add additional packages if present
		if len(toolchain.AdditionalPackages) > 0 {
			for _, pkg := range toolchain.AdditionalPackages {
				fmt.Fprintf(&msg, " %s", pkg)
			}
		}

		if toolchain.BinutilsPackage != "" {
			fmt.Fprintf(&msg, " %s", toolchain.BinutilsPackage)
		}

		msg.WriteString("\n")
	}

	// Add additional distribution-specific guidance
	msg.WriteString("\n" + i18n.T("errors.cross_compilation.path_note") + "\n")

	if distro == constants.DistroArch {
		if targetArch == constants.ArchI686 {
			msg.WriteString(i18n.T("errors.cross_compilation.arch_multilib_note") + "\n")
		} else {
			msg.WriteString(i18n.T("errors.cross_compilation.arch_prefix_note") + "\n")
		}
	}

	// Add tip about skipping validation
	msg.WriteString("\n" + i18n.T("errors.cross_compilation.skip_validation_tip") + "\n")

	return errors.New(errors.ErrTypeBuild, msg.String()).
		WithOperation("ValidateToolchain")
}
