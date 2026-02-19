package constants

import (
	"slices"
	"testing"
)

func TestFormatConstants(t *testing.T) {
	tests := []struct {
		name     string
		format   string
		expected string
	}{
		{"APK format", FormatAPK, "apk"},
		{"DEB format", FormatDEB, "deb"},
		{"RPM format", FormatRPM, "rpm"},
		{"Pacman format", FormatPacman, "pacman"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.format != tt.expected {
				t.Errorf("%s = %q, want %q", tt.name, tt.format, tt.expected)
			}
		})
	}
}

func TestRPMGroups(t *testing.T) {
	if RPMGroups == nil {
		t.Fatal("RPMGroups is nil")
	}

	expectedGroups := []string{
		"admin", "devel", "libs", "games", "graphics", "net", "text", "web",
	}

	for _, group := range expectedGroups {
		if _, exists := RPMGroups[group]; !exists {
			t.Errorf("RPMGroups missing expected group: %s", group)
		}
	}

	for group, rpmGroup := range RPMGroups {
		if rpmGroup == "" {
			t.Errorf("RPMGroups[%s] is empty", group)
		}
	}
}

func TestRPMDistros(t *testing.T) {
	if RPMDistros == nil {
		t.Fatal("RPMDistros is nil")
	}

	expectedDistros := []string{
		"almalinux", "fedora", "rhel", "rocky",
	}

	for _, distro := range expectedDistros {
		if _, exists := RPMDistros[distro]; !exists {
			t.Errorf("RPMDistros missing expected distro: %s", distro)
		}
	}

	for distro, suffix := range RPMDistros {
		if suffix == "" {
			t.Errorf("RPMDistros[%s] is empty", distro)
		}

		if suffix[0] != '.' {
			t.Errorf("RPMDistros[%s] suffix %q should start with '.'", distro, suffix)
		}
	}
}

func TestGetBuildDeps(t *testing.T) {
	deps := GetBuildDeps()

	if deps == nil {
		t.Fatal("GetBuildDeps() returned nil")
	}

	if len(deps.APK) == 0 {
		t.Error("APK build deps is empty")
	}

	if len(deps.DEB) == 0 {
		t.Error("DEB build deps is empty")
	}

	if len(deps.RPM) == 0 {
		t.Error("RPM build deps is empty")
	}

	if len(deps.Pacman) == 0 {
		t.Error("Pacman build deps is empty")
	}

	if !contains(deps.APK, "alpine-sdk") {
		t.Error("APK deps should contain alpine-sdk")
	}

	if !contains(deps.DEB, "build-essential") {
		t.Error("DEB deps should contain build-essential")
	}

	if !contains(deps.RPM, "gcc") {
		t.Error("RPM deps should contain gcc")
	}

	if !contains(deps.Pacman, "base-devel") {
		t.Error("Pacman deps should contain base-devel")
	}
}

func TestGetInstallArgs(t *testing.T) {
	tests := []struct {
		name     string
		format   string
		expected []string
	}{
		{"APK install args", FormatAPK, []string{"add", "--allow-untrusted"}},
		{"DEB install args", FormatDEB, []string{"--allow-downgrades", "--assume-yes", "install"}},
		{"RPM install args", FormatRPM, []string{"-y", "install"}},
		{"Pacman install args", FormatPacman, []string{"-S", "--noconfirm", "--needed"}},
		{"Unknown format", "unknown", []string{}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := GetInstallArgs(tt.format)
			if !equalSlices(result, tt.expected) {
				t.Errorf("GetInstallArgs(%q) = %v, want %v", tt.format, result, tt.expected)
			}
		})
	}
}

func TestBuildDepsConsistency(t *testing.T) {
	deps := GetBuildDeps()

	if len(deps.RPM) < len(deps.DEB) {
		t.Error("RPM should have more build deps than DEB (it typically requires more packages)")
	}

	for _, depList := range [][]string{deps.APK, deps.DEB, deps.RPM, deps.Pacman} {
		for _, dep := range depList {
			if dep == "" {
				t.Error("Build dependency list contains empty string")
			}
		}
	}
}

func TestDistroFormat(t *testing.T) {
	tests := []struct {
		distro string
		want   string
	}{
		// APK
		{"alpine", FormatAPK},
		{"Alpine", FormatAPK}, // case-insensitive
		// DEB family
		{"ubuntu", FormatDEB},
		{"debian", FormatDEB},
		{"linuxmint", FormatDEB},
		{"pop", FormatDEB},
		// RPM family – canonical Releases names
		{"fedora", FormatRPM},
		{"rhel", FormatRPM},
		{"centos", FormatRPM},
		{"rocky", FormatRPM},
		{"almalinux", FormatRPM},
		{"amzn", FormatRPM},
		{"ol", FormatRPM},
		{"opensuse-leap", FormatRPM},
		{"opensuse-tumbleweed", FormatRPM},
		// RPM family – legacy/alternate names
		{"alma", FormatRPM},
		{"opensuse", FormatRPM},
		{"suse", FormatRPM},
		// Pacman
		{"arch", FormatPacman},
		// Unknown
		{"unknown", ""},
		{"", ""},
	}

	for _, tt := range tests {
		t.Run(tt.distro, func(t *testing.T) {
			got := DistroFormat(tt.distro)
			if got != tt.want {
				t.Errorf("DistroFormat(%q) = %q, want %q", tt.distro, got, tt.want)
			}
		})
	}
}

func contains(slice []string, item string) bool {
	return slices.Contains(slice, item)
}

func equalSlices(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}

	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}

	return true
}
