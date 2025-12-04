package constants

import (
	"testing"
)

func TestGetArchMapping(t *testing.T) {
	mapping := GetArchMapping()

	if mapping == nil {
		t.Fatal("GetArchMapping() returned nil")
	}

	if mapping.APK == nil {
		t.Error("APK mapping is nil")
	}

	if mapping.DEB == nil {
		t.Error("DEB mapping is nil")
	}

	if mapping.RPM == nil {
		t.Error("RPM mapping is nil")
	}

	if mapping.Pacman == nil {
		t.Error("Pacman mapping is nil")
	}
}

func TestArchitectureMapping_TranslateArch(t *testing.T) {
	mapping := GetArchMapping()

	tests := []struct {
		name     string
		format   string
		arch     string
		expected string
	}{
		{"APK x86_64", "apk", "x86_64", "x86_64"},
		{"APK i686", "apk", "i686", "x86"},
		{"APK aarch64", "apk", "aarch64", "aarch64"},
		{"APK any", "apk", "any", "all"},

		{"DEB x86_64", "deb", "x86_64", "amd64"},
		{"DEB i686", "deb", "i686", "i386"},
		{"DEB aarch64", "deb", "aarch64", "arm64"},
		{"DEB any", "deb", "any", "all"},

		{"RPM x86_64", "rpm", "x86_64", "x86_64"},
		{"RPM i686", "rpm", "i686", "i686"},
		{"RPM any", "rpm", "any", "noarch"},

		{"Pacman x86_64", "pacman", "x86_64", "x86_64"},
		{"Pacman any", "pacman", "any", "any"},

		{"Unknown format", "unknown", "x86_64", "x86_64"},
		{"Unknown arch", "deb", "unknown_arch", "unknown_arch"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := mapping.TranslateArch(tt.format, tt.arch)
			if result != tt.expected {
				t.Errorf("TranslateArch(%q, %q) = %q, want %q", tt.format, tt.arch, result, tt.expected)
			}
		})
	}
}

func TestArchMappingConsistency(t *testing.T) {
	mapping := GetArchMapping()

	commonArchs := []string{"x86_64", "i686", "aarch64", "any"}

	for _, arch := range commonArchs {
		t.Run("arch_"+arch, func(t *testing.T) {
			apkResult := mapping.TranslateArch("apk", arch)
			debResult := mapping.TranslateArch("deb", arch)
			rpmResult := mapping.TranslateArch("rpm", arch)
			pacmanResult := mapping.TranslateArch("pacman", arch)

			if apkResult == "" || debResult == "" || rpmResult == "" || pacmanResult == "" {
				t.Errorf("Empty translation for arch %s: APK=%s, DEB=%s, RPM=%s, Pacman=%s",
					arch, apkResult, debResult, rpmResult, pacmanResult)
			}
		})
	}
}

func TestArchMappingKeys(t *testing.T) {
	mapping := GetArchMapping()

	expectedKeys := []string{"x86_64", "i686", "aarch64", "arm", "armv6h", "armv7h", "any"}

	checkKeys := func(t *testing.T, name string, m map[string]string) {
		t.Helper()

		for _, key := range expectedKeys {
			if _, exists := m[key]; !exists {
				t.Errorf("%s mapping missing key: %s", name, key)
			}
		}
	}

	checkKeys(t, "APK", mapping.APK)
	checkKeys(t, "DEB", mapping.DEB)
	checkKeys(t, "RPM", mapping.RPM)
	checkKeys(t, "Pacman", mapping.Pacman)
}

// ==================== Task 2.4: NormalizeArchitecture Tests ====================

// TestNormalizeArchitectureCanonical tests that canonical names are returned unchanged.
func TestNormalizeArchitectureCanonical(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		arch     string
		expected string
	}{
		{"x86_64 canonical", ArchX86_64, ArchX86_64},
		{"i686 canonical", ArchI686, ArchI686},
		{"aarch64 canonical", ArchAarch64, ArchAarch64},
		{"armv7 canonical", ArchArmv7, ArchArmv7},
		{"armv6 canonical", ArchArmv6, ArchArmv6},
		{"ppc64le canonical", ArchPpc64le, ArchPpc64le},
		{"s390x canonical", ArchS390x, ArchS390x},
		{"riscv64 canonical", ArchRiscv64, ArchRiscv64},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			result := NormalizeArchitecture(tt.arch)
			if result != tt.expected {
				t.Errorf("NormalizeArchitecture(%q) = %q, want %q", tt.arch, result, tt.expected)
			}
		})
	}
}

// TestNormalizeArchitectureX86_64Aliases tests x86_64 architecture aliases.
func TestNormalizeArchitectureX86_64Aliases(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		alias string
	}{
		{"amd64", "amd64"},
		{"x86-64", "x86-64"},
		{"x64", "x64"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			result := NormalizeArchitecture(tt.alias)
			if result != ArchX86_64 {
				t.Errorf("NormalizeArchitecture(%q) = %q, want %q", tt.alias, result, ArchX86_64)
			}
		})
	}
}

// TestNormalizeArchitectureI686Aliases tests i686 architecture aliases.
func TestNormalizeArchitectureI686Aliases(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		alias string
	}{
		{"i386", "i386"},
		{"i586", "i586"},
		{"x86", "x86"},
		{"i486", "i486"},
		{"ia32", "ia32"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			result := NormalizeArchitecture(tt.alias)
			if result != ArchI686 {
				t.Errorf("NormalizeArchitecture(%q) = %q, want %q", tt.alias, result, ArchI686)
			}
		})
	}
}

// TestNormalizeArchitectureAarch64Aliases tests aarch64 architecture aliases.
func TestNormalizeArchitectureAarch64Aliases(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		alias string
	}{
		{"arm64", "arm64"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			result := NormalizeArchitecture(tt.alias)
			if result != ArchAarch64 {
				t.Errorf("NormalizeArchitecture(%q) = %q, want %q", tt.alias, result, ArchAarch64)
			}
		})
	}
}

// TestNormalizeArchitectureArmv7Aliases tests armv7 architecture aliases.
func TestNormalizeArchitectureArmv7Aliases(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		alias string
	}{
		{"armv7h", "armv7h"},
		{"armv7l", "armv7l"},
		{"armhf", "armhf"},
		{"armv7hl", "armv7hl"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			result := NormalizeArchitecture(tt.alias)
			if result != ArchArmv7 {
				t.Errorf("NormalizeArchitecture(%q) = %q, want %q", tt.alias, result, ArchArmv7)
			}
		})
	}
}

// TestNormalizeArchitectureArmv6Aliases tests armv6 architecture aliases.
func TestNormalizeArchitectureArmv6Aliases(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		alias string
	}{
		{"armv6h", "armv6h"},
		{"armv6l", "armv6l"},
		{"arm", "arm"},
		{"armel", "armel"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			result := NormalizeArchitecture(tt.alias)
			if result != ArchArmv6 {
				t.Errorf("NormalizeArchitecture(%q) = %q, want %q", tt.alias, result, ArchArmv6)
			}
		})
	}
}

// TestNormalizeArchitecturePpc64leAliases tests ppc64le architecture aliases.
func TestNormalizeArchitecturePpc64leAliases(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		alias string
	}{
		{"powerpc64le", "powerpc64le"},
		{"ppc64el", "ppc64el"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			result := NormalizeArchitecture(tt.alias)
			if result != ArchPpc64le {
				t.Errorf("NormalizeArchitecture(%q) = %q, want %q", tt.alias, result, ArchPpc64le)
			}
		})
	}
}

// TestNormalizeArchitectureRiscv64Aliases tests riscv64 architecture aliases.
func TestNormalizeArchitectureRiscv64Aliases(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		alias string
	}{
		{"riscv64gc", "riscv64gc"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			result := NormalizeArchitecture(tt.alias)
			if result != ArchRiscv64 {
				t.Errorf("NormalizeArchitecture(%q) = %q, want %q", tt.alias, result, ArchRiscv64)
			}
		})
	}
}

// TestNormalizeArchitectureUnknown tests that unknown architectures are returned unchanged.
func TestNormalizeArchitectureUnknown(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		arch string
	}{
		{"unknown_arch", "unknown_arch"},
		{"mips", "mips"},
		{"sparc", "sparc"},
		{"alpha", "alpha"},
		{"empty", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			result := NormalizeArchitecture(tt.arch)
			if result != tt.arch {
				t.Errorf("NormalizeArchitecture(%q) = %q, want %q (unchanged)", tt.arch, result, tt.arch)
			}
		})
	}
}

// TestNormalizeArchitectureCompleteness tests that all common aliases are covered.
func TestNormalizeArchitectureCompleteness(t *testing.T) {
	t.Parallel()

	// Test all aliases map to one of the canonical names
	canonicalNames := map[string]bool{
		ArchX86_64:  true,
		ArchI686:    true,
		ArchAarch64: true,
		ArchArmv7:   true,
		ArchArmv6:   true,
		ArchPpc64le: true,
		ArchS390x:   true,
		ArchRiscv64: true,
	}

	allAliases := []string{
		// x86_64 aliases
		"amd64", "x86-64", "x64",
		// i686 aliases
		"i386", "i586", "x86", "i486", "ia32",
		// aarch64 aliases
		"arm64",
		// armv7 aliases
		"armv7h", "armv7l", "armhf", "armv7hl",
		// armv6 aliases
		"armv6h", "armv6l", "arm", "armel",
		// ppc64le aliases
		"powerpc64le", "ppc64el",
		// riscv64 aliases
		"riscv64gc",
	}

	for _, alias := range allAliases {
		t.Run("alias_"+alias, func(t *testing.T) {
			result := NormalizeArchitecture(alias)
			if !canonicalNames[result] {
				t.Errorf("NormalizeArchitecture(%q) = %q, not a canonical name", alias, result)
			}
		})
	}
}

// TestGetReverseMappingAPK tests reverse mapping for APK format.
func TestGetReverseMappingAPK(t *testing.T) {
	t.Parallel()

	reverseMap := GetReverseMapping("apk")

	tests := []struct {
		formatSpecific string
		canonical      string
	}{
		{"x86_64", "x86_64"},
		{"x86", "i686"},
		{"aarch64", "aarch64"},
		{"armv7h", "armv7h"},
		{"armv6h", "armv6h"},
		{"armhf", "arm"},
		{"all", "any"},
	}

	for _, tt := range tests {
		t.Run(tt.formatSpecific, func(t *testing.T) {
			result, exists := reverseMap[tt.formatSpecific]
			if !exists {
				t.Errorf("GetReverseMapping('apk')[%q] does not exist", tt.formatSpecific)
				return
			}

			if result != tt.canonical {
				t.Errorf("GetReverseMapping('apk')[%q] = %q, want %q", tt.formatSpecific, result, tt.canonical)
			}
		})
	}
}

// TestGetReverseMappingDEB tests reverse mapping for DEB format.
func TestGetReverseMappingDEB(t *testing.T) {
	t.Parallel()

	reverseMap := GetReverseMapping("deb")

	tests := []struct {
		formatSpecific string
		canonical      string
	}{
		{"amd64", "x86_64"},
		{"i386", "i686"},
		{"arm64", "aarch64"},
		{"armhf", "armv7"},
		{"armel", "armv6"},
		{"all", "any"},
	}

	for _, tt := range tests {
		t.Run(tt.formatSpecific, func(t *testing.T) {
			result, exists := reverseMap[tt.formatSpecific]
			if !exists {
				t.Errorf("GetReverseMapping('deb')[%q] does not exist", tt.formatSpecific)
				return
			}

			if result != tt.canonical {
				t.Errorf("GetReverseMapping('deb')[%q] = %q, want %q", tt.formatSpecific, result, tt.canonical)
			}
		})
	}
}

// TestGetReverseMappingRPM tests reverse mapping for RPM format.
func TestGetReverseMappingRPM(t *testing.T) {
	t.Parallel()

	reverseMap := GetReverseMapping("rpm")

	tests := []struct {
		formatSpecific string
		canonical      string
	}{
		{"x86_64", "x86_64"},
		{"i686", "i686"},
		{"aarch64", "aarch64"},
		{"armv7h", "armv7h"},
		{"noarch", "any"},
	}

	for _, tt := range tests {
		t.Run(tt.formatSpecific, func(t *testing.T) {
			result, exists := reverseMap[tt.formatSpecific]
			if !exists {
				t.Errorf("GetReverseMapping('rpm')[%q] does not exist", tt.formatSpecific)
				return
			}

			if result != tt.canonical {
				t.Errorf("GetReverseMapping('rpm')[%q] = %q, want %q", tt.formatSpecific, result, tt.canonical)
			}
		})
	}
}

// TestGetReverseMappingPacman tests reverse mapping for Pacman format.
func TestGetReverseMappingPacman(t *testing.T) {
	t.Parallel()

	reverseMap := GetReverseMapping("pacman")

	tests := []struct {
		formatSpecific string
		canonical      string
	}{
		{"x86_64", "x86_64"},
		{"i686", "i686"},
		{"aarch64", "aarch64"},
		{"armv7h", "armv7h"},
		{"armv6h", "armv6h"},
		{"any", "any"},
	}

	for _, tt := range tests {
		t.Run(tt.formatSpecific, func(t *testing.T) {
			result, exists := reverseMap[tt.formatSpecific]
			if !exists {
				t.Errorf("GetReverseMapping('pacman')[%q] does not exist", tt.formatSpecific)
				return
			}

			if result != tt.canonical {
				t.Errorf("GetReverseMapping('pacman')[%q] = %q, want %q", tt.formatSpecific, result, tt.canonical)
			}
		})
	}
}

// TestGetReverseMappingUnknownFormat tests reverse mapping for unknown format.
func TestGetReverseMappingUnknownFormat(t *testing.T) {
	t.Parallel()

	reverseMap := GetReverseMapping("unknown")

	if len(reverseMap) != 0 {
		t.Errorf("GetReverseMapping('unknown') should return empty map, got %d entries", len(reverseMap))
	}
}

// TestNormalizeAndTranslateRoundTrip tests round-trip normalization and translation.
func TestNormalizeAndTranslateRoundTrip(t *testing.T) {
	t.Parallel()

	archMapping := GetArchMapping()

	tests := []struct {
		name           string
		inputAlias     string
		format         string
		expectedOutput string
	}{
		{"amd64 to DEB", "amd64", "deb", "amd64"},
		{"arm64 to DEB", "arm64", "deb", "arm64"},
		{"i386 to APK", "i386", "apk", "x86"},
		{"armhf to DEB", "armhf", "deb", "armhf"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Normalize alias to canonical
			canonical := NormalizeArchitecture(tt.inputAlias)
			// Translate canonical to format-specific
			formatSpecific := archMapping.TranslateArch(tt.format, canonical)

			if formatSpecific != tt.expectedOutput {
				t.Errorf("Normalize(%q) -> %q -> TranslateArch(%q) = %q, want %q",
					tt.inputAlias, canonical, tt.format, formatSpecific, tt.expectedOutput)
			}
		})
	}
}
