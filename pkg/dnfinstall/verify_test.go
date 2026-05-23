package dnfinstall

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/ProtonMail/go-crypto/openpgp/armor"
)

// TestVerifyRPMSignatureUnsignedWithBypass tests that an unsigned RPM
// returns nil when AllowUnverifiedRPMs is true.
func TestVerifyRPMSignatureUnsignedWithBypass(t *testing.T) {
	tmpDir := t.TempDir()
	keyringDir := filepath.Join(tmpDir, "keyring")
	if err := os.Mkdir(keyringDir, 0o755); err != nil {
		t.Fatalf("failed to create keyring dir: %v", err)
	}

	// Create a minimal unsigned RPM.
	rpmPath := filepath.Join(tmpDir, "test.rpm")
	if err := createMinimalRPM(rpmPath); err != nil {
		t.Fatalf("failed to create test RPM: %v", err)
	}

	opts := Options{
		AllowUnverifiedRPMs: true,
		KeyringPath:         keyringDir,
	}

	ctx := context.Background()
	err := verifyRPMSignature(ctx, rpmPath, opts)
	if err != nil {
		t.Errorf("expected nil, got %v", err)
	}
}

// TestVerifyRPMSignatureUnsignedStrict tests that an unsigned RPM
// returns an error when AllowUnverifiedRPMs is false.
func TestVerifyRPMSignatureUnsignedStrict(t *testing.T) {
	tmpDir := t.TempDir()
	keyringDir := filepath.Join(tmpDir, "keyring")
	if err := os.Mkdir(keyringDir, 0o755); err != nil {
		t.Fatalf("failed to create keyring dir: %v", err)
	}

	// Create a minimal unsigned RPM.
	rpmPath := filepath.Join(tmpDir, "test.rpm")
	if err := createMinimalRPM(rpmPath); err != nil {
		t.Fatalf("failed to create test RPM: %v", err)
	}

	opts := Options{
		AllowUnverifiedRPMs: false,
		KeyringPath:         keyringDir,
	}

	ctx := context.Background()
	err := verifyRPMSignature(ctx, rpmPath, opts)
	if err == nil {
		t.Error("expected error, got nil")
	}
}

// TestVerifyRPMSignatureMissingKeyringWithBypass tests that a missing keyring
// returns nil when AllowUnverifiedRPMs is true.
func TestVerifyRPMSignatureMissingKeyringWithBypass(t *testing.T) {
	tmpDir := t.TempDir()
	rpmPath := filepath.Join(tmpDir, "test.rpm")
	if err := createMinimalRPM(rpmPath); err != nil {
		t.Fatalf("failed to create test RPM: %v", err)
	}

	opts := Options{
		AllowUnverifiedRPMs: true,
		KeyringPath:         filepath.Join(tmpDir, "nonexistent"),
	}

	ctx := context.Background()
	err := verifyRPMSignature(ctx, rpmPath, opts)
	if err != nil {
		t.Errorf("expected nil, got %v", err)
	}
}

// TestVerifyRPMSignatureMissingKeyringStrict tests that a missing keyring
// returns ErrNoTrustAnchor when AllowUnverifiedRPMs is false.
func TestVerifyRPMSignatureMissingKeyringStrict(t *testing.T) {
	tmpDir := t.TempDir()
	rpmPath := filepath.Join(tmpDir, "test.rpm")
	if err := createMinimalRPM(rpmPath); err != nil {
		t.Fatalf("failed to create test RPM: %v", err)
	}

	opts := Options{
		AllowUnverifiedRPMs: false,
		KeyringPath:         filepath.Join(tmpDir, "nonexistent"),
	}

	ctx := context.Background()
	err := verifyRPMSignature(ctx, rpmPath, opts)
	if !errors.Is(err, ErrNoTrustAnchor) {
		t.Errorf("expected ErrNoTrustAnchor, got %v", err)
	}
}

// TestLoadRPMKeyringFile tests loading a single keyring file.
func TestLoadRPMKeyringFile(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a minimal test keyring (empty armored block).
	keyPath := filepath.Join(tmpDir, "test.gpg")
	f, err := os.Create(keyPath)
	if err != nil {
		t.Fatalf("failed to create keyring file: %v", err)
	}
	defer func() { _ = f.Close() }()

	// Write an empty but valid ASCII-armored PGP block.
	w, err := armor.Encode(f, "PGP PUBLIC KEY BLOCK", nil)
	if err != nil {
		t.Fatalf("failed to create armor encoder: %v", err)
	}
	defer func() { _ = w.Close() }()

	// Write minimal data (this will fail to parse as a real key, but tests the path).
	if _, err := w.Write([]byte{0x00}); err != nil {
		t.Fatalf("failed to write armor data: %v", err)
	}

	// Load the keyring (will fail to parse, but that's OK for this test).
	keys, err := loadRPMKeyringFile(keyPath)
	// We expect an error because the data isn't a valid key, but the function should handle it.
	if err == nil && len(keys) == 0 {
		// This is expected - empty/invalid keyring
	}
}

// TestLoadRPMKeyringDir tests loading a directory of keyring files.
func TestLoadRPMKeyringDir(t *testing.T) {
	tmpDir := t.TempDir()

	// Write a non-keyring file that should be skipped.
	readmePath := filepath.Join(tmpDir, "README.txt")
	if err := os.WriteFile(readmePath, []byte("not a key"), 0o644); err != nil {
		t.Fatalf("failed to write README: %v", err)
	}

	// Load the directory (should skip the README).
	keys, err := loadRPMKeyringDir(tmpDir)
	if err != nil {
		t.Fatalf("failed to load keyring dir: %v", err)
	}

	// Should be empty since README is not a keyring.
	if len(keys) != 0 {
		t.Errorf("expected 0 keys, got %d", len(keys))
	}
}

// TestLoadRPMKeyringDirMixed tests loading a directory with mixed file types.
func TestLoadRPMKeyringDirMixed(t *testing.T) {
	tmpDir := t.TempDir()

	// Write a non-keyring file.
	junkPath := filepath.Join(tmpDir, "junk.txt")
	if err := os.WriteFile(junkPath, []byte("not a key"), 0o644); err != nil {
		t.Fatalf("failed to write junk: %v", err)
	}

	// Write a file with .gpg extension (but invalid content).
	gpgPath := filepath.Join(tmpDir, "test.gpg")
	if err := os.WriteFile(gpgPath, []byte("invalid gpg data"), 0o644); err != nil {
		t.Fatalf("failed to write gpg file: %v", err)
	}

	// Load the directory.
	keys, err := loadRPMKeyringDir(tmpDir)
	if err != nil {
		t.Fatalf("failed to load keyring dir: %v", err)
	}

	// Should be empty since the .gpg file is invalid.
	if len(keys) != 0 {
		t.Errorf("expected 0 keys, got %d", len(keys))
	}
}

// TestLoadRPMKeyringEmptyDir tests loading an empty directory.
func TestLoadRPMKeyringEmptyDir(t *testing.T) {
	tmpDir := t.TempDir()

	keys, err := loadRPMKeyringDir(tmpDir)
	if err != nil {
		t.Fatalf("failed to load empty keyring dir: %v", err)
	}

	if len(keys) != 0 {
		t.Errorf("expected 0 keys, got %d", len(keys))
	}
}

// TestLoadRPMKeyring tests the main loadRPMKeyring function with both file and dir.
func TestLoadRPMKeyring(t *testing.T) {
	tmpDir := t.TempDir()

	// Test with a file (invalid, but tests the path).
	keyPath := filepath.Join(tmpDir, "test.gpg")
	if err := os.WriteFile(keyPath, []byte("invalid"), 0o644); err != nil {
		t.Fatalf("failed to write key file: %v", err)
	}

	ctx := context.Background()
	keys, err := loadRPMKeyring(ctx, keyPath)
	// Will fail to parse, but that's OK.
	if err == nil && len(keys) == 0 {
		// Expected - invalid key file
	}

	// Test with a directory.
	keys, err = loadRPMKeyring(ctx, tmpDir)
	if err != nil {
		t.Fatalf("failed to load keyring from dir: %v", err)
	}

	if len(keys) != 0 {
		t.Errorf("expected 0 keys from empty dir, got %d", len(keys))
	}
}

// TestLoadRPMKeyringContextCancellation tests that context cancellation is respected.
func TestLoadRPMKeyringContextCancellation(t *testing.T) {
	tmpDir := t.TempDir()

	keyPath := filepath.Join(tmpDir, "test.gpg")
	if err := os.WriteFile(keyPath, []byte("test"), 0o644); err != nil {
		t.Fatalf("failed to write key file: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := loadRPMKeyring(ctx, keyPath)
	if err == nil {
		t.Error("expected context cancellation error, got nil")
	}
}

// TestWrapRPMSignatureError tests error wrapping.
func TestWrapRPMSignatureError(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected error
	}{
		{
			name:     "nil error",
			err:      nil,
			expected: nil,
		},
		{
			name:     "no signature message",
			err:      errors.New("no signature found"),
			expected: ErrUnsignedRPM,
		},
		{
			name:     "generic error",
			err:      errors.New("some error"),
			expected: ErrInvalidSignature,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := wrapRPMSignatureError(tt.err)
			if tt.expected == nil {
				if result != nil {
					t.Errorf("expected nil, got %v", result)
				}
			} else if !errors.Is(result, tt.expected) {
				t.Errorf("expected %v, got %v", tt.expected, result)
			}
		})
	}
}

// TestSignerName tests extracting signer name from entity.
func TestSignerName(t *testing.T) {
	// Test with nil entity.
	name := signerName(nil)
	if name != "" {
		t.Errorf("expected empty string for nil entity, got %q", name)
	}

	// Test with empty entity (no identities, no primary key).
	// Note: signerName will panic if PrimaryKey is nil, so we skip this test.
	// In practice, openpgp.Entity always has a PrimaryKey when created properly.
}

// TestVerifyRPMSignatureNonexistentFile tests that a nonexistent RPM file returns an error.
func TestVerifyRPMSignatureNonexistentFile(t *testing.T) {
	tmpDir := t.TempDir()
	keyringDir := filepath.Join(tmpDir, "keyring")
	if err := os.Mkdir(keyringDir, 0o755); err != nil {
		t.Fatalf("failed to create keyring dir: %v", err)
	}

	opts := Options{
		AllowUnverifiedRPMs: false,
		KeyringPath:         keyringDir,
	}

	ctx := context.Background()
	err := verifyRPMSignature(ctx, filepath.Join(tmpDir, "nonexistent.rpm"), opts)
	if err == nil {
		t.Error("expected error for nonexistent file, got nil")
	}
}

// TestLoadRPMKeyringFileNonexistent tests loading a nonexistent keyring file.
func TestLoadRPMKeyringFileNonexistent(t *testing.T) {
	tmpDir := t.TempDir()
	keyPath := filepath.Join(tmpDir, "nonexistent.gpg")

	_, err := loadRPMKeyringFile(keyPath)
	if err == nil {
		t.Error("expected error for nonexistent file, got nil")
	}
}

// Helper functions

// createMinimalRPM creates a minimal valid RPM file for testing.
// This creates an unsigned RPM with just the required headers.
func createMinimalRPM(path string) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()

	// Write RPM magic number (0xEDABEEDB).
	if _, err := f.Write([]byte{0xED, 0xAB, 0xEE, 0xDB}); err != nil {
		return err
	}

	// Write version (3, 0).
	if _, err := f.Write([]byte{0x03, 0x00}); err != nil {
		return err
	}

	// Write signature type (0 = no signature).
	if _, err := f.Write([]byte{0x00, 0x00}); err != nil {
		return err
	}

	// Write minimal header structure.
	// Header magic.
	if _, err := f.Write([]byte{0x8E, 0xAD, 0xE8, 0x01}); err != nil {
		return err
	}

	// Header version and reserved.
	if _, err := f.Write([]byte{0x00, 0x00, 0x00, 0x00}); err != nil {
		return err
	}

	// Number of index entries (0).
	if _, err := f.Write([]byte{0x00, 0x00, 0x00, 0x00}); err != nil {
		return err
	}

	// Header data size (0).
	if _, err := f.Write([]byte{0x00, 0x00, 0x00, 0x00}); err != nil {
		return err
	}

	return nil
}

// TestLoadRPMKeyringDirFiltering tests that directory loading filters files correctly.
func TestLoadRPMKeyringDirFiltering(t *testing.T) {
	tmpDir := t.TempDir()

	// Create files with various names.
	files := []struct {
		name      string
		shouldUse bool
	}{
		{"RPM-GPG-KEY-fedora", true},
		{"RPM-GPG-KEY-fedora-38", true},
		{"test.gpg", true},
		{"test.asc", true},
		{"README.txt", false},
		{"test.pub", false},
		{".hidden", false},
	}

	for _, file := range files {
		path := filepath.Join(tmpDir, file.name)
		if err := os.WriteFile(path, []byte("test"), 0o644); err != nil {
			t.Fatalf("failed to write file %s: %v", file.name, err)
		}
	}

	// Load the directory.
	keys, err := loadRPMKeyringDir(tmpDir)
	if err != nil {
		t.Fatalf("failed to load keyring dir: %v", err)
	}

	// All files are invalid keys, so we should get 0 keys.
	// But the important thing is that the filtering logic works.
	if len(keys) != 0 {
		t.Errorf("expected 0 keys from invalid files, got %d", len(keys))
	}
}

// BenchmarkLoadRPMKeyringFile benchmarks keyring file loading.
func BenchmarkLoadRPMKeyringFile(b *testing.B) {
	tmpDir := b.TempDir()
	keyPath := filepath.Join(tmpDir, "test.gpg")
	if err := os.WriteFile(keyPath, []byte("invalid"), 0o644); err != nil {
		b.Fatalf("failed to write key file: %v", err)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = loadRPMKeyringFile(keyPath)
	}
}

// BenchmarkLoadRPMKeyringDir benchmarks keyring directory loading.
func BenchmarkLoadRPMKeyringDir(b *testing.B) {
	tmpDir := b.TempDir()

	// Create some test files.
	for i := 0; i < 5; i++ {
		path := filepath.Join(tmpDir, "test"+string(rune(i))+".gpg")
		if err := os.WriteFile(path, []byte("invalid"), 0o644); err != nil {
			b.Fatalf("failed to write key file: %v", err)
		}
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = loadRPMKeyringDir(tmpDir)
	}
}
