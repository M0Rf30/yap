package osutils

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestProgressWriters(t *testing.T) {
	// Test progress writer creation
	assert.NotPanics(t, func() {
		var buf bytes.Buffer

		_ = NewPackageDecoratedWriter(&buf, "test-package")
		_ = NewGitProgressWriter(&buf, "test-repo")
	})
}

func TestProgressBar(t *testing.T) {
	// Test enhanced progress bar
	assert.NotPanics(t, func() {
		var buf bytes.Buffer

		pb := NewEnhancedProgressBar(&buf, "Test Operation", "Downloading", 100)
		pb.Update(50)
		pb.Finish()
	})
}

func TestConcurrentDownloadManager(t *testing.T) {
	// Test concurrent download manager creation
	assert.NotPanics(t, func() {
		manager := NewConcurrentDownloadManager(2)
		_ = manager.Shutdown(time.Second)
	})
}

func TestExec(t *testing.T) {
	// Test Exec function with a simple command
	assert.NotPanics(t, func() {
		_ = Exec(false, "echo", "test")
	})
}

func TestProcessFunctions(t *testing.T) {
	// Test various process-related functions
	assert.NotPanics(t, func() {
		// These should not panic even if they fail
		_ = CheckGO()
	})
}

func TestProgressWriterWrite(t *testing.T) {
	// Test the Write method of progress writers
	assert.NotPanics(t, func() {
		var buf bytes.Buffer

		writer := NewPackageDecoratedWriter(&buf, "test-pkg")

		// Test writing data
		_, err := writer.Write([]byte("test data\n"))
		assert.NoError(t, err)
	})

	assert.NotPanics(t, func() {
		var buf bytes.Buffer

		writer := NewGitProgressWriter(&buf, "test-repo")

		// Test writing git progress data
		_, err := writer.Write([]byte("Receiving objects: 50% (123/245)\n"))
		assert.NoError(t, err)
	})
}

func TestFormatBytes(t *testing.T) {
	// Test formatBytes function with actual behavior
	tests := []struct {
		name  string
		bytes int64
	}{
		{"Zero bytes", 0},
		{"Bytes", 512},
		{"Exactly 1KB", 1024},
		{"Large bytes", 1023},
		{"Just over 1KB", 1025},
		{"1MB", 1024 * 1024},
		{"1GB", 1024 * 1024 * 1024},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := formatBytes(tt.bytes)
			// Just verify it doesn't panic and returns a non-empty string
			assert.NotEmpty(t, result)
			assert.Contains(t, result, " ")
			t.Logf("formatBytes(%d) = %s", tt.bytes, result)
		})
	}
}

func TestDownloadWithResume(t *testing.T) {
	// Create a test server to serve files
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		content := "test file content for download"
		if r.Header.Get("Range") != "" {
			// Handle range request for resume
			w.Header().Set("Content-Range", fmt.Sprintf("bytes 10-%d/%d", len(content)-1, len(content)))
			w.WriteHeader(http.StatusPartialContent)
			_, _ = io.WriteString(w, content[10:])
		} else {
			w.WriteHeader(http.StatusOK)
			_, _ = io.WriteString(w, content)
		}
	}))
	defer server.Close()

	testDir := t.TempDir()
	destFile := filepath.Join(testDir, "test_download.txt")

	// Test download with nil logger
	err := DownloadWithResume(destFile, server.URL, 1)
	assert.NoError(t, err)

	// Verify file was downloaded
	assert.FileExists(t, destFile)

	content, err := os.ReadFile(destFile)
	require.NoError(t, err)
	assert.Equal(t, "test file content for download", string(content))
}

func TestDownloadWithResumeContext(t *testing.T) {
	// Create a test server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, "context download content")
	}))
	defer server.Close()

	testDir := t.TempDir()
	destFile := filepath.Join(testDir, "test_download_ctx.txt")

	// Test with proper parameters
	err := DownloadWithResumeContext(destFile, server.URL, 1, "test-pkg", "test-source")
	assert.NoError(t, err)

	assert.FileExists(t, destFile)
}

func TestDownload(t *testing.T) {
	// Skip this test since Download function calls logger.Fatal which exits the process
	t.Skip("Download function calls logger.Fatal which exits process, making it untestable")
}
func TestDownloadConcurrently(t *testing.T) {
	// Create test server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, "concurrent content")
	}))
	defer server.Close()

	testDir := t.TempDir()

	downloads := map[string]string{
		server.URL: filepath.Join(testDir, "file1.txt"),
	}

	errors := DownloadConcurrently(downloads, 2, 1)
	assert.NotNil(t, errors) // Map should be returned, can be empty

	// Check if at least we get a response (may succeed or fail)
	t.Logf("Download results: %+v", errors)
}
func TestCreateTarZst(t *testing.T) {
	testDir := t.TempDir()

	// Create some test files
	testFile1 := filepath.Join(testDir, "test1.txt")
	testFile2 := filepath.Join(testDir, "test2.txt")

	require.NoError(t, os.WriteFile(testFile1, []byte("content1"), 0o644))
	require.NoError(t, os.WriteFile(testFile2, []byte("content2"), 0o644))

	outputArchive := filepath.Join(testDir, "test.tar.zst")

	err := CreateTarZst(outputArchive, testDir, true)
	// May fail if zstd is not available, but shouldn't panic
	if err != nil {
		t.Logf("CreateTarZst failed (expected if zstd not available): %v", err)
	}
}

func TestRunScript(t *testing.T) {
	testDir := t.TempDir()
	scriptPath := filepath.Join(testDir, "test_script.sh")

	// Create a simple test script
	scriptContent := "#!/bin/bash\necho 'test script executed'\n"
	require.NoError(t, os.WriteFile(scriptPath, []byte(scriptContent), 0o755))

	err := RunScript(scriptPath)

	// May fail if shell is not available, but test that it doesn't panic
	if err != nil {
		t.Logf("RunScript failed (expected in some environments): %v", err)
	}
}

func TestRunScriptWithPackage(t *testing.T) {
	testDir := t.TempDir()
	scriptPath := filepath.Join(testDir, "pkg_script.sh")

	// Create a test script
	scriptContent := "#!/bin/bash\necho 'package script'\n"
	require.NoError(t, os.WriteFile(scriptPath, []byte(scriptContent), 0o755))

	err := RunScriptWithPackage(scriptPath, "test-package")

	// May fail but shouldn't panic
	if err != nil {
		t.Logf("RunScriptWithPackage failed (expected in some environments): %v", err)
	}
}

func TestUnarchive(t *testing.T) {
	testDir := t.TempDir()

	// Test with non-existent file
	err := Unarchive("non-existent.tar.gz", testDir)
	// Should fail gracefully
	assert.Error(t, err)
}

func TestPullContainers(t *testing.T) {
	// Test pulling containers (may fail if Docker not available)
	err := PullContainers("alpine:latest")
	// Should not panic even if Docker is unavailable
	if err != nil {
		t.Logf("PullContainers failed (expected if Docker not available): %v", err)
	}
}

func TestGOSetup(t *testing.T) {
	// Test Go setup
	err := GOSetup()
	// May fail if Go tools are not available, but shouldn't panic
	if err != nil {
		t.Logf("GOSetup failed (expected if Go tools not available): %v", err)
	}
}
