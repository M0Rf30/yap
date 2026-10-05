//nolint:testpackage
package dnfcache

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newBodyServer(t *testing.T, body string) *httptest.Server {
	t.Helper()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)

	return srv
}

// TestDownloadVerifiedMismatchKeepsExistingFile verifies that a bad body never
// replaces (or deletes) a previously cached file at the destination.
func TestDownloadVerifiedMismatchKeepsExistingFile(t *testing.T) {
	srv := newBodyServer(t, "tampered")
	dest := filepath.Join(t.TempDir(), "pkg.rpm")

	require.NoError(t, os.WriteFile(dest, []byte("good-old"), 0o600))

	err := downloadVerifiedOnce(context.Background(), srv.URL, dest,
		strings.Repeat("0", 64))
	require.Error(t, err)

	data, rerr := os.ReadFile(dest) //nolint:gosec
	require.NoError(t, rerr)
	assert.Equal(t, "good-old", string(data))

	_, serr := os.Stat(dest + ".tmp")
	assert.True(t, os.IsNotExist(serr), "temp file must be removed")
}

// TestDownloadVerifiedUppercaseChecksum verifies case-insensitive comparison.
func TestDownloadVerifiedUppercaseChecksum(t *testing.T) {
	body := "payload"
	srv := newBodyServer(t, body)
	dest := filepath.Join(t.TempDir(), "pkg.rpm")
	sum := sha256.Sum256([]byte(body))

	err := downloadVerifiedOnce(context.Background(), srv.URL, dest,
		strings.ToUpper(hex.EncodeToString(sum[:])))
	require.NoError(t, err)

	data, rerr := os.ReadFile(dest) //nolint:gosec
	require.NoError(t, rerr)
	assert.Equal(t, body, string(data))
}

// TestDownloadVerifiedNoChecksumAccepted verifies an empty checksum still
// downloads (with a warning) rather than failing.
func TestDownloadVerifiedNoChecksumAccepted(t *testing.T) {
	srv := newBodyServer(t, "abc")
	dest := filepath.Join(t.TempDir(), "pkg.rpm")

	require.NoError(t, downloadVerifiedOnce(context.Background(), srv.URL, dest, ""))

	data, rerr := os.ReadFile(dest) //nolint:gosec
	require.NoError(t, rerr)
	assert.Equal(t, "abc", string(data))
}

// TestDownloadRPMEmptyHref verifies that a package without a location is
// rejected instead of resolving to the repo root.
func TestDownloadRPMEmptyHref(t *testing.T) {
	pkg := &PackageInfo{Name: "x", BaseURL: "http://127.0.0.1:1/"}

	_, err := downloadRPM(context.Background(), pkg, t.TempDir())
	require.Error(t, err)
}
