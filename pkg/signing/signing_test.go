package signing_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/M0Rf30/yap/v2/pkg/signing"
)

func TestNewSignerDisabled(t *testing.T) {
	cfg := signing.Config{
		Enabled: false,
		KeyPath: "",
	}

	signer, err := signing.NewSigner(signing.FormatDEB, cfg)
	require.NoError(t, err)
	require.NotNil(t, signer)

	// Noop signer should not error on Sign
	ctx := context.Background()
	err = signer.Sign(ctx, "/tmp/artifact")
	assert.NoError(t, err)
}

func TestNewSignerEnabled(t *testing.T) {
	tmpDir := t.TempDir()
	keyPath := filepath.Join(tmpDir, "test.gpg")

	// Create a minimal test key file
	if err := os.WriteFile(keyPath, []byte("-----BEGIN PGP PRIVATE KEY BLOCK-----\n\n-----END PGP PRIVATE KEY BLOCK-----"), 0o600); err != nil {
		t.Fatalf("Failed to create test key file: %v", err)
	}

	cfg := signing.Config{
		Enabled: true,
		KeyPath: keyPath,
	}

	// This will fail because the key is not valid, but it tests the path
	_, err := signing.NewSigner(signing.FormatDEB, cfg)
	assert.Error(t, err) // Expected to fail with invalid key
}

func TestNewSignerEnabledNoKey(t *testing.T) {
	cfg := signing.Config{
		Enabled: true,
		KeyPath: "/nonexistent/key.gpg",
	}

	_, err := signing.NewSigner(signing.FormatDEB, cfg)
	assert.Error(t, err) // Expected to fail with missing key
}
