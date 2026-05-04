package pkgbuild

import (
	"os"
	"path/filepath"
	"testing"
)

func TestReadChangelog_Empty(t *testing.T) {
	p := &PKGBUILD{
		Changelog: "",
		StartDir:  "/tmp",
	}

	data, err := p.ReadChangelog()
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if data != nil {
		t.Fatalf("expected nil data, got %v", data)
	}
}

func TestReadChangelog_FileNotFound(t *testing.T) {
	p := &PKGBUILD{
		Changelog: "nonexistent.txt",
		StartDir:  "/tmp",
	}

	_, err := p.ReadChangelog()
	if err == nil {
		t.Fatal("expected error for nonexistent file")
	}
}

func TestReadChangelog_Success(t *testing.T) {
	// Create a temporary directory and file
	tmpDir := t.TempDir()
	changelogPath := filepath.Join(tmpDir, "CHANGELOG.md")
	changelogContent := "# Changelog\n\n## Version 1.0\n- Initial release\n"

	err := os.WriteFile(changelogPath, []byte(changelogContent), 0o644)
	if err != nil {
		t.Fatalf("failed to create test changelog: %v", err)
	}

	p := &PKGBUILD{
		Changelog: "CHANGELOG.md",
		StartDir:  tmpDir,
	}

	data, err := p.ReadChangelog()
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if string(data) != changelogContent {
		t.Fatalf("expected %q, got %q", changelogContent, string(data))
	}
}

func TestReadChangelog_AbsolutePath(t *testing.T) {
	// Create a temporary directory and file
	tmpDir := t.TempDir()
	changelogPath := filepath.Join(tmpDir, "CHANGELOG.md")
	changelogContent := "# Changelog\n\n## Version 1.0\n- Initial release\n"

	err := os.WriteFile(changelogPath, []byte(changelogContent), 0o644)
	if err != nil {
		t.Fatalf("failed to create test changelog: %v", err)
	}

	p := &PKGBUILD{
		Changelog: changelogPath,
		StartDir:  "/some/other/dir",
	}

	data, err := p.ReadChangelog()
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if string(data) != changelogContent {
		t.Fatalf("expected %q, got %q", changelogContent, string(data))
	}
}
