package files

import (
	"os"
	"testing"
	"time"
)

func TestFileTypeConstants(t *testing.T) {
	expectedConstants := map[string]any{
		"TagLink":             int(TagLink),
		"TagDirectory":        int(TagDirectory),
		"TypeFile":            TypeFile,
		"TypeDir":             TypeDir,
		"TypeImplicitDir":     TypeImplicitDir,
		"TypeSymlink":         TypeSymlink,
		"TypeTree":            TypeTree,
		"TypeConfig":          TypeConfig,
		"TypeConfigNoReplace": TypeConfigNoReplace,
	}

	actualConstants := map[string]any{
		"TagLink":             int(TagLink),
		"TagDirectory":        int(TagDirectory),
		"TypeFile":            TypeFile,
		"TypeDir":             TypeDir,
		"TypeImplicitDir":     TypeImplicitDir,
		"TypeSymlink":         TypeSymlink,
		"TypeTree":            TypeTree,
		"TypeConfig":          TypeConfig,
		"TypeConfigNoReplace": TypeConfigNoReplace,
	}

	for name, expected := range expectedConstants {
		if actual := actualConstants[name]; actual != expected {
			t.Errorf("Constant %s = %v, want %v", name, actual, expected)
		}
	}
}

func TestEntryIsRegularFileExtended(t *testing.T) {
	tests := []struct {
		name     string
		mode     os.FileMode
		expected bool
	}{
		{"regular file", 0o644, true},
		{"executable file", 0o755, true},
		{"directory", os.ModeDir | 0o755, false},
		{"symlink", os.ModeSymlink | 0o777, false},
		{"device", os.ModeDevice | 0o644, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			entry := &Entry{Mode: tt.mode}
			if result := entry.IsRegularFile(); result != tt.expected {
				t.Errorf("IsRegularFile() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestEntryIsDirectoryExtended(t *testing.T) {
	tests := []struct {
		name     string
		mode     os.FileMode
		expected bool
	}{
		{"regular file", 0o644, false},
		{"directory", os.ModeDir | 0o755, true},
		{"symlink", os.ModeSymlink | 0o777, false},
		{"symlink to directory", os.ModeSymlink | 0o777, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			entry := &Entry{Mode: tt.mode}
			if result := entry.IsDirectory(); result != tt.expected {
				t.Errorf("IsDirectory() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestEntryIsSymlinkExtended(t *testing.T) {
	tests := []struct {
		name     string
		mode     os.FileMode
		expected bool
	}{
		{"regular file", 0o644, false},
		{"directory", os.ModeDir | 0o755, false},
		{"symlink", os.ModeSymlink | 0o777, true},
		{"symlink to directory", os.ModeSymlink | os.ModeDir | 0o777, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			entry := &Entry{Mode: tt.mode}
			if result := entry.IsSymlink(); result != tt.expected {
				t.Errorf("IsSymlink() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestEntryIsConfigFileExtended(t *testing.T) {
	tests := []struct {
		name     string
		fileType string
		expected bool
	}{
		{"regular file", TypeFile, false},
		{"directory", TypeDir, false},
		{"config file", TypeConfig, true},
		{"config no replace", TypeConfigNoReplace, true},
		{"symlink", TypeSymlink, false},
		{"empty type", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			entry := &Entry{Type: tt.fileType}
			if result := entry.IsConfigFile(); result != tt.expected {
				t.Errorf("IsConfigFile() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestEntryStruct(t *testing.T) {
	now := time.Now()
	sha256 := []byte{0x01, 0x02, 0x03}

	entry := &Entry{
		Source:      "/path/to/source",
		Destination: "/usr/bin/app",
		Type:        TypeFile,
		Mode:        0o755,
		Size:        1024,
		ModTime:     now,
		LinkTarget:  "",
		SHA256:      sha256,
		IsBackup:    false,
	}

	if entry.Source != "/path/to/source" {
		t.Errorf("Source = %q, want %q", entry.Source, "/path/to/source")
	}

	if entry.Destination != "/usr/bin/app" {
		t.Errorf("Destination = %q, want %q", entry.Destination, "/usr/bin/app")
	}

	if entry.Type != TypeFile {
		t.Errorf("Type = %q, want %q", entry.Type, TypeFile)
	}

	if entry.Mode != 0o755 {
		t.Errorf("Mode = %o, want %o", entry.Mode, 0o755)
	}

	if entry.Size != 1024 {
		t.Errorf("Size = %d, want %d", entry.Size, 1024)
	}

	if !entry.ModTime.Equal(now) {
		t.Errorf("ModTime = %v, want %v", entry.ModTime, now)
	}

	if len(entry.SHA256) != 3 {
		t.Errorf("SHA256 length = %d, want %d", len(entry.SHA256), 3)
	}

	if entry.IsBackup {
		t.Errorf("IsBackup = %v, want %v", entry.IsBackup, false)
	}
}

func TestEntryMethodsConsistency(t *testing.T) {
	tests := []struct {
		name      string
		mode      os.FileMode
		fileType  string
		isRegular bool
		isDir     bool
		isSymlink bool
		isConfig  bool
	}{
		{"regular file", 0o644, TypeFile, true, false, false, false},
		{"directory", os.ModeDir | 0o755, TypeDir, false, true, false, false},
		{"symlink", os.ModeSymlink | 0o777, TypeSymlink, false, false, true, false},
		{"config file", 0o644, TypeConfig, true, false, false, true},
		{"config no replace", 0o644, TypeConfigNoReplace, true, false, false, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			entry := &Entry{Mode: tt.mode, Type: tt.fileType}

			if result := entry.IsRegularFile(); result != tt.isRegular {
				t.Errorf("IsRegularFile() = %v, want %v", result, tt.isRegular)
			}

			if result := entry.IsDirectory(); result != tt.isDir {
				t.Errorf("IsDirectory() = %v, want %v", result, tt.isDir)
			}

			if result := entry.IsSymlink(); result != tt.isSymlink {
				t.Errorf("IsSymlink() = %v, want %v", result, tt.isSymlink)
			}

			if result := entry.IsConfigFile(); result != tt.isConfig {
				t.Errorf("IsConfigFile() = %v, want %v", result, tt.isConfig)
			}
		})
	}
}
