package aptcache_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/M0Rf30/yap/v2/pkg/aptcache"
)

// TestDownloadHashMismatchLeavesNoFile is the H-3 regression: a SHA-256
// mismatch must NOT leave a partial / corrupt artifact at the destination
// path. The previous implementation wrote directly to destFile and only
// errored after the bad bytes were already on disk.
func TestDownloadHashMismatchLeavesNoFile(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("this is not the expected content"))
	}))
	defer srv.Close()

	dir := t.TempDir()
	dest := filepath.Join(dir, "pkg.deb")

	err := aptcache.DownloadAndVerifyForTesting(
		context.Background(),
		srv.URL, dest,
		"0000000000000000000000000000000000000000000000000000000000000000", // wrong hash
		int64(len("this is not the expected content")),
	)
	if err == nil {
		t.Fatal("expected hash mismatch error, got nil")
	}

	// destFile must NOT exist.
	if _, statErr := os.Stat(dest); !os.IsNotExist(statErr) {
		t.Fatalf("hash-mismatched download left an artifact at %q (err=%v)",
			dest, statErr)
	}
}

// TestDownloadSizeMismatchLeavesNoFile mirrors the hash-mismatch case for
// the size-mismatch path.
func TestDownloadSizeMismatchLeavesNoFile(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("short"))
	}))
	defer srv.Close()

	dir := t.TempDir()
	dest := filepath.Join(dir, "pkg.deb")

	err := aptcache.DownloadAndVerifyForTesting(
		context.Background(),
		srv.URL, dest,
		"", // skip hash check
		99999,
	)
	if err == nil {
		t.Fatal("expected size mismatch error, got nil")
	}

	if _, statErr := os.Stat(dest); !os.IsNotExist(statErr) {
		t.Fatalf("size-mismatched download left an artifact at %q", dest)
	}
}
