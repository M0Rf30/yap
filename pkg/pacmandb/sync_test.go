//nolint:testpackage // exercises unexported syncRepoTo / handleConfigKeyValue
package pacmandb

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestSyncRepoRemovesStaleSig: when the new .db is fetched but its .sig is
// unavailable, the previous .sig must not linger next to the new .db.
func TestSyncRepoRemovesStaleSig(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/core/os/x86_64/core.db" {
			_, _ = w.Write([]byte("new-db"))

			return
		}

		http.NotFound(w, r)
	}))
	t.Cleanup(srv.Close)

	dir := t.TempDir()
	sigPath := filepath.Join(dir, "core.db.sig")
	require.NoError(t, os.WriteFile(sigPath, []byte("old-sig"), 0o644))

	repo := Repo{Name: "core", Servers: []string{srv.URL + "/$repo/os/$arch"}}
	require.NoError(t, syncRepoTo(t.Context(), dir, repo, "x86_64"))

	got, err := os.ReadFile(filepath.Join(dir, "core.db"))
	require.NoError(t, err)
	assert.Equal(t, "new-db", string(got))

	_, statErr := os.Stat(sigPath)
	assert.True(t, os.IsNotExist(statErr), "stale .sig must be removed")
}

// TestSyncRepoReplacesSig: when a .sig is served it replaces the old one.
func TestSyncRepoReplacesSig(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/core/os/x86_64/core.db":
			_, _ = w.Write([]byte("db"))
		case "/core/os/x86_64/core.db.sig":
			_, _ = w.Write([]byte("new-sig"))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)

	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "core.db.sig"), []byte("old-sig"), 0o644))

	repo := Repo{Name: "core", Servers: []string{srv.URL + "/$repo/os/$arch"}}
	require.NoError(t, syncRepoTo(t.Context(), dir, repo, "x86_64"))

	got, err := os.ReadFile(filepath.Join(dir, "core.db.sig"))
	require.NoError(t, err)
	assert.Equal(t, "new-sig", string(got))
}

func TestHandleConfigKeyValueIncludeGlob(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "a.list"),
		[]byte("Server = https://a.example.org/$repo\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "b.list"),
		[]byte("Server = https://b.example.org/$repo\n"), 0o644))

	repo := &Repo{Name: "core"}
	require.NoError(t, handleConfigKeyValue(&Config{}, repo, "Include", filepath.Join(dir, "*.list")))
	assert.Equal(t, []string{"https://a.example.org/$repo", "https://b.example.org/$repo"}, repo.Servers)
}
