package signing_test

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/ProtonMail/go-crypto/openpgp"
	"github.com/ProtonMail/go-crypto/openpgp/armor"

	"github.com/M0Rf30/yap/v2/pkg/signing"
)

// generateTestGPGKey generates a test GPG key and returns its ASCII-armored
// private key bytes. External tests only need the armored bytes; the internal
// package test (gpg_internal_test.go) keeps the entity for direct verification.
func generateTestGPGKey(t *testing.T) []byte {
	t.Helper()

	entity, err := openpgp.NewEntity("Test User", "test", "test@example.com", nil)
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

	return privKeyBuf.Bytes()
}

// TestNewGPGSignerInvalidKey tests that invalid key format is rejected.
func TestNewGPGSignerInvalidKey(t *testing.T) {
	tmpDir := t.TempDir()
	keyPath := filepath.Join(tmpDir, "invalid.gpg")

	if err := os.WriteFile(keyPath, []byte("not a valid GPG key"), 0o600); err != nil {
		t.Fatalf("Failed to write key file: %v", err)
	}

	cfg := signing.Config{
		Enabled: true,
		KeyPath: keyPath,
	}

	_, err := signing.NewGPGSigner(cfg, signing.FormatDEB)
	if err == nil {
		t.Errorf("NewGPGSigner() should reject invalid key")
	}
}

// TestNewGPGSignerMissingFile tests that missing key file is handled.
func TestNewGPGSignerMissingFile(t *testing.T) {
	cfg := signing.Config{
		Enabled: true,
		KeyPath: "/nonexistent/path/key.gpg",
	}

	_, err := signing.NewGPGSigner(cfg, signing.FormatDEB)
	if err == nil {
		t.Errorf("NewGPGSigner() should reject missing file")
	}
}

// TestGPGSignerSignDEB tests signing a DEB package.
func TestGPGSignerSignDEB(t *testing.T) {
	tmpDir := t.TempDir()
	keyPath := filepath.Join(tmpDir, "test.gpg")
	debPath := filepath.Join(tmpDir, "test.deb")

	// Generate key and write to file
	keyPEM := generateTestGPGKey(t)
	if err := os.WriteFile(keyPath, keyPEM, 0o600); err != nil {
		t.Fatalf("Failed to write key file: %v", err)
	}

	// Create fake DEB file
	debData := []byte("fake deb package data")
	if err := os.WriteFile(debPath, debData, 0o644); err != nil {
		t.Fatalf("Failed to write DEB file: %v", err)
	}

	// Create signer
	cfg := signing.Config{
		Enabled: true,
		KeyPath: keyPath,
	}

	signer, err := signing.NewGPGSigner(cfg, signing.FormatDEB)
	if err != nil {
		t.Fatalf("NewGPGSigner() error = %v", err)
	}

	// Sign the DEB
	ctx := context.Background()

	err = signer.Sign(ctx, debPath)
	if err != nil {
		t.Fatalf("Sign() error = %v", err)
	}

	// Verify the signature file exists
	sigPath := debPath + ".asc"
	if _, err := os.Stat(sigPath); err != nil {
		t.Fatalf("Signature file not created: %v", err)
	}

	// Verify the signature file is not empty
	sigData, err := os.ReadFile(sigPath)
	if err != nil {
		t.Fatalf("Failed to read signature file: %v", err)
	}

	if len(sigData) == 0 {
		t.Errorf("Signature file is empty")
	}

	// Verify it's ASCII-armored
	if !bytes.Contains(sigData, []byte("-----BEGIN PGP SIGNATURE-----")) {
		t.Errorf("Signature is not ASCII-armored")
	}
}

// TestGPGSignerSignPacman tests signing a Pacman package.
func TestGPGSignerSignPacman(t *testing.T) {
	tmpDir := t.TempDir()
	keyPath := filepath.Join(tmpDir, "test.gpg")
	pkgPath := filepath.Join(tmpDir, "test.pkg.tar.zst")

	// Generate key and write to file
	keyPEM := generateTestGPGKey(t)
	if err := os.WriteFile(keyPath, keyPEM, 0o600); err != nil {
		t.Fatalf("Failed to write key file: %v", err)
	}

	// Create fake Pacman package file
	pkgData := []byte("fake pacman package data")
	if err := os.WriteFile(pkgPath, pkgData, 0o644); err != nil {
		t.Fatalf("Failed to write package file: %v", err)
	}

	// Create signer
	cfg := signing.Config{
		Enabled: true,
		KeyPath: keyPath,
	}

	signer, err := signing.NewGPGSigner(cfg, signing.FormatPacman)
	if err != nil {
		t.Fatalf("NewGPGSigner() error = %v", err)
	}

	// Sign the package
	ctx := context.Background()

	err = signer.Sign(ctx, pkgPath)
	if err != nil {
		t.Fatalf("Sign() error = %v", err)
	}

	// Verify the signature file exists
	sigPath := pkgPath + ".sig"
	if _, err := os.Stat(sigPath); err != nil {
		t.Fatalf("Signature file not created: %v", err)
	}

	// Verify the signature file is not empty
	sigData, err := os.ReadFile(sigPath)
	if err != nil {
		t.Fatalf("Failed to read signature file: %v", err)
	}

	if len(sigData) == 0 {
		t.Errorf("Signature file is empty")
	}

	// Verify it's binary (NOT ASCII-armored)
	if bytes.Contains(sigData, []byte("-----BEGIN PGP SIGNATURE-----")) {
		t.Errorf("Pacman signature should be binary, not ASCII-armored")
	}
}

// TestGPGSignerSignRPM tests signing an RPM package.
func TestGPGSignerSignRPM(t *testing.T) {
	tmpDir := t.TempDir()
	keyPath := filepath.Join(tmpDir, "test.gpg")
	rpmPath := filepath.Join(tmpDir, "test.rpm")

	// Generate key and write to file
	keyPEM := generateTestGPGKey(t)
	if err := os.WriteFile(keyPath, keyPEM, 0o600); err != nil {
		t.Fatalf("Failed to write key file: %v", err)
	}

	// Create fake RPM file
	rpmData := []byte("fake rpm package data")
	if err := os.WriteFile(rpmPath, rpmData, 0o644); err != nil {
		t.Fatalf("Failed to write RPM file: %v", err)
	}

	// Create signer
	cfg := signing.Config{
		Enabled: true,
		KeyPath: keyPath,
	}

	signer, err := signing.NewGPGSigner(cfg, signing.FormatRPM)
	if err != nil {
		t.Fatalf("NewGPGSigner() error = %v", err)
	}

	// Sign the RPM
	ctx := context.Background()

	err = signer.Sign(ctx, rpmPath)
	if err != nil {
		t.Fatalf("Sign() error = %v", err)
	}

	// Verify the signature file exists
	sigPath := rpmPath + ".asc"
	if _, err := os.Stat(sigPath); err != nil {
		t.Fatalf("Signature file not created: %v", err)
	}

	// Verify the signature file is not empty
	sigData, err := os.ReadFile(sigPath)
	if err != nil {
		t.Fatalf("Failed to read signature file: %v", err)
	}

	if len(sigData) == 0 {
		t.Errorf("Signature file is empty")
	}
}

// TestGPGSignerGetSigningFunction tests the RPM signing function.
func TestGPGSignerGetSigningFunction(t *testing.T) {
	tmpDir := t.TempDir()
	keyPath := filepath.Join(tmpDir, "test.gpg")

	// Generate key and write to file
	keyPEM := generateTestGPGKey(t)
	if err := os.WriteFile(keyPath, keyPEM, 0o600); err != nil {
		t.Fatalf("Failed to write key file: %v", err)
	}

	// Create signer
	cfg := signing.Config{
		Enabled: true,
		KeyPath: keyPath,
	}

	signer, err := signing.NewGPGSigner(cfg, signing.FormatRPM)
	if err != nil {
		t.Fatalf("NewGPGSigner() error = %v", err)
	}

	// Get signing function
	sigFunc := signer.GetSigningFunction()
	if sigFunc == nil {
		t.Errorf("GetSigningFunction() returned nil")
	}

	// Test the signing function
	data := []byte("test data to sign")

	signature, err := sigFunc(data)
	if err != nil {
		t.Fatalf("Signing function error = %v", err)
	}

	if len(signature) == 0 {
		t.Errorf("Signature is empty")
	}
}

// TestGPGSignerContextCancellation tests that context cancellation is respected.
func TestGPGSignerContextCancellation(t *testing.T) {
	tmpDir := t.TempDir()
	keyPath := filepath.Join(tmpDir, "test.gpg")
	debPath := filepath.Join(tmpDir, "test.deb")

	// Generate key and write to file
	keyPEM := generateTestGPGKey(t)
	if err := os.WriteFile(keyPath, keyPEM, 0o600); err != nil {
		t.Fatalf("Failed to write key file: %v", err)
	}

	// Create fake DEB file
	debData := []byte("fake deb package data")
	if err := os.WriteFile(debPath, debData, 0o644); err != nil {
		t.Fatalf("Failed to write DEB file: %v", err)
	}

	// Create signer
	cfg := signing.Config{
		Enabled: true,
		KeyPath: keyPath,
	}

	signer, err := signing.NewGPGSigner(cfg, signing.FormatDEB)
	if err != nil {
		t.Fatalf("NewGPGSigner() error = %v", err)
	}

	// Create a cancelled context
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	// Try to sign with cancelled context
	err = signer.Sign(ctx, debPath)
	if err == nil {
		t.Errorf("Sign() should fail with cancelled context")
	}
}
