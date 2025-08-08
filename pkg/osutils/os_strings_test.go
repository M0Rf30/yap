package osutils

import (
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestContains(t *testing.T) {
	tests := []struct {
		name     string
		array    []string
		str      string
		expected bool
	}{
		{
			name:     "String found in array",
			array:    []string{"apple", "banana", "cherry"},
			str:      "banana",
			expected: true,
		},
		{
			name:     "String not found in array",
			array:    []string{"apple", "banana", "cherry"},
			str:      "orange",
			expected: false,
		},
		{
			name:     "Empty array",
			array:    []string{},
			str:      "test",
			expected: false,
		},
		{
			name:     "Empty string in array with empty string",
			array:    []string{"", "test"},
			str:      "",
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := Contains(tt.array, tt.str)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestGetArchitecture(t *testing.T) {
	// Test that GetArchitecture returns a non-empty string
	result := GetArchitecture()
	assert.NotEmpty(t, result)

	// Test some known mappings based on current runtime
	switch runtime.GOARCH {
	case "amd64":
		assert.Equal(t, "x86_64", result)
	case "386":
		assert.Equal(t, "i686", result)
	case "arm":
		assert.Equal(t, "armv7h", result)
	case "arm64":
		assert.Equal(t, "aarch64", result)
	default:
		// For other architectures, just ensure it's not empty
		assert.NotEmpty(t, result)
	}
}

func TestParseOSRelease(t *testing.T) {
	// Test that ParseOSRelease doesn't panic
	// Note: This may fail if /etc/os-release doesn't exist, but that's expected
	assert.NotPanics(t, func() {
		_, _ = ParseOSRelease()
	})
}

func TestOSReleaseStruct(t *testing.T) {
	// Test OSRelease struct creation
	osrel := OSRelease{ID: "ubuntu"}
	assert.Equal(t, "ubuntu", osrel.ID)
}
