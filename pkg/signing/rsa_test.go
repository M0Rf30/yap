package signing_test

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/M0Rf30/yap/v2/pkg/signing"
)

// createFakeAPK creates a fake APK file with two gzip streams (control + data)
// and returns the concatenated bytes. The internal-package test uses its own
// helper that exposes the individual streams.
func createFakeAPK(t *testing.T) []byte {
	t.Helper()
	// Create control.tar.gz
	controlBuf := bytes.NewBuffer(nil)
	gzControl := gzip.NewWriter(controlBuf)
	twControl := tar.NewWriter(gzControl)

	controlContent := []byte("control file content")
	hdr := &tar.Header{
		Name:    ".PKGINFO",
		Size:    int64(len(controlContent)),
		Mode:    0o644,
		ModTime: time.Unix(0, 0),
	}

	if err := twControl.WriteHeader(hdr); err != nil {
		t.Fatalf("Failed to write control header: %v", err)
	}

	if _, err := twControl.Write(controlContent); err != nil {
		t.Fatalf("Failed to write control data: %v", err)
	}

	if err := twControl.Close(); err != nil {
		t.Fatalf("Failed to close control tar: %v", err)
	}

	if err := gzControl.Close(); err != nil {
		t.Fatalf("Failed to close control gzip: %v", err)
	}

	controlTarGz := controlBuf.Bytes()

	// Create data.tar.gz
	dataBuf := bytes.NewBuffer(nil)
	gzData := gzip.NewWriter(dataBuf)
	twData := tar.NewWriter(gzData)

	dataContent := []byte("data file content")
	dataHdr := &tar.Header{
		Name:    "usr/bin/myapp",
		Size:    int64(len(dataContent)),
		Mode:    0o755,
		ModTime: time.Unix(0, 0),
	}

	if err := twData.WriteHeader(dataHdr); err != nil {
		t.Fatalf("Failed to write data header: %v", err)
	}

	if _, err := twData.Write(dataContent); err != nil {
		t.Fatalf("Failed to write data content: %v", err)
	}

	if err := twData.Close(); err != nil {
		t.Fatalf("Failed to close data tar: %v", err)
	}

	if err := gzData.Close(); err != nil {
		t.Fatalf("Failed to close data gzip: %v", err)
	}

	dataTarGz := dataBuf.Bytes()

	// Concatenate: control + data
	apk := bytes.NewBuffer(nil)
	apk.Write(controlTarGz)
	apk.Write(dataTarGz)

	return apk.Bytes()
}

// generateTestRSAKey generates a test RSA private key and returns its
// PEM-encoded bytes. The internal-package test uses its own helper that also
// returns the live *rsa.PrivateKey for direct signature verification.
func generateTestRSAKey(t *testing.T) []byte {
	t.Helper()

	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("Failed to generate RSA key: %v", err)
	}

	privBytes := x509.MarshalPKCS1PrivateKey(key)

	return pem.EncodeToMemory(&pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: privBytes,
	})
}

// TestNewRSASignerInvalidPEM tests that invalid PEM format is rejected.
func TestNewRSASignerInvalidPEM(t *testing.T) {
	tmpDir := t.TempDir()
	keyPath := filepath.Join(tmpDir, "invalid.key")

	if err := os.WriteFile(keyPath, []byte("not a valid PEM"), 0o600); err != nil {
		t.Fatalf("Failed to write key file: %v", err)
	}

	cfg := signing.Config{
		Enabled: true,
		KeyPath: keyPath,
	}

	_, err := signing.NewRSASigner(cfg)
	if err == nil {
		t.Errorf("NewRSASigner() should reject invalid PEM")
	}
}

// TestNewRSASignerMissingFile tests that missing key file is handled.
func TestNewRSASignerMissingFile(t *testing.T) {
	cfg := signing.Config{
		Enabled: true,
		KeyPath: "/nonexistent/path/key.pem",
	}

	_, err := signing.NewRSASigner(cfg)
	if err == nil {
		t.Errorf("NewRSASigner() should reject missing file")
	}
}

// TestRSASignerSign tests signing a fake APK.
func TestRSASignerSign(t *testing.T) {
	tmpDir := t.TempDir()
	keyPath := filepath.Join(tmpDir, "test.key")
	apkPath := filepath.Join(tmpDir, "test.apk")

	// Generate key and write to file
	keyPEM := generateTestRSAKey(t)
	if err := os.WriteFile(keyPath, keyPEM, 0o600); err != nil {
		t.Fatalf("Failed to write key file: %v", err)
	}

	// Create fake APK
	apkData := createFakeAPK(t)
	if err := os.WriteFile(apkPath, apkData, 0o644); err != nil {
		t.Fatalf("Failed to write APK file: %v", err)
	}

	// Create signer
	cfg := signing.Config{
		Enabled: true,
		KeyPath: keyPath,
		KeyName: "testkey",
	}

	signer, err := signing.NewRSASigner(cfg)
	if err != nil {
		t.Fatalf("NewRSASigner() error = %v", err)
	}

	// Sign the APK
	ctx := context.Background()

	err = signer.Sign(ctx, apkPath)
	if err != nil {
		t.Fatalf("Sign() error = %v", err)
	}

	// Verify the signed APK exists and is larger
	signedData, err := os.ReadFile(apkPath)
	if err != nil {
		t.Fatalf("Failed to read signed APK: %v", err)
	}

	if len(signedData) <= len(apkData) {
		t.Errorf("Signed APK should be larger than original, original=%d, signed=%d",
			len(apkData), len(signedData))
	}

	// Verify the signature entry exists in the first tar entry
	entryName, err := extractSignatureEntryName(t, signedData)
	if err != nil {
		t.Fatalf("Failed to extract entry name: %v", err)
	}

	if entryName != ".SIGN.RSA.testkey.rsa.pub" {
		t.Errorf("Expected entry name '.SIGN.RSA.testkey.rsa.pub', got %q", entryName)
	}
}

// TestRSASignerDefaultKeyName tests that default key name is derived from file.
func TestRSASignerDefaultKeyName(t *testing.T) {
	tmpDir := t.TempDir()
	keyPath := filepath.Join(tmpDir, "mykey.pem")
	apkPath := filepath.Join(tmpDir, "test.apk")

	// Generate key
	keyPEM := generateTestRSAKey(t)
	if err := os.WriteFile(keyPath, keyPEM, 0o600); err != nil {
		t.Fatalf("Failed to write key file: %v", err)
	}

	// Create fake APK
	apkData := createFakeAPK(t)
	if err := os.WriteFile(apkPath, apkData, 0o644); err != nil {
		t.Fatalf("Failed to write APK file: %v", err)
	}

	// Create signer WITHOUT explicit key name
	cfg := signing.Config{
		Enabled: true,
		KeyPath: keyPath,
		KeyName: "", // Empty — should default to "mykey"
	}

	signer, err := signing.NewRSASigner(cfg)
	if err != nil {
		t.Fatalf("NewRSASigner() error = %v", err)
	}

	ctx := context.Background()

	err = signer.Sign(ctx, apkPath)
	if err != nil {
		t.Fatalf("Sign() error = %v", err)
	}

	// Verify the signature entry name contains "mykey"
	signedData, err := os.ReadFile(apkPath)
	if err != nil {
		t.Fatalf("Failed to read signed APK: %v", err)
	}

	entryName, err := extractSignatureEntryName(t, signedData)
	if err != nil {
		t.Fatalf("Failed to extract entry name: %v", err)
	}

	if entryName != ".SIGN.RSA.mykey.rsa.pub" {
		t.Errorf("Expected entry name '.SIGN.RSA.mykey.rsa.pub', got %q", entryName)
	}
}

// extractSignatureEntryName extracts the entry name from the first tar entry.
func extractSignatureEntryName(t *testing.T, apkData []byte) (string, error) {
	t.Helper()

	reader := bytes.NewReader(apkData)

	gz, err := gzip.NewReader(reader)
	if err != nil {
		return "", err
	}

	defer func() {
		_ = gz.Close()
	}()

	tr := tar.NewReader(gz)

	hdr, err := tr.Next()
	if err != nil {
		return "", err
	}

	return hdr.Name, nil
}
