package files

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCalculateSHA256(t *testing.T) {
	tempDir := t.TempDir()
	testFile := filepath.Join(tempDir, "test.txt")
	testContent := "Hello, World!"

	err := os.WriteFile(testFile, []byte(testContent), 0o644)
	if err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	hash, err := CalculateSHA256(testFile)
	if err != nil {
		t.Fatalf("CalculateSHA256 failed: %v", err)
	}

	if len(hash) != 32 {
		t.Fatalf("Expected hash length 32, got %d", len(hash))
	}
}

func TestCalculateSHA256NonExistentFile(t *testing.T) {
	_, err := CalculateSHA256("/non/existent/file")
	if err == nil {
		t.Fatal("Expected error for non-existent file, got nil")
	}
}

func TestCheckWritable(t *testing.T) {
	tempDir := t.TempDir()
	testFile := filepath.Join(tempDir, "test.txt")

	err := os.WriteFile(testFile, []byte("test"), 0o644)
	if err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	err = CheckWritable(testFile)
	if err != nil {
		t.Fatalf("CheckWritable failed: %v", err)
	}
}

func TestCheckWritableNonWritable(t *testing.T) {
	tempDir := t.TempDir()
	testFile := filepath.Join(tempDir, "test.txt")

	err := os.WriteFile(testFile, []byte("test"), 0o444) // Read-only
	if err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	err = CheckWritable(testFile)
	if err == nil {
		t.Fatal("Expected error for non-writable file, got nil")
	}
}

func TestCheckWritableNonExistent(t *testing.T) {
	err := CheckWritable("/non/existent/file")
	if err == nil {
		t.Fatal("Expected error for non-existent file, got nil")
	}
}

func TestChmod(t *testing.T) {
	tempDir := t.TempDir()
	testFile := filepath.Join(tempDir, "test.txt")

	err := os.WriteFile(testFile, []byte("test"), 0o644)
	if err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	err = Chmod(testFile, 0o755)
	if err != nil {
		t.Fatalf("Chmod failed: %v", err)
	}

	info, err := os.Stat(testFile)
	if err != nil {
		t.Fatalf("Failed to stat file: %v", err)
	}

	if info.Mode().Perm() != 0o755 {
		t.Fatalf("Expected file mode 0o755, got %o", info.Mode().Perm())
	}
}

func TestChmodNonExistent(t *testing.T) {
	err := Chmod("/non/existent/file", 0o755)
	if err == nil {
		t.Fatal("Expected error for non-existent file, got nil")
	}
}

func TestCreate(t *testing.T) {
	tempDir := t.TempDir()
	testFile := filepath.Join(tempDir, "test.txt")

	file, err := Create(testFile)
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	defer func() { _ = file.Close() }()

	if _, err := os.Stat(testFile); os.IsNotExist(err) {
		t.Fatal("File was not created")
	}
}

func TestCreateWrite(t *testing.T) {
	tempDir := t.TempDir()
	testFile := filepath.Join(tempDir, "test.txt")
	testContent := "Hello, World!"

	err := CreateWrite(testFile, testContent)
	if err != nil {
		t.Fatalf("CreateWrite failed: %v", err)
	}

	content, err := os.ReadFile(testFile)
	if err != nil {
		t.Fatalf("Failed to read file: %v", err)
	}

	if string(content) != testContent {
		t.Fatalf("Expected content '%s', got '%s'", testContent, string(content))
	}
}

func TestExists(t *testing.T) {
	tempDir := t.TempDir()
	testFile := filepath.Join(tempDir, "test.txt")

	// File doesn't exist yet
	if Exists(testFile) {
		t.Fatal("File should not exist yet")
	}

	// Create file
	err := os.WriteFile(testFile, []byte("test"), 0o644)
	if err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	// File should exist now
	if !Exists(testFile) {
		t.Fatal("File should exist now")
	}

	// Directory should also work
	if !Exists(tempDir) {
		t.Fatal("Directory should exist")
	}
}

func TestExistsMakeDir(t *testing.T) {
	tempDir := t.TempDir()
	newDir := filepath.Join(tempDir, "new", "nested", "dir")

	err := ExistsMakeDir(newDir)
	if err != nil {
		t.Fatalf("ExistsMakeDir failed: %v", err)
	}

	if !Exists(newDir) {
		t.Fatal("Directory was not created")
	}

	// Should not error if directory already exists
	err = ExistsMakeDir(newDir)
	if err != nil {
		t.Fatalf("ExistsMakeDir failed for existing directory: %v", err)
	}
}

func TestFilename(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"/path/to/file.txt", "file.txt"},
		{"file.txt", "file.txt"},
		{"/path/to/", ""},
		{"", ""},
		{"/single", "single"},
	}

	for _, test := range tests {
		result := Filename(test.input)
		if result != test.expected {
			t.Fatalf("Filename(%q) = %q, expected %q", test.input, result, test.expected)
		}
	}
}

func TestGetDirSize(t *testing.T) {
	tempDir := t.TempDir()

	// Create some test files
	file1 := filepath.Join(tempDir, "file1.txt")
	file2 := filepath.Join(tempDir, "file2.txt")

	err := os.WriteFile(file1, []byte("Hello"), 0o644) // 5 bytes
	if err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	err = os.WriteFile(file2, []byte("World!"), 0o644) // 6 bytes
	if err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	size, err := GetDirSize(tempDir)
	if err != nil {
		t.Fatalf("GetDirSize failed: %v", err)
	}

	expectedSize := int64(11) // 5 + 6 bytes
	if size != expectedSize {
		t.Fatalf("Expected size %d, got %d", expectedSize, size)
	}
}

func TestGetDirSizeNonExistent(t *testing.T) {
	_, err := GetDirSize("/non/existent/directory")
	if err == nil {
		t.Fatal("Expected error for non-existent directory, got nil")
	}
}

func TestGetFileType(t *testing.T) {
	tempDir := t.TempDir()
	testFile := filepath.Join(tempDir, "test.txt")

	err := os.WriteFile(testFile, []byte("test"), 0o644)
	if err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	// This will return empty string for non-ELF files, which is expected
	fileType := GetFileType(testFile)
	// We don't assert a specific value since it depends on the file content
	_ = fileType
}

func TestGetFileTypeSymlink(t *testing.T) {
	tempDir := t.TempDir()
	targetFile := filepath.Join(tempDir, "target.txt")
	linkFile := filepath.Join(tempDir, "link.txt")

	err := os.WriteFile(targetFile, []byte("test"), 0o644)
	if err != nil {
		t.Fatalf("Failed to create target file: %v", err)
	}

	err = os.Symlink(targetFile, linkFile)
	if err != nil {
		t.Fatalf("Failed to create symlink: %v", err)
	}

	fileType := GetFileType(linkFile)
	if fileType != "" {
		t.Fatal("Symlinks should return empty file type")
	}
}

func TestGetFileTypeNonExistent(t *testing.T) {
	fileType := GetFileType("/non/existent/file")
	if fileType != "" {
		t.Fatal("Non-existent files should return empty file type")
	}
}

func TestIsEmptyDir(t *testing.T) {
	tempDir := t.TempDir()

	// Create empty directory
	emptyDir := filepath.Join(tempDir, "empty")

	err := os.Mkdir(emptyDir, 0o755)
	if err != nil {
		t.Fatalf("Failed to create empty directory: %v", err)
	}

	// Create non-empty directory
	nonEmptyDir := filepath.Join(tempDir, "nonempty")

	err = os.Mkdir(nonEmptyDir, 0o755)
	if err != nil {
		t.Fatalf("Failed to create non-empty directory: %v", err)
	}

	testFile := filepath.Join(nonEmptyDir, "file.txt")

	err = os.WriteFile(testFile, []byte("test"), 0o644)
	if err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	// Test with DirEntry
	entries, err := os.ReadDir(tempDir)
	if err != nil {
		t.Fatalf("Failed to read directory: %v", err)
	}

	var emptyDirEntry, nonEmptyDirEntry os.DirEntry

	for _, entry := range entries {
		if entry.Name() == "empty" {
			emptyDirEntry = entry
		} else if entry.Name() == "nonempty" {
			nonEmptyDirEntry = entry
		}
	}

	if !IsEmptyDir(emptyDir, emptyDirEntry) {
		t.Fatal("Empty directory should be detected as empty")
	}

	if IsEmptyDir(nonEmptyDir, nonEmptyDirEntry) {
		t.Fatal("Non-empty directory should not be detected as empty")
	}
}

func TestIsStaticLibrary(t *testing.T) {
	tempDir := t.TempDir()

	// Test with .a extension
	aFile := filepath.Join(tempDir, "test.a")

	err := os.WriteFile(aFile, []byte("test"), 0o644)
	if err != nil {
		t.Fatalf("Failed to create .a file: %v", err)
	}

	if !IsStaticLibrary(aFile) {
		t.Fatal("File with .a extension should be detected as static library")
	}

	// Test with archive magic number
	archiveFile := filepath.Join(tempDir, "archive")

	err = os.WriteFile(archiveFile, []byte("!<arch>\ntest"), 0o644)
	if err != nil {
		t.Fatalf("Failed to create archive file: %v", err)
	}

	if !IsStaticLibrary(archiveFile) {
		t.Fatal("File with archive magic number should be detected as static library")
	}

	// Test with regular file
	regularFile := filepath.Join(tempDir, "regular.txt")

	err = os.WriteFile(regularFile, []byte("test"), 0o644)
	if err != nil {
		t.Fatalf("Failed to create regular file: %v", err)
	}

	if IsStaticLibrary(regularFile) {
		t.Fatal("Regular file should not be detected as static library")
	}
}

func TestIsStaticLibraryNonExistent(t *testing.T) {
	if IsStaticLibrary("/non/existent/file") {
		t.Fatal("Non-existent file should not be detected as static library")
	}
}

func TestOpen(t *testing.T) {
	tempDir := t.TempDir()
	testFile := filepath.Join(tempDir, "test.txt")
	testContent := "Hello, World!"

	err := os.WriteFile(testFile, []byte(testContent), 0o644)
	if err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	file, err := Open(testFile)
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}

	defer func() { _ = file.Close() }()

	content := make([]byte, len(testContent))

	_, err = file.Read(content)
	if err != nil {
		t.Fatalf("Failed to read file: %v", err)
	}

	if string(content) != testContent {
		t.Fatalf("Expected content '%s', got '%s'", testContent, string(content))
	}
}

func TestOpenNonExistent(t *testing.T) {
	_, err := Open("/non/existent/file")
	if err == nil {
		t.Fatal("Expected error for non-existent file, got nil")
	}
}
