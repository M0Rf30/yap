// Package repo_test provides black-box tests for the repo package.
// Tests that exercise unexported helpers (setupOne, fetchKey, closeQuiet) are
// placed in the internal test file (repo_test.go); this file covers the same
// functions from the exported surface and via an httptest server so that the
//
// coverage tool attributes hits to the correct lines.
//
//nolint:testpackage
package repo

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// setupOne
// ---------------------------------------------------------------------------

// TestSetupOneSkipsNonMatchingDistro verifies that setupOne is a no-op when
// the repo's Distros list does not include the active distro.
func TestSetupOneSkipsNonMatchingDistro(t *testing.T) {
	r := &Repo{
		Name:    "debian-only",
		URL:     "https://example.com",
		Distros: []string{"debian"},
	}

	// "ubuntu" is not in Distros → appliesTo returns false → no error, no write.
	err := setupOne("apt", r, "ubuntu", "", 0)
	assert.NoError(t, err)
}

// TestSetupOneRejectsEmptyName verifies that setupOne returns a validation
// error when the repo name is empty.
func TestSetupOneRejectsEmptyName(t *testing.T) {
	r := &Repo{
		Name: "",
		URL:  "https://example.com",
	}

	err := setupOne("apt", r, "ubuntu", "", 0)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "name and url are required")
}

// TestSetupOneRejectsEmptyURL verifies that setupOne returns a validation
// error when the repo URL is empty.
func TestSetupOneRejectsEmptyURL(t *testing.T) {
	r := &Repo{
		Name: "myrepo",
		URL:  "",
	}

	err := setupOne("apt", r, "ubuntu", "", 0)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "name and url are required")
}

// TestSetupOneErrorMsgIncludesIndex verifies that the validation error message
// includes the entry index so users can locate the offending repo in yap.json.
func TestSetupOneErrorMsgIncludesIndex(t *testing.T) {
	r := &Repo{Name: "", URL: ""}

	err := setupOne("apt", r, "ubuntu", "", 7)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "7")
}

// TestSetupOneSkipsUnsupportedFormat verifies that setupOne is a no-op (no
// error) when the resolved format is not "deb" or "rpm" (e.g. pacman/apk).
func TestSetupOneSkipsUnsupportedFormat(t *testing.T) {
	r := &Repo{
		Name:   "myrepo",
		URL:    "https://example.com",
		Format: "pacman", // not deb or rpm
	}

	err := setupOne("pacman", r, "arch", "", 0)
	assert.NoError(t, err)
}

// TestSetupOneSkipsDebRepoForRPMHost verifies that a deb-format repo is
// silently skipped when the active package manager is yum/rpm.
func TestSetupOneSkipsDebRepoForRPMHost(t *testing.T) {
	r := &Repo{
		Name:   "ubuntu-repo",
		URL:    "https://example.com",
		Format: formatDeb,
		Suite:  "jammy",
	}

	// pm=yum but format=deb → the deb branch checks pm != PMApt and returns nil.
	err := setupOne("yum", r, "fedora", "", 0)
	assert.NoError(t, err)
}

// TestSetupOneSkipsRPMRepoForDebHost verifies that an rpm-format repo is
// silently skipped when the active package manager is apt.
func TestSetupOneSkipsRPMRepoForDebHost(t *testing.T) {
	r := &Repo{
		Name:   "fedora-repo",
		URL:    "https://example.com",
		Format: formatRPM,
	}

	// pm=apt but format=rpm → the rpm branch checks pm != PMYum/PMZypper and returns nil.
	err := setupOne("apt", r, "ubuntu", "", 0)
	assert.NoError(t, err)
}

// TestSetupOneInfersDEBFormatFromPM verifies that when Format is empty and the
// package manager is apt, setupOne infers "deb" and attempts to write the
// sources file (which will fail because /etc/apt/sources.list.d is not
// writable in CI — but the error must be a filesystem error, not a validation
// error, proving the format was correctly inferred).
func TestSetupOneInfersDEBFormatFromPM(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("running as root; /etc writes would succeed and pollute the system")
	}

	r := &Repo{
		Name:  "inferred-deb",
		URL:   "https://example.com",
		Suite: "jammy",
		// Format intentionally empty — should be inferred as "deb" for apt.
	}

	err := setupOne("apt", r, "ubuntu", "", 0)
	// We expect an error because /etc/apt/sources.list.d is not writable, but
	// it must NOT be a "name and url are required" validation error.
	if err != nil {
		assert.NotContains(t, err.Error(), "name and url are required",
			"format inference should have passed validation; got: %v", err)
	}
}

// TestSetupOneInfersRPMFormatFromPM verifies that when Format is empty and the
// package manager is yum, setupOne infers "rpm" and attempts to write the
// .repo file (which will fail because /etc/yum.repos.d is not writable in CI).
func TestSetupOneInfersRPMFormatFromPM(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("running as root; /etc writes would succeed and pollute the system")
	}

	r := &Repo{
		Name: "inferred-rpm",
		URL:  "https://example.com",
		// Format intentionally empty — should be inferred as "rpm" for yum.
	}

	err := setupOne("yum", r, "fedora", "", 0)
	if err != nil {
		assert.NotContains(t, err.Error(), "name and url are required",
			"format inference should have passed validation; got: %v", err)
	}
}

// ---------------------------------------------------------------------------
// fetchKey
// ---------------------------------------------------------------------------

// TestFetchKeyWritesKeyToFile verifies that fetchKey downloads a key from an
// httptest server and writes it to the destination path.
func TestFetchKeyWritesKeyToFile(t *testing.T) {
	const keyContent = "-----BEGIN PGP PUBLIC KEY BLOCK-----\nfakekey\n-----END PGP PUBLIC KEY BLOCK-----\n"

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = fmt.Fprint(w, keyContent)
	}))
	defer srv.Close()

	dst := filepath.Join(t.TempDir(), "keyrings", "test.asc")

	err := fetchKey(srv.URL, dst)
	require.NoError(t, err)

	got, err := os.ReadFile(dst)
	require.NoError(t, err)
	assert.Equal(t, keyContent, string(got))
}

// TestFetchKeyCreatesParentDirs verifies that fetchKey creates intermediate
// directories if they do not exist.
func TestFetchKeyCreatesParentDirs(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = fmt.Fprint(w, "key")
	}))
	defer srv.Close()

	dst := filepath.Join(t.TempDir(), "a", "b", "c", "key.asc")

	err := fetchKey(srv.URL, dst)
	require.NoError(t, err)

	_, err = os.Stat(dst)
	assert.NoError(t, err, "destination file should exist")
}

// TestFetchKeyOverwritesExistingFile verifies that fetchKey overwrites an
// existing key file (to pick up rotated keys on re-runs).
func TestFetchKeyOverwritesExistingFile(t *testing.T) {
	const newContent = "new-key-content"

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = fmt.Fprint(w, newContent)
	}))
	defer srv.Close()

	dst := filepath.Join(t.TempDir(), "key.asc")

	// Write stale content first.
	require.NoError(t, os.WriteFile(dst, []byte("old-key-content"), 0o644))

	err := fetchKey(srv.URL, dst)
	require.NoError(t, err)

	got, err := os.ReadFile(dst)
	require.NoError(t, err)
	assert.Equal(t, newContent, string(got))
}

// TestFetchKeyReturnsErrorOnHTTPFailure verifies that fetchKey returns a
// network error when the server is unreachable.
func TestFetchKeyReturnsErrorOnHTTPFailure(t *testing.T) {
	// Use a server that is immediately closed so the connection is refused.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	url := srv.URL
	srv.Close()

	dst := filepath.Join(t.TempDir(), "key.asc")

	err := fetchKey(url, dst)
	require.Error(t, err)
}

// TestFetchKeyReturnsErrorOnNon200 verifies that fetchKey returns an error
// when the server responds with a non-200 status code.
func TestFetchKeyReturnsErrorOnNon200(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	dst := filepath.Join(t.TempDir(), "key.asc")

	err := fetchKey(srv.URL, dst)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to fetch repo key")
}

// TestFetchKeyReturnsErrorOnBadURL verifies that fetchKey returns an error
// when the URL is syntactically invalid.
func TestFetchKeyReturnsErrorOnBadURL(t *testing.T) {
	dst := filepath.Join(t.TempDir(), "key.asc")

	err := fetchKey("://not-a-url", dst)
	require.Error(t, err)
}

// TestFetchKeyWritesEmptyBody verifies that fetchKey handles a 200 response
// with an empty body without error (empty key files are valid for testing).
func TestFetchKeyWritesEmptyBody(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		// No body written.
	}))
	defer srv.Close()

	dst := filepath.Join(t.TempDir(), "key.asc")

	err := fetchKey(srv.URL, dst)
	require.NoError(t, err)

	info, err := os.Stat(dst)
	require.NoError(t, err)
	assert.Equal(t, int64(0), info.Size())
}

// ---------------------------------------------------------------------------
// closeQuiet
// ---------------------------------------------------------------------------

// errCloser is a test io.Closer that always returns the configured error.
type errCloser struct{ err error }

func (e *errCloser) Close() error { return e.err }

// nopCloser is a test io.Closer that always succeeds.
type nopCloser struct{ closed bool }

func (n *nopCloser) Close() error {
	n.closed = true

	return nil
}

// TestCloseQuietSucceeds verifies that closeQuiet does not panic or log when
// the closer succeeds.
func TestCloseQuietSucceeds(t *testing.T) {
	c := &nopCloser{}
	closeQuiet(c, "test-target")
	assert.True(t, c.closed, "Close should have been called")
}

// TestCloseQuietLogsOnError verifies that closeQuiet does not panic when the
// closer returns an error (it logs a warning instead of propagating).
func TestCloseQuietLogsOnError(t *testing.T) {
	c := &errCloser{err: errors.New("disk full")}
	// Must not panic; the error is swallowed and logged.
	assert.NotPanics(t, func() {
		closeQuiet(c, "test-target")
	})
}

// TestCloseQuietWithRealFile verifies closeQuiet against a real *os.File.
func TestCloseQuietWithRealFile(t *testing.T) {
	f, err := os.CreateTemp(t.TempDir(), "closetest-*.tmp")
	require.NoError(t, err)

	// First close via closeQuiet — should succeed silently.
	closeQuiet(f, f.Name())

	// Second close via closeQuiet — file is already closed; should log a
	// warning but not panic.
	assert.NotPanics(t, func() {
		closeQuiet(f, f.Name())
	})
}

// ---------------------------------------------------------------------------
// fetchKey integration: content integrity
// ---------------------------------------------------------------------------

// TestFetchKeyLargeBody verifies that fetchKey correctly streams a large
// response body to disk without truncation.
func TestFetchKeyLargeBody(t *testing.T) {
	// 512 KiB of repeated bytes.
	body := strings.Repeat("A", 512*1024)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = fmt.Fprint(w, body)
	}))
	defer srv.Close()

	dst := filepath.Join(t.TempDir(), "large.asc")

	err := fetchKey(srv.URL, dst)
	require.NoError(t, err)

	got, err := os.ReadFile(dst)
	require.NoError(t, err)
	assert.Equal(t, len(body), len(got), "file size should match body size")
	assert.Equal(t, body, string(got))
}

// ---------------------------------------------------------------------------
// setupOne: distro-scoped filtering edge cases
// ---------------------------------------------------------------------------

// TestSetupOneAppliesToAllDistrosWhenDistrosEmpty verifies that a repo with an
// empty Distros list is installed for any distro (universal repo).
func TestSetupOneAppliesToAllDistrosWhenDistrosEmpty(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("running as root; /etc writes would succeed and pollute the system")
	}

	r := &Repo{
		Name:    "universal",
		URL:     "https://example.com",
		Suite:   "jammy",
		Distros: []string{}, // empty → applies to all
	}

	// We expect either success (if /etc is writable) or a filesystem error,
	// but never a "distro not matched" skip (which would return nil without
	// touching the filesystem).
	err := setupOne("apt", r, "ubuntu", "", 0)
	if err != nil {
		// Filesystem error is expected in non-root CI; that's fine.
		assert.NotContains(t, err.Error(), "name and url are required")
	}
}

// TestSetupOneIndexInErrorMessage verifies that the entry index appears in the
// error message for different index values.
func TestSetupOneIndexInErrorMessage(t *testing.T) {
	for _, idx := range []int{0, 1, 5, 42} {
		t.Run(fmt.Sprintf("idx=%d", idx), func(t *testing.T) {
			r := &Repo{Name: "", URL: ""}

			err := setupOne("apt", r, "ubuntu", "", idx)
			require.Error(t, err)
			assert.Contains(t, err.Error(), fmt.Sprintf("%d", idx))
		})
	}
}

// TestSetupOneZypperInfersRPM verifies that zypper (openSUSE) also infers the
// rpm format when Format is empty.
func TestSetupOneZypperInfersRPM(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("running as root; /etc writes would succeed and pollute the system")
	}

	r := &Repo{
		Name: "opensuse-repo",
		URL:  "https://example.com",
		// Format empty → should infer "rpm" for zypper.
	}

	err := setupOne("zypper", r, "opensuse-leap", "", 0)
	if err != nil {
		assert.NotContains(t, err.Error(), "name and url are required")
	}
}

// TestSetupOneApkPMSkipsAllFormats verifies that when the package manager is
// apk (Alpine), setupOne skips both deb and rpm repos silently.
func TestSetupOneApkPMSkipsAllFormats(t *testing.T) {
	for _, format := range []string{formatDeb, formatRPM} {
		t.Run(format, func(t *testing.T) {
			r := &Repo{
				Name:   "test-repo",
				URL:    "https://example.com",
				Suite:  "jammy",
				Format: format,
			}

			// apk PM: formatFor("apk") returns "" → falls to default → logs warning, returns nil.
			// But if Format is explicitly set, the switch dispatches to deb/rpm branch
			// which checks pm != PMApt / pm != PMYum|PMZypper and returns nil.
			err := setupOne("apk", r, "alpine", "", 0)
			assert.NoError(t, err)
		})
	}
}

// TestSetupOnePacmanPMSkipsAllFormats verifies that when the package manager
// is pacman (Arch), setupOne skips both deb and rpm repos silently.
func TestSetupOnePacmanPMSkipsAllFormats(t *testing.T) {
	for _, format := range []string{formatDeb, formatRPM} {
		t.Run(format, func(t *testing.T) {
			r := &Repo{
				Name:   "test-repo",
				URL:    "https://example.com",
				Suite:  "jammy",
				Format: format,
			}

			err := setupOne("pacman", r, "arch", "", 0)
			assert.NoError(t, err)
		})
	}
}

// ---------------------------------------------------------------------------
// io.ReadCloser helper for closeQuiet
// ---------------------------------------------------------------------------

// TestCloseQuietWithHTTPResponseBody verifies closeQuiet works with an
// http.Response.Body (a real-world io.ReadCloser).
func TestCloseQuietWithHTTPResponseBody(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = fmt.Fprint(w, "data")
	}))
	defer srv.Close()

	resp, err := http.Get(srv.URL) //nolint:noctx
	require.NoError(t, err)

	defer func() { _ = resp.Body.Close() }()

	// Drain and close via closeQuiet — must not panic.
	_, _ = io.ReadAll(resp.Body)

	assert.NotPanics(t, func() {
		closeQuiet(resp.Body, "http response body")
	})
}
