// Package constants provides centralized constants and mappings for all package formats.
package constants

// Canonical Architecture Names
//
// YAP uses a consistent set of canonical architecture names internally.
// These names are used in toolchain definitions and cross-compilation logic.
// External architecture names (aliases) are normalized to these canonical names.
//
// Canonical Names:
//   - x86_64   : 64-bit x86 (also known as amd64, x86-64)
//   - i686     : 32-bit x86 (also known as i386, i586, x86)
//   - aarch64  : 64-bit ARM (also known as arm64)
//   - armv7    : 32-bit ARMv7 (also known as armv7h, armv7l, armhf)
//   - armv6    : 32-bit ARMv6 (also known as armv6h, armv6l, arm)
//   - ppc64le  : 64-bit PowerPC little-endian
//   - s390x    : 64-bit IBM System z
//   - riscv64  : 64-bit RISC-V
//
// Format-Specific Translations:
// These canonical names are translated to format-specific names when building packages:
//   - APK: x86_64, x86, aarch64, armv7, armhf
//   - DEB: amd64, i386, arm64, armhf, armel
//   - RPM: x86_64, i686, aarch64, armv7hl
//   - Pacman: x86_64, i686, aarch64, armv7h, armv6h
const (
	// ArchX86_64 represents 64-bit x86 architecture (canonical name).
	ArchX86_64 = "x86_64"
	// ArchI686 represents 32-bit x86 architecture (canonical name).
	ArchI686 = "i686"
	// ArchAarch64 represents 64-bit ARM architecture (canonical name).
	ArchAarch64 = "aarch64"
	// ArchArmv7 represents 32-bit ARMv7 architecture (canonical name).
	ArchArmv7 = "armv7"
	// ArchArmv6 represents 32-bit ARMv6 architecture (canonical name).
	ArchArmv6 = "armv6"
	// ArchPpc64le represents 64-bit PowerPC little-endian architecture (canonical name).
	ArchPpc64le = "ppc64le"
	// ArchS390x represents 64-bit IBM System z architecture (canonical name).
	ArchS390x = "s390x"
	// ArchRiscv64 represents 64-bit RISC-V architecture (canonical name).
	ArchRiscv64 = "riscv64"
)

// Architecture alias constants (non-canonical names used by various distros).
//
// These are the format-specific or distro-specific spellings of architectures
// that get translated to canonical names via NormalizeArchitecture.
const (
	// ArchAmd64 is the Debian/Go alias for x86_64.
	ArchAmd64 = "amd64"
	// ArchI386 is the Debian alias for i686.
	ArchI386 = "i386"
	// ArchArm64 is the Debian/Go alias for aarch64.
	ArchArm64 = "arm64"
	// ArchArm is the bare ARM identifier (used as armv6 alias in many places).
	ArchArm = "arm"
	// ArchArmel is the Debian alias for armv6 (soft-float).
	ArchArmel = "armel"
	// ArchArmhf is the Debian alias for armv7 (hard-float).
	ArchArmhf = "armhf"
	// ArchArmv7h is the Pacman/RPM alias for armv7.
	ArchArmv7h = "armv7h"
	// ArchArmv6h is the Pacman alias for armv6.
	ArchArmv6h = "armv6h"
	// ArchArmv7l is the Linux uname alias for armv7.
	ArchArmv7l = "armv7l"
	// ArchArmv7hl is the RPM alias for armv7.
	ArchArmv7hl = "armv7hl"
	// ArchArmv6l is the Linux uname alias for armv6.
	ArchArmv6l = "armv6l"
	// ArchAny represents architecture-independent packages.
	ArchAny = "any"
	// ArchAll is the Debian/APK alias for "any".
	ArchAll = "all"
	// ArchNoarch is the RPM alias for "any".
	ArchNoarch = "noarch"
	// ArchX8664Dash is the dashed alias for x86_64 (used by some tools).
	ArchX8664Dash = "x86-64"
	// ArchX64 is a short alias for x86_64.
	ArchX64 = "x64"
	// ArchX86 is the i686 alias used by APK and others.
	ArchX86 = "x86"
	// ArchI586 is an i686 alias.
	ArchI586 = "i586"
	// ArchI486 is an i686 alias.
	ArchI486 = "i486"
	// ArchIa32 is an i686 alias.
	ArchIa32 = "ia32"
	// ArchPowerpc64le is a long alias for ppc64le.
	ArchPowerpc64le = "powerpc64le"
	// ArchPpc64el is the Debian alias for ppc64le.
	ArchPpc64el = "ppc64el"
	// ArchPpc64 is the big-endian PowerPC 64.
	ArchPpc64 = "ppc64"
	// ArchRiscv64gc is an alias for riscv64 with general+compressed extension.
	ArchRiscv64gc = "riscv64gc"
	// ArchMips is 32-bit big-endian MIPS.
	ArchMips = "mips"
	// ArchMipsle is 32-bit little-endian MIPS.
	ArchMipsle = "mipsle"
)

// Cross-compilation toolchain triplet constants.
//
// These are GNU triplets used to identify cross-compilers and target sysroots.
const (
	// TripletAarch64Linux is the GNU triplet for aarch64-linux-gnu.
	TripletAarch64Linux = "aarch64-linux-gnu"
	// TripletArmLinuxHf is the GNU triplet for arm-linux-gnueabihf (armv7).
	TripletArmLinuxHf = "arm-linux-gnueabihf"
	// TripletI686Linux is the GNU triplet for i686-linux-gnu.
	TripletI686Linux = "i686-linux-gnu"
	// TripletX8664Linux is the GNU triplet for x86_64-linux-gnu.
	TripletX8664Linux = "x86_64-linux-gnu"
	// TripletPpc64leLinux is the GNU triplet for powerpc64le-linux-gnu.
	TripletPpc64leLinux = "powerpc64le-linux-gnu"
	// TripletS390xLinux is the GNU triplet for s390x-linux-gnu.
	TripletS390xLinux = "s390x-linux-gnu"
	// TripletRiscv64Linux is the GNU triplet for riscv64-linux-gnu.
	TripletRiscv64Linux = "riscv64-linux-gnu"
)

// ArchitectureMapping provides a unified interface for architecture translations.
type ArchitectureMapping struct {
	APK    map[string]string
	DEB    map[string]string
	RPM    map[string]string
	Pacman map[string]string
}

// GetArchMapping returns the unified architecture mappings for all package formats.
func GetArchMapping() *ArchitectureMapping {
	return &ArchitectureMapping{
		APK: map[string]string{
			ArchX86_64:  ArchX86_64,
			ArchI686:    ArchX86,
			ArchAarch64: ArchAarch64,
			ArchArmv7h:  ArchArmv7h,
			ArchArmv6h:  ArchArmv6h,
			ArchArm:     ArchArmhf,
			ArchAny:     ArchAll,
		},
		DEB: map[string]string{
			ArchX86_64:  ArchAmd64,
			ArchI686:    ArchI386,
			ArchAarch64: ArchArm64,
			ArchArmv6:   ArchArmel,
			ArchArmv6h:  ArchArmel,
			ArchArm:     ArchArmel,
			ArchArmv7:   ArchArmhf,
			ArchArmv7h:  ArchArmhf,
			ArchAny:     ArchAll,
		},
		RPM: map[string]string{
			ArchX86_64:  ArchX86_64,
			ArchI686:    ArchI686,
			ArchAarch64: ArchAarch64,
			ArchArmv7h:  ArchArmv7h,
			ArchArmv6h:  ArchArmv6h,
			ArchArm:     ArchArm,
			ArchAny:     ArchNoarch,
		},
		Pacman: map[string]string{
			ArchX86_64:  ArchX86_64,
			ArchI686:    ArchI686,
			ArchAarch64: ArchAarch64,
			ArchArm:     ArchArm,
			ArchArmv6h:  ArchArmv6h,
			ArchArmv7h:  ArchArmv7h,
			ArchAny:     ArchAny,
		},
	}
}

// TranslateArch translates an architecture for a specific package format.
func (am *ArchitectureMapping) TranslateArch(format, arch string) string {
	var mapping map[string]string

	switch format {
	case FormatAPK:
		mapping = am.APK
	case FormatDEB:
		mapping = am.DEB
	case FormatRPM:
		mapping = am.RPM
	case FormatPacman:
		mapping = am.Pacman
	default:
		return arch // Return original if format unknown
	}

	if translated, exists := mapping[arch]; exists {
		return translated
	}

	return arch // Return original if not found in mapping
}

// NormalizeArchitecture converts architecture aliases to canonical architecture names.
// This ensures consistent architecture naming throughout YAP, regardless of input format.
//
// Common aliases supported:
//   - amd64, x86-64 → x86_64
//   - i386, i586, x86 → i686
//   - arm64 → aarch64
//   - armv7h, armv7l, armhf, armv7hl → armv7
//   - armv6h, armv6l, arm, armel → armv6
//
// If the architecture is already canonical or unknown, it is returned unchanged.
//
// Example:
//
//	NormalizeArchitecture("amd64")  // returns "x86_64"
//	NormalizeArchitecture("arm64")  // returns "aarch64"
//	NormalizeArchitecture("x86_64") // returns "x86_64" (already canonical)
func NormalizeArchitecture(arch string) string {
	// Architecture alias mappings to canonical names
	aliasMap := map[string]string{
		// x86_64 aliases
		ArchAmd64:     ArchX86_64,
		ArchX8664Dash: ArchX86_64,
		ArchX64:       ArchX86_64,

		// i686 aliases
		ArchI386: ArchI686,
		ArchI586: ArchI686,
		ArchX86:  ArchI686,
		ArchI486: ArchI686,
		ArchIa32: ArchI686,

		// aarch64 aliases
		ArchArm64: ArchAarch64,

		// armv7 aliases
		ArchArmv7h:  ArchArmv7,
		ArchArmv7l:  ArchArmv7,
		ArchArmhf:   ArchArmv7,
		ArchArmv7hl: ArchArmv7,

		// armv6 aliases
		ArchArmv6h: ArchArmv6,
		ArchArmv6l: ArchArmv6,
		ArchArm:    ArchArmv6,
		ArchArmel:  ArchArmv6,

		// ppc64le aliases
		ArchPowerpc64le: ArchPpc64le,
		ArchPpc64el:     ArchPpc64le,

		// riscv64 aliases
		ArchRiscv64gc: ArchRiscv64,
	}

	// Check if it's an alias
	if canonical, exists := aliasMap[arch]; exists {
		return canonical
	}

	// Already canonical or unknown - return as-is
	return arch
}

// GetReverseMapping returns a reverse mapping from format-specific architecture names
// to canonical names for a given package format.
//
// This is useful for converting format-specific architecture names back to canonical
// names when parsing existing packages or user input.
//
// When multiple canonical names map to the same format-specific name, the canonical
// architecture name (from the Arch* constants) is preferred over aliases.
//
// Example:
//
//	reverseMap := GetReverseMapping("deb")
//	canonical := reverseMap["amd64"]  // returns "x86_64"
func GetReverseMapping(format string) map[string]string {
	archMapping := GetArchMapping()

	var forwardMap map[string]string

	switch format {
	case FormatAPK:
		forwardMap = archMapping.APK
	case FormatDEB:
		forwardMap = archMapping.DEB
	case FormatRPM:
		forwardMap = archMapping.RPM
	case FormatPacman:
		forwardMap = archMapping.Pacman
	default:
		return make(map[string]string)
	}

	// Define canonical names for prioritization
	canonicalNames := map[string]bool{
		ArchX86_64:  true,
		ArchI686:    true,
		ArchAarch64: true,
		ArchArmv7:   true,
		ArchArmv6:   true,
		ArchPpc64le: true,
		ArchS390x:   true,
		ArchRiscv64: true,
		ArchAny:     true, // "any" is also considered canonical
	}

	// Create reverse mapping, preferring canonical names
	reverseMap := make(map[string]string)
	for canonical, formatSpecific := range forwardMap {
		// Only set if not already set, or if this is a canonical name
		if existing, exists := reverseMap[formatSpecific]; !exists || !canonicalNames[existing] {
			reverseMap[formatSpecific] = canonical
		}
	}

	return reverseMap
}
