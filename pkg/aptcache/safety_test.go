package aptcache_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/M0Rf30/yap/v2/pkg/aptcache"
)

const zeroSHA256 = "0000000000000000000000000000000000000000000000000000000000000000"

// downloadOne serves body from a test server, indexes a single package
// "widget" with the supplied (possibly bogus) integrity fields, runs
// Download, and returns the error plus the destination path.
func downloadOne(t *testing.T, body, shaHex string, size int64) (string, error) {
	t.Helper()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)

	stanza := fmt.Sprintf("Package: widget\nArchitecture: amd64\nVersion: 1.0\n"+
		"Filename: pool/w/widget_1.0_amd64.deb\nSize: %d\nSHA256: %s\n\n", size, shaHex)

	c := aptcache.NewCacheForTesting()
	require.NoError(t, c.ParseDeb822WithBaseURLForTesting(
		strings.NewReader(stanza), false, srv.URL+"/"))

	dir := t.TempDir()
	err := c.Download(context.Background(), dir, []string{"widget"})

	return filepath.Join(dir, "widget_1.0_amd64.deb"), err
}

// TestDownloadHashMismatchLeavesNoFile is the H-3 regression: a SHA-256
// mismatch must NOT leave a corrupt artifact at the destination path.
func TestDownloadHashMismatchLeavesNoFile(t *testing.T) {
	t.Parallel()

	const body = "this is not the expected content"

	dest, err := downloadOne(t, body, zeroSHA256, int64(len(body)))
	require.Error(t, err, "expected hash mismatch error")

	_, statErr := os.Stat(dest)
	require.True(t, os.IsNotExist(statErr), "hash-mismatched download left an artifact")
}

// TestDownloadSizeMismatchLeavesNoFile mirrors the hash-mismatch case for
// the size-mismatch path.
func TestDownloadSizeMismatchLeavesNoFile(t *testing.T) {
	t.Parallel()

	sum := sha256.Sum256([]byte("short"))

	dest, err := downloadOne(t, "short", hex.EncodeToString(sum[:]), 99999)
	require.Error(t, err, "expected size mismatch error")

	_, statErr := os.Stat(dest)
	require.True(t, os.IsNotExist(statErr), "size-mismatched download left an artifact")
}

// TestDownloadRejectsUnverifiableIndexEntries: an empty, malformed
// (odd-length / wrong-length) SHA256 or a missing / oversized Size must
// fail before any bytes are fetched instead of yielding an unverified .deb.
func TestDownloadRejectsUnverifiableIndexEntries(t *testing.T) {
	t.Parallel()

	sum := sha256.Sum256([]byte("payload"))
	good := hex.EncodeToString(sum[:])

	cases := []struct {
		name string
		sha  string
		size int64
	}{
		{"empty sha", "", 7},
		{"odd-length sha", good[:63], 7},
		{"short sha", good[:32], 7},
		{"non-hex sha", strings.Repeat("zz", 32), 7},
		{"zero size", good, 0},
		{"oversized", good, 3 << 30},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			dest, err := downloadOne(t, "payload", tc.sha, tc.size)
			require.Error(t, err)

			_, statErr := os.Stat(dest)
			require.True(t, os.IsNotExist(statErr), "no artifact may be written")
		})
	}
}
