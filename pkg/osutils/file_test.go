package osutils_test

import (
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/M0Rf30/yap/v2/pkg/osutils"
)

const nonexistentFilePath = "/nonexistent/file.txt"

type fileTestCase struct {
	name        string
	setupFile   func(t *testing.T) string
	expectError bool
}

func getStandardFileTestCases() []fileTestCase {
	return []fileTestCase{
		{
			name: "Valid file",
			setupFile: func(t *testing.T) string {
				t.Helper()
				tempDir := t.TempDir()
				filePath := filepath.Join(tempDir, "test.txt")
				err := os.WriteFile(filePath, []byte("content"), 0o600)
				require.NoError(t, err)

				return filePath
			},
			expectError: false,
		},
		{
			name: "Non-existent file",
			setupFile: func(_ *testing.T) string {
				return nonexistentFilePath
			},
			expectError: true,
		},
	}
}

func TestCalculateSHA256(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name        string
		setupFile   func(t *testing.T) string
		expectError bool
	}{
		{
			name: "Valid file",
			setupFile: func(_ *testing.T) string {
				tempDir := t.TempDir()
				filePath := filepath.Join(tempDir, "test.txt")
				err := os.WriteFile(filePath, []byte("test content"), 0o600)
				require.NoError(t, err)

				return filePath
			},
			expectError: false,
		},
		{
			name: "Empty file",
			setupFile: func(_ *testing.T) string {
				tempDir := t.TempDir()
				filePath := filepath.Join(tempDir, "empty.txt")
				err := os.WriteFile(filePath, []byte{}, 0o600)
				require.NoError(t, err)

				return filePath
			},
			expectError: false,
		},
		{
			name: "Non-existent file",
			setupFile: func(_ *testing.T) string {
				return nonexistentFilePath
			},
			expectError: true,
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			filePath := testCase.setupFile(t)
			hash, err := osutils.CalculateSHA256(filePath)

			if testCase.expectError {
				require.Error(t, err)
				assert.Nil(t, hash)
			} else {
				require.NoError(t, err)
				assert.NotNil(t, hash)
				assert.Len(t, hash, 32) // SHA256 produces 32 bytes
			}
		})
	}
}

func TestCheckWritable(t *testing.T) {
	t.Parallel()

	tests := getStandardFileTestCases()
	// Update the test case names for this specific function
	tests[0].name = "Writable file"

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			filePath := testCase.setupFile(t)

			err := osutils.CheckWritable(filePath)

			if testCase.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestChmod(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name        string
		setupFile   func(t *testing.T) string
		perm        os.FileMode
		expectError bool
	}{
		{
			name: "Change permissions successfully",
			setupFile: func(_ *testing.T) string {
				tempDir := t.TempDir()
				filePath := filepath.Join(tempDir, "test.txt")
				err := os.WriteFile(filePath, []byte("content"), 0o600)
				require.NoError(t, err)

				return filePath
			},
			perm:        0o755,
			expectError: false,
		},
		{
			name: "Non-existent file",
			setupFile: func(_ *testing.T) string {
				return nonexistentFilePath
			},
			perm:        0o600,
			expectError: true,
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			filePath := testCase.setupFile(t)
			err := osutils.Chmod(filePath, testCase.perm)

			if testCase.expectError {
				assert.Error(t, err)
			} else {
				require.NoError(t, err)

				// Verify permissions were changed
				info, err := os.Stat(filePath)
				require.NoError(t, err)
				assert.Equal(t, testCase.perm, info.Mode().Perm())
			}
		})
	}
}

func TestCreate(t *testing.T) {
	t.Parallel()
	tempDir := t.TempDir()
	filePath := filepath.Join(tempDir, "newfile.txt")

	file, err := osutils.Create(filePath)
	require.NoError(t, err)
	assert.NotNil(t, file)

	err = file.Close()
	require.NoError(t, err)

	// Verify file was created
	_, err = os.Stat(filePath)
	assert.NoError(t, err)
}

func TestCreateWrite(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name        string
		setupPath   func(t *testing.T) string
		data        string
		expectError bool
	}{
		{
			name: "osutils.Create and write successfully",
			setupPath: func(_ *testing.T) string {
				tempDir := t.TempDir()

				return filepath.Join(tempDir, "test.txt")
			},
			data:        "test data",
			expectError: false,
		},
		{
			name: "Write to invalid path",
			setupPath: func(_ *testing.T) string {
				return "/root/unauthorized/file.txt" // Should fail on most systems
			},
			data:        "test data",
			expectError: true,
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			filePath := testCase.setupPath(t)
			err := osutils.CreateWrite(filePath, testCase.data)

			if testCase.expectError {
				assert.Error(t, err)
			} else {
				require.NoError(t, err)

				// Verify file content
				content, err := os.ReadFile(filePath)
				require.NoError(t, err)
				assert.Equal(t, testCase.data, string(content))
			}
		})
	}
}

func TestExists(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name      string
		setupPath func(t *testing.T) string
		expected  bool
	}{
		{
			name: "Existing file",
			setupPath: func(_ *testing.T) string {
				tempDir := t.TempDir()
				filePath := filepath.Join(tempDir, "exists.txt")
				err := os.WriteFile(filePath, []byte("content"), 0o600)
				require.NoError(t, err)

				return filePath
			},
			expected: true,
		},
		{
			name: "Existing directory",
			setupPath: func(_ *testing.T) string {
				return t.TempDir()
			},
			expected: true,
		},
		{
			name: "Non-existent path",
			setupPath: func(_ *testing.T) string {
				return "/nonexistent/path"
			},
			expected: false,
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			path := testCase.setupPath(t)
			result := osutils.Exists(path)
			assert.Equal(t, testCase.expected, result)
		})
	}
}

func TestExistsMakeDir(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name        string
		setupPath   func(t *testing.T) string
		expectError bool
	}{
		{
			name: "osutils.Create new directory",
			setupPath: func(_ *testing.T) string {
				tempDir := t.TempDir()

				return filepath.Join(tempDir, "newdir")
			},
			expectError: false,
		},
		{
			name: "Existing directory",
			setupPath: func(_ *testing.T) string {
				return t.TempDir()
			},
			expectError: false,
		},
		{
			name: "osutils.Create nested directories",
			setupPath: func(_ *testing.T) string {
				tempDir := t.TempDir()

				return filepath.Join(tempDir, "deep", "nested", "path")
			},
			expectError: false,
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			path := testCase.setupPath(t)
			err := osutils.ExistsMakeDir(path)

			if testCase.expectError {
				assert.Error(t, err)
			} else {
				require.NoError(t, err)

				// Verify directory exists
				info, err := os.Stat(path)
				require.NoError(t, err)
				assert.True(t, info.IsDir())
			}
		})
	}
}

func TestFilename(t *testing.T) {
	t.Parallel()

	tests := []struct {
		path     string
		expected string
	}{
		{
			path:     "/path/to/file.txt",
			expected: "file.txt",
		},
		{
			path:     "simple-file.zip",
			expected: "simple-file.zip",
		},
		{
			path:     "/path/with/trailing/slash/",
			expected: "",
		},
		{
			path:     "",
			expected: "",
		},
		{
			path:     "no-path-separators",
			expected: "no-path-separators",
		},
		{
			path:     "/single/file",
			expected: "file",
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.path, func(t *testing.T) {
			t.Parallel()

			result := osutils.Filename(testCase.path)
			assert.Equal(t, testCase.expected, result)
		})
	}
}

func TestGetDirSize(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name      string
		setupDir  func(t *testing.T) string
		wantError bool
	}{
		{
			name: "Directory with files",
			setupDir: func(t *testing.T) string {
				t.Helper()
				tempDir := t.TempDir()

				// osutils.Create files of known sizes
				err := os.WriteFile(filepath.Join(tempDir, "file1.txt"), []byte("12345"), 0o600) // 5 bytes
				require.NoError(t, err)
				err = os.WriteFile(filepath.Join(tempDir, "file2.txt"), []byte("1234567890"), 0o600) // 10 bytes
				require.NoError(t, err)

				return tempDir
			},
			wantError: false,
		},
		{
			name: "Empty directory",
			setupDir: func(t *testing.T) string {
				t.Helper()

				return t.TempDir()
			},
			wantError: false,
		},
		{
			name: "Directory with subdirectories",
			setupDir: func(t *testing.T) string {
				t.Helper()
				tempDir := t.TempDir()

				subDir := filepath.Join(tempDir, "subdir")
				err := os.MkdirAll(subDir, 0o755)
				require.NoError(t, err)

				err = os.WriteFile(filepath.Join(subDir, "subfile.txt"), []byte("content"), 0o600)
				require.NoError(t, err)

				return tempDir
			},
			wantError: false,
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			path := testCase.setupDir(t)
			size, err := osutils.GetDirSize(path)

			if testCase.wantError {
				assert.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.GreaterOrEqual(t, size, int64(0))
			}
		})
	}
}

func TestIsEmptyDir(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		setupDir func(t *testing.T) (string, os.DirEntry)
		expected bool
	}{
		{
			name: "Existing empty directory",
			setupDir: func(t *testing.T) (string, os.DirEntry) {
				t.Helper()
				tempDir := t.TempDir()
				info, err := os.Stat(tempDir)
				require.NoError(t, err)

				return tempDir, fs.FileInfoToDirEntry(info)
			},
			expected: true,
		},
		{
			name: "Directory with files",
			setupDir: func(t *testing.T) (string, os.DirEntry) {
				t.Helper()

				tempDir := t.TempDir()
				err := os.WriteFile(filepath.Join(tempDir, "file.txt"), []byte("content"), 0o600)
				require.NoError(t, err)
				info, err := os.Stat(tempDir)
				require.NoError(t, err)

				return tempDir, fs.FileInfoToDirEntry(info)
			},
			expected: false,
		},
		{
			name: "Regular file (not directory)",
			setupDir: func(t *testing.T) (string, os.DirEntry) {
				t.Helper()

				tempDir := t.TempDir()
				filePath := filepath.Join(tempDir, "file.txt")
				err := os.WriteFile(filePath, []byte("content"), 0o600)
				require.NoError(t, err)
				info, err := os.Stat(filePath)
				require.NoError(t, err)

				return filePath, fs.FileInfoToDirEntry(info)
			},
			expected: false,
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			path, dirEntry := testCase.setupDir(t)
			result := osutils.IsEmptyDir(path, dirEntry)
			assert.Equal(t, testCase.expected, result)
		})
	}
}

func TestIsStaticLibrary(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name      string
		setupFile func(t *testing.T) string
		expected  bool
	}{
		{
			name: "File with .a extension",
			setupFile: func(_ *testing.T) string {
				tempDir := t.TempDir()
				filePath := filepath.Join(tempDir, "lib.a")
				err := os.WriteFile(filePath, []byte("!<arch>\n"), 0o600)
				require.NoError(t, err)

				return filePath
			},
			expected: true,
		},
		{
			name: "Archive with magic string",
			setupFile: func(_ *testing.T) string {
				tempDir := t.TempDir()
				filePath := filepath.Join(tempDir, "library")
				err := os.WriteFile(filePath, []byte("!<arch>\nsome content"), 0o600)
				require.NoError(t, err)

				return filePath
			},
			expected: true,
		},
		{
			name: "Regular file without magic",
			setupFile: func(_ *testing.T) string {
				tempDir := t.TempDir()
				filePath := filepath.Join(tempDir, "regular.txt")
				err := os.WriteFile(filePath, []byte("regular content"), 0o600)
				require.NoError(t, err)

				return filePath
			},
			expected: false,
		},
		{
			name: "Non-existent file with .a extension",
			setupFile: func(_ *testing.T) string {
				return "/nonexistent/file.a"
			},
			expected: true, // .a extension takes precedence even if file doesn't exist
		},
		{
			name: "File too small for magic check",
			setupFile: func(_ *testing.T) string {
				tempDir := t.TempDir()
				filePath := filepath.Join(tempDir, "small.a")
				err := os.WriteFile(filePath, []byte("!<"), 0o600)
				require.NoError(t, err)

				return filePath
			},
			expected: true, // .a extension takes precedence
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			path := testCase.setupFile(t)
			result := osutils.IsStaticLibrary(path)
			assert.Equal(t, testCase.expected, result)
		})
	}
}

func TestOpen(t *testing.T) {
	t.Parallel()

	tests := getStandardFileTestCases()

	// Update the test case names for this specific function
	tests[0].name = "Open existing file"
	tests[1].name = "Open non-existent file"

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			filePath := testCase.setupFile(t)
			file, err := osutils.Open(filePath)

			if testCase.expectError {
				require.Error(t, err)
				assert.Nil(t, file)
			} else {
				require.NoError(t, err)
				assert.NotNil(t, file)
				err = file.Close()
				assert.NoError(t, err)
			}
		})
	}
}
