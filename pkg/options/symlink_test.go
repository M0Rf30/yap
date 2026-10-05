//nolint:testpackage // Internal testing of options package methods
package options

import (
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestZipManSymlinks(t *testing.T) {
	t.Parallel()

	pkgDir := t.TempDir()
	manDir := filepath.Join(pkgDir, "usr", "share", "man", "man1")
	require.NoError(t, os.MkdirAll(manDir, 0o755))

	require.NoError(t, os.WriteFile(filepath.Join(manDir, "b.1"), []byte("page"), 0o644))
	// Link sorts before its target so the target is gzipped later in the walk.
	require.NoError(t, os.Symlink("b.1", filepath.Join(manDir, "a.1")))

	// Absolute link to a host file: must be neither read nor replaced.
	host := filepath.Join(t.TempDir(), "host.1")
	require.NoError(t, os.WriteFile(host, []byte("host"), 0o644))
	require.NoError(t, os.Symlink(host, filepath.Join(manDir, "c.1")))

	require.NoError(t, ZipMan(pkgDir))

	assert.FileExists(t, filepath.Join(manDir, "b.1.gz"))

	// Relative link re-pointed at the compressed target.
	dest, err := os.Readlink(filepath.Join(manDir, "a.1.gz"))
	require.NoError(t, err)
	assert.Equal(t, "b.1.gz", dest)
	assert.NoFileExists(t, filepath.Join(manDir, "a.1"))

	// Absolute link untouched, host file untouched.
	info, err := os.Lstat(filepath.Join(manDir, "c.1"))
	require.NoError(t, err)
	assert.NotZero(t, info.Mode()&fs.ModeSymlink)
	assert.NoFileExists(t, filepath.Join(manDir, "c.1.gz"))
	assert.FileExists(t, host)
}

func TestProcessFileSkipsSymlinks(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	target := filepath.Join(t.TempDir(), "host-bin")
	require.NoError(t, os.WriteFile(target, []byte("x"), 0o444))

	link := filepath.Join(dir, "link")
	require.NoError(t, os.Symlink(target, link))

	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	require.Len(t, entries, 1)

	require.NoError(t, processFile(link, entries[0], nil))

	info, err := os.Stat(target)
	require.NoError(t, err)
	assert.Equal(t, fs.FileMode(0o444), info.Mode().Perm(), "target must not be chmod'ed")
}
