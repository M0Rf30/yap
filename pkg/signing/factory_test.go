package signing_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/M0Rf30/yap/v2/pkg/signing"
)

// TestNewSignerUnknownFormat verifies that an unsupported format returns an error.
func TestNewSignerUnknownFormat(t *testing.T) {
	cfg := signing.Config{
		Enabled: true,
		KeyPath: "/some/key.gpg",
	}

	_, err := signing.NewSigner("unknown_format", cfg)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported format")
}

// TestNewSignerEnabledEmptyKeyPath verifies that an empty KeyPath with signing
// enabled returns a configuration error before any format dispatch.
func TestNewSignerEnabledEmptyKeyPath(t *testing.T) {
	cfg := signing.Config{
		Enabled: true,
		KeyPath: "",
	}

	_, err := signing.NewSigner(signing.FormatDEB, cfg)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no key path")
}

// TestNewSignerAPKFormat verifies that FormatAPK routes to an RSASigner.
func TestNewSignerAPKFormat(t *testing.T) {
	tmpDir := t.TempDir()
	keyPath := filepath.Join(tmpDir, "test.key")

	keyPEM := generateTestRSAKey(t)
	require.NoError(t, os.WriteFile(keyPath, keyPEM, 0o600))

	cfg := signing.Config{
		Enabled: true,
		KeyPath: keyPath,
		KeyName: "testkey",
	}

	signer, err := signing.NewSigner(signing.FormatAPK, cfg)
	require.NoError(t, err)
	require.NotNil(t, signer)
}

// TestNewSignerGPGFormats verifies that DEB/RPM/Pacman formats route to a GPGSigner.
func TestNewSignerGPGFormats(t *testing.T) {
	tmpDir := t.TempDir()
	keyPath := filepath.Join(tmpDir, "test.gpg")

	keyPEM := generateTestGPGKey(t)
	require.NoError(t, os.WriteFile(keyPath, keyPEM, 0o600))

	cfg := signing.Config{
		Enabled: true,
		KeyPath: keyPath,
	}

	for _, format := range []signing.Format{signing.FormatDEB, signing.FormatRPM, signing.FormatPacman} {
		t.Run(string(format), func(t *testing.T) {
			signer, err := signing.NewSigner(format, cfg)
			require.NoError(t, err)
			require.NotNil(t, signer)
		})
	}
}

// TestNewSignerDisabledIgnoresFormat verifies that a disabled config always
// returns a NoopSigner regardless of format or key path.
func TestNewSignerDisabledIgnoresFormat(t *testing.T) {
	for _, format := range []signing.Format{
		signing.FormatAPK, signing.FormatDEB, signing.FormatRPM, signing.FormatPacman, "unknown",
	} {
		t.Run(string(format), func(t *testing.T) {
			cfg := signing.Config{Enabled: false}
			signer, err := signing.NewSigner(format, cfg)
			require.NoError(t, err)
			require.NotNil(t, signer)
		})
	}
}
