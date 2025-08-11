package archive

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCreateTarZst(t *testing.T) {
	// Create a temporary directory for testing
	tempDir := t.TempDir()
	sourceDir := filepath.Join(tempDir, "source")
	outputFile := filepath.Join(tempDir, "test.tar.zst")

	// Create source directory with a test file
	err := os.MkdirAll(sourceDir, 0o755)
	if err != nil {
		t.Fatalf("Failed to create source directory: %v", err)
	}

	testFile := filepath.Join(sourceDir, "test.txt")

	err = os.WriteFile(testFile, []byte("test content"), 0o644)
	if err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	// Test creating tar.zst archive
	err = CreateTarZst(sourceDir, outputFile, false)
	if err != nil {
		t.Fatalf("CreateTarZst failed: %v", err)
	}

	// Verify the output file exists
	if _, err := os.Stat(outputFile); os.IsNotExist(err) {
		t.Fatalf("Output file was not created")
	}
}

func TestCreateTarGz(t *testing.T) {
	// Create a temporary directory for testing
	tempDir := t.TempDir()
	sourceDir := filepath.Join(tempDir, "source")
	outputFile := filepath.Join(tempDir, "test.tar.gz")

	// Create source directory with a test file
	err := os.MkdirAll(sourceDir, 0o755)
	if err != nil {
		t.Fatalf("Failed to create source directory: %v", err)
	}

	testFile := filepath.Join(sourceDir, "test.txt")

	err = os.WriteFile(testFile, []byte("test content"), 0o644)
	if err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	// Test creating tar.gz archive
	err = CreateTarGz(sourceDir, outputFile, false)
	if err != nil {
		t.Fatalf("CreateTarGz failed: %v", err)
	}

	// Verify the output file exists
	if _, err := os.Stat(outputFile); os.IsNotExist(err) {
		t.Fatalf("Output file was not created")
	}
}

func TestCreateTarZstWithDirectories(t *testing.T) {
	// Create a temporary directory for testing
	tempDir := t.TempDir()
	sourceDir := filepath.Join(tempDir, "source")
	outputFile := filepath.Join(tempDir, "test.tar.zst")

	// Create source directory with subdirectories
	subDir := filepath.Join(sourceDir, "subdir")

	err := os.MkdirAll(subDir, 0o755)
	if err != nil {
		t.Fatalf("Failed to create subdirectory: %v", err)
	}

	testFile := filepath.Join(subDir, "test.txt")

	err = os.WriteFile(testFile, []byte("test content"), 0o644)
	if err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	// Test creating tar.zst archive with directories
	err = CreateTarZst(sourceDir, outputFile, true)
	if err != nil {
		t.Fatalf("CreateTarZst failed: %v", err)
	}

	// Verify the output file exists
	if _, err := os.Stat(outputFile); os.IsNotExist(err) {
		t.Fatalf("Output file was not created")
	}
}

func TestCreateTarZstInvalidSourceDir(t *testing.T) {
	tempDir := t.TempDir()
	outputFile := filepath.Join(tempDir, "test.tar.zst")
	invalidSourceDir := "/non/existent/directory"

	err := CreateTarZst(invalidSourceDir, outputFile, false)
	if err == nil {
		t.Fatal("Expected error for invalid source directory, got nil")
	}
}

func TestCreateTarGzInvalidSourceDir(t *testing.T) {
	tempDir := t.TempDir()
	outputFile := filepath.Join(tempDir, "test.tar.gz")
	invalidSourceDir := "/non/existent/directory"

	err := CreateTarGz(invalidSourceDir, outputFile, false)
	if err == nil {
		t.Fatal("Expected error for invalid source directory, got nil")
	}
}

func TestExtract(t *testing.T) {
	// Create a temporary directory for testing
	tempDir := t.TempDir()
	sourceDir := filepath.Join(tempDir, "source")
	archiveFile := filepath.Join(tempDir, "test.tar.zst")
	extractDir := filepath.Join(tempDir, "extract")

	// Create source directory with a test file
	err := os.MkdirAll(sourceDir, 0o755)
	if err != nil {
		t.Fatalf("Failed to create source directory: %v", err)
	}

	testFile := filepath.Join(sourceDir, "test.txt")

	err = os.WriteFile(testFile, []byte("test content"), 0o644)
	if err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	// Create archive
	err = CreateTarZst(sourceDir, archiveFile, false)
	if err != nil {
		t.Fatalf("Failed to create archive: %v", err)
	}

	// Create extract directory
	err = os.MkdirAll(extractDir, 0o755)
	if err != nil {
		t.Fatalf("Failed to create extract directory: %v", err)
	}

	// Test extraction
	err = Extract(archiveFile, extractDir)
	if err != nil {
		t.Fatalf("Extract failed: %v", err)
	}

	// Verify extracted file exists
	extractedFile := filepath.Join(extractDir, "test.txt")
	if _, err := os.Stat(extractedFile); os.IsNotExist(err) {
		t.Fatalf("Extracted file was not found")
	}

	// Verify content
	content, err := os.ReadFile(extractedFile)
	if err != nil {
		t.Fatalf("Failed to read extracted file: %v", err)
	}

	if string(content) != "test content" {
		t.Fatalf("Extracted file content mismatch. Got: %s, Expected: test content", string(content))
	}
}

func TestExtractInvalidArchive(t *testing.T) {
	tempDir := t.TempDir()
	extractDir := filepath.Join(tempDir, "extract")
	invalidArchive := "/non/existent/archive.tar.zst"

	err := os.MkdirAll(extractDir, 0o755)
	if err != nil {
		t.Fatalf("Failed to create extract directory: %v", err)
	}

	err = Extract(invalidArchive, extractDir)
	if err == nil {
		t.Fatal("Expected error for invalid archive file, got nil")
	}
}
