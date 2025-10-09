package crypto

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCalculateSHA256(t *testing.T) {
	// Create a temporary file for testing
	tempDir := t.TempDir()
	testFile := filepath.Join(tempDir, "test.txt")
	testContent := "Hello, World!"

	err := os.WriteFile(testFile, []byte(testContent), 0o644)
	if err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	// Calculate SHA256
	hash, err := CalculateSHA256(testFile)
	if err != nil {
		t.Fatalf("CalculateSHA256 failed: %v", err)
	}

	// Verify hash is not empty
	if len(hash) == 0 {
		t.Fatal("Hash should not be empty")
	}

	// SHA256 should always return 32 bytes
	if len(hash) != 32 {
		t.Fatalf("Expected hash length 32, got %d", len(hash))
	}
}

func TestCalculateSHA256NonExistentFile(t *testing.T) {
	// Test with non-existent file
	_, err := CalculateSHA256("/non/existent/file")
	if err == nil {
		t.Fatal("Expected error for non-existent file, got nil")
	}
}

func TestCalculateSHA256FromReader(t *testing.T) {
	testContent := "Hello, World!"
	reader := strings.NewReader(testContent)

	// Calculate SHA256 from reader
	hash, err := CalculateSHA256FromReader(reader)
	if err != nil {
		t.Fatalf("CalculateSHA256FromReader failed: %v", err)
	}

	// Verify hash is not empty
	if len(hash) == 0 {
		t.Fatal("Hash should not be empty")
	}

	// SHA256 should always return 32 bytes
	if len(hash) != 32 {
		t.Fatalf("Expected hash length 32, got %d", len(hash))
	}
}

func TestCalculateSHA256Consistency(t *testing.T) {
	// Create a temporary file for testing
	tempDir := t.TempDir()
	testFile := filepath.Join(tempDir, "test.txt")
	testContent := "Hello, World!"

	err := os.WriteFile(testFile, []byte(testContent), 0o644)
	if err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	// Calculate SHA256 from file
	hashFromFile, err := CalculateSHA256(testFile)
	if err != nil {
		t.Fatalf("CalculateSHA256 failed: %v", err)
	}

	// Calculate SHA256 from reader with same content
	reader := strings.NewReader(testContent)

	hashFromReader, err := CalculateSHA256FromReader(reader)
	if err != nil {
		t.Fatalf("CalculateSHA256FromReader failed: %v", err)
	}

	// Hashes should be identical
	if len(hashFromFile) != len(hashFromReader) {
		t.Fatal("Hash lengths should be equal")
	}

	for i := range hashFromFile {
		if hashFromFile[i] != hashFromReader[i] {
			t.Fatal("Hashes should be identical")
		}
	}
}

func TestVerifySHA256(t *testing.T) {
	// Create a temporary file for testing
	tempDir := t.TempDir()
	testFile := filepath.Join(tempDir, "test.txt")
	testContent := "Hello, World!"

	err := os.WriteFile(testFile, []byte(testContent), 0o644)
	if err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	// Calculate the expected hash
	expectedHash, err := CalculateSHA256(testFile)
	if err != nil {
		t.Fatalf("Failed to calculate expected hash: %v", err)
	}

	// Verify with correct hash
	isValid, err := VerifySHA256(testFile, expectedHash)
	if err != nil {
		t.Fatalf("VerifySHA256 failed: %v", err)
	}

	if !isValid {
		t.Fatal("File should be valid with correct hash")
	}
}

func TestVerifySHA256Invalid(t *testing.T) {
	// Create a temporary file for testing
	tempDir := t.TempDir()
	testFile := filepath.Join(tempDir, "test.txt")
	testContent := "Hello, World!"

	err := os.WriteFile(testFile, []byte(testContent), 0o644)
	if err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	// Use an incorrect hash (all zeros)
	incorrectHash := make([]byte, 32)

	// Verify with incorrect hash
	isValid, err := VerifySHA256(testFile, incorrectHash)
	if err != nil {
		t.Fatalf("VerifySHA256 failed: %v", err)
	}

	if isValid {
		t.Fatal("File should not be valid with incorrect hash")
	}
}

func TestVerifySHA256DifferentLengths(t *testing.T) {
	// Create a temporary file for testing
	tempDir := t.TempDir()
	testFile := filepath.Join(tempDir, "test.txt")
	testContent := "Hello, World!"

	err := os.WriteFile(testFile, []byte(testContent), 0o644)
	if err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	// Use hash with different length
	shortHash := make([]byte, 16) // Half the length of SHA256

	// Verify with different length hash
	isValid, err := VerifySHA256(testFile, shortHash)
	if err != nil {
		t.Fatalf("VerifySHA256 failed: %v", err)
	}

	if isValid {
		t.Fatal("File should not be valid with different length hash")
	}
}

func TestVerifySHA256NonExistentFile(t *testing.T) {
	hash := make([]byte, 32)

	// Test with non-existent file
	_, err := VerifySHA256("/non/existent/file", hash)
	if err == nil {
		t.Fatal("Expected error for non-existent file, got nil")
	}
}
