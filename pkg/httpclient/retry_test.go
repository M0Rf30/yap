package httpclient_test

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	"github.com/M0Rf30/yap/v2/pkg/httpclient"
)

// TestMain shrinks the retry backoff so error-path tests that exercise
// transient failures (5xx, refused connections) don't sleep through the
// production backoff schedule.
func TestMain(m *testing.M) {
	httpclient.SetRetryPolicy(3, time.Millisecond)
	os.Exit(m.Run())
}

// TestIsRetryable tests the transient-vs-definitive error classification.
func TestIsRetryable(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want bool
	}{
		{"nil", nil, false},
		{"context canceled", context.Canceled, false},
		{"context deadline", context.DeadlineExceeded, false},
		{"too large", httpclient.ErrTooLarge, false},
		{"http 404", &httpclient.HTTPStatusError{Code: 404, URL: "u"}, false},
		{"http 403", &httpclient.HTTPStatusError{Code: 403, URL: "u"}, false},
		{"http 408", &httpclient.HTTPStatusError{Code: 408, URL: "u"}, true},
		{"http 429", &httpclient.HTTPStatusError{Code: 429, URL: "u"}, true},
		{"http 500", &httpclient.HTTPStatusError{Code: 500, URL: "u"}, true},
		{"http 503", &httpclient.HTTPStatusError{Code: 503, URL: "u"}, true},
		{"unexpected EOF", io.ErrUnexpectedEOF, true},
		{"conn reset", syscall.ECONNRESET, true},
		{"conn refused", syscall.ECONNREFUSED, true},
		{"generic", errors.New("boom"), false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := httpclient.IsRetryable(tc.err); got != tc.want {
				t.Errorf("IsRetryable(%v) = %v, want %v", tc.err, got, tc.want)
			}
		})
	}
}

// TestFetchBytesRetriesTransient5xx tests that FetchBytes survives a mirror
// that returns 5xx before recovering.
func TestFetchBytesRetriesTransient5xx(t *testing.T) {
	var hits atomic.Int32

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if hits.Add(1) <= 2 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}

		_, _ = w.Write([]byte("payload"))
	}))
	defer srv.Close()

	data, err := httpclient.FetchBytes(context.Background(), srv.URL, 1024)
	if err != nil {
		t.Fatalf("FetchBytes failed: %v", err)
	}

	if string(data) != "payload" {
		t.Errorf("unexpected body: %q", data)
	}

	if got := hits.Load(); got != 3 {
		t.Errorf("expected 3 attempts, got %d", got)
	}
}

// TestFetchBytesNoRetryOn404 tests that a definitive 404 is returned
// immediately without burning the retry budget.
func TestFetchBytesNoRetryOn404(t *testing.T) {
	var hits atomic.Int32

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits.Add(1)
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	_, err := httpclient.FetchBytes(context.Background(), srv.URL, 1024)
	if err == nil {
		t.Fatal("expected error")
	}

	var statusErr *httpclient.HTTPStatusError
	if !errors.As(err, &statusErr) || statusErr.Code != http.StatusNotFound {
		t.Errorf("expected HTTP 404 error, got %v", err)
	}

	if got := hits.Load(); got != 1 {
		t.Errorf("expected 1 attempt, got %d", got)
	}
}

// TestFetchBytesRetryBudgetExhausted tests that a permanently failing
// server fails after exactly the configured number of attempts.
func TestFetchBytesRetryBudgetExhausted(t *testing.T) {
	var hits atomic.Int32

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits.Add(1)
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer srv.Close()

	_, err := httpclient.FetchBytes(context.Background(), srv.URL, 1024)
	if err == nil {
		t.Fatal("expected error")
	}

	if got := hits.Load(); got != 3 {
		t.Errorf("expected 3 attempts, got %d", got)
	}
}

// TestFetchToFileRetriesTruncatedBody tests that a mid-body disconnect
// (server advertises more bytes than it sends) is retried and leaves a
// complete file on the recovered attempt.
func TestFetchToFileRetriesTruncatedBody(t *testing.T) {
	var hits atomic.Int32

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if hits.Add(1) == 1 {
			// Advertise 100 bytes, send 5, then slam the connection.
			w.Header().Set("Content-Length", "100")
			_, _ = w.Write([]byte("trunc"))

			if f, ok := w.(http.Flusher); ok {
				f.Flush()
			}

			conn, _, _ := w.(http.Hijacker).Hijack()
			_ = conn.Close()

			return
		}

		_, _ = w.Write([]byte("complete"))
	}))
	defer srv.Close()

	dest := filepath.Join(t.TempDir(), "out.bin")

	if err := httpclient.FetchToFile(context.Background(), srv.URL, dest, 1024); err != nil {
		t.Fatalf("FetchToFile failed: %v", err)
	}

	data, err := os.ReadFile(dest) //nolint:gosec
	if err != nil {
		t.Fatalf("read dest: %v", err)
	}

	if string(data) != "complete" {
		t.Errorf("unexpected file content: %q", data)
	}

	if got := hits.Load(); got != 2 {
		t.Errorf("expected 2 attempts, got %d", got)
	}
}

// TestFetchBytesConditionalRetries tests that the conditional fetch path is
// covered by the retry loop too.
func TestFetchBytesConditionalRetries(t *testing.T) {
	var hits atomic.Int32

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if hits.Add(1) == 1 {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		_, _ = w.Write([]byte("fresh"))
	}))
	defer srv.Close()

	data, notModified, err := httpclient.FetchBytesConditional(
		context.Background(), srv.URL, 1024, time.Now().Add(-time.Hour))
	if err != nil {
		t.Fatalf("FetchBytesConditional failed: %v", err)
	}

	if notModified {
		t.Error("expected a fresh body, got notModified")
	}

	if string(data) != "fresh" {
		t.Errorf("unexpected body: %q", data)
	}

	if got := hits.Load(); got != 2 {
		t.Errorf("expected 2 attempts, got %d", got)
	}
}

// TestWithRetryContextCancelled tests that cancellation during backoff
// stops further attempts and surfaces the last error.
func TestWithRetryContextCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())

	var calls atomic.Int32

	err := httpclient.WithRetry(ctx, "test", func() error {
		calls.Add(1)
		cancel() // cancel while "in flight": backoff select must bail out

		return &httpclient.HTTPStatusError{Code: 503, URL: "test"}
	})
	if err == nil {
		t.Fatal("expected error")
	}

	if got := calls.Load(); got != 1 {
		t.Errorf("expected 1 attempt, got %d", got)
	}
}

// TestSetRetryPolicyClampsAttempts tests that a non-positive attempt count
// is clamped to a single attempt instead of disabling fetches entirely.
func TestSetRetryPolicyClampsAttempts(t *testing.T) {
	defer httpclient.SetRetryPolicy(3, time.Millisecond)

	httpclient.SetRetryPolicy(0, -time.Second)

	var hits atomic.Int32

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits.Add(1)
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer srv.Close()

	if _, err := httpclient.FetchBytes(context.Background(), srv.URL, 1024); err == nil {
		t.Fatal("expected error")
	}

	if got := hits.Load(); got != 1 {
		t.Errorf("expected 1 attempt, got %d", got)
	}
}
