package command

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRootCommand(t *testing.T) {
	tests := []struct {
		name        string
		args        []string
		expectError bool
	}{
		{
			name:        "Root command help",
			args:        []string{"--help"},
			expectError: false,
		},
		{
			name:        "Root command version",
			args:        []string{"version"},
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Save original args
			originalArgs := os.Args

			defer func() { os.Args = originalArgs }()

			// Set test args
			os.Args = append([]string{"yap"}, tt.args...)

			// Test should not panic
			assert.NotPanics(t, func() {
				// Create a copy of rootCmd for testing
				testCmd := rootCmd
				testCmd.SetArgs(tt.args)
				_ = testCmd.Execute()
			})
		})
	}
}

func TestGetLongDescription(t *testing.T) {
	description := getLongDescription()

	// Check that description contains expected content
	assert.Contains(t, description, "Yet Another Packager")
	assert.Contains(t, description, "YAP (Yet Another Packager)")
}

func TestIsNoColorEnabled(t *testing.T) {
	// Test default value
	result := IsNoColorEnabled()
	assert.False(t, result) // Should be false by default

	// Test with no-color flag set
	noColor = true
	result = IsNoColorEnabled()
	assert.True(t, result)

	// Reset for other tests
	noColor = false
}
