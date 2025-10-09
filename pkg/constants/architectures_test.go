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
