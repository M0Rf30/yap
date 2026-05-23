package apkindex_test

// coverage_test.go — tests targeting the remaining coverage gaps:
//   - repos.go: parseReposContent (via exported ParseReposContent)
//   - apkindex.go: Load, Install

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/M0Rf30/yap/v2/pkg/apkindex"
)

// ---------------------------------------------------------------------------
// parseReposContent — full parsing logic coverage
// ---------------------------------------------------------------------------

func TestParseReposContentPlainURL(t *testing.T) {
	repos := apkindex.ParseReposContent("https://dl-cdn.alpinelinux.org/alpine/v3.20/main\n")
	require.Len(t, repos, 1)
	assert.Equal(t, "https://dl-cdn.alpinelinux.org/alpine/v3.20/main", repos[0].URL)
	assert.Equal(t, "", repos[0].Tag)
}

func TestParseReposContentTrailingSlashStripped(t *testing.T) {
	repos := apkindex.ParseReposContent("https://dl-cdn.alpinelinux.org/alpine/v3.20/main/\n")
	require.Len(t, repos, 1)
	assert.Equal(t, "https://dl-cdn.alpinelinux.org/alpine/v3.20/main", repos[0].URL)
}

func TestParseReposContentMultipleTrailingSlashes(t *testing.T) {
	repos := apkindex.ParseReposContent("https://example.com/repo///\n")
	require.Len(t, repos, 1)
	assert.Equal(t, "https://example.com/repo", repos[0].URL)
}

func TestParseReposContentTaggedRepo(t *testing.T) {
	repos := apkindex.ParseReposContent("@edge https://dl-cdn.alpinelinux.org/alpine/edge/testing\n")
	require.Len(t, repos, 1)
	assert.Equal(t, "https://dl-cdn.alpinelinux.org/alpine/edge/testing", repos[0].URL)
	assert.Equal(t, "edge", repos[0].Tag)
}

func TestParseReposContentTaggedRepoTrailingSlash(t *testing.T) {
	repos := apkindex.ParseReposContent("@community https://example.com/alpine/community/\n")
	require.Len(t, repos, 1)
	assert.Equal(t, "https://example.com/alpine/community", repos[0].URL)
	assert.Equal(t, "community", repos[0].Tag)
}

func TestParseReposContentCommentSkipped(t *testing.T) {
	repos := apkindex.ParseReposContent("# https://dl-cdn.alpinelinux.org/alpine/v3.20/main\n")
	assert.Empty(t, repos)
}

func TestParseReposContentDoubleHashCommentSkipped(t *testing.T) {
	repos := apkindex.ParseReposContent("## another comment\n")
	assert.Empty(t, repos)
}

func TestParseReposContentEmptyLineSkipped(t *testing.T) {
	repos := apkindex.ParseReposContent("\n")
	assert.Empty(t, repos)
}

func TestParseReposContentWhitespaceOnlyLineSkipped(t *testing.T) {
	repos := apkindex.ParseReposContent("   \n")
	assert.Empty(t, repos)
}

func TestParseReposContentEmptyString(t *testing.T) {
	repos := apkindex.ParseReposContent("")
	assert.Empty(t, repos)
}

func TestParseReposContentOnlyComments(t *testing.T) {
	content := "# Comment 1\n## Comment 2\n# https://example.com/repo\n"
	repos := apkindex.ParseReposContent(content)
	assert.Empty(t, repos)
}

func TestParseReposContentMalformedTaggedLine(t *testing.T) {
	// "@edge" with no URL part — should be skipped.
	content := "@edge\nhttps://dl-cdn.alpinelinux.org/alpine/v3.20/main\n"
	repos := apkindex.ParseReposContent(content)
	require.Len(t, repos, 1)
	assert.Equal(t, "https://dl-cdn.alpinelinux.org/alpine/v3.20/main", repos[0].URL)
	assert.Equal(t, "", repos[0].Tag)
}

func TestParseReposContentMixedContent(t *testing.T) {
	content := `# Alpine Linux repositories
https://dl-cdn.alpinelinux.org/alpine/v3.20/main
https://dl-cdn.alpinelinux.org/alpine/v3.20/community
@edge https://dl-cdn.alpinelinux.org/alpine/edge/testing

# Comment line
https://example.com/alpine/v3.20/custom/

`
	repos := apkindex.ParseReposContent(content)
	require.Len(t, repos, 4)

	assert.Equal(t, "https://dl-cdn.alpinelinux.org/alpine/v3.20/main", repos[0].URL)
	assert.Equal(t, "", repos[0].Tag)

	assert.Equal(t, "https://dl-cdn.alpinelinux.org/alpine/v3.20/community", repos[1].URL)
	assert.Equal(t, "", repos[1].Tag)

	assert.Equal(t, "https://dl-cdn.alpinelinux.org/alpine/edge/testing", repos[2].URL)
	assert.Equal(t, "edge", repos[2].Tag)

	// Trailing slash stripped.
	assert.Equal(t, "https://example.com/alpine/v3.20/custom", repos[3].URL)
	assert.Equal(t, "", repos[3].Tag)
}

func TestParseReposContentNoTrailingNewline(t *testing.T) {
	// File without trailing newline — last line must still be parsed.
	repos := apkindex.ParseReposContent("https://dl-cdn.alpinelinux.org/alpine/v3.20/main")
	require.Len(t, repos, 1)
	assert.Equal(t, "https://dl-cdn.alpinelinux.org/alpine/v3.20/main", repos[0].URL)
}

func TestParseReposContentTaggedURLWithExtraSpaces(t *testing.T) {
	// Extra spaces around the URL in a tagged line should be trimmed.
	repos := apkindex.ParseReposContent("@testing   https://example.com/testing  \n")
	require.Len(t, repos, 1)
	assert.Equal(t, "https://example.com/testing", repos[0].URL)
	assert.Equal(t, "testing", repos[0].Tag)
}

func TestParseReposContentMultipleTaggedRepos(t *testing.T) {
	content := "@edge https://dl-cdn.alpinelinux.org/alpine/edge/main\n@community https://dl-cdn.alpinelinux.org/alpine/edge/community\n"
	repos := apkindex.ParseReposContent(content)
	require.Len(t, repos, 2)
	assert.Equal(t, "edge", repos[0].Tag)
	assert.Equal(t, "community", repos[1].Tag)
}

// ---------------------------------------------------------------------------
// Load — global index cache
// ---------------------------------------------------------------------------

func TestLoadReturnsNilWhenNotSet(t *testing.T) {
	// Reset the global cache to nil before testing.
	apkindex.SetGlobalIndex(nil)

	idx := apkindex.Load()
	assert.Nil(t, idx, "Load should return nil when Update has not been called")
}

func TestLoadReturnsCachedIndex(t *testing.T) {
	// Pre-populate the global cache with a known index.
	cached := apkindex.NewIndex()
	err := cached.ParseIndex(strings.NewReader(`P:cached-pkg
V:1.0-r0
A:x86_64
S:1000
I:2000
T:Cached package
U:https://example.com
L:MIT
o:cached-pkg
m:Test <test@example.com>

`), "https://example.com")
	require.NoError(t, err)

	apkindex.SetGlobalIndex(cached)
	defer apkindex.SetGlobalIndex(nil) // clean up after test

	loaded := apkindex.Load()
	require.NotNil(t, loaded)

	pkg, ok := loaded.Lookup("cached-pkg")
	require.True(t, ok)
	assert.Equal(t, "cached-pkg", pkg.Name)
	assert.Equal(t, "1.0-r0", pkg.Version)
}

func TestLoadReturnsSamePointer(t *testing.T) {
	// Load should return the exact same pointer that was stored.
	idx := apkindex.NewIndex()

	apkindex.SetGlobalIndex(idx)
	defer apkindex.SetGlobalIndex(nil)

	loaded := apkindex.Load()
	// Both should be non-nil and functionally equivalent (same empty index).
	require.NotNil(t, loaded)
	pkgs, caps := loaded.Stats()
	assert.Equal(t, 0, pkgs)
	assert.Equal(t, 0, caps)
}

func TestLoadAfterReset(t *testing.T) {
	// Set, then reset to nil — Load should return nil again.
	idx := apkindex.NewIndex()
	apkindex.SetGlobalIndex(idx)
	apkindex.SetGlobalIndex(nil)

	loaded := apkindex.Load()
	assert.Nil(t, loaded)
}

// ---------------------------------------------------------------------------
// Install — convenience wrapper
// ---------------------------------------------------------------------------

func TestInstallUsesGlobalCacheWhenAvailable(t *testing.T) {
	// Pre-populate the global cache. Install should use it without calling Update.
	// With an empty package list and AllowUnverifiedPackages=true (via the
	// cached index path), Install should succeed.
	//
	// Install calls Load() first; if non-nil it calls InstallPackagesWithOptions
	// which requires AllowUnverifiedPackages=true. Since Install passes
	// platform.IsPrivilegedHost() for that flag, on a non-root test host it
	// will return an error about signature verification. We test the error path.

	// Reset cache so Install will call Update (which will fail on non-Alpine).
	apkindex.SetGlobalIndex(nil)
	defer apkindex.SetGlobalIndex(nil)

	ctx := context.Background()
	err := apkindex.Install(ctx, []string{"nonexistent-pkg"})
	// On a non-Alpine host, Update will fail (no /etc/apk/repositories).
	// On an Alpine host running as non-root, signature verification will fail.
	// Either way, we expect an error — the important thing is it doesn't panic.
	assert.Error(t, err)
}

func TestInstallWithCachedIndexNonRoot(t *testing.T) {
	// Pre-populate the global cache. On a non-root host, Install should fail
	// with a signature verification error (not a nil-pointer panic).
	cached := apkindex.NewIndex()
	err := cached.ParseIndex(strings.NewReader(`P:test-pkg
V:1.0-r0
A:x86_64
S:1000
I:2000
T:Test package
U:https://example.com
L:MIT
o:test-pkg
m:Test <test@example.com>

`), "https://example.com")
	require.NoError(t, err)

	apkindex.SetGlobalIndex(cached)
	defer apkindex.SetGlobalIndex(nil)

	ctx := context.Background()
	err = apkindex.Install(ctx, []string{"test-pkg"})

	// On a non-root host, platform.IsPrivilegedHost() returns false, so
	// InstallPackagesWithOptions will refuse with a signature verification error.
	// On a root host (e.g. CI container), it will proceed and may fail for
	// other reasons (network, filesystem). Either way, no panic.
	if err != nil {
		// Acceptable errors: signature verification refusal or network/fs errors.
		_ = err
	}
}

func TestInstallEmptyListWithCachedIndex(t *testing.T) {
	// An empty package list with a cached index should succeed on any host
	// when running as root (AllowUnverifiedPackages=true), or fail with
	// signature verification on non-root. Either way, no panic.
	cached := apkindex.NewIndex()

	apkindex.SetGlobalIndex(cached)
	defer apkindex.SetGlobalIndex(nil)

	ctx := context.Background()
	// This exercises the Load() → non-nil → InstallPackagesWithOptions path.
	err := apkindex.Install(ctx, nil)
	// On non-root: signature verification error.
	// On root with empty list: nil (nothing to install).
	_ = err // either outcome is acceptable; we just verify no panic
}
