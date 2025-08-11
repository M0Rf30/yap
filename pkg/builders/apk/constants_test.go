package apk

import (
	"testing"
)

func TestAPKArchs(t *testing.T) {
	if APKArchs == nil {
		t.Fatal("APKArchs is nil")
	}

	expectedArchs := map[string]string{
		"x86_64":  "x86_64",
		"i686":    "x86",
		"aarch64": "aarch64",
		"armv7h":  "armv7h",
		"armv6h":  "armv6h",
		"any":     "all",
	}

	for arch, expected := range expectedArchs {
		if actual, exists := APKArchs[arch]; !exists {
			t.Errorf("APKArchs missing arch: %s", arch)
		} else if actual != expected {
			t.Errorf("APKArchs[%s] = %s, want %s", arch, actual, expected)
		}
	}

	for arch, mapped := range APKArchs {
		if mapped == "" {
			t.Errorf("APKArchs[%s] is empty", arch)
		}
	}
}

func TestAPKArchsMapping(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"x86_64", "x86_64"},
		{"i686", "x86"},
		{"aarch64", "aarch64"},
		{"armv7h", "armv7h"},
		{"armv6h", "armv6h"},
		{"any", "all"},
	}

	for _, tt := range tests {
		t.Run("mapping_"+tt.input, func(t *testing.T) {
			if result, exists := APKArchs[tt.input]; !exists {
				t.Errorf("APKArchs[%s] does not exist", tt.input)
			} else if result != tt.expected {
				t.Errorf("APKArchs[%s] = %s, want %s", tt.input, result, tt.expected)
			}
		})
	}
}

func TestAPKArchsConsistency(t *testing.T) {
	commonArchs := []string{"x86_64", "aarch64", "any"}

	for _, arch := range commonArchs {
		if _, exists := APKArchs[arch]; !exists {
			t.Errorf("APKArchs missing common arch: %s", arch)
		}
	}

	if len(APKArchs) == 0 {
		t.Error("APKArchs should not be empty")
	}
}
