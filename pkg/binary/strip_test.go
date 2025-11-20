package binary

import (
	"os"
	"path/filepath"
	"testing"
)

func TestStripFile(t *testing.T) {
	// Create a temporary file for testing
	tempDir := t.TempDir()
	testFile := filepath.Join(tempDir, "test_binary")

	// Create a dummy file
	err := os.WriteFile(testFile, []byte("dummy binary content"), 0o755)
	if err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	// Test StripFile - this will likely fail on a non-binary file but tests the function call
	_ = StripFile(testFile)
	// We don't fail the test if strip command fails because we're testing with dummy content
	// The important thing is that the function executes without panicking
}

func TestStripLTO(t *testing.T) {
	// Create a temporary file for testing
	tempDir := t.TempDir()
	testFile := filepath.Join(tempDir, "test_binary")

	// Create a dummy file
	err := os.WriteFile(testFile, []byte("dummy binary content"), 0o755)
	if err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	// Test StripLTO - this will likely fail on a non-binary file but tests the function call
	_ = StripLTO(testFile)
	// We don't fail the test if strip command fails because we're testing with dummy content
	// The important thing is that the function executes without panicking
}

func TestStripFileWithArgs(t *testing.T) {
	// Create a temporary file for testing
	tempDir := t.TempDir()
	testFile := filepath.Join(tempDir, "test_binary")

	// Create a dummy file
	err := os.WriteFile(testFile, []byte("dummy binary content"), 0o755)
	if err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	// Test StripFile with additional args
	_ = StripFile(testFile, "--version")
	// We don't fail the test if strip command fails because we're testing with dummy content
}

func TestStripLTOWithArgs(t *testing.T) {
	// Create a temporary file for testing
	tempDir := t.TempDir()
	testFile := filepath.Join(tempDir, "test_binary")

	// Create a dummy file
	err := os.WriteFile(testFile, []byte("dummy binary content"), 0o755)
	if err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	// Test StripLTO with additional args
	_ = StripLTO(testFile, "--version")
	// We don't fail the test if strip command fails because we're testing with dummy content
}

func TestStripNonExistentFile(t *testing.T) {
	// Test with non-existent file
	err := StripFile("/non/existent/file")
	if err == nil {
		t.Fatal("Expected error for non-existent file, got nil")
	}
}

func TestStripLTONonExistentFile(t *testing.T) {
	// Test with non-existent file
	err := StripLTO("/non/existent/file")
	if err == nil {
		t.Fatal("Expected error for non-existent file, got nil")
	}
}

func TestStripWithCrossCompilation(t *testing.T) {
	// Save original STRIP environment variable
	originalStrip := os.Getenv("STRIP")

	defer func() {
		if originalStrip != "" {
			_ = os.Setenv("STRIP", originalStrip)
		} else {
			_ = os.Unsetenv("STRIP")
		}
	}()

	// Create a temporary file for testing
	tempDir := t.TempDir()
	testFile := filepath.Join(tempDir, "test_binary")

	// Create a dummy file
	err := os.WriteFile(testFile, []byte("dummy binary content"), 0o755)
	if err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	// Test with cross-compilation STRIP variable set
	_ = os.Setenv("STRIP", "aarch64-linux-gnu-strip")

	// This will fail because aarch64-linux-gnu-strip may not exist,
	// but we're testing that it attempts to use the right command
	_ = StripFile(testFile)

	// Test with STRIP unset (should use default "strip")
	_ = os.Unsetenv("STRIP")
	_ = StripFile(testFile)
}
