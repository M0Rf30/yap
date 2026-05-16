//nolint:testpackage // Internal benchmarking of source package methods
package source

import (
	"crypto/sha256"
	"crypto/sha512"
	"encoding/hex"
	"errors"
	"io"
	"math/rand"
	"os"
	"path/filepath"
	"testing"

	"github.com/M0Rf30/yap/v2/pkg/logger"
)

// TestMain silences logger output so benchmark measurements aren't polluted
// by per-iteration Info log lines from validateSource.
func TestMain(m *testing.M) {
	logger.SetWriter(io.Discard)
	os.Exit(m.Run())
}

// BenchmarkValidateSourceSHA256_50MB measures hash computation over a 50MB synthetic blob.
func BenchmarkValidateSourceSHA256_50MB(b *testing.B) {
	// Setup: create a 50MB temporary file with deterministic content
	tmpDir := b.TempDir()
	testFile := filepath.Join(tmpDir, "test-50mb.bin")

	// Generate deterministic content using seeded random source
	rng := rand.New(rand.NewSource(1))

	const fileSize = 50 * 1024 * 1024 // 50MB

	f, err := os.Create(testFile)
	if err != nil {
		b.Fatalf("Failed to create test file: %v", err)
	}
	defer func() { _ = f.Close() }()

	// Write 50MB of deterministic random data
	if _, err := io.CopyN(f, rng, fileSize); err != nil && !errors.Is(err, io.EOF) {
		b.Fatalf("Failed to write test file: %v", err)
	}

	_ = f.Close()

	// Compute expected SHA256 hash for validation
	expectedHash := computeSHA256(testFile)

	// Reset timer after setup
	b.ResetTimer()

	// Benchmark: validate the source file N times
	for i := 0; i < b.N; i++ {
		src := &Source{
			Hash:           expectedHash,
			SourceItemPath: "test-50mb.bin",
			SourceItemURI:  "file:///test-50mb.bin",
		}

		if err := src.validateSource(testFile); err != nil {
			b.Fatalf("Validation failed: %v", err)
		}
	}
}

// BenchmarkValidateSourceSHA512_50MB measures hash computation over a 50MB synthetic blob.
func BenchmarkValidateSourceSHA512_50MB(b *testing.B) {
	// Setup: create a 50MB temporary file with deterministic content
	tmpDir := b.TempDir()
	testFile := filepath.Join(tmpDir, "test-50mb.bin")

	// Generate deterministic content using seeded random source
	rng := rand.New(rand.NewSource(1))

	const fileSize = 50 * 1024 * 1024 // 50MB

	f, err := os.Create(testFile)
	if err != nil {
		b.Fatalf("Failed to create test file: %v", err)
	}
	defer func() { _ = f.Close() }()

	// Write 50MB of deterministic random data
	if _, err := io.CopyN(f, rng, fileSize); err != nil && !errors.Is(err, io.EOF) {
		b.Fatalf("Failed to write test file: %v", err)
	}

	_ = f.Close()

	// Compute expected SHA512 hash for validation
	expectedHash := computeSHA512(testFile)

	// Reset timer after setup
	b.ResetTimer()

	// Benchmark: validate the source file N times
	for i := 0; i < b.N; i++ {
		src := &Source{
			Hash:           expectedHash,
			SourceItemPath: "test-50mb.bin",
			SourceItemURI:  "file:///test-50mb.bin",
		}

		if err := src.validateSource(testFile); err != nil {
			b.Fatalf("Validation failed: %v", err)
		}
	}
}

// computeSHA256 computes the SHA256 hash of a file.
func computeSHA256(filePath string) string {
	f, err := os.Open(filePath)
	if err != nil {
		panic(err)
	}
	defer func() { _ = f.Close() }()

	hasher := sha256.New()
	if _, err := io.Copy(hasher, f); err != nil {
		panic(err)
	}

	return hex.EncodeToString(hasher.Sum(nil))
}

// computeSHA512 computes the SHA512 hash of a file.
func computeSHA512(filePath string) string {
	f, err := os.Open(filePath)
	if err != nil {
		panic(err)
	}
	defer func() { _ = f.Close() }()

	hasher := sha512.New()
	if _, err := io.Copy(hasher, f); err != nil {
		panic(err)
	}

	return hex.EncodeToString(hasher.Sum(nil))
}
