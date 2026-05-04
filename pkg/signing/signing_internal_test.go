package signing

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Tests for internal functions that need package-level access
// These tests are in the signing package (not signing_test) to access unexported functions

func TestResolveKeyPathPriority(t *testing.T) {
	// Create temporary key files for testing
	tmpDir := t.TempDir()
	cliKeyPath := filepath.Join(tmpDir, "cli.key")
	envKeyPath := filepath.Join(tmpDir, "env.key")
	configKeyPath := filepath.Join(tmpDir, "config.key")

	// Create the key files
	for _, path := range []string{cliKeyPath, envKeyPath, configKeyPath} {
		if err := os.WriteFile(path, []byte("test key"), 0o600); err != nil {
			t.Fatalf("Failed to create test key file: %v", err)
		}
	}

	// Test CLI flag takes priority
	keyPath, err := resolveKeyPath(FormatDEB, cliKeyPath, configKeyPath)
	require.NoError(t, err)
	assert.Equal(t, cliKeyPath, keyPath)

	// Test env var used when no CLI flag
	keyPath, err = resolveKeyPath(FormatDEB, "", configKeyPath)
	require.NoError(t, err)
	// Should resolve to config key since no env var is set
	assert.Equal(t, configKeyPath, keyPath)
}

func TestResolvePassphrasePriority(t *testing.T) {
	// Test CLI flag takes priority
	pass := resolvePassphrase(FormatDEB, "cli-pass", "config-pass")
	assert.Equal(t, "cli-pass", pass)

	// Test config used when no CLI flag
	pass = resolvePassphrase(FormatDEB, "", "config-pass")
	assert.Equal(t, "config-pass", pass)

	// Test empty when neither provided
	pass = resolvePassphrase(FormatDEB, "", "")
	assert.Equal(t, "", pass)
}

func TestAlgorithmForFormat(t *testing.T) {
	tests := []struct {
		format    Format
		algorithm Algorithm
	}{
		{FormatAPK, AlgorithmRSA},
		{FormatDEB, AlgorithmGPG},
		{FormatRPM, AlgorithmGPG},
		{FormatPacman, AlgorithmGPG},
	}

	for _, tt := range tests {
		t.Run(string(tt.format), func(t *testing.T) {
			algo := algorithmForFormat(tt.format)
			assert.Equal(t, tt.algorithm, algo)
		})
	}
}
