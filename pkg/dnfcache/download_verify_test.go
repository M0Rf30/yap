// SPDX-FileCopyrightText: 2021-2026 Gianluca Boiano
// SPDX-License-Identifier: GPL-3.0-only

//nolint:testpackage
package dnfcache

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
	"sync/atomic"
	"testing"
	"time"

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

// TestDownloadAllFailsFast verifies that the first failed download cancels
// the remaining ones instead of letting them run to completion.
func TestDownloadAllFailsFast(t *testing.T) {
	var slowHits atomic.Int32

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "bad.rpm") {
			http.NotFound(w, r)
			return
		}

		slowHits.Add(1)

		select {
		case <-r.Context().Done():
		case <-time.After(10 * time.Second):
		}
	}))
	defer srv.Close()

	pkgs := []*PackageInfo{{Name: "bad", BaseURL: srv.URL + "/", LocationHref: "bad.rpm"}}

	for i := range 12 {
		name := fmt.Sprintf("slow%d", i)
		pkgs = append(pkgs, &PackageInfo{
			Name: name, BaseURL: srv.URL + "/", LocationHref: name + ".rpm",
		})
	}

	start := time.Now()

	_, err := newCache().downloadAll(context.Background(), pkgs, t.TempDir())
	require.Error(t, err)
	assert.Less(t, time.Since(start), 5*time.Second)
	assert.Less(t, int(slowHits.Load()), len(pkgs)-1, "queued downloads must be skipped")
}

// TestResolveDepsSoftThenHardUnresolved verifies a name first reached via a
// weak edge is still reported when later required as a hard dependency.
func TestResolveDepsSoftThenHardUnresolved(t *testing.T) {
	c := newCache()

	c.mu.Lock()
	c.addPackage(&PackageInfo{
		Name: "app", Arch: "x86_64", LocationHref: "app.rpm",
		Recommends: []string{"ghost"}, Requires: []string{"ghost"},
	})
	c.mu.Unlock()

	_, unres, err := c.ResolveDeps(context.Background(), []string{"app"})
	require.NoError(t, err)

	// Requires are walked before Recommends, so hard comes first here; flip
	// the order via a second package to cover soft-then-hard.
	assert.Contains(t, unres, "ghost")

	c2 := newCache()

	c2.mu.Lock()
	c2.addPackage(&PackageInfo{
		Name: "weak", Arch: "x86_64", LocationHref: "weak.rpm", Recommends: []string{"ghost"},
	})
	c2.addPackage(&PackageInfo{
		Name: "hard", Arch: "x86_64", LocationHref: "hard.rpm", Requires: []string{"ghost"},
	})
	c2.mu.Unlock()

	_, unres, err = c2.ResolveDeps(context.Background(), []string{"weak", "hard"})
	require.NoError(t, err)
	assert.Equal(t, []string{"ghost"}, unres)
}
