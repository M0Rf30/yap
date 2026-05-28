package aptcache_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"path"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/M0Rf30/yap/v2/pkg/aptcache"
)

// The vendor-ffmpeg cross-build failure is the motivating real case:
// PKGBUILD declares vendor-ffmpeg only, but libavcodec.so has DT_NEEDED
// entries for libvpx.so / libx264.so which come from sibling packages
// (vendor-libvpx, vendor-x264) the PKGBUILD does not declare. Without
// transitive resolution, ld fails at cross-link time.
//
// We mimic that shape with three packages plus an already-installed libc6
// to confirm the "skip installed" path also works.
const vendorPackagesStanza = `Package: vendor-ffmpeg
Architecture: arm64
Version: 5.1.4
Filename: pool/main/c/vendor-ffmpeg/vendor-ffmpeg_5.1.4_arm64.deb
Size: 5
SHA256: %s
Depends: vendor-libvpx, vendor-x264, libc6
Description: ffmpeg with vendor patches

Package: vendor-libvpx
Architecture: arm64
Version: 1.13.1
Filename: pool/main/c/vendor-libvpx/vendor-libvpx_1.13.1_arm64.deb
Size: 5
SHA256: %s
Depends: libc6
Description: VP8/VP9 codec library

Package: vendor-x264
Architecture: arm64
Version: 164
Filename: pool/main/c/vendor-x264/vendor-x264_164_arm64.deb
Size: 5
SHA256: %s
Depends: libc6
Description: H.264 codec library

`

const vendorInstalledStanza = `Package: libc6
Status: install ok installed
Architecture: arm64
Version: 2.35-0ubuntu3
Description: GNU C Library

`

// TestDownloadClosurePullsTransitiveDeps is the regression test for the
// vendor-ffmpeg cross-build failure described above.
//
// Setup: declare only vendor-ffmpeg as a seed. Expectation:
//   - closure also contains vendor-libvpx and vendor-x264;
//   - libc6 is NOT in the closure because it is marked Installed;
//   - all three transitive .deb files land in destDir.
func TestDownloadClosurePullsTransitiveDeps(t *testing.T) {
	t.Parallel()

	// Each package's .deb is the literal string "data\n" → 5 bytes, hashable.
	const debBody = "data\n"

	sum := sha256.Sum256([]byte(debBody))
	hashHex := hex.EncodeToString(sum[:])

	mux := http.NewServeMux()
	srv := httptest.NewServer(mux)

	t.Cleanup(srv.Close)

	var (
		servedMu sync.Mutex
		served   = map[string]int{}
	)

	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		servedMu.Lock()
		served[path.Base(r.URL.Path)]++
		servedMu.Unlock()
		_, _ = w.Write([]byte(debBody))
	})

	packages := strings.ReplaceAll(vendorPackagesStanza, "%s", hashHex)

	c := aptcache.NewCacheForTesting()
	require.NoError(t, c.ParseDeb822WithBaseURLForTesting(
		strings.NewReader(packages), false, srv.URL+"/"))
	require.NoError(t, c.ParseDeb822ForTesting(
		strings.NewReader(vendorInstalledStanza), true))

	destDir := t.TempDir()

	resolved, unresolved, err := c.DownloadClosure(
		context.Background(), destDir, []string{"vendor-ffmpeg"})
	require.NoError(t, err)
	assert.Empty(t, unresolved, "all deps should resolve")

	names := make([]string, 0, len(resolved))
	for _, p := range resolved {
		names = append(names, p.Name)
	}

	assert.Contains(t, names, "vendor-ffmpeg", "the declared seed must be in the closure")
	assert.Contains(t, names, "vendor-libvpx", "transitive dep must be pulled")
	assert.Contains(t, names, "vendor-x264", "transitive dep must be pulled")
	assert.NotContains(t, names, "libc6",
		"already-installed deps must be skipped (their edges still walked)")

	// Three .deb files must actually have been fetched.
	assert.Equal(t, 1, served["vendor-ffmpeg_5.1.4_arm64.deb"])
	assert.Equal(t, 1, served["vendor-libvpx_1.13.1_arm64.deb"])
	assert.Equal(t, 1, served["vendor-x264_164_arm64.deb"])

	// Dependency order: a dependency must appear before its dependents.
	posFfmpeg := indexOf(names, "vendor-ffmpeg")
	posVpx := indexOf(names, "vendor-libvpx")
	posX264 := indexOf(names, "vendor-x264")

	assert.Less(t, posVpx, posFfmpeg, "libvpx must come before ffmpeg")
	assert.Less(t, posX264, posFfmpeg, "x264 must come before ffmpeg")
}

// TestDownloadClosureHandlesDiamond verifies that a package reachable via
// two distinct paths is downloaded exactly once.
func TestDownloadClosureHandlesDiamond(t *testing.T) {
	t.Parallel()

	const debBody = "x"

	sum := sha256.Sum256([]byte(debBody))
	hashHex := hex.EncodeToString(sum[:])

	const diamondStanza = `Package: top
Architecture: amd64
Version: 1.0
Filename: pool/t/top.deb
Size: 1
SHA256: %s
Depends: left, right
Description: top of diamond

Package: left
Architecture: amd64
Version: 1.0
Filename: pool/l/left.deb
Size: 1
SHA256: %s
Depends: bottom
Description: left arm

Package: right
Architecture: amd64
Version: 1.0
Filename: pool/r/right.deb
Size: 1
SHA256: %s
Depends: bottom
Description: right arm

Package: bottom
Architecture: amd64
Version: 1.0
Filename: pool/b/bottom.deb
Size: 1
SHA256: %s
Description: bottom of diamond

`

	mux := http.NewServeMux()
	srv := httptest.NewServer(mux)

	t.Cleanup(srv.Close)

	var (
		hitsMu sync.Mutex
		hits   = map[string]int{}
	)

	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		hitsMu.Lock()
		hits[path.Base(r.URL.Path)]++
		hitsMu.Unlock()
		_, _ = w.Write([]byte(debBody))
	})

	stanza := strings.ReplaceAll(diamondStanza, "%s", hashHex)

	c := aptcache.NewCacheForTesting()
	require.NoError(t, c.ParseDeb822WithBaseURLForTesting(
		strings.NewReader(stanza), false, srv.URL+"/"))

	resolved, _, err := c.DownloadClosure(
		context.Background(), t.TempDir(), []string{"top"})
	require.NoError(t, err)

	// Exactly 4 packages, bottom counted once.
	assert.Len(t, resolved, 4)
	assert.Equal(t, 1, hits["bottom.deb"], "diamond bottom must be fetched once, not twice")
}

func indexOf(s []string, target string) int {
	for i, v := range s {
		if v == target {
			return i
		}
	}

	return -1
}
