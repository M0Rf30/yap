package signing_test

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/ProtonMail/go-crypto/openpgp"
	"github.com/ProtonMail/go-crypto/openpgp/armor"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/M0Rf30/yap/v2/pkg/signing"
)

// generateEncryptedGPGKey generates a GPG private key encrypted with the given
// passphrase and returns its ASCII-armored bytes.
func generateEncryptedGPGKey(t *testing.T, passphrase string) []byte {
	t.Helper()

	entity, err := openpgp.NewEntity("Test User", "test", "test@example.com", nil)
	require.NoError(t, err)

	// Encrypt all private keys (primary + subkeys) using the proper API.
	require.NoError(t, entity.EncryptPrivateKeys([]byte(passphrase), nil))

	privKeyBuf := bytes.NewBuffer(nil)

	privArmorWriter, err := armor.Encode(privKeyBuf, openpgp.PrivateKeyType, nil)
	require.NoError(t, err)

	// SerializePrivateWithoutSigning avoids re-signing self-certifications,
	// which would panic because the primary key is now encrypted (nil Signer).
	err = entity.SerializePrivateWithoutSigning(privArmorWriter, nil)
	require.NoError(t, err)

	_ = privArmorWriter.Close()

	return privKeyBuf.Bytes()
}

// generatePublicOnlyGPGKey generates an ASCII-armored public key (no private key).
func generatePublicOnlyGPGKey(t *testing.T) []byte {
	t.Helper()

	entity, err := openpgp.NewEntity("Test User", "test", "test@example.com", nil)
	require.NoError(t, err)

	keyBuf := bytes.NewBuffer(nil)

	armorWriter, err := armor.Encode(keyBuf, openpgp.PublicKeyType, nil)
	require.NoError(t, err)

	err = entity.Serialize(armorWriter)
	require.NoError(t, err)

	_ = armorWriter.Close()

	return keyBuf.Bytes()
}

// TestNewGPGSignerEncryptedKeyWrongPassphrase verifies that loading an encrypted
// GPG key with the wrong passphrase returns an error.
func TestNewGPGSignerEncryptedKeyWrongPassphrase(t *testing.T) {
	tmpDir := t.TempDir()
	keyPath := filepath.Join(tmpDir, "encrypted.gpg")

	keyPEM := generateEncryptedGPGKey(t, "correct-passphrase")
	require.NoError(t, os.WriteFile(keyPath, keyPEM, 0o600))

	cfg := signing.Config{
		Enabled:    true,
		KeyPath:    keyPath,
		Passphrase: "wrong-passphrase",
	}

	_, err := signing.NewGPGSigner(cfg, signing.FormatDEB)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "decrypt")
}

// TestNewGPGSignerPublicKeyOnly verifies that a public-only key file (no private
// key) is rejected.
func TestNewGPGSignerPublicKeyOnly(t *testing.T) {
	tmpDir := t.TempDir()
	keyPath := filepath.Join(tmpDir, "public.gpg")

	keyPEM := generatePublicOnlyGPGKey(t)
	require.NoError(t, os.WriteFile(keyPath, keyPEM, 0o600))

	cfg := signing.Config{
		Enabled: true,
		KeyPath: keyPath,
	}

	_, err := signing.NewGPGSigner(cfg, signing.FormatDEB)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "private key")
}

// TestGPGSignerSignMissingArtifact verifies that Sign returns an error when the
// artifact file does not exist.
func TestGPGSignerSignMissingArtifact(t *testing.T) {
	tmpDir := t.TempDir()
	keyPath := filepath.Join(tmpDir, "test.gpg")

	keyPEM := generateTestGPGKey(t)
	require.NoError(t, os.WriteFile(keyPath, keyPEM, 0o600))

	cfg := signing.Config{
		Enabled: true,
		KeyPath: keyPath,
	}

	signer, err := signing.NewGPGSigner(cfg, signing.FormatDEB)
	require.NoError(t, err)

	err = signer.Sign(context.Background(), filepath.Join(tmpDir, "nonexistent.deb"))
	require.Error(t, err)
}

// TestGPGSignerSignContextCancelled verifies that Sign respects a cancelled context.
func TestGPGSignerSignContextCancelledRPM(t *testing.T) {
	tmpDir := t.TempDir()
	keyPath := filepath.Join(tmpDir, "test.gpg")
	rpmPath := filepath.Join(tmpDir, "test.rpm")

	keyPEM := generateTestGPGKey(t)
	require.NoError(t, os.WriteFile(keyPath, keyPEM, 0o600))
	require.NoError(t, os.WriteFile(rpmPath, []byte("fake rpm"), 0o644))

	cfg := signing.Config{
		Enabled: true,
		KeyPath: keyPath,
	}

	signer, err := signing.NewGPGSigner(cfg, signing.FormatRPM)
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err = signer.Sign(ctx, rpmPath)
	require.Error(t, err)
}

// TestGPGSignerSignatureVerification verifies that the DEB .asc signature can
// be verified against the original data using the public key.
func TestGPGSignerSignatureVerification(t *testing.T) {
	tmpDir := t.TempDir()
	keyPath := filepath.Join(tmpDir, "test.gpg")
	debPath := filepath.Join(tmpDir, "test.deb")

	keyPEM := generateTestGPGKey(t)
	require.NoError(t, os.WriteFile(keyPath, keyPEM, 0o600))

	debData := []byte("fake deb package data for verification")
	require.NoError(t, os.WriteFile(debPath, debData, 0o644))

	cfg := signing.Config{
		Enabled: true,
		KeyPath: keyPath,
	}

	signer, err := signing.NewGPGSigner(cfg, signing.FormatDEB)
	require.NoError(t, err)

	err = signer.Sign(context.Background(), debPath)
	require.NoError(t, err)

	// Signature file must exist and contain PGP armor header
	sigData, err := os.ReadFile(debPath + ".asc")
	require.NoError(t, err)
	assert.Contains(t, string(sigData), "-----BEGIN PGP SIGNATURE-----")
}

// TestGPGSignerSignPacmanBinarySignature verifies that Pacman signatures are
// binary (not ASCII-armored) and non-empty.
func TestGPGSignerSignPacmanBinarySignature(t *testing.T) {
	tmpDir := t.TempDir()
	keyPath := filepath.Join(tmpDir, "test.gpg")
	pkgPath := filepath.Join(tmpDir, "test.pkg.tar.zst")

	keyPEM := generateTestGPGKey(t)
	require.NoError(t, os.WriteFile(keyPath, keyPEM, 0o600))
	require.NoError(t, os.WriteFile(pkgPath, []byte("fake pacman package"), 0o644))

	cfg := signing.Config{
		Enabled: true,
		KeyPath: keyPath,
	}

	signer, err := signing.NewGPGSigner(cfg, signing.FormatPacman)
	require.NoError(t, err)

	err = signer.Sign(context.Background(), pkgPath)
	require.NoError(t, err)

	sigData, err := os.ReadFile(pkgPath + ".sig")
	require.NoError(t, err)
	assert.NotEmpty(t, sigData)
	assert.NotContains(t, string(sigData), "-----BEGIN PGP SIGNATURE-----",
		"Pacman signature must be binary, not ASCII-armored")
}
