package signing_test

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/M0Rf30/yap/v2/pkg/signing"
)

// TestNewRSASignerPKCS8Key verifies that a PKCS#8-encoded RSA private key is
// accepted by NewRSASigner.
func TestNewRSASignerPKCS8Key(t *testing.T) {
	tmpDir := t.TempDir()
	keyPath := filepath.Join(tmpDir, "pkcs8.key")

	// Generate RSA key and encode as PKCS#8
	privKey, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)

	pkcs8Bytes, err := x509.MarshalPKCS8PrivateKey(privKey)
	require.NoError(t, err)

	keyPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "PRIVATE KEY",
		Bytes: pkcs8Bytes,
	})

	require.NoError(t, os.WriteFile(keyPath, keyPEM, 0o600))

	cfg := signing.Config{
		Enabled: true,
		KeyPath: keyPath,
		KeyName: "pkcs8key",
	}

	signer, err := signing.NewRSASigner(cfg)
	require.NoError(t, err)
	require.NotNil(t, signer)
}

// TestNewRSASignerPKCS8NonRSAKey verifies that a PKCS#8 key that is NOT an RSA
// key (e.g. ECDSA) is rejected with a meaningful error.
func TestNewRSASignerPKCS8NonRSAKey(t *testing.T) {
	tmpDir := t.TempDir()
	keyPath := filepath.Join(tmpDir, "ecdsa.key")

	// Generate an ECDSA key and encode as PKCS#8
	ecKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)

	pkcs8Bytes, err := x509.MarshalPKCS8PrivateKey(ecKey)
	require.NoError(t, err)

	keyPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "PRIVATE KEY",
		Bytes: pkcs8Bytes,
	})

	require.NoError(t, os.WriteFile(keyPath, keyPEM, 0o600))

	cfg := signing.Config{
		Enabled: true,
		KeyPath: keyPath,
	}

	_, err = signing.NewRSASigner(cfg)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not an RSA private key")
}

// TestNewRSASignerInvalidPKCS8 verifies that a PEM block with a "PRIVATE KEY"
// header but invalid DER content is rejected.
func TestNewRSASignerInvalidPKCS8(t *testing.T) {
	tmpDir := t.TempDir()
	keyPath := filepath.Join(tmpDir, "bad_pkcs8.key")

	// Valid PEM header but garbage DER bytes
	keyPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "PRIVATE KEY",
		Bytes: []byte("this is not valid DER"),
	})

	require.NoError(t, os.WriteFile(keyPath, keyPEM, 0o600))

	cfg := signing.Config{
		Enabled: true,
		KeyPath: keyPath,
	}

	_, err := signing.NewRSASigner(cfg)
	require.Error(t, err)
}

// TestRSASignerSignContextCancelled verifies that Sign respects a cancelled context.
func TestRSASignerSignContextCancelled(t *testing.T) {
	tmpDir := t.TempDir()
	keyPath := filepath.Join(tmpDir, "test.key")
	apkPath := filepath.Join(tmpDir, "test.apk")

	keyPEM := generateTestRSAKey(t)
	require.NoError(t, os.WriteFile(keyPath, keyPEM, 0o600))

	apkData := createFakeAPK(t)
	require.NoError(t, os.WriteFile(apkPath, apkData, 0o644))

	cfg := signing.Config{
		Enabled: true,
		KeyPath: keyPath,
		KeyName: "testkey",
	}

	signer, err := signing.NewRSASigner(cfg)
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	err = signer.Sign(ctx, apkPath)
	require.Error(t, err)
}

// TestRSASignerSignMissingArtifact verifies that Sign returns an error when the
// artifact file does not exist.
func TestRSASignerSignMissingArtifact(t *testing.T) {
	tmpDir := t.TempDir()
	keyPath := filepath.Join(tmpDir, "test.key")

	keyPEM := generateTestRSAKey(t)
	require.NoError(t, os.WriteFile(keyPath, keyPEM, 0o600))

	cfg := signing.Config{
		Enabled: true,
		KeyPath: keyPath,
		KeyName: "testkey",
	}

	signer, err := signing.NewRSASigner(cfg)
	require.NoError(t, err)

	err = signer.Sign(context.Background(), filepath.Join(tmpDir, "nonexistent.apk"))
	require.Error(t, err)
}

// TestRSASignerSignInvalidGzip verifies that Sign returns an error when the
// artifact is not a valid gzip stream (i.e. not a valid APK).
func TestRSASignerSignInvalidGzip(t *testing.T) {
	tmpDir := t.TempDir()
	keyPath := filepath.Join(tmpDir, "test.key")
	apkPath := filepath.Join(tmpDir, "bad.apk")

	keyPEM := generateTestRSAKey(t)
	require.NoError(t, os.WriteFile(keyPath, keyPEM, 0o600))

	// Write garbage that is not a valid gzip stream
	require.NoError(t, os.WriteFile(apkPath, []byte("not a gzip stream"), 0o644))

	cfg := signing.Config{
		Enabled: true,
		KeyPath: keyPath,
		KeyName: "testkey",
	}

	signer, err := signing.NewRSASigner(cfg)
	require.NoError(t, err)

	err = signer.Sign(context.Background(), apkPath)
	require.Error(t, err)
}
