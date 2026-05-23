package httpclient_test

import (
	stderrors "errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/M0Rf30/yap/v2/pkg/httpclient"
)

// TestAtomicWrite_SuccessfulWrite tests that AtomicWrite successfully writes
// a file via temp file + rename.
func TestAtomicWrite_SuccessfulWrite(t *testing.T) {
	tmpDir := t.TempDir()
	destPath := filepath.Join(tmpDir, "output.txt")

	testContent := "hello world"

	err := httpclient.AtomicWrite(destPath, func(w io.Writer) error {
		_, err := io.WriteString(w, testContent)
		return err
	})
	if err != nil {
		t.Fatalf("AtomicWrite() returned error: %v", err)
	}

	// Verify file was created
	if _, err := os.Stat(destPath); err != nil {
		t.Fatalf("Output file not created: %v", err)
	}

	// Verify content
	data, err := os.ReadFile(destPath)
	if err != nil {
		t.Fatalf("Failed to read output file: %v", err)
	}

	if string(data) != testContent {
		t.Errorf("File content = %q, want %q", string(data), testContent)
	}

	// Verify temp file was cleaned up
	tmpPath := destPath + ".tmp"
	if _, err := os.Stat(tmpPath); !stderrors.Is(err, os.ErrNotExist) {
		t.Errorf("Temp file %q should not exist after successful write", tmpPath)
	}
}

// TestAtomicWrite_ErrorFromFn tests that AtomicWrite cleans up temp file
// when fn returns an error.
func TestAtomicWrite_ErrorFromFn(t *testing.T) {
	tmpDir := t.TempDir()
	destPath := filepath.Join(tmpDir, "output.txt")

	testErr := fmt.Errorf("write failed")

	err := httpclient.AtomicWrite(destPath, func(w io.Writer) error {
		return testErr
	})
	if err == nil {
		t.Fatal("AtomicWrite() returned nil error, want error from fn")
	}

	if !stderrors.Is(err, testErr) {
		t.Errorf("AtomicWrite() returned %v, want %v", err, testErr)
	}

	// Verify destination file was not created
	if _, err := os.Stat(destPath); !stderrors.Is(err, os.ErrNotExist) {
		t.Errorf("Destination file should not exist when fn fails")
	}

	// Verify temp file was cleaned up
	tmpPath := destPath + ".tmp"
	if _, err := os.Stat(tmpPath); !stderrors.Is(err, os.ErrNotExist) {
		t.Errorf("Temp file should be cleaned up when fn fails")
	}
}

// TestAtomicWrite_CloseError tests that AtomicWrite handles close errors.
func TestAtomicWrite_CloseError(t *testing.T) {
	tmpDir := t.TempDir()

	// Use a read-only directory to cause close/rename to fail
	readOnlyDir := filepath.Join(tmpDir, "readonly")
	if err := os.Mkdir(readOnlyDir, 0o555); err != nil {
		t.Fatalf("Failed to create read-only directory: %v", err)
	}
	defer func() { _ = os.Chmod(readOnlyDir, 0o755) }() // Restore for cleanup

	readOnlyPath := filepath.Join(readOnlyDir, "output.txt")

	err := httpclient.AtomicWrite(readOnlyPath, func(w io.Writer) error {
		_, err := io.WriteString(w, "test")
		return err
	})
	if err == nil {
		t.Fatal("AtomicWrite() returned nil error, want error for read-only directory")
	}
}

// TestAtomicWrite_Atomicity tests that partial writes don't leave incomplete files.
func TestAtomicWrite_Atomicity(t *testing.T) {
	tmpDir := t.TempDir()
	destPath := filepath.Join(tmpDir, "output.txt")

	// First write
	err := httpclient.AtomicWrite(destPath, func(w io.Writer) error {
		_, err := io.WriteString(w, "first")
		return err
	})
	if err != nil {
		t.Fatalf("First write failed: %v", err)
	}

	// Second write that fails
	err = httpclient.AtomicWrite(destPath, func(w io.Writer) error {
		return fmt.Errorf("second write failed")
	})
	if err == nil {
		t.Fatal("Second write should have failed")
	}

	// Verify file still contains first write (atomicity preserved)
	data, err := os.ReadFile(destPath)
	if err != nil {
		t.Fatalf("Failed to read file: %v", err)
	}

	if string(data) != "first" {
		t.Errorf("File content = %q, want %q (atomicity violated)", string(data), "first")
	}
}

// TestFetchBytes_SuccessfulFetch tests FetchBytes with a successful response.
func TestFetchBytes_SuccessfulFetch(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = fmt.Fprint(w, "test content")
	}))
	defer server.Close()

	data, err := httpclient.FetchBytes(testContext(), server.URL, httpclient.DefaultMaxBytes)
	if err != nil {
		t.Fatalf("FetchBytes() returned error: %v", err)
	}

	if string(data) != "test content" {
		t.Errorf("FetchBytes() = %q, want %q", string(data), "test content")
	}
}

// TestFetchBytes_SizeCapExceeded tests FetchBytes when body exceeds maxBytes.
func TestFetchBytes_SizeCapExceeded(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		// Write more than the cap
		for range 1000 {
			_, _ = fmt.Fprint(w, "x")
		}
	}))
	defer server.Close()

	maxBytes := int64(100)

	data, err := httpclient.FetchBytes(testContext(), server.URL, maxBytes)
	if err == nil {
		t.Fatal("FetchBytes() returned nil error, want ErrTooLarge")
	}

	if !stderrors.Is(err, httpclient.ErrTooLarge) {
		t.Errorf("FetchBytes() error = %v, want ErrTooLarge", err)
	}

	if data != nil {
		t.Errorf("FetchBytes() returned data when error occurred: %v", data)
	}
}

// TestFetchBytes_HTTPError tests FetchBytes with non-2xx status.
func TestFetchBytes_HTTPError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = fmt.Fprint(w, "not found")
	}))
	defer server.Close()

	data, err := httpclient.FetchBytes(testContext(), server.URL, httpclient.DefaultMaxBytes)
	if err == nil {
		t.Fatal("FetchBytes() returned nil error, want HTTPStatusError")
	}

	var statusErr *httpclient.HTTPStatusError
	if !stderrors.As(err, &statusErr) {
		t.Fatalf("FetchBytes() error is not *HTTPStatusError: %T", err)
	}

	if statusErr.Code != http.StatusNotFound {
		t.Errorf("Status code = %d, want %d", statusErr.Code, http.StatusNotFound)
	}

	if data != nil {
		t.Errorf("FetchBytes() returned data on error: %v", data)
	}
}

// TestFetchBytes_DefaultMaxBytes tests that non-positive maxBytes falls back
// to DefaultMaxBytes.
func TestFetchBytes_DefaultMaxBytes(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = fmt.Fprint(w, "small content")
	}))
	defer server.Close()

	// Test with 0 (should use DefaultMaxBytes)
	data, err := httpclient.FetchBytes(testContext(), server.URL, 0)
	if err != nil {
		t.Fatalf("FetchBytes(0) returned error: %v", err)
	}

	if string(data) != "small content" {
		t.Errorf("FetchBytes(0) = %q, want %q", string(data), "small content")
	}

	// Test with negative (should use DefaultMaxBytes)
	data, err = httpclient.FetchBytes(testContext(), server.URL, -1)
	if err != nil {
		t.Fatalf("FetchBytes(-1) returned error: %v", err)
	}

	if string(data) != "small content" {
		t.Errorf("FetchBytes(-1) = %q, want %q", string(data), "small content")
	}
}

// TestFetchBytes_ExactlyAtCap tests FetchBytes when body is exactly at maxBytes.
func TestFetchBytes_ExactlyAtCap(t *testing.T) {
	content := strings.Repeat("x", 100)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = fmt.Fprint(w, content)
	}))
	defer server.Close()

	data, err := httpclient.FetchBytes(testContext(), server.URL, int64(len(content)))
	if err != nil {
		t.Fatalf("FetchBytes() returned error: %v", err)
	}

	if string(data) != content {
		t.Errorf("FetchBytes() returned %d bytes, want %d", len(data), len(content))
	}
}

// TestFetchToFile_SuccessfulFetch tests FetchToFile with a successful response.
func TestFetchToFile_SuccessfulFetch(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = fmt.Fprint(w, "file content")
	}))
	defer server.Close()

	tmpDir := t.TempDir()
	destPath := filepath.Join(tmpDir, "output.txt")

	err := httpclient.FetchToFile(testContext(), server.URL, destPath, httpclient.DefaultMaxBytes)
	if err != nil {
		t.Fatalf("FetchToFile() returned error: %v", err)
	}

	// Verify file was created
	data, err := os.ReadFile(destPath)
	if err != nil {
		t.Fatalf("Failed to read output file: %v", err)
	}

	if string(data) != "file content" {
		t.Errorf("File content = %q, want %q", string(data), "file content")
	}

	// Verify temp file was cleaned up
	tmpPath := destPath + ".tmp"
	if _, err := os.Stat(tmpPath); !stderrors.Is(err, os.ErrNotExist) {
		t.Errorf("Temp file should not exist after successful write")
	}
}

// TestFetchToFile_ContentLengthPreflight tests that FetchToFile rejects
// responses with Content-Length exceeding maxBytes.
func TestFetchToFile_ContentLengthPreflight(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", "1000")
		w.WriteHeader(http.StatusOK)
		// Don't actually write the body - we're testing preflight rejection
	}))
	defer server.Close()

	tmpDir := t.TempDir()
	destPath := filepath.Join(tmpDir, "output.txt")

	maxBytes := int64(100)

	err := httpclient.FetchToFile(testContext(), server.URL, destPath, maxBytes)
	if err == nil {
		t.Fatal("FetchToFile() returned nil error, want ErrTooLarge for Content-Length")
	}

	if !stderrors.Is(err, httpclient.ErrTooLarge) {
		t.Errorf("FetchToFile() error = %v, want ErrTooLarge", err)
	}

	// Verify file was not created
	if _, err := os.Stat(destPath); !stderrors.Is(err, os.ErrNotExist) {
		t.Errorf("File should not be created when Content-Length exceeds cap")
	}
}

// TestFetchToFile_BodyExceedsCap tests that FetchToFile rejects when actual
// body exceeds maxBytes (even if Content-Length is missing).
func TestFetchToFile_BodyExceedsCap(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		// Write more than the cap, without Content-Length header
		for range 1000 {
			_, _ = fmt.Fprint(w, "x")
		}
	}))
	defer server.Close()

	tmpDir := t.TempDir()
	destPath := filepath.Join(tmpDir, "output.txt")

	maxBytes := int64(100)

	err := httpclient.FetchToFile(testContext(), server.URL, destPath, maxBytes)
	if err == nil {
		t.Fatal("FetchToFile() returned nil error, want ErrTooLarge")
	}

	if !stderrors.Is(err, httpclient.ErrTooLarge) {
		t.Errorf("FetchToFile() error = %v, want ErrTooLarge", err)
	}

	// Verify file was not created (atomic write should have cleaned up)
	if _, err := os.Stat(destPath); !stderrors.Is(err, os.ErrNotExist) {
		t.Errorf("File should not be created when body exceeds cap")
	}
}

// TestFetchToFile_HTTPError tests FetchToFile with non-2xx status.
func TestFetchToFile_HTTPError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = fmt.Fprint(w, "server error")
	}))
	defer server.Close()

	tmpDir := t.TempDir()
	destPath := filepath.Join(tmpDir, "output.txt")

	err := httpclient.FetchToFile(testContext(), server.URL, destPath, httpclient.DefaultMaxBytes)
	if err == nil {
		t.Fatal("FetchToFile() returned nil error, want HTTPStatusError")
	}

	var statusErr *httpclient.HTTPStatusError
	if !stderrors.As(err, &statusErr) {
		t.Fatalf("FetchToFile() error is not *HTTPStatusError: %T", err)
	}

	if statusErr.Code != http.StatusInternalServerError {
		t.Errorf("Status code = %d, want %d", statusErr.Code, http.StatusInternalServerError)
	}

	// Verify file was not created
	if _, err := os.Stat(destPath); !stderrors.Is(err, os.ErrNotExist) {
		t.Errorf("File should not be created on HTTP error")
	}
}

// TestFetchToFile_DefaultMaxBytes tests that non-positive maxBytes falls back
// to DefaultMaxBytes.
func TestFetchToFile_DefaultMaxBytes(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = fmt.Fprint(w, "content")
	}))
	defer server.Close()

	tmpDir := t.TempDir()
	destPath := filepath.Join(tmpDir, "output.txt")

	// Test with 0 (should use DefaultMaxBytes)
	err := httpclient.FetchToFile(testContext(), server.URL, destPath, 0)
	if err != nil {
		t.Fatalf("FetchToFile(0) returned error: %v", err)
	}

	data, err := os.ReadFile(destPath)
	if err != nil {
		t.Fatalf("Failed to read file: %v", err)
	}

	if string(data) != "content" {
		t.Errorf("File content = %q, want %q", string(data), "content")
	}
}

// TestFetchToFile_Atomicity tests that partial downloads don't leave incomplete files.
func TestFetchToFile_Atomicity(t *testing.T) {
	tmpDir := t.TempDir()
	destPath := filepath.Join(tmpDir, "output.txt")

	// First successful download
	server1 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = fmt.Fprint(w, "first")
	}))
	defer server1.Close()

	err := httpclient.FetchToFile(testContext(), server1.URL, destPath, httpclient.DefaultMaxBytes)
	if err != nil {
		t.Fatalf("First download failed: %v", err)
	}

	// Second download that exceeds cap
	server2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)

		for range 1000 {
			_, _ = fmt.Fprint(w, "x")
		}
	}))
	defer server2.Close()

	err = httpclient.FetchToFile(testContext(), server2.URL, destPath, int64(100))
	if err == nil {
		t.Fatal("Second download should have failed")
	}

	// Verify file still contains first download (atomicity preserved)
	data, err := os.ReadFile(destPath)
	if err != nil {
		t.Fatalf("Failed to read file: %v", err)
	}

	if string(data) != "first" {
		t.Errorf("File content = %q, want %q (atomicity violated)", string(data), "first")
	}
}

// TestFetchToFile_ExactlyAtCap tests FetchToFile when body is exactly at maxBytes.
func TestFetchToFile_ExactlyAtCap(t *testing.T) {
	content := strings.Repeat("y", 100)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = fmt.Fprint(w, content)
	}))
	defer server.Close()

	tmpDir := t.TempDir()
	destPath := filepath.Join(tmpDir, "output.txt")

	err := httpclient.FetchToFile(testContext(), server.URL, destPath, int64(len(content)))
	if err != nil {
		t.Fatalf("FetchToFile() returned error: %v", err)
	}

	data, err := os.ReadFile(destPath)
	if err != nil {
		t.Fatalf("Failed to read file: %v", err)
	}

	if string(data) != content {
		t.Errorf("File content length = %d, want %d", len(data), len(content))
	}
}

// testContext returns a context for testing (non-cancellable).
func testContext() interface {
	Deadline() (time.Time, bool)
	Done() <-chan struct{}
	Err() error
	Value(key any) any
} {
	return &testCtx{}
}

type testCtx struct{}

func (tc *testCtx) Deadline() (time.Time, bool) {
	return time.Time{}, false
}

func (tc *testCtx) Done() <-chan struct{} {
	return nil
}

func (tc *testCtx) Err() error {
	return nil
}

func (tc *testCtx) Value(key any) any {
	return nil
}
