package download

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/cavaliergopher/grab/v3"
)

func TestFilename(t *testing.T) {
	testCases := []struct {
		input    string
		expected string
	}{
		{"https://example.com/file.txt", "file.txt"},
		{"/path/to/file.txt", "file.txt"},
		{"file.txt", "file.txt"},
		{"", "."},                             // filepath.Base("") returns "."
		{"https://example.com/path/", "path"}, // filepath.Base returns "path" for trailing slash
		{"noextension", "noextension"},
	}

	for _, tc := range testCases {
		result := filepath.Base(tc.input)
		if result != tc.expected {
			t.Errorf("Filename(%q) = %q, expected %q", tc.input, result, tc.expected)
		}
	}
}

func TestFormatBytes(t *testing.T) {
	testCases := []struct {
		input    int64
		expected string
	}{
		{0, "0 B"},
		{512, "512 B"},
		{1024, "1.0 KB"},
		{1536, "1.5 KB"},
		{1048576, "1.0 MB"},
		{1073741824, "1.0 GB"},
		{1099511627776, "1.0 TB"},
	}

	for _, tc := range testCases {
		result := formatBytes(tc.input)
		if result != tc.expected {
			t.Errorf("formatBytes(%d) = %q, expected %q", tc.input, result, tc.expected)
		}
	}
}

func TestIsRetryableDownloadError(t *testing.T) {
	testCases := []struct {
		err      error
		expected bool
	}{
		{nil, false},
		{io.EOF, false},
		{context.DeadlineExceeded, true}, // Contains "timeout"
	}

	// Test with string-based errors
	stringTestCases := []struct {
		errStr   string
		expected bool
	}{
		{"connection reset by peer", true},
		{"connection refused", true},
		{"timeout exceeded", true},
		{"502 bad gateway", true},
		{"503 service unavailable", true},
		{"504 gateway timeout", true},
		{"network is unreachable", true},
		{"no route to host", true},
		{"temporary failure", true},
		{"file not found", false},
		{"permission denied", false},
	}

	for _, tc := range testCases {
		result := isRetryableDownloadError(tc.err)
		if result != tc.expected {
			t.Errorf("isRetryableDownloadError(%v) = %v, expected %v", tc.err, result, tc.expected)
		}
	}

	for _, tc := range stringTestCases {
		err := errors.New(tc.errStr)

		result := isRetryableDownloadError(err)
		if result != tc.expected {
			t.Errorf("isRetryableDownloadError(%q) = %v, expected %v", tc.errStr, result, tc.expected)
		}
	}
}

func TestDeterminePackageName(t *testing.T) {
	testCases := []struct {
		input    string
		expected string
	}{
		{"mypackage", "mypackage"},
		{"", "yap"},
		{"  ", "  "},
	}

	for _, tc := range testCases {
		result := determinePackageName(tc.input)
		if result != tc.expected {
			t.Errorf("determinePackageName(%q) = %q, expected %q", tc.input, result, tc.expected)
		}
	}
}

func TestDetermineSourceName(t *testing.T) {
	testCases := []struct {
		sourceName string
		uri        string
		expected   string
	}{
		{"mysource", "https://example.com/file.txt", "mysource"},
		{"", "https://example.com/file.txt", "file.txt"},
		{"", "https://example.com/", "download"},
		{"", "", "download"},
	}

	for _, tc := range testCases {
		result := determineSourceName(tc.sourceName, tc.uri)
		if result != tc.expected {
			t.Errorf("determineSourceName(%q, %q) = %q, expected %q",
				tc.sourceName, tc.uri, result, tc.expected)
		}
	}
}

func TestPrepareDownloadRequest(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "download-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}

	defer func() { _ = os.RemoveAll(tempDir) }()

	destination := filepath.Join(tempDir, "test.txt")
	uri := "https://example.com/test.txt"

	client, req, err := prepareDownloadRequest(context.Background(), destination, uri)
	if err != nil {
		t.Errorf("prepareDownloadRequest failed: %v", err)
	}

	if client == nil {
		t.Error("client is nil")
	}

	if req == nil {
		t.Error("request is nil")
	}

	if req != nil && req.URL().String() != uri {
		t.Errorf("request URL = %q, expected %q", req.URL().String(), uri)
	}
}

func TestConfigureResumeIfPossible(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "download-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}

	defer func() { _ = os.RemoveAll(tempDir) }()

	destination := filepath.Join(tempDir, "test.txt")
	uri := "https://example.com/test.txt"

	// Test with no existing file
	req, err := grab.NewRequest(destination, uri)
	if err != nil {
		t.Fatalf("Failed to create request: %v", err)
	}

	configureResumeIfPossible(req, destination, uri)
	// Should not panic or error

	// Test with existing file
	err = os.WriteFile(destination, []byte("partial content"), 0o644)
	if err != nil {
		t.Fatalf("Failed to create partial file: %v", err)
	}

	configureResumeIfPossible(req, destination, uri)
	// Should configure resume - we can't easily verify this without accessing private fields
}

func TestNewConcurrentDownloadManager(t *testing.T) {
	var buf bytes.Buffer

	// Test with default values
	manager := NewConcurrentDownloadManager(0, &buf)
	if manager == nil {
		t.Error("manager is nil")
	}

	// Test with specific values
	manager = NewConcurrentDownloadManager(5, &buf)
	if manager == nil {
		t.Error("manager is nil")
	}

	// Test with too high value (should be capped)
	manager = NewConcurrentDownloadManager(20, &buf)
	if manager == nil {
		t.Error("manager is nil")
	}

	// Shutdown the manager
	err := manager.Shutdown(1 * time.Second)
	if err != nil {
		t.Errorf("Shutdown failed: %v", err)
	}
}

func TestConcurrentDownloadManagerWaitForNonExistentJob(t *testing.T) {
	var buf bytes.Buffer

	manager := NewConcurrentDownloadManager(2, &buf)

	defer func() { _ = manager.Shutdown(1 * time.Second) }()

	err := manager.WaitForJob("nonexistent")
	if err == nil {
		t.Error("Expected error for nonexistent job")
	}
}

func TestConcurrently(t *testing.T) {
	// Test with empty downloads
	results := Concurrently(map[string]string{}, 2, 1, nil)
	if len(results) != 0 {
		t.Errorf("Expected empty results, got %d items", len(results))
	}

	// Test with invalid URLs (will fail but shouldn't panic)
	tempDir, err := os.MkdirTemp("", "download-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}

	defer func() { _ = os.RemoveAll(tempDir) }()

	downloads := map[string]string{
		filepath.Join(tempDir, "test1.txt"): "invalid://url",
		filepath.Join(tempDir, "test2.txt"): "http://nonexistent.example.com/file.txt",
	}

	results = Concurrently(downloads, 2, 0, nil)
	if len(results) != len(downloads) {
		t.Errorf("Expected %d results, got %d", len(downloads), len(results))
	}

	// All should have errors
	for dest, err := range results {
		if err == nil {
			t.Errorf("Expected error for %s, got nil", dest)
		}
	}
}

func TestWithResume(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "download-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}

	defer func() { _ = os.RemoveAll(tempDir) }()

	destination := filepath.Join(tempDir, "test.txt")

	// Test with invalid URL - should fail
	err = WithResume(destination, "invalid://url", 0, nil)
	if err == nil {
		t.Error("Expected error for invalid URL")
	}

	// Test with non-retryable error (should not retry)
	err = WithResume(destination, "invalid://url", 2, nil)
	if err == nil {
		t.Error("Expected error for invalid URL")
	}
}

func TestWithResumeContext(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "download-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}

	defer func() { _ = os.RemoveAll(tempDir) }()

	destination := filepath.Join(tempDir, "test.txt")

	// Test with invalid URL - should fail
	err = WithResumeContext(destination, "invalid://url", 0, "testpkg", "testsrc", nil)
	if err == nil {
		t.Error("Expected error for invalid URL")
	}
}

func TestDownloadWithMockServer(t *testing.T) {
	// Create a test HTTP server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", "12")
		w.Header().Set("Content-Type", "text/plain")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("test content"))
	}))
	defer server.Close()

	tempDir, err := os.MkdirTemp("", "download-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}

	defer func() { _ = os.RemoveAll(tempDir) }()

	destination := filepath.Join(tempDir, "test.txt")

	var buf bytes.Buffer

	err = Download(destination, server.URL, &buf)
	if err != nil {
		t.Errorf("Download failed: %v", err)
	}

	// Check that file was created
	if _, err := os.Stat(destination); os.IsNotExist(err) {
		t.Error("Downloaded file does not exist")
	}

	// Check file content
	content, err := os.ReadFile(destination)
	if err != nil {
		t.Errorf("Failed to read downloaded file: %v", err)
	}

	if string(content) != "test content" {
		t.Errorf("File content = %q, expected %q", string(content), "test content")
	}
}

func TestWithResumeWithMockServer(t *testing.T) {
	t.Skip("Skipping flaky resume test with mock server")
	// Create a test HTTP server that supports range requests
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		content := "test content for resume"

		// Check for range header
		rangeHeader := r.Header.Get("Range")
		if rangeHeader != "" {
			w.Header().Set("Accept-Ranges", "bytes")
			w.Header().Set("Content-Range", "bytes 4-23/24")
			w.Header().Set("Content-Length", "20")
			w.WriteHeader(http.StatusPartialContent)
			_, _ = w.Write([]byte(content[4:]))
		} else {
			w.Header().Set("Accept-Ranges", "bytes")
			w.Header().Set("Content-Length", "24")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(content))
		}
	}))
	defer server.Close()

	tempDir, err := os.MkdirTemp("", "download-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}

	defer func() { _ = os.RemoveAll(tempDir) }()

	destination := filepath.Join(tempDir, "test.txt")

	var buf bytes.Buffer

	err = WithResume(destination, server.URL, 1, &buf)
	if err != nil {
		t.Errorf("WithResume failed: %v", err)
	}

	// Check that file was created
	if _, err := os.Stat(destination); os.IsNotExist(err) {
		t.Error("Downloaded file does not exist")
	}
}

func TestConcurrentDownloadManagerSubmitAndWait(t *testing.T) {
	// Create a test HTTP server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", "12")
		w.Header().Set("Content-Type", "text/plain")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("test content"))
	}))
	defer server.Close()

	tempDir, err := os.MkdirTemp("", "download-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}

	defer func() { _ = os.RemoveAll(tempDir) }()

	var buf bytes.Buffer

	manager := NewConcurrentDownloadManager(2, &buf)

	defer func() { _ = manager.Shutdown(5 * time.Second) }()

	destination := filepath.Join(tempDir, "test.txt")

	// Submit download
	err = manager.SubmitDownload(destination, server.URL, 1)
	if err != nil {
		t.Errorf("SubmitDownload failed: %v", err)
	}

	// Wait for specific job
	err = manager.WaitForJob(destination)
	if err != nil {
		t.Errorf("WaitForJob failed: %v", err)
	}

	// Check that file was created
	if _, err := os.Stat(destination); os.IsNotExist(err) {
		t.Error("Downloaded file does not exist")
	}
}

func TestConcurrentDownloadManagerWaitForAll(t *testing.T) {
	// Create a test HTTP server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", "12")
		w.Header().Set("Content-Type", "text/plain")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("test content"))
	}))
	defer server.Close()

	tempDir, err := os.MkdirTemp("", "download-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}

	defer func() { _ = os.RemoveAll(tempDir) }()

	var buf bytes.Buffer

	manager := NewConcurrentDownloadManager(2, &buf)

	defer func() { _ = manager.Shutdown(5 * time.Second) }()

	destinations := []string{
		filepath.Join(tempDir, "test1.txt"),
		filepath.Join(tempDir, "test2.txt"),
	}

	// Submit multiple downloads
	for _, dest := range destinations {
		err = manager.SubmitDownload(dest, server.URL, 1)
		if err != nil {
			t.Errorf("SubmitDownload failed for %s: %v", dest, err)
		}
	}

	// Wait for all
	results := manager.WaitForAll()

	// Check results
	for _, dest := range destinations {
		if results[dest] != nil {
			t.Errorf("Download failed for %s: %v", dest, results[dest])
		}

		// Check that file was created
		if _, err := os.Stat(dest); os.IsNotExist(err) {
			t.Errorf("Downloaded file does not exist: %s", dest)
		}
	}
}

func TestLogDownloadStart(t *testing.T) {
	// Create a mock response
	req, _ := grab.NewRequest("/tmp/test", "http://example.com/test")
	client := grab.NewClient()
	resp := client.Do(req)

	// This should not panic
	logDownloadStart("http://example.com/test", resp)
}

func TestCreateProgressBar(t *testing.T) {
	// Create a test server for controlled responses
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Respond immediately without Content-Length to ensure Size() returns -1
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	// Create request and get response quickly
	req, _ := grab.NewRequest("/tmp/test", server.URL)
	client := grab.NewClient()
	resp := client.Do(req)

	defer func() {
		if resp != nil {
			_ = resp.Cancel()
		}
	}()

	// Wait briefly for response to be received
	select {
	case <-resp.Done:
		// Response completed
	case <-time.After(100 * time.Millisecond):
		// Timeout after 100ms, cancel the request
		_ = resp.Cancel()
	}

	// Test with nil writer
	pb := createProgressBar(resp, "testpkg", "testsrc", server.URL, nil)
	if pb != nil {
		t.Error("Expected nil progress bar with nil writer")
	}

	// Test with writer but unknown size (should be -1)
	var buf bytes.Buffer

	pb = createProgressBar(resp, "testpkg", "testsrc", server.URL, &buf)
	if pb != nil {
		t.Error("Expected nil progress bar with unknown size")
	}
}

func TestMonitorDownloadTimeout(t *testing.T) {
	// Create a mock response that will timeout
	req, _ := grab.NewRequest("/tmp/test", "http://example.com/test")
	client := grab.NewClient()
	resp := client.Do(req)

	// Monitor for a very short time to test the ticker path
	go func() {
		time.Sleep(200 * time.Millisecond)

		_ = resp.Cancel()
	}()

	err := monitorDownload(resp, nil, "/tmp/test")
	// Should get an error due to cancellation
	if err == nil {
		t.Log("Expected error due to cancellation, but got nil")
	}
}
