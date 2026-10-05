//nolint:testpackage // exercises unexported fetchComponentIndex / updateSourceIn
package aptrepo

import (
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/M0Rf30/yap/v2/pkg/aptcache"
)

const testPackagesPath = "main/binary-amd64/Packages"

func sha256Hex(b []byte) string {
	sum := sha256.Sum256(b)

	return hex.EncodeToString(sum[:])
}

// packagesServer serves body at the dists/<suite>/main/binary-amd64/Packages
// path and counts hits.
func packagesServer(t *testing.T, body []byte, hits *atomic.Int64) *httptest.Server {
	t.Helper()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/"+testPackagesPath) {
			hits.Add(1)

			_, _ = w.Write(body)

			return
		}

		http.NotFound(w, r)
	}))
	t.Cleanup(srv.Close)

	return srv
}

func releaseFor(body []byte, hash string) *Release {
	return &Release{SHA256: map[string]hashEntry{
		testPackagesPath: {Hash: hash, Size: int64(len(body))},
	}}
}

func TestFetchComponentIndexWritesAtomically(t *testing.T) {
	t.Parallel()

	body := []byte("Package: foo\nVersion: 1\n\n")

	var hits atomic.Int64

	srv := packagesServer(t, body, &hits)
	src := &aptcache.SourceEntry{URL: srv.URL, Suite: "noble", Components: []string{"main"}}
	dir := t.TempDir()

	require.NoError(t, fetchComponentIndex(t.Context(), dir, src, "main", "amd64",
		releaseFor(body, sha256Hex(body))))

	dest := filepath.Join(dir, encodeListFilename(srv.URL, "noble", testPackagesPath))
	got, err := os.ReadFile(dest)
	require.NoError(t, err)
	assert.Equal(t, body, got)

	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	assert.Len(t, entries, 1, "no temp files may be left behind")
}

// TestFetchComponentIndexDuplicateSourcesDownloadOnce: duplicate deb lines
// for the same (url, suite, component, arch) must not race on the same file.
func TestFetchComponentIndexDuplicateSourcesDownloadOnce(t *testing.T) {
	t.Parallel()

	body := []byte(strings.Repeat("Package: foo\nVersion: 1\n\n", 100))

	var hits atomic.Int64

	srv := packagesServer(t, body, &hits)
	src := &aptcache.SourceEntry{URL: srv.URL, Suite: "noble", Components: []string{"main"}}
	rel := releaseFor(body, sha256Hex(body))
	dir := t.TempDir()

	var wg sync.WaitGroup

	for range 8 {
		wg.Go(func() {
			assert.NoError(t, fetchComponentIndex(t.Context(), dir, src, "main", "amd64", rel))
		})
	}

	wg.Wait()

	assert.Equal(t, int64(1), hits.Load(), "duplicate sources must share one download")
}

func TestFetchComponentIndexHashMismatchWritesNothing(t *testing.T) {
	t.Parallel()

	body := []byte("Package: foo\n\n")

	var hits atomic.Int64

	srv := packagesServer(t, body, &hits)
	src := &aptcache.SourceEntry{URL: srv.URL, Suite: "noble", Components: []string{"main"}}
	dir := t.TempDir()

	err := fetchComponentIndex(t.Context(), dir, src, "main", "amd64",
		releaseFor(body, strings.Repeat("0", 64)))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "SHA256 mismatch")

	entries, readErr := os.ReadDir(dir)
	require.NoError(t, readErr)
	assert.Empty(t, entries)
}

// TestFetchComponentIndexPreservesFetchError: a transport failure must be
// reported as such, not masked as "no Packages variant found".
func TestFetchComponentIndexPreservesFetchError(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "boom", http.StatusNotFound)
	}))
	t.Cleanup(srv.Close)

	body := []byte("x")
	src := &aptcache.SourceEntry{URL: srv.URL, Suite: "noble", Components: []string{"main"}}

	err := fetchComponentIndex(t.Context(), t.TempDir(), src, "main", "amd64",
		releaseFor(body, sha256Hex(body)))
	require.Error(t, err)
	require.NotErrorIs(t, err, errNoPackagesVariant)
	assert.Contains(t, err.Error(), "404")
}

func TestFetchComponentIndexNotListedIsNoVariant(t *testing.T) {
	t.Parallel()

	src := &aptcache.SourceEntry{URL: "http://127.0.0.1:1", Suite: "noble"}

	err := fetchComponentIndex(t.Context(), t.TempDir(), src, "main", "amd64",
		&Release{SHA256: map[string]hashEntry{}})
	require.ErrorIs(t, err, errNoPackagesVariant)
}

// TestUpdateSourceReportsComponentFailures: when a component's index fails
// verification, updateSource must return the error rather than (n, nil).
func TestUpdateSourceReportsComponentFailures(t *testing.T) {
	t.Parallel()

	body := []byte("Package: foo\n\n")
	badHash := strings.Repeat("0", 64)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/InRelease"):
			_, _ = w.Write([]byte("Suite: noble\nSHA256:\n " + sha256Hex(body) + " 14 main/binary-amd64/Packages\n " +
				badHash + " 14 universe/binary-amd64/Packages\n"))
		case strings.HasSuffix(r.URL.Path, "/Packages"):
			_, _ = w.Write(body)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)

	src := &aptcache.SourceEntry{
		URL: srv.URL, Suite: "noble", Components: []string{"main", "universe", "multiverse"},
		SignedBy: "/nonexistent/yap-test-keyring.gpg",
	}

	n, err := updateSourceIn(t.Context(), t.TempDir(), src, "amd64",
		Options{AllowUnverifiedRepos: true}, newReleaseCache())

	assert.Equal(t, 1, n, "only main verifies; multiverse is simply not listed")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "SHA256 mismatch")
	assert.NotContains(t, err.Error(), "no Packages variant",
		"components absent from Release are tolerated")
}
