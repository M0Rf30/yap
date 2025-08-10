// Package constants provides centralized constants and mappings for all package formats.
package constants

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
			"arm":     "armel",
			"armv6h":  "armel",
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
