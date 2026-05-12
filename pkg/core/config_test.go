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

func TestConfigImmutability(t *testing.T) {
	// Get the same config twice and ensure they're the same reference
	config1 := GetConfig("apk")
	config2 := GetConfig("apk")

	// They should point to the same object
	if config1 != config2 {
		t.Fatal("GetConfig should return the same reference for the same package manager")
	}
}
