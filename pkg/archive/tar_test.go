package archive_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/M0Rf30/yap/v2/pkg/archive"
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
	err = archive.CreateTarZst(context.Background(), sourceDir, outputFile, false)
	if err != nil {
		t.Fatalf("CreateTarZst failed: %v", err)
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
	err = archive.CreateTarZst(context.Background(), sourceDir, outputFile, true)
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

	err := archive.CreateTarZst(context.Background(), invalidSourceDir, outputFile, false)
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
	err = archive.CreateTarZst(context.Background(), sourceDir, archiveFile, false)
	if err != nil {
		t.Fatalf("Failed to create archive: %v", err)
	}

	// Create extract directory
	err = os.MkdirAll(extractDir, 0o755)
	if err != nil {
		t.Fatalf("Failed to create extract directory: %v", err)
	}

	// Test extraction
	err = archive.Extract(context.Background(), archiveFile, extractDir)
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

	err = archive.Extract(context.Background(), invalidArchive, extractDir)
	if err == nil {
		t.Fatal("Expected error for invalid archive file, got nil")
	}
}

func TestCreateTarCompressedWithGzip(t *testing.T) {
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
	err = archive.CreateTarCompressed(context.Background(), sourceDir, outputFile,
		"gzip", false)
	if err != nil {
		t.Fatalf("CreateTarCompressed with gzip failed: %v", err)
	}

	// Verify the output file exists
	if _, err := os.Stat(outputFile); os.IsNotExist(err) {
		t.Fatalf("Output file was not created")
	}

	// Verify gzip magic bytes (1f 8b)
	file, err := os.Open(outputFile)
	if err != nil {
		t.Fatalf("Failed to open output file: %v", err)
	}

	defer func() { _ = file.Close() }()

	magic := make([]byte, 2)

	_, err = file.Read(magic)
	if err != nil {
		t.Fatalf("Failed to read magic bytes: %v", err)
	}

	if magic[0] != 0x1f || magic[1] != 0x8b {
		t.Fatalf("Invalid gzip magic bytes: got %x %x, expected 1f 8b",
			magic[0], magic[1])
	}
}

func TestCreateTarCompressedWithXz(t *testing.T) {
	// Create a temporary directory for testing
	tempDir := t.TempDir()
	sourceDir := filepath.Join(tempDir, "source")
	outputFile := filepath.Join(tempDir, "test.tar.xz")

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

	// Test creating tar.xz archive
	err = archive.CreateTarCompressed(context.Background(), sourceDir, outputFile,
		"xz", false)
	if err != nil {
		t.Fatalf("CreateTarCompressed with xz failed: %v", err)
	}

	// Verify the output file exists
	if _, err := os.Stat(outputFile); os.IsNotExist(err) {
		t.Fatalf("Output file was not created")
	}

	// Verify xz magic bytes (fd 37 7a 58 5a 00)
	file, err := os.Open(outputFile)
	if err != nil {
		t.Fatalf("Failed to open output file: %v", err)
	}

	defer func() { _ = file.Close() }()

	magic := make([]byte, 6)

	_, err = file.Read(magic)
	if err != nil {
		t.Fatalf("Failed to read magic bytes: %v", err)
	}

	expectedMagic := []byte{0xfd, 0x37, 0x7a, 0x58, 0x5a, 0x00}

	for i, b := range magic {
		if b != expectedMagic[i] {
			t.Fatalf("Invalid xz magic bytes at position %d: got %x, expected %x",
				i, b, expectedMagic[i])
		}
	}
}

func TestCreateTarCompressedWithZstd(t *testing.T) {
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
	err = archive.CreateTarCompressed(context.Background(), sourceDir, outputFile,
		"zstd", false)
	if err != nil {
		t.Fatalf("CreateTarCompressed with zstd failed: %v", err)
	}

	// Verify the output file exists
	if _, err := os.Stat(outputFile); os.IsNotExist(err) {
		t.Fatalf("Output file was not created")
	}
}

func TestCreateTarCompressedInvalidCompression(t *testing.T) {
	tempDir := t.TempDir()
	sourceDir := filepath.Join(tempDir, "source")
	outputFile := filepath.Join(tempDir, "test.tar")

	err := os.MkdirAll(sourceDir, 0o755)
	if err != nil {
		t.Fatalf("Failed to create source directory: %v", err)
	}

	// Test with invalid compression algorithm
	err = archive.CreateTarCompressed(context.Background(), sourceDir, outputFile,
		"invalid", false)
	if err == nil {
		t.Fatal("Expected error for invalid compression algorithm, got nil")
	}
}

func TestCreateTarCompressedDefaultCompression(t *testing.T) {
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

	// Test creating tar archive with empty compression (should default to zstd)
	err = archive.CreateTarCompressed(context.Background(), sourceDir, outputFile,
		"", false)
	if err != nil {
		t.Fatalf("CreateTarCompressed with empty compression failed: %v", err)
	}

	// Verify the output file exists
	if _, err := os.Stat(outputFile); os.IsNotExist(err) {
		t.Fatalf("Output file was not created")
	}
}
