// SPDX-FileCopyrightText: 2021-2026 Gianluca Boiano
// SPDX-License-Identifier: GPL-3.0-only

package signing

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/ProtonMail/go-crypto/openpgp"
	"github.com/ProtonMail/go-crypto/openpgp/armor"
	"github.com/stretchr/testify/require"
)

// generateTestGPGKey generates a test GPG key and returns both the entity
// and its ASCII-armored bytes.
func generateTestGPGKey(t *testing.T) (entity *openpgp.Entity, armored []byte) {
	t.Helper()

	var err error

	entity, err = openpgp.NewEntity("Test User", "test", "test@example.com", nil)
	if err != nil {
		t.Fatalf("Failed to generate GPG key: %v", err)
	}

	// Encode to ASCII-armored format
	keyBuf := bytes.NewBuffer(nil)

	armorWriter, err := armor.Encode(keyBuf, openpgp.PublicKeyType, nil)
	if err != nil {
		t.Fatalf("Failed to create armor writer: %v", err)
	}

	err = entity.Serialize(armorWriter)
	if err != nil {
		t.Fatalf("Failed to serialize public key: %v", err)
	}

	_ = armorWriter.Close()

	// Now encode the private key
	privKeyBuf := bytes.NewBuffer(nil)

	privArmorWriter, err := armor.Encode(privKeyBuf, openpgp.PrivateKeyType, nil)
	if err != nil {
		t.Fatalf("Failed to create private armor writer: %v", err)
	}

	err = entity.SerializePrivate(privArmorWriter, nil)
	if err != nil {
		t.Fatalf("Failed to serialize private key: %v", err)
	}

	_ = privArmorWriter.Close()

	return entity, privKeyBuf.Bytes()
}

// TestNewGPGSignerValidKey tests loading a valid GPG private key.
func TestNewGPGSignerValidKey(t *testing.T) {
	tmpDir := t.TempDir()
	keyPath := filepath.Join(tmpDir, "test.gpg")

	_, keyPEM := generateTestGPGKey(t)

	if err := os.WriteFile(keyPath, keyPEM, 0o600); err != nil {
		t.Fatalf("Failed to write key file: %v", err)
	}

	cfg := Config{
		Enabled: true,
		KeyPath: keyPath,
	}

	signer, err := NewGPGSigner(cfg, FormatDEB)
	require.NotNil(t, signer)
	require.NoError(t, err)
	require.NotNil(t, signer.entity)
}

// armoredEncryptedKeyWithSigningSubkey returns a private key whose primary key
// and signing subkey are both protected by the passphrase.
func armoredEncryptedKeyWithSigningSubkey(t *testing.T, passphrase string) []byte {
	t.Helper()

	entity, err := openpgp.NewEntity("Test User", "test", "test@example.com", nil)
	require.NoError(t, err)
	require.NoError(t, entity.AddSigningSubkey(nil))
	require.NoError(t, entity.EncryptPrivateKeys([]byte(passphrase), nil))

	buf := bytes.NewBuffer(nil)
	w, err := armor.Encode(buf, openpgp.PrivateKeyType, nil)
	require.NoError(t, err)
	require.NoError(t, entity.SerializePrivateWithoutSigning(w, nil))
	require.NoError(t, w.Close())

	return buf.Bytes()
}

// TestGPGSignerEncryptedSigningSubkey verifies that passphrase-protected
// signing subkeys are decrypted so that Sign succeeds.
func TestGPGSignerEncryptedSigningSubkey(t *testing.T) {
	tmpDir := t.TempDir()
	keyPath := filepath.Join(tmpDir, "test.gpg")
	artifact := filepath.Join(tmpDir, "pkg.deb")

	require.NoError(t, os.WriteFile(keyPath,
		armoredEncryptedKeyWithSigningSubkey(t, "secret"), 0o600))
	require.NoError(t, os.WriteFile(artifact, []byte("payload"), 0o600))

	signer, err := NewGPGSigner(Config{Enabled: true, KeyPath: keyPath, Passphrase: "secret"},
		FormatDEB)
	require.NoError(t, err)

	require.NoError(t, signer.Sign(t.Context(), artifact))
	require.FileExists(t, artifact+".asc")

	_, err = NewGPGSigner(Config{Enabled: true, KeyPath: keyPath, Passphrase: "wrong"},
		FormatDEB)
	require.Error(t, err)
}
