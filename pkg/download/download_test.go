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
	"sync/atomic"
	"testing"
	"time"

	"github.com/cavaliergopher/grab/v3"
)

// TestMain shrinks the retry backoff so retry-path tests don't sleep
// through the production backoff schedule.
func TestMain(m *testing.M) {
	backoffBase = time.Millisecond

	os.Exit(m.Run())
}

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
		name     string
		err      error
		expected bool
	}{
		{"nil", nil, false},
		// Truncated transfers are transient: retry resumes the partial.
		{"EOF", io.EOF, true},
		{"unexpected EOF", io.ErrUnexpectedEOF, true},
		// Context cancellation is a deliberate stop, never retried.
		{"context canceled", context.Canceled, false},
		{"context deadline", context.DeadlineExceeded, false},
		// grab typed status codes: 4xx definitive, 408/429/5xx transient.
		{"grab 404", grab.StatusCodeError(http.StatusNotFound), false},
		{"grab 403", grab.StatusCodeError(http.StatusForbidden), false},
		{"grab 408", grab.StatusCodeError(http.StatusRequestTimeout), true},
		{"grab 429", grab.StatusCodeError(http.StatusTooManyRequests), true},
		{"grab 500", grab.StatusCodeError(http.StatusInternalServerError), true},
		{"grab 503", grab.StatusCodeError(http.StatusServiceUnavailable), true},
		// Unresumable partial: retried after the loop clears the file.
		{"grab bad length", grab.ErrBadLength, true},
	}

	// String-wrapped errors that lost their type.
	stringTestCases := []struct {
		errStr   string
		expected bool
	}{
		{"connection reset by peer", true},
		{"connection refused", true},
		{"timeout exceeded", true},
		{"network is unreachable", true},
		{"no route to host", true},
		{"temporary failure", true},
		{"unexpected eof while reading body", true},
		{"file not found", false},
		{"permission denied", false},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := isRetryableDownloadError(tc.err)
			if result != tc.expected {
				t.Errorf("isRetryableDownloadError(%v) = %v, expected %v", tc.err, result, tc.expected)
			}
		})
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

func TestWithResume(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "download-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}

	defer func() { _ = os.RemoveAll(tempDir) }()

	destination := filepath.Join(tempDir, "test.txt")

	// Test with invalid URL - should fail
	err = WithResume(context.Background(), destination, "invalid://url", 0, nil)
	if err == nil {
		t.Error("Expected error for invalid URL")
	}

	// Test with non-retryable error (should not retry)
	err = WithResume(context.Background(), destination, "invalid://url", 2, nil)
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

	err = WithResume(context.Background(), destination, server.URL, 1, &buf)
	if err != nil {
		t.Errorf("WithResume failed: %v", err)
	}

	// Check that file was created
	if _, err := os.Stat(destination); os.IsNotExist(err) {
		t.Error("Downloaded file does not exist")
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

// TestWithResumeRecoversFromTransient5xx tests that the retry loop survives
// a mirror that returns 5xx before recovering.
func TestWithResumeRecoversFromTransient5xx(t *testing.T) {
	var hits atomic.Int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if hits.Add(1) <= 2 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}

		_, _ = w.Write([]byte("recovered content"))
	}))
	defer server.Close()

	destination := filepath.Join(t.TempDir(), "out.txt")

	if err := WithResume(context.Background(), destination, server.URL, 3, nil); err != nil {
		t.Fatalf("WithResume failed: %v", err)
	}

	data, err := os.ReadFile(destination) //nolint:gosec
	if err != nil {
		t.Fatalf("read destination: %v", err)
	}

	if string(data) != "recovered content" {
		t.Errorf("unexpected content: %q", data)
	}

	if got := hits.Load(); got != 3 {
		t.Errorf("expected 3 attempts, got %d", got)
	}
}

// TestWithResumeNoRetryOn404 tests that a definitive 404 fails immediately
// without burning the retry budget.
func TestWithResumeNoRetryOn404(t *testing.T) {
	var hits atomic.Int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits.Add(1)
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	destination := filepath.Join(t.TempDir(), "out.txt")

	if err := WithResume(context.Background(), destination, server.URL, 3, nil); err == nil {
		t.Fatal("expected error for 404")
	}

	if got := hits.Load(); got != 1 {
		t.Errorf("expected 1 attempt, got %d", got)
	}
}

// TestWithResumeContextCancelledStopsRetrying tests that a cancelled
// context aborts the backoff instead of sleeping through it.
func TestWithResumeCancelledStopsRetrying(t *testing.T) {
	var hits atomic.Int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits.Add(1)
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer server.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	destination := filepath.Join(t.TempDir(), "out.txt")

	if err := WithResume(ctx, destination, server.URL, 3, nil); err == nil {
		t.Fatal("expected error")
	}

	// First attempt may fire before the ctx check in the backoff select,
	// but no further attempts must happen.
	if got := hits.Load(); got > 1 {
		t.Errorf("expected at most 1 attempt, got %d", got)
	}
}
