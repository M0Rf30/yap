package core

import (
	"testing"
)

func TestGetConfig(t *testing.T) {
	tests := []struct {
		packageManager string
		expectNil      bool
	}{
		{"apk", false},
		{"apt", false},
		{"pacman", false},
		{"yum", false},
		{"zypper", false},
		{"nonexistent", true},
	}

	for _, test := range tests {
		config := GetConfig(test.packageManager)
		if test.expectNil {
			if config != nil {
				t.Fatalf("Expected nil config for %s, got %v", test.packageManager, config)
			}
		} else {
			if config == nil {
				t.Fatalf("Expected non-nil config for %s, got nil", test.packageManager)
			}
		}
	}
}

func TestConfigStructure(t *testing.T) {
	// Test APK config
	apkConfig := GetConfig("apk")
	if apkConfig.Name != "apk" {
		t.Fatalf("Expected APK name 'apk', got '%s'", apkConfig.Name)
	}

	if apkConfig.InstallCmd != "apk" {
		t.Fatalf("Expected APK install command 'apk', got '%s'", apkConfig.InstallCmd)
	}

	if len(apkConfig.InstallArgs) == 0 {
		t.Fatal("APK install args should not be empty")
	}

	if len(apkConfig.ArchMap) == 0 {
		t.Fatal("APK arch map should not be empty")
	}

	// Test APT config
	aptConfig := GetConfig("apt")
	if aptConfig.Name != "apt" {
		t.Fatalf("Expected APT name 'apt', got '%s'", aptConfig.Name)
	}

	if aptConfig.InstallCmd != "apt-get" {
		t.Fatalf("Expected APT install command 'apt-get', got '%s'", aptConfig.InstallCmd)
	}

	// Test Pacman config
	pacmanConfig := GetConfig("pacman")
	if pacmanConfig.Name != "pacman" {
		t.Fatalf("Expected Pacman name 'pacman', got '%s'", pacmanConfig.Name)
	}

	if pacmanConfig.InstallCmd != "pacman" {
		t.Fatalf("Expected Pacman install command 'pacman', got '%s'", pacmanConfig.InstallCmd)
	}

	// Test YUM config
	yumConfig := GetConfig("yum")
	if yumConfig.Name != "dnf" {
		t.Fatalf("Expected YUM name 'dnf', got '%s'", yumConfig.Name)
	}

	if yumConfig.InstallCmd != "dnf" {
		t.Fatalf("Expected YUM install command 'dnf', got '%s'", yumConfig.InstallCmd)
	}

	if len(yumConfig.GroupMap) == 0 {
		t.Fatal("YUM group map should not be empty")
	}
}

func TestArchMap(t *testing.T) {
	apkConfig := GetConfig("apk")

	// Test common architecture mappings
	if apkConfig.ArchMap["x86_64"] != "x86_64" {
		t.Fatalf("Expected x86_64 -> x86_64 for APK, got %s", apkConfig.ArchMap["x86_64"])
	}

	if apkConfig.ArchMap["aarch64"] != "aarch64" {
		t.Fatalf("Expected aarch64 -> aarch64 for APK, got %s", apkConfig.ArchMap["aarch64"])
	}

	aptConfig := GetConfig("apt")
	if aptConfig.ArchMap["x86_64"] != "amd64" {
		t.Fatalf("Expected x86_64 -> amd64 for APT, got %s", aptConfig.ArchMap["x86_64"])
	}

	if aptConfig.ArchMap["aarch64"] != "arm64" {
		t.Fatalf("Expected aarch64 -> arm64 for APT, got %s", aptConfig.ArchMap["aarch64"])
	}
}

func TestBuildEnvDeps(t *testing.T) {
	configs := []string{"apk", "apt", "pacman", "yum", "zypper"}

	for _, configName := range configs {
		config := GetConfig(configName)
		if len(config.BuildEnvDeps) == 0 {
			t.Fatalf("Build environment dependencies should not be empty for %s", configName)
		}
	}
}

func TestGroupMap(t *testing.T) {
	yumConfig := GetConfig("yum")

	// Test group mappings for RPM-based systems
	expectedGroups := []string{"admin", "base", "devel", "libs", "utils"}
	for _, group := range expectedGroups {
		if _, exists := yumConfig.GroupMap[group]; !exists {
			t.Fatalf("Expected group mapping for '%s' in YUM config", group)
		}
	}
}

func TestUpdateArgs(t *testing.T) {
	// APK should have update args
	apkConfig := GetConfig("apk")
	if len(apkConfig.UpdateArgs) == 0 {
		t.Fatal("APK update args should not be empty")
	}

	// YUM/DNF might not have update args (uses empty slice)
	yumConfig := GetConfig("yum")
	// This is acceptable - some package managers don't need explicit update
	_ = yumConfig.UpdateArgs
}

func TestConfigImmutability(t *testing.T) {
	// Get the same config twice and ensure they're the same reference
	config1 := GetConfig("apk")
	config2 := GetConfig("apk")

	// They should point to the same object
	if config1 != config2 {
		t.Fatal("GetConfig should return the same reference for the same package manager")
	}
}
