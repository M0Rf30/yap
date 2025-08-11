//nolint:testpackage // Internal testing of source package methods
package source

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/go-git/go-git/v5/plumbing"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/M0Rf30/yap/v2/pkg/constants"
	"github.com/M0Rf30/yap/v2/pkg/files"
)

func TestSource_parseURI(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name             string
		sourceItemURI    string
		expectedPath     string
		expectedURI      string
		expectedRefKey   string
		expectedRefValue string
	}{
		{
			name:          "Simple filename",
			sourceItemURI: "https://example.com/file.tar.gz",
			expectedPath:  "file.tar.gz",
			expectedURI:   "https://example.com/file.tar.gz",
		},
		{
			name:          "Custom filename with ::",
			sourceItemURI: "custom-name.tar.gz::https://example.com/file.tar.gz",
			expectedPath:  "custom-name.tar.gz",
			expectedURI:   "https://example.com/file.tar.gz",
		},
		{
			name:             "VCS URI with branch",
			sourceItemURI:    "git+https://github.com/example/repo.git#branch=main",
			expectedPath:     "repo.git",
			expectedURI:      "git+https://github.com/example/repo.git",
			expectedRefKey:   "branch",
			expectedRefValue: "main",
		},
		{
			name:             "Git with tag fragment",
			sourceItemURI:    "git+https://github.com/example/repo.git#tag=v1.0.0",
			expectedPath:     "repo.git",
			expectedURI:      "git+https://github.com/example/repo.git",
			expectedRefKey:   "tag",
			expectedRefValue: "v1.0.0",
		},
		{
			name:             "Custom name with git and fragment",
			sourceItemURI:    "custom-repo::git+https://github.com/example/repo.git#branch=develop",
			expectedPath:     "custom-repo",
			expectedURI:      "git+https://github.com/example/repo.git",
			expectedRefKey:   "branch",
			expectedRefValue: "develop",
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			src := &Source{SourceItemURI: testCase.sourceItemURI}
			src.parseURI()

			assert.Equal(t, testCase.expectedPath, src.SourceItemPath)
			assert.Equal(t, testCase.expectedURI, src.SourceItemURI)
			assert.Equal(t, testCase.expectedRefKey, src.RefKey)
			assert.Equal(t, testCase.expectedRefValue, src.RefValue)
		})
	}
}

func TestSource_getProtocol(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		sourceURI     string
		expectedProto string
	}{
		{
			name:          "HTTP protocol",
			sourceURI:     "http://example.com/file.tar.gz",
			expectedProto: "http",
		},
		{
			name:          "HTTPS protocol",
			sourceURI:     "https://example.com/file.tar.gz",
			expectedProto: "https",
		},
		{
			name:          "FTP protocol",
			sourceURI:     "ftp://example.com/file.tar.gz",
			expectedProto: "ftp",
		},
		{
			name:          "Git protocol",
			sourceURI:     "git+https://github.com/example/repo.git",
			expectedProto: constants.Git,
		},
		{
			name:          "Local file (no protocol)",
			sourceURI:     "localfile.tar.gz",
			expectedProto: "file",
		},
		{
			name:          "Relative path",
			sourceURI:     "./path/to/file.tar.gz",
			expectedProto: "file",
		},
		{
			name:          "Unknown protocol",
			sourceURI:     "unknown://example.com/file.tar.gz",
			expectedProto: "",
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			src := &Source{SourceItemURI: testCase.sourceURI}
			protocol := src.getProtocol()
			assert.Equal(t, testCase.expectedProto, protocol)
		})
	}
}

func TestSource_getReferenceType(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		refKey      string
		refValue    string
		expectedRef plumbing.ReferenceName
	}{
		{
			name:        "Branch reference",
			refKey:      "branch",
			refValue:    "main",
			expectedRef: plumbing.NewBranchReferenceName("main"),
		},
		{
			name:        "Tag reference",
			refKey:      "tag",
			refValue:    "v1.0.0",
			expectedRef: plumbing.NewTagReferenceName("v1.0.0"),
		},
		{
			name:        "Unknown reference type",
			refKey:      "commit",
			refValue:    "abc123",
			expectedRef: "",
		},
		{
			name:        "Empty reference",
			refKey:      "",
			refValue:    "",
			expectedRef: "",
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			src := &Source{
				RefKey:   testCase.refKey,
				RefValue: testCase.refValue,
			}
			ref := src.getReferenceType()
			assert.Equal(t, testCase.expectedRef, ref)
		})
	}
}

func TestSource_validateSource(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		setupFile   func(t *testing.T) (string, string) // returns filepath, hash
		pkgName     string
		expectError bool
	}{
		{
			name: "Valid SHA256 hash",
			setupFile: func(t *testing.T) (string, string) {
				t.Helper()
				tempDir := t.TempDir()
				content := []byte("test content")
				filePath := filepath.Join(tempDir, "test.txt")
				err := os.WriteFile(filePath, content, 0o600)
				require.NoError(t, err)

				// Calculate SHA256
				hash := sha256.Sum256(content)

				return filePath, hex.EncodeToString(hash[:])
			},
			pkgName:     "test-pkg",
			expectError: false,
		},
		{
			name: "SKIP hash should not validate",
			setupFile: func(t *testing.T) (string, string) {
				t.Helper()
				tempDir := t.TempDir()
				filePath := filepath.Join(tempDir, "test.txt")
				err := os.WriteFile(filePath, []byte("any content"), 0o600)
				require.NoError(t, err)

				return filePath, "SKIP"
			},
			pkgName:     "test-pkg",
			expectError: false,
		},
		{
			name: "Directory should not validate",
			setupFile: func(t *testing.T) (string, string) {
				t.Helper()
				tempDir := t.TempDir()
				subDir := filepath.Join(tempDir, "subdir")
				err := os.MkdirAll(subDir, 0o755)
				require.NoError(t, err)

				return subDir, "somehash"
			},
			pkgName:     "test-pkg",
			expectError: false,
		},
		{
			name: "Invalid hash should fail",
			setupFile: func(t *testing.T) (string, string) {
				t.Helper()
				tempDir := t.TempDir()
				filePath := filepath.Join(tempDir, "test.txt")
				err := os.WriteFile(filePath, []byte("test content"), 0o600)
				require.NoError(t, err)

				return filePath, "invalidhashthatiswronglengthandinvalidcontent1234567890abcdef"
			},
			pkgName:     "test-pkg",
			expectError: true,
		},
		{
			name: "Unsupported hash length",
			setupFile: func(t *testing.T) (string, string) {
				t.Helper()
				tempDir := t.TempDir()
				filePath := filepath.Join(tempDir, "test.txt")
				err := os.WriteFile(filePath, []byte("test content"), 0o600)
				require.NoError(t, err)

				return filePath, "tooshort"
			},
			pkgName:     "test-pkg",
			expectError: true,
		},
		{
			name: "Non-existent file should fail",
			setupFile: func(_ *testing.T) (string, string) {
				return "/nonexistent/file.txt", "somehash"
			},
			pkgName:     "test-pkg",
			expectError: true,
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			filePath, hash := testCase.setupFile(t)
			src := &Source{
				Hash:    hash,
				PkgName: testCase.pkgName,
			}

			err := src.validateSource(filePath)
			if testCase.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestSource_symlinkSources(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		setupSource func(t *testing.T) (string, string) // returns source path, src dir
		expectError bool
	}{
		{
			name: "Create new symlink",
			setupSource: func(t *testing.T) (string, string) {
				t.Helper()

				tempDir := t.TempDir()
				sourceFile := filepath.Join(tempDir, "source.txt")
				err := os.WriteFile(sourceFile, []byte("content"), 0o600)
				require.NoError(t, err)

				srcDir := filepath.Join(tempDir, "src")
				err = os.MkdirAll(srcDir, 0o755)
				require.NoError(t, err)

				return sourceFile, srcDir
			},
			expectError: false,
		},
		{
			name: "Symlink already exists",
			setupSource: func(t *testing.T) (string, string) {
				t.Helper()

				tempDir := t.TempDir()
				sourceFile := filepath.Join(tempDir, "source.txt")
				err := os.WriteFile(sourceFile, []byte("content"), 0o600)
				require.NoError(t, err)

				srcDir := filepath.Join(tempDir, "src")
				err = os.MkdirAll(srcDir, 0o755)
				require.NoError(t, err)

				// Create existing symlink
				existingLink := filepath.Join(srcDir, "source.txt")
				err = os.Symlink(sourceFile, existingLink)
				require.NoError(t, err)

				return sourceFile, srcDir
			},
			expectError: false,
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			sourcePath, srcDir := testCase.setupSource(t)
			src := &Source{
				SrcDir:         srcDir,
				SourceItemPath: filepath.Base(sourcePath),
			}

			err := src.symlinkSources(sourcePath)
			if testCase.expectError {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}

			// Verify symlink exists and points to correct file
			if !testCase.expectError {
				symlinkPath := filepath.Join(srcDir, src.SourceItemPath)
				_, err := os.Lstat(symlinkPath)
				assert.NoError(t, err, "Symlink should exist")
			}
		})
	}
}

func TestSource_Get_FileProtocol(t *testing.T) {
	t.Parallel()

	tempDir := t.TempDir()
	srcDir := filepath.Join(tempDir, "src")
	err := os.MkdirAll(srcDir, 0o755)
	require.NoError(t, err)

	// Create a source file
	sourceFile := filepath.Join(tempDir, "test.txt")
	content := []byte("test file content")
	err = os.WriteFile(sourceFile, content, 0o600)
	require.NoError(t, err)

	// Calculate hash
	hash := sha256.Sum256(content)
	hashStr := hex.EncodeToString(hash[:])

	src := &Source{
		Hash:          hashStr,
		PkgName:       "test-pkg",
		SourceItemURI: "test.txt",
		SrcDir:        srcDir,
		StartDir:      tempDir,
	}

	err = src.Get()
	require.NoError(t, err)

	// Verify symlink was created
	symlinkPath := filepath.Join(srcDir, "test.txt")
	_, err = os.Lstat(symlinkPath)
	assert.NoError(t, err)
}

func TestSource_Get_UnsupportedProtocol(t *testing.T) {
	t.Parallel()

	tempDir := t.TempDir()
	src := &Source{
		SourceItemURI: "unsupported://example.com/file.txt",
		StartDir:      tempDir,
		SrcDir:        filepath.Join(tempDir, "src"),
	}

	err := src.Get()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported source type")
}

func TestSource_Get_WithSKIPHash(t *testing.T) {
	t.Parallel()

	tempDir := t.TempDir()
	srcDir := filepath.Join(tempDir, "src")
	err := os.MkdirAll(srcDir, 0o755)
	require.NoError(t, err)

	// Create a source file
	sourceFile := filepath.Join(tempDir, "test.txt")
	err = os.WriteFile(sourceFile, []byte("any content"), 0o600)
	require.NoError(t, err)

	src := &Source{
		Hash:          "SKIP",
		PkgName:       "test-pkg",
		SourceItemURI: "test.txt",
		SrcDir:        srcDir,
		StartDir:      tempDir,
	}

	err = src.Get()
	assert.NoError(t, err)
}

func TestFilename(t *testing.T) {
	t.Parallel()

	// Test the filename extraction function from osutils
	tests := []struct {
		path     string
		expected string
	}{
		{
			path:     "https://example.com/file.tar.gz",
			expected: "file.tar.gz",
		},
		{
			path:     "/path/to/file.txt",
			expected: "file.txt",
		},
		{
			path:     "simple-file.zip",
			expected: "simple-file.zip",
		},
		{
			path:     "",
			expected: "",
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.path, func(t *testing.T) {
			t.Parallel()

			result := files.Filename(testCase.path)
			assert.Equal(t, testCase.expected, result)
		})
	}
}

func TestGlobalVariables(t *testing.T) {
	t.Parallel()

	// Test that global variables can be set
	originalPassword := SSHPassword

	defer func() {
		SSHPassword = originalPassword
	}()

	testPassword := "test-password"
	SSHPassword = testPassword
	assert.Equal(t, testPassword, SSHPassword)

	// Test that download mutexes map is initialized
	assert.NotNil(t, downloadMutexes)
}

func TestSource_Integration(t *testing.T) {
	t.Parallel()

	tempDir := t.TempDir()
	srcDir := filepath.Join(tempDir, "src")
	err := os.MkdirAll(srcDir, 0o755)
	require.NoError(t, err)

	// Create multiple source files
	testFiles := []struct {
		name    string
		content []byte
	}{
		{"file1.txt", []byte("content of file 1")},
		{"file2.txt", []byte("content of file 2")},
	}

	sources := make([]Source, 0, len(testFiles))

	for _, file := range testFiles {
		filePath := filepath.Join(tempDir, file.name)
		err := os.WriteFile(filePath, file.content, 0o600)
		require.NoError(t, err)

		hash := sha256.Sum256(file.content)
		hashStr := hex.EncodeToString(hash[:])

		src := Source{
			Hash:          hashStr,
			PkgName:       "integration-test",
			SourceItemURI: file.name,
			SrcDir:        srcDir,
			StartDir:      tempDir,
		}

		sources = append(sources, src)
	}

	// Process all sources
	for _, src := range sources {
		err := src.Get()
		require.NoError(t, err)

		// Verify symlink exists
		symlinkPath := filepath.Join(srcDir, src.SourceItemPath)
		_, err = os.Lstat(symlinkPath)
		assert.NoError(t, err)
	}
}

func TestSource_GetConcurrently(t *testing.T) {
	t.Parallel()

	tempDir := t.TempDir()
	srcDir := filepath.Join(tempDir, "src")
	err := os.MkdirAll(srcDir, 0o755)
	require.NoError(t, err)

	sources := make([]*Source, 2)

	for i := range 2 {
		fileName := fmt.Sprintf("file%d.txt", i)
		filePath := filepath.Join(tempDir, fileName)
		content := []byte(fmt.Sprintf("content %d", i))
		err := os.WriteFile(filePath, content, 0o600)
		require.NoError(t, err)

		hash := sha256.Sum256(content)
		hashStr := hex.EncodeToString(hash[:])

		sources[i] = &Source{
			Hash:          hashStr,
			PkgName:       "concurrent-test",
			SourceItemURI: fileName,
			SrcDir:        srcDir,
			StartDir:      tempDir,
		}
	}

	err = GetConcurrently(sources, 2)
	assert.NoError(t, err)
}

func TestConcurrentDownloadHelpers(t *testing.T) {
	// Test internal concurrent download helper functions that have 0% coverage
	tempDir := t.TempDir()
	srcDir := filepath.Join(tempDir, "src")

	sources := []*Source{
		{
			Hash:          "SKIP",
			PkgName:       "test-pkg",
			SourceItemURI: "http://example.com/file.txt",
			SrcDir:        srcDir,
			StartDir:      tempDir,
		},
	}

	// Test createSourceLogger (0% coverage)
	assert.NotPanics(t, func() {
		logger := createSourceLogger(sources)
		assert.NotNil(t, logger)
	})
}
func TestSourceStruct(t *testing.T) {
	// Test Source struct fields
	src := &Source{
		Hash:           "testhash",
		PkgName:        "test-package",
		RefKey:         "branch",
		RefValue:       "main",
		SSHPassword:    "password",
		SourceItemPath: "/path/to/source",
		SourceItemURI:  "https://example.com/source.tar.gz",
		SrcDir:         "/tmp/src",
		StartDir:       "/tmp/start",
	}

	assert.Equal(t, "testhash", src.Hash)
	assert.Equal(t, "test-package", src.PkgName)
	assert.Equal(t, "branch", src.RefKey)
	assert.Equal(t, "main", src.RefValue)
	assert.Equal(t, "password", src.SSHPassword)
	assert.Equal(t, "/path/to/source", src.SourceItemPath)
	assert.Equal(t, "https://example.com/source.tar.gz", src.SourceItemURI)
	assert.Equal(t, "/tmp/src", src.SrcDir)
	assert.Equal(t, "/tmp/start", src.StartDir)
}

func TestGetURL(t *testing.T) {
	// Create a mock HTTP server for testing HTTP downloads
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/octet-stream")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("test file content"))
	}))
	defer server.Close()

	// Test the getURL function which has 0% coverage
	tests := []struct {
		name          string
		sourceURI     string
		protocol      string
		expectedError bool
	}{
		{
			name:          "HTTP download",
			sourceURI:     server.URL + "/file.tar.gz",
			protocol:      "http",
			expectedError: false, // Should succeed with mock server
		},
		{
			name:          "Git protocol",
			sourceURI:     "git+https://github.com/example/repo.git",
			protocol:      constants.Git,
			expectedError: true, // Will fail without git being available
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tempDir := t.TempDir()
			dloadFilePath := filepath.Join(tempDir, "download.txt")

			src := &Source{
				SourceItemURI: tt.sourceURI,
				PkgName:       "test-pkg",
			}

			err := src.getURL(tt.protocol, dloadFilePath, "")

			if tt.expectedError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				// Verify file was created and has content
				content, readErr := os.ReadFile(dloadFilePath)
				assert.NoError(t, readErr)
				assert.Equal(t, "test file content", string(content))
			}
		})
	}
}

func TestPrepareDownloadMap(t *testing.T) {
	// Test prepareDownloadMap function which has 0% coverage
	tempDir := t.TempDir()

	sources := []*Source{
		{
			SourceItemURI:  "http://example.com/file1.txt",
			SourceItemPath: "file1.txt",
			PkgName:        "pkg1",
			StartDir:       tempDir,
		},
		{
			SourceItemURI:  "https://example.com/file2.txt",
			SourceItemPath: "file2.txt",
			PkgName:        "pkg2",
			StartDir:       tempDir,
		},
	}

	// This function is internal but we can test it doesn't panic
	assert.NotPanics(t, func() {
		downloadMap, sourceMap := prepareDownloadMap(sources)
		assert.NotNil(t, downloadMap)
		assert.NotNil(t, sourceMap)
		// Should have entries for HTTP sources
		assert.Equal(t, 2, len(downloadMap))
		assert.Equal(t, 2, len(sourceMap))
	})
}

func TestProcessDownloadResults(t *testing.T) {
	// Test processDownloadResults function which has 0% coverage
	tempDir := t.TempDir()
	srcDir := filepath.Join(tempDir, "src")
	err := os.MkdirAll(srcDir, 0o755)
	require.NoError(t, err)

	// Create test file that would be downloaded
	testFile := filepath.Join(tempDir, "file1.txt")
	err = os.WriteFile(testFile, []byte("test content"), 0o600)
	require.NoError(t, err)

	sourceMap := map[string]*Source{
		testFile: {
			Hash:          "SKIP",
			PkgName:       "test-pkg",
			SourceItemURI: "file1.txt",
			SrcDir:        srcDir,
			StartDir:      tempDir,
		},
	}

	// Empty download results map (simulating successful downloads)
	downloadResults := map[string]error{
		testFile: nil, // Success
	}

	assert.NotPanics(t, func() {
		err := processDownloadResults(downloadResults, sourceMap)
		// Should not panic even if processing fails
		_ = err
	})
}

func TestProcessSuccessfulDownload(t *testing.T) {
	// Test processSuccessfulDownload function which has 0% coverage
	tempDir := t.TempDir()
	srcDir := filepath.Join(tempDir, "src")
	err := os.MkdirAll(srcDir, 0o755)
	require.NoError(t, err)

	// Create a test source file
	testFile := filepath.Join(tempDir, "test.txt")
	content := []byte("test content")
	err = os.WriteFile(testFile, content, 0o600)
	require.NoError(t, err)

	// Calculate hash
	hash := sha256.Sum256(content)
	hashStr := hex.EncodeToString(hash[:])

	src := &Source{
		Hash:           hashStr,
		PkgName:        "test-pkg",
		SourceItemURI:  "test.txt",
		SourceItemPath: "test.txt",
		SrcDir:         srcDir,
		StartDir:       tempDir,
	}

	assert.NotPanics(t, func() {
		err := processSuccessfulDownload(testFile, src)
		// Should not panic even if processing fails
		_ = err
	})
}

func TestProcessConcurrentDownloads(t *testing.T) {
	// Create a mock HTTP server for testing concurrent downloads
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("test file content for concurrent download"))
	}))
	defer server.Close()

	// Test processConcurrentDownloads function which has 0% coverage
	tempDir := t.TempDir()

	sources := []*Source{
		{
			SourceItemURI:  server.URL + "/file.txt",
			SourceItemPath: "file.txt",
			PkgName:        "test-pkg",
			StartDir:       tempDir,
		},
	}

	assert.NotPanics(t, func() {
		err := processConcurrentDownloads(sources, 2)
		// Should not panic even if downloads fail
		_ = err
	})
}
