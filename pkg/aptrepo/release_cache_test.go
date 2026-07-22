//nolint:testpackage // tests cover the unexported releaseCache dedup helper
package aptrepo

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/M0Rf30/yap/v2/pkg/aptcache"
	"github.com/M0Rf30/yap/v2/pkg/errors"
)

// TestReleaseCacheDedup asserts that concurrent fetches for the same
// (url, suite, signedBy, allowUnverified) key hit the underlying fetcher
// exactly once, while distinct keys fetch independently. This is the
// contract that prevents stock sources.list layouts (one suite split
// across several deb lines) from re-downloading the same InRelease.
func TestReleaseCacheDedup(t *testing.T) {
	t.Parallel()

	var calls atomic.Int64

	c := &releaseCache{
		fn: func(_ context.Context, _, _, _ string, _ bool) (*Release, error) {
			calls.Add(1)

			return &Release{}, nil
		},
		m: make(map[string]*releaseCacheEntry),
	}

	var wg sync.WaitGroup

	// 10 concurrent duplicates of one key + one distinct suite.
	for range 10 {
		wg.Go(func() {
			rel, err := c.fetch(t.Context(), "http://archive.ubuntu.com/ubuntu/", "jammy", "", false)
			assert.NoError(t, err)
			assert.NotNil(t, rel)
		})
	}

	wg.Wait()

	_, err := c.fetch(t.Context(), "http://archive.ubuntu.com/ubuntu/", "jammy-updates", "", false)
	require.NoError(t, err)

	assert.Equal(t, int64(2), calls.Load(), "one fetch per unique key")
}

// TestReleaseCacheCachesErrors asserts a failed fetch is not retried for
// duplicate sources: a dead mirror should fail once per suite, not once
// per deb line.
func TestReleaseCacheCachesErrors(t *testing.T) {
	t.Parallel()

	var calls atomic.Int64

	fetchErr := errors.New(errors.ErrTypeNetwork, "mirror down")
	c := &releaseCache{
		fn: func(_ context.Context, _, _, _ string, _ bool) (*Release, error) {
			calls.Add(1)

			return nil, fetchErr
		},
		m: make(map[string]*releaseCacheEntry),
	}

	for range 3 {
		_, err := c.fetch(t.Context(), "http://security.ubuntu.com/ubuntu/", "jammy-security", "", false)
		require.ErrorIs(t, err, fetchErr)
	}

	assert.Equal(t, int64(1), calls.Load(), "error cached, no retry per duplicate")
}

// TestReleaseCacheKeyIncludesTrustInputs asserts signedBy and
// allowUnverified participate in the cache key — two sources for the same
// suite with different trust anchors must not share a verification result.
func TestReleaseCacheKeyIncludesTrustInputs(t *testing.T) {
	t.Parallel()

	var calls atomic.Int64

	c := &releaseCache{
		fn: func(_ context.Context, _, _, _ string, _ bool) (*Release, error) {
			calls.Add(1)

			return &Release{}, nil
		},
		m: make(map[string]*releaseCacheEntry),
	}

	ctx := t.Context()
	_, _ = c.fetch(ctx, "http://repo.example/", "stable", "", false)
	_, _ = c.fetch(ctx, "http://repo.example/", "stable", "/usr/share/keyrings/a.gpg", false)
	_, _ = c.fetch(ctx, "http://repo.example/", "stable", "", true)

	assert.Equal(t, int64(3), calls.Load(), "signedBy and allowUnverified are key inputs")
}

// TestUpdateSourceDedupsInReleaseFetches drives the real wire path
// (updateSource → releaseCache.fetch → fetchRelease → httpFetch) against a
// counting HTTP server with the stock Ubuntu jammy sources.list layout:
// one (url, suite) split across several deb lines with different
// components. Exactly one InRelease request per unique suite must hit the
// mirror, not one per deb line.
func TestUpdateSourceDedupsInReleaseFetches(t *testing.T) {
	t.Parallel()

	var inReleaseHits atomic.Int64

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/InRelease") {
			inReleaseHits.Add(1)
			// Unsigned minimal Release body — accepted under
			// AllowUnverifiedRepos. Empty SHA256 manifest means every
			// component index lookup misses, which updateSource tolerates.
			_, _ = w.Write([]byte("Suite: jammy\nSHA256:\n"))

			return
		}

		http.NotFound(w, r)
	}))
	defer srv.Close()

	// Stock jammy layout: jammy ×3 deb lines, jammy-updates ×2. SignedBy
	// points at a nonexistent keyring so trust resolution deterministically
	// yields no anchor (bypassed via AllowUnverifiedRepos) regardless of
	// whether the host has real apt keyrings — CI runners do.
	noKey := "/nonexistent/yap-test-keyring.gpg"
	sources := []aptcache.SourceEntry{
		{URL: srv.URL, Suite: "jammy", Components: []string{"main", "restricted"}, SignedBy: noKey},
		{URL: srv.URL, Suite: "jammy", Components: []string{"universe"}, SignedBy: noKey},
		{URL: srv.URL, Suite: "jammy", Components: []string{"multiverse"}, SignedBy: noKey},
		{URL: srv.URL, Suite: "jammy-updates", Components: []string{"main"}, SignedBy: noKey},
		{URL: srv.URL, Suite: "jammy-updates", Components: []string{"universe"}, SignedBy: noKey},
	}

	rc := newReleaseCache()
	opts := Options{AllowUnverifiedRepos: true}

	var wg sync.WaitGroup
	for i := range sources {
		wg.Go(func() {
			_, err := updateSource(t.Context(), &sources[i], "amd64", opts, rc)
			assert.NoError(t, err)
		})
	}

	wg.Wait()

	assert.Equal(t, int64(2), inReleaseHits.Load(),
		"one InRelease fetch per unique (url, suite), not per deb line")
}
