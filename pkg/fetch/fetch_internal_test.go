// SPDX-FileCopyrightText: 2021-2026 Gianluca Boiano
// SPDX-License-Identifier: GPL-3.0-only

package fetch

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

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

func fastOpts(retries int) Options {
	return Options{MaxRetries: retries, BackoffBase: time.Millisecond}
}

func TestFileDownloadsContent(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("hello"))
	}))
	defer server.Close()

	destination := filepath.Join(t.TempDir(), "out.txt")

	var progress bytes.Buffer

	opts := fastOpts(0)
	opts.Progress = &progress

	if err := File(context.Background(), destination, server.URL, opts); err != nil {
		t.Fatalf("File failed: %v", err)
	}

	data, err := os.ReadFile(destination) //nolint:gosec
	if err != nil {
		t.Fatalf("read destination: %v", err)
	}

	if string(data) != "hello" {
		t.Errorf("unexpected content: %q", data)
	}

	if !strings.Contains(progress.String(), "100%") {
		t.Errorf("progress bar never completed: %q", progress.String())
	}
}

func TestFileRecoversFromTransient5xx(t *testing.T) {
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

	if err := File(context.Background(), destination, server.URL, fastOpts(3)); err != nil {
		t.Fatalf("File failed: %v", err)
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

// A definitive 404 fails immediately without burning the retry budget.
func TestFileNoRetryOn404(t *testing.T) {
	var hits atomic.Int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits.Add(1)
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	destination := filepath.Join(t.TempDir(), "out.txt")

	if err := File(context.Background(), destination, server.URL, fastOpts(3)); err == nil {
		t.Fatal("expected error for 404")
	}

	if got := hits.Load(); got != 1 {
		t.Errorf("expected 1 attempt, got %d", got)
	}
}

// File issues exactly MaxRetries+1 requests against a mirror that always
// answers 503.
func TestFileHonorsMaxRetries(t *testing.T) {
	var hits atomic.Int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits.Add(1)
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer server.Close()

	destination := filepath.Join(t.TempDir(), "out.txt")

	if err := File(context.Background(), destination, server.URL, fastOpts(2)); err == nil {
		t.Fatal("expected error")
	}

	if got := hits.Load(); got != 3 {
		t.Errorf("expected 3 attempts, got %d", got)
	}
}

func TestFileCancelledStopsRetrying(t *testing.T) {
	var hits atomic.Int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits.Add(1)
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer server.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	destination := filepath.Join(t.TempDir(), "out.txt")

	if err := File(ctx, destination, server.URL, fastOpts(3)); err == nil {
		t.Fatal("expected error")
	}

	if got := hits.Load(); got > 1 {
		t.Errorf("expected at most 1 attempt, got %d", got)
	}
}

// TestSetMaxRetries verifies SetMaxRetries round-trips through MaxRetries
// and clamps negative values to 0.
func TestSetMaxRetries(t *testing.T) {
	previous := MaxRetries()
	defer SetMaxRetries(previous)

	SetMaxRetries(7)

	if got := MaxRetries(); got != 7 {
		t.Errorf("MaxRetries() = %d, want 7", got)
	}

	SetMaxRetries(-3)

	if got := MaxRetries(); got != 0 {
		t.Errorf("MaxRetries() = %d, want 0 for negative override", got)
	}
}

// TestParseMaxRetriesEnv table-tests the environment-variable parsing
// helper directly, since the sync.Once-guarded public path can only be
// exercised once per process.
func TestParseMaxRetriesEnv(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want int
		ok   bool
	}{
		{name: "empty", raw: "", want: 0, ok: false},
		{name: "valid", raw: "7", want: 7, ok: true},
		{name: "zero", raw: "0", want: 0, ok: true},
		{name: "negative", raw: "-1", want: 0, ok: false},
		{name: "non-numeric", raw: "abc", want: 0, ok: false},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, ok := parseMaxRetriesEnv(test.raw)
			if ok != test.ok || got != test.want {
				t.Errorf("parseMaxRetriesEnv(%q) = (%d, %v), want (%d, %v)",
					test.raw, got, ok, test.want, test.ok)
			}
		})
	}
}

func TestProgressBarAbortNeverCompletes(t *testing.T) {
	var buf bytes.Buffer

	pb := NewProgressBar(&buf, "pkg", "file", 100)

	buf.Reset()
	pb.Abort(10)

	out := buf.String()
	if !strings.Contains(out, " 10%") || strings.Contains(out, "100%") {
		t.Errorf("Abort should render actual progress, got %q", out)
	}
}
