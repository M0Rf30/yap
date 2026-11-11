// SPDX-FileCopyrightText: 2021-2026 Gianluca Boiano
// SPDX-License-Identifier: GPL-3.0-only

//nolint:testpackage // exercises unexported syncTo
package pacmandb

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/M0Rf30/pkgformat/gpg"
	"github.com/M0Rf30/pkgformat/httpclient"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func noRetry() *httpclient.Client {
	return httpclient.New(httpclient.Options{Retry: httpclient.RetryPolicy{Attempts: 1, BaseDelay: -1}})
}

// noKeyring is a keyring directory without any key.
func noKeyring(t *testing.T) string {
	t.Helper()

	return t.TempDir()
}

// withKey is a keyring directory holding one freshly generated public key.
func withKey(t *testing.T) string {
	t.Helper()

	entity, err := gpg.GenerateEntity("test", "", "test@example.com", gpg.AlgorithmED25519)
	require.NoError(t, err)

	f, err := os.Create(filepath.Join(t.TempDir(), "test.gpg"))
	require.NoError(t, err)

	require.NoError(t, entity.Serialize(f))
	require.NoError(t, f.Close())

	return filepath.Dir(f.Name())
}

func writeConf(t *testing.T, body string) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), "pacman.conf")
	require.NoError(t, os.WriteFile(path, []byte(body), 0o644))

	return path
}

// repoDB is a minimal valid empty gzip database the server hands out.
var repoDB = []byte{
	0x1f, 0x8b, 0x08, 0, 0, 0, 0, 0, 0, 0xff, 0x1, 0, 0, 0xff, 0xff, 0, 0, 0, 0, 0, 0, 0, 0, 0,
}

func TestSyncWritesDatabasesFromConfAndMirrorlist(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/core/os/x86_64/core.db", "/extra/os/x86_64/extra.db":
			_, _ = w.Write(repoDB)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)

	mirrors := filepath.Join(t.TempDir(), "mirrorlist")
	require.NoError(t, os.WriteFile(mirrors, []byte("Server = "+srv.URL+"/$repo/os/$arch\n"), 0o644))

	conf := writeConf(t, "[options]\nArchitecture = x86_64\n\n[core]\nInclude = "+mirrors+
		"\n\n[extra]\nServer = "+srv.URL+"/$repo/os/$arch\n")
	dir := filepath.Join(t.TempDir(), "sync")

	n, err := syncTo(t.Context(), conf, dir, noKeyring(t), noRetry())
	require.NoError(t, err)
	assert.Equal(t, 2, n)

	for _, name := range []string{"core.db", "extra.db"} {
		assert.FileExists(t, filepath.Join(dir, name))
	}
}

func TestSyncRemovesStaleSignature(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/core/os/x86_64/core.db" {
			_, _ = w.Write(repoDB)

			return
		}

		http.NotFound(w, r)
	}))
	t.Cleanup(srv.Close)

	dir := t.TempDir()
	sig := filepath.Join(dir, "core.db.sig")
	require.NoError(t, os.WriteFile(sig, []byte("old-sig"), 0o644))

	conf := writeConf(t, "[options]\nArchitecture = x86_64\n[core]\nServer = "+srv.URL+"/$repo/os/$arch\n")

	n, err := syncTo(t.Context(), conf, dir, noKeyring(t), noRetry())
	require.NoError(t, err)
	assert.Equal(t, 1, n)
	assert.NoFileExists(t, sig, "a signature of the previous database must not linger")
}

func TestSyncKeepsGoingAfterFailedRepo(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/good/x86_64/good.db" {
			_, _ = w.Write(repoDB)

			return
		}

		http.NotFound(w, r)
	}))
	t.Cleanup(srv.Close)

	conf := writeConf(t, "[options]\nArchitecture = x86_64\n[bad]\nServer = "+srv.URL+"/nowhere\n"+
		"[good]\nServer = "+srv.URL+"/$repo/$arch\n")
	dir := t.TempDir()

	n, err := syncTo(t.Context(), conf, dir, noKeyring(t), noRetry())
	require.Error(t, err)
	assert.Equal(t, 1, n)
	assert.FileExists(t, filepath.Join(dir, "good.db"))
}

func TestSyncMissingConf(t *testing.T) {
	absent := filepath.Join(t.TempDir(), "absent.conf")

	n, err := syncTo(t.Context(), absent, t.TempDir(), noKeyring(t), noRetry())
	require.Error(t, err)
	assert.Zero(t, n)
}

func serveCoreDB(t *testing.T) *httptest.Server {
	t.Helper()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/core/os/x86_64/core.db" {
			_, _ = w.Write(repoDB)

			return
		}

		http.NotFound(w, r)
	}))
	t.Cleanup(srv.Close)

	return srv
}

func TestSyncEnforcesSigLevelWhenKeysAreTrusted(t *testing.T) {
	srv := serveCoreDB(t)
	conf := writeConf(t, "[options]\nArchitecture = x86_64\nSigLevel = DatabaseRequired\n"+
		"[core]\nServer = "+srv.URL+"/$repo/os/$arch\n")
	dir := t.TempDir()

	n, err := syncTo(t.Context(), conf, dir, withKey(t), noRetry())
	require.Error(t, err, "the served database carries no signature")
	assert.Zero(t, n)
	assert.NoFileExists(t, filepath.Join(dir, "core.db"))
}

func TestSyncSkipsReposWithoutSyncUsage(t *testing.T) {
	srv := serveCoreDB(t)
	conf := writeConf(t, "[options]\nArchitecture = x86_64\n"+
		"[core]\nServer = "+srv.URL+"/$repo/os/$arch\nUsage = Search\n")
	dir := t.TempDir()

	n, err := syncTo(t.Context(), conf, dir, noKeyring(t), noRetry())
	require.NoError(t, err)
	assert.Zero(t, n)
	assert.NoFileExists(t, filepath.Join(dir, "core.db"))
}
