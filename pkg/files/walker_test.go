package files

import (
	"os"
	"path/filepath"
	"testing"
)

func TestEntry_IsRegularFile(t *testing.T) {
	entry := &Entry{
		Mode: 0o644, // Regular file mode
	}

	if !entry.IsRegularFile() {
		t.Fatal("Entry should be detected as regular file")
	}

	entry.Mode = os.ModeDir
	if entry.IsRegularFile() {
		t.Fatal("Directory entry should not be detected as regular file")
	}
}

func TestEntry_IsDirectory(t *testing.T) {
	entry := &Entry{
		Mode: os.ModeDir | 0o755,
	}

	if !entry.IsDirectory() {
		t.Fatal("Entry should be detected as directory")
	}

	entry.Mode = 0o644
	if entry.IsDirectory() {
		t.Fatal("Regular file entry should not be detected as directory")
	}
}

func TestEntry_IsSymlink(t *testing.T) {
	entry := &Entry{
		Mode: os.ModeSymlink | 0o777,
	}

	if !entry.IsSymlink() {
		t.Fatal("Entry should be detected as symlink")
	}

	entry.Mode = 0o644
	if entry.IsSymlink() {
		t.Fatal("Regular file entry should not be detected as symlink")
	}
}

func TestEntry_IsConfigFile(t *testing.T) {
	tests := []struct {
		entryType string
		expected  bool
	}{
		{TypeConfig, true},
		{TypeConfigNoReplace, true},
		{TypeFile, false},
		{TypeDir, false},
		{TypeSymlink, false},
	}

	for _, test := range tests {
		entry := &Entry{
			Type: test.entryType,
		}

		result := entry.IsConfigFile()
		if result != test.expected {
			t.Fatalf("Entry with type %s: expected IsConfigFile()=%v, got %v",
				test.entryType, test.expected, result)
		}
	}
}

func TestNewWalker(t *testing.T) {
	baseDir := "/test/dir"
	options := WalkOptions{
		SkipDotFiles: true,
		BackupFiles:  []string{"/etc/config"},
		SkipPatterns: []string{"*.tmp"},
	}

	walker := NewWalker(baseDir, options)

	if walker.BaseDir != baseDir {
		t.Fatalf("Expected BaseDir %s, got %s", baseDir, walker.BaseDir)
	}

	if walker.Options.SkipDotFiles != options.SkipDotFiles {
		t.Fatal("SkipDotFiles option not set correctly")
	}

	if len(walker.Options.BackupFiles) != len(options.BackupFiles) {
		t.Fatal("BackupFiles option not set correctly")
	}

	if len(walker.Options.SkipPatterns) != len(options.SkipPatterns) {
		t.Fatal("SkipPatterns option not set correctly")
	}
}

func TestWalker_Walk(t *testing.T) {
	tempDir := t.TempDir()

	// Create test directory structure
	subDir := filepath.Join(tempDir, "subdir")

	err := os.Mkdir(subDir, 0o755)
	if err != nil {
		t.Fatalf("Failed to create subdirectory: %v", err)
	}

	testFile := filepath.Join(tempDir, "test.txt")

	err = os.WriteFile(testFile, []byte("test content"), 0o644)
	if err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	subFile := filepath.Join(subDir, "subfile.txt")

	err = os.WriteFile(subFile, []byte("sub content"), 0o644)
	if err != nil {
		t.Fatalf("Failed to create sub file: %v", err)
	}

	// Create walker
	options := WalkOptions{
		SkipDotFiles: false,
		BackupFiles:  []string{},
		SkipPatterns: []string{},
	}
	walker := NewWalker(tempDir, options)

	entries, err := walker.Walk()
	if err != nil {
		t.Fatalf("Walk failed: %v", err)
	}

	// Should have at least 2 entries: test.txt, subfile.txt (subdir might be empty and excluded)
	if len(entries) < 2 {
		t.Fatalf("Expected at least 2 entries, got %d", len(entries))
	}

	// Check that we have file entries
	hasFile := false

	for _, entry := range entries {
		if entry.Type == TypeFile {
			hasFile = true
		}
	}

	if !hasFile {
		t.Fatal("Expected to find file entries")
	}
}

func TestWalker_WalkWithSkipDotFiles(t *testing.T) {
	tempDir := t.TempDir()

	// Create dot file
	dotFile := filepath.Join(tempDir, ".hidden")

	err := os.WriteFile(dotFile, []byte("hidden"), 0o644)
	if err != nil {
		t.Fatalf("Failed to create dot file: %v", err)
	}

	regularFile := filepath.Join(tempDir, "regular.txt")

	err = os.WriteFile(regularFile, []byte("regular"), 0o644)
	if err != nil {
		t.Fatalf("Failed to create regular file: %v", err)
	}

	// Create walker that skips dot files
	options := WalkOptions{
		SkipDotFiles: true,
		BackupFiles:  []string{},
		SkipPatterns: []string{},
	}
	walker := NewWalker(tempDir, options)

	entries, err := walker.Walk()
	if err != nil {
		t.Fatalf("Walk failed: %v", err)
	}

	// Check that dot file was skipped
	for _, entry := range entries {
		if filepath.Base(entry.Source) == ".hidden" {
			t.Fatal("Dot file should have been skipped")
		}
	}
}

func TestWalker_WalkWithSkipPatterns(t *testing.T) {
	tempDir := t.TempDir()

	// Create files matching and not matching pattern
	tmpFile := filepath.Join(tempDir, "temp.tmp")

	err := os.WriteFile(tmpFile, []byte("temp"), 0o644)
	if err != nil {
		t.Fatalf("Failed to create tmp file: %v", err)
	}

	regularFile := filepath.Join(tempDir, "regular.txt")

	err = os.WriteFile(regularFile, []byte("regular"), 0o644)
	if err != nil {
		t.Fatalf("Failed to create regular file: %v", err)
	}

	// Create walker that skips .tmp files
	options := WalkOptions{
		SkipDotFiles: false,
		BackupFiles:  []string{},
		SkipPatterns: []string{"*.tmp"},
	}
	walker := NewWalker(tempDir, options)

	entries, err := walker.Walk()
	if err != nil {
		t.Fatalf("Walk failed: %v", err)
	}

	// Check that .tmp file was skipped
	for _, entry := range entries {
		if filepath.Base(entry.Source) == "temp.tmp" {
			t.Fatal("File matching skip pattern should have been skipped")
		}
	}
}

func TestWalker_WalkWithBackupFiles(t *testing.T) {
	tempDir := t.TempDir()

	configFile := filepath.Join(tempDir, "config")

	err := os.WriteFile(configFile, []byte("config"), 0o644)
	if err != nil {
		t.Fatalf("Failed to create config file: %v", err)
	}

	// Create walker with backup files
	options := WalkOptions{
		SkipDotFiles: false,
		BackupFiles:  []string{"/config"},
		SkipPatterns: []string{},
	}
	walker := NewWalker(tempDir, options)

	entries, err := walker.Walk()
	if err != nil {
		t.Fatalf("Walk failed: %v", err)
	}

	// Check that config file is marked as backup
	found := false

	for _, entry := range entries {
		if filepath.Base(entry.Source) == "config" {
			found = true

			if !entry.IsBackup {
				t.Fatal("Config file should be marked as backup")
			}

			if entry.Type != TypeConfigNoReplace {
				t.Fatalf("Expected Type %s, got %s", TypeConfigNoReplace, entry.Type)
			}
		}
	}

	if !found {
		t.Fatal("Config file not found in entries")
	}
}

func TestWalker_WalkWithSymlink(t *testing.T) {
	tempDir := t.TempDir()

	// Create target file and symlink
	targetFile := filepath.Join(tempDir, "target.txt")

	err := os.WriteFile(targetFile, []byte("target"), 0o644)
	if err != nil {
		t.Fatalf("Failed to create target file: %v", err)
	}

	linkFile := filepath.Join(tempDir, "link.txt")

	err = os.Symlink(targetFile, linkFile)
	if err != nil {
		t.Fatalf("Failed to create symlink: %v", err)
	}

	options := WalkOptions{}
	walker := NewWalker(tempDir, options)

	entries, err := walker.Walk()
	if err != nil {
		t.Fatalf("Walk failed: %v", err)
	}

	// Find symlink entry
	found := false

	for _, entry := range entries {
		if filepath.Base(entry.Source) == "link.txt" {
			found = true

			if entry.Type != TypeSymlink {
				t.Fatalf("Expected Type %s, got %s", TypeSymlink, entry.Type)
			}

			if entry.LinkTarget != targetFile {
				t.Fatalf("Expected LinkTarget %s, got %s", targetFile, entry.LinkTarget)
			}
		}
	}

	if !found {
		t.Fatal("Symlink entry not found")
	}
}

func TestCalculateDataHash(t *testing.T) {
	tempDir := t.TempDir()

	// Create test files
	file1 := filepath.Join(tempDir, "file1.txt")

	err := os.WriteFile(file1, []byte("content1"), 0o644)
	if err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	file2 := filepath.Join(tempDir, "file2.txt")

	err = os.WriteFile(file2, []byte("content2"), 0o644)
	if err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	hash, err := CalculateDataHash(tempDir, []string{})
	if err != nil {
		t.Fatalf("CalculateDataHash failed: %v", err)
	}

	if hash == "" {
		t.Fatal("Hash should not be empty")
	}

	if len(hash) != 64 { // SHA256 hex string length
		t.Fatalf("Expected hash length 64, got %d", len(hash))
	}
}

func TestCalculateDataHashWithSkipPatterns(t *testing.T) {
	tempDir := t.TempDir()

	// Create files, one matching skip pattern
	file1 := filepath.Join(tempDir, "file1.txt")

	err := os.WriteFile(file1, []byte("content1"), 0o644)
	if err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	tmpFile := filepath.Join(tempDir, "temp.tmp")

	err = os.WriteFile(tmpFile, []byte("temp content"), 0o644)
	if err != nil {
		t.Fatalf("Failed to create tmp file: %v", err)
	}

	hash1, err := CalculateDataHash(tempDir, []string{})
	if err != nil {
		t.Fatalf("CalculateDataHash failed: %v", err)
	}

	hash2, err := CalculateDataHash(tempDir, []string{"*.tmp"})
	if err != nil {
		t.Fatalf("CalculateDataHash with skip patterns failed: %v", err)
	}

	// Hashes should be different when skipping files
	if hash1 == hash2 {
		t.Fatal("Hashes should be different when skipping files")
	}
}

func TestCalculateDataHashNonExistent(t *testing.T) {
	_, err := CalculateDataHash("/non/existent/directory", []string{})
	if err == nil {
		t.Fatal("Expected error for non-existent directory, got nil")
	}
}
