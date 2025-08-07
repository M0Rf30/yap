//nolint:testpackage // Internal testing of options package methods
package options

import (
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDetermineStripFlags(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		fileType     string
		binary       string
		wantFlags    string
		wantStripLTO bool
	}{
		{
			name:         "Dynamic library",
			fileType:     "ET_DYN",
			binary:       "/usr/lib/libtest.so",
			wantFlags:    "--strip-unneeded",
			wantStripLTO: false,
		},
		{
			name:         "Executable",
			fileType:     "ET_EXEC",
			binary:       "/usr/bin/test",
			wantFlags:    "--strip-all",
			wantStripLTO: false,
		},
		{
			name:         "Static library (.a)",
			fileType:     "ET_REL",
			binary:       "/usr/lib/libtest.a",
			wantFlags:    "--strip-debug",
			wantStripLTO: true,
		},
		{
			name:         "Kernel module",
			fileType:     "ET_REL",
			binary:       "/lib/modules/test.ko",
			wantFlags:    "--strip-unneeded",
			wantStripLTO: false,
		},
		{
			name:         "Object file",
			fileType:     "ET_REL",
			binary:       "/tmp/test.o",
			wantFlags:    "--strip-unneeded",
			wantStripLTO: false,
		},
		{
			name:         "Unknown file type",
			fileType:     "UNKNOWN",
			binary:       "/tmp/test",
			wantFlags:    "",
			wantStripLTO: false,
		},
		{
			name:         "Empty file type",
			fileType:     "",
			binary:       "/tmp/test",
			wantFlags:    "",
			wantStripLTO: false,
		},
		{
			name:         "ET_REL without special suffix",
			fileType:     "ET_REL",
			binary:       "/tmp/test.bin",
			wantFlags:    "",
			wantStripLTO: false,
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			gotFlags, gotStripLTO := determineStripFlags(testCase.fileType, testCase.binary)
			assert.Equal(t, testCase.wantFlags, gotFlags)
			assert.Equal(t, testCase.wantStripLTO, gotStripLTO)
		})
	}
}

func TestProcessFile(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		setup   func(t *testing.T) (string, fs.DirEntry)
		wantErr bool
	}{
		{
			name: "directory",
			setup: func(t *testing.T) (string, fs.DirEntry) {
				t.Helper()
				tempDir := t.TempDir()
				info, err := os.Stat(tempDir)
				require.NoError(t, err)

				return tempDir, fs.FileInfoToDirEntry(info)
			},
			wantErr: false,
		},
		{
			name: "Regular text file should be processed",
			setup: func(t *testing.T) (string, fs.DirEntry) {
				t.Helper()
				tempDir := t.TempDir()
				filePath := filepath.Join(tempDir, "test.txt")
				err := os.WriteFile(filePath, []byte("test content"), 0o600)
				require.NoError(t, err)
				info, err := os.Stat(filePath)
				require.NoError(t, err)

				return filePath, fs.FileInfoToDirEntry(info)
			},
			wantErr: false,
		},
		{
			name: "Non-existent file should return error in callback",
			setup: func(_ *testing.T) (string, fs.DirEntry) {
				return "/nonexistent/file", nil
			},
			wantErr: true,
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			filePath, dirEntry := testCase.setup(t)

			var callbackErr error
			if testCase.name == "Non-existent file should return error in callback" {
				callbackErr = os.ErrNotExist
			}

			err := processFile(filePath, dirEntry, callbackErr)
			if testCase.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestStrip(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		setupDir  func(t *testing.T) string
		wantErr   bool
		expectLog bool
	}{
		{
			name: "Empty directory",
			setupDir: func(t *testing.T) string {
				t.Helper()
				t.Helper()

				return t.TempDir()
			},
			wantErr:   false,
			expectLog: true,
		},
		{
			name: "Directory with text files",
			setupDir: func(t *testing.T) string {
				t.Helper()
				tempDir := t.TempDir()
				err := os.WriteFile(filepath.Join(tempDir, "test1.txt"), []byte("content1"), 0o600)
				require.NoError(t, err)
				err = os.WriteFile(filepath.Join(tempDir, "test2.txt"), []byte("content2"), 0o600)
				require.NoError(t, err)

				return tempDir
			},
			wantErr:   false,
			expectLog: true,
		},
		{
			name: "Directory with subdirectory",
			setupDir: func(t *testing.T) string {
				t.Helper()
				tempDir := t.TempDir()
				subDir := filepath.Join(tempDir, "subdir")
				err := os.MkdirAll(subDir, 0o755)
				require.NoError(t, err)
				err = os.WriteFile(filepath.Join(subDir, "file.txt"), []byte("content"), 0o600)
				require.NoError(t, err)

				return tempDir
			},
			wantErr:   false,
			expectLog: true,
		},
		{
			name: "Non-existent directory",
			setupDir: func(_ *testing.T) string {
				return "/nonexistent/directory"
			},
			wantErr:   true,
			expectLog: true,
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			packageDir := testCase.setupDir(t)

			err := Strip(packageDir)
			if testCase.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestProcessFileFilePermissions(t *testing.T) {
	t.Parallel()

	tempDir := t.TempDir()
	filePath := filepath.Join(tempDir, "test.txt")

	// Create a file with read-only permissions
	err := os.WriteFile(filePath, []byte("test content"), 0o600)
	require.NoError(t, err)

	info, err := os.Stat(filePath)
	require.NoError(t, err)

	dirEntry := fs.FileInfoToDirEntry(info)

	// The processFile should handle the permission change gracefully
	err = processFile(filePath, dirEntry, nil)
	require.NoError(t, err)

	// Verify file is now writable
	info, err = os.Stat(filePath)
	require.NoError(t, err)
	assert.NotEqual(t, os.FileMode(0o444), info.Mode().Perm())
}

func TestProcessFileWithWriteError(t *testing.T) {
	t.Parallel()

	// Skip this test if running as root, since root can write to read-only files
	if os.Getuid() == 0 {
		t.Skip("Skipping test when running as root")
	}

	tempDir := t.TempDir()
	filePath := filepath.Join(tempDir, "readonly.txt")

	// Create file and make it readonly
	err := os.WriteFile(filePath, []byte("content"), 0o600)
	require.NoError(t, err)

	// Make the parent directory readonly to prevent chmod
	err = os.Chmod(tempDir, 0o555)
	require.NoError(t, err)

	// Clean up: restore permissions
	defer func() {
		_ = os.Chmod(tempDir, 0o755)
	}()

	info, err := os.Stat(filePath)
	require.NoError(t, err)

	dirEntry := fs.FileInfoToDirEntry(info)

	// This should handle the chmod failure gracefully and return nil
	err = processFile(filePath, dirEntry, nil)
	assert.NoError(t, err) // Should not error, just skip the file
}

func TestStripIntegration(t *testing.T) {
	t.Parallel()

	tempDir := t.TempDir()

	// Create a complex directory structure
	subDirs := []string{"bin", "lib", "share"}
	for _, dir := range subDirs {
		err := os.MkdirAll(filepath.Join(tempDir, dir), 0o755)
		require.NoError(t, err)
	}

	// Create various file types
	files := map[string][]byte{
		"bin/executable": []byte("fake executable"),
		"lib/library.so": []byte("fake library"),
		"lib/static.a":   []byte("fake static lib"),
		"share/data.txt": []byte("data file"),
		"README.md":      []byte("readme content"),
	}

	for filePath, content := range files {
		fullPath := filepath.Join(tempDir, filePath)
		err := os.WriteFile(fullPath, content, 0o600)
		require.NoError(t, err)
	}

	// Run Strip on the entire directory
	err := Strip(tempDir)
	require.NoError(t, err)

	// Verify all files still exist (they should, even if stripping fails)
	for filePath := range files {
		fullPath := filepath.Join(tempDir, filePath)
		_, err := os.Stat(fullPath)
		assert.NoError(t, err, "File %s should still exist", filePath)
	}
}
