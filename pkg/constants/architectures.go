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
			"x86_64":  "x86_64",
			"i686":    "x86",
			"aarch64": "aarch64",
			"armv7h":  "armv7h",
			"armv6h":  "armv6h",
			"arm":     "armhf",
			"any":     "all",
		},
		DEB: map[string]string{
			"x86_64":  "amd64",
			"i686":    "i386",
			"aarch64": "arm64",
			"armv6":   "armel",
			"armv6h":  "armel",
			"arm":     "armel",
			"armv7":   "armhf",
			"armv7h":  "armhf",
			"any":     "all",
		},
		RPM: map[string]string{
			"x86_64":  "x86_64",
			"i686":    "i686",
			"aarch64": "aarch64",
			"armv7h":  "armv7h",
			"armv6h":  "armv6h",
			"arm":     "arm",
			"any":     "noarch",
		},
		Pacman: map[string]string{
			"x86_64":  "x86_64",
			"i686":    "i686",
			"aarch64": "aarch64",
			"arm":     "arm",
			"armv6h":  "armv6h",
			"armv7h":  "armv7h",
			"any":     "any",
		},
	}
}

// TranslateArch translates an architecture for a specific package format.
func (am *ArchitectureMapping) TranslateArch(format, arch string) string {
	var mapping map[string]string

	switch format {
	case "apk":
		mapping = am.APK
	case "deb":
		mapping = am.DEB
	case "rpm":
		mapping = am.RPM
	case "pacman":
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
		"amd64":  ArchX86_64,
		"x86-64": ArchX86_64,
		"x64":    ArchX86_64,

		// i686 aliases
		"i386": ArchI686,
		"i586": ArchI686,
		"x86":  ArchI686,
		"i486": ArchI686,
		"ia32": ArchI686,

		// aarch64 aliases
		"arm64": ArchAarch64,

		// armv7 aliases
		"armv7h":  ArchArmv7,
		"armv7l":  ArchArmv7,
		"armhf":   ArchArmv7,
		"armv7hl": ArchArmv7,

		// armv6 aliases
		"armv6h": ArchArmv6,
		"armv6l": ArchArmv6,
		"arm":    ArchArmv6,
		"armel":  ArchArmv6,

		// ppc64le aliases
		"powerpc64le": ArchPpc64le,
		"ppc64el":     ArchPpc64le,

		// riscv64 aliases
		"riscv64gc": ArchRiscv64,
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
	case "apk":
		forwardMap = archMapping.APK
	case "deb":
		forwardMap = archMapping.DEB
	case "rpm":
		forwardMap = archMapping.RPM
	case "pacman":
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
		"any":       true, // "any" is also considered canonical
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
