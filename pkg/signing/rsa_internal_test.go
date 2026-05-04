package signing

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha1" // nolint:gosec // APK protocol mandates SHA1
	"crypto/x509"
	"encoding/pem"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// generateTestRSAKey generates a test RSA private key and returns both the
// key and its PEM-encoded bytes.
func generateTestRSAKey(t *testing.T) (key *rsa.PrivateKey, pemData []byte) {
	t.Helper()

	var err error

	key, err = rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("Failed to generate RSA key: %v", err)
	}

	// Encode to PEM
	privBytes := x509.MarshalPKCS1PrivateKey(key)
	pemData = pem.EncodeToMemory(&pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: privBytes,
	})

	return key, pemData
}

// createFakeAPK creates a fake APK file with two gzip streams (control + data).
func createFakeAPK(t *testing.T) (apkBytes, controlData, dataData []byte) {
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

	return apk.Bytes(), controlTarGz, dataTarGz
}

// extractSignatureFromAPK extracts the signature bytes from the first tar entry.
func extractSignatureFromAPK(t *testing.T, apkData []byte) ([]byte, error) {
	t.Helper()

	reader := bytes.NewReader(apkData)

	gz, err := gzip.NewReader(reader)
	if err != nil {
		return nil, err
	}

	defer func() {
		_ = gz.Close()
	}()

	tr := tar.NewReader(gz)

	_, err = tr.Next()
	if err != nil {
		return nil, err
	}

	signature, err := io.ReadAll(tr)
	if err != nil {
		return nil, err
	}

	return signature, nil
}

// TestNewRSASignerValidKey tests loading a valid RSA private key.
func TestNewRSASignerValidKey(t *testing.T) {
	tmpDir := t.TempDir()
	keyPath := filepath.Join(tmpDir, "test.key")

	_, keyPEM := generateTestRSAKey(t)

	if err := os.WriteFile(keyPath, keyPEM, 0o600); err != nil {
		t.Fatalf("Failed to write key file: %v", err)
	}

	cfg := Config{
		Enabled: true,
		KeyPath: keyPath,
		KeyName: "testkey",
	}

	signer, err := NewRSASigner(cfg)
	require.NotNil(t, signer)
	require.NoError(t, err)
	require.NotNil(t, signer.key)
}

// TestRSASignerSignatureValidation tests that the signature is valid.
// This test is in the internal package because it accesses extractFirstGzipStream.
func TestRSASignerSignatureValidation(t *testing.T) {
	tmpDir := t.TempDir()
	keyPath := filepath.Join(tmpDir, "test.key")
	apkPath := filepath.Join(tmpDir, "test.apk")

	// Generate key
	privKey, keyPEM := generateTestRSAKey(t)
	if err := os.WriteFile(keyPath, keyPEM, 0o600); err != nil {
		t.Fatalf("Failed to write key file: %v", err)
	}

	// Create fake APK
	apkData, _, _ := createFakeAPK(t)
	if err := os.WriteFile(apkPath, apkData, 0o644); err != nil {
		t.Fatalf("Failed to write APK file: %v", err)
	}

	// Find where the first gzip stream (control.tar.gz) ends
	dataStart, err := extractFirstGzipStream(apkData)
	if err != nil {
		t.Fatalf("Failed to extract first gzip stream: %v", err)
	}

	controlTarGzCompressed := apkData[:dataStart]

	// Sign the APK
	cfg := Config{
		Enabled: true,
		KeyPath: keyPath,
		KeyName: "testkey",
	}

	signer, err := NewRSASigner(cfg)
	if err != nil {
		t.Fatalf("NewRSASigner() error = %v", err)
	}

	ctx := context.Background()

	err = signer.Sign(ctx, apkPath)
	if err != nil {
		t.Fatalf("Sign() error = %v", err)
	}

	// Read signed APK and extract signature
	signedData, err := os.ReadFile(apkPath)
	if err != nil {
		t.Fatalf("Failed to read signed APK: %v", err)
	}

	// Extract signature from first gzip stream
	signature, err := extractSignatureFromAPK(t, signedData)
	if err != nil {
		t.Fatalf("Failed to extract signature: %v", err)
	}

	// Verify signature against control.tar.gz (the compressed bytes)
	// nolint:gosec // SHA1 required by APK format
	hash := sha1.Sum(controlTarGzCompressed)

	t.Logf("Control tar.gz size: %d, signature size: %d", len(controlTarGzCompressed), len(signature))

	err = rsa.VerifyPKCS1v15(&privKey.PublicKey, crypto.SHA1, hash[:], signature)
	if err != nil {
		t.Errorf("Signature verification failed: %v", err)
		t.Logf("Hash: %x", hash[:])
	}
}
