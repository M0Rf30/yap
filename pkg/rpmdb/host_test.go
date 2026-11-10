// SPDX-FileCopyrightText: 2021-2026 Gianluca Boiano
// SPDX-License-Identifier: GPL-3.0-only

package rpmdb //nolint:testpackage

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// useRoot points the host reader at a throw-away root for the test.
func useRoot(t *testing.T, root string) {
	t.Helper()

	orig := hostRoot
	hostRoot = root

	t.Cleanup(func() { hostRoot = orig })
}

// hostWithPackage returns a root whose SQLite rpmdb holds test-pkg, written by
// the Writer: the header it serialises is what the reader has to decode.
func hostWithPackage(t *testing.T) string {
	t.Helper()

	root := t.TempDir()

	w, err := OpenWriter(t.Context(), filepath.Join(root, "var", "lib", "rpm", "rpmdb.sqlite"))
	require.NoError(t, err)
	require.NoError(t, w.Install(t.Context(), buildTestRPM(t), []InstalledFile{
		{Path: "/usr/bin/test", Mode: 0o755, Size: 10},
	}))
	require.NoError(t, w.Close())

	return root
}

func TestListInstalled(t *testing.T) {
	useRoot(t, hostWithPackage(t))

	names, err := ListInstalled(t.Context())
	require.NoError(t, err)
	assert.Equal(t, []string{"test-pkg"}, names)
}

func TestListInstalledProvides(t *testing.T) {
	useRoot(t, hostWithPackage(t))

	provides, err := ListInstalledProvides(t.Context())
	require.NoError(t, err)
	assert.Equal(t, []string{"test-pkg", "test-pkg-lib"}, provides)
}

func TestFilterInstalled(t *testing.T) {
	useRoot(t, hostWithPackage(t))

	missing, err := FilterInstalled(t.Context(), []string{"zlib", "test-pkg", "bash"})
	require.NoError(t, err)
	assert.Equal(t, []string{"zlib", "bash"}, missing)

	missing, err = FilterInstalled(t.Context(), []string{"test-pkg"})
	require.NoError(t, err)
	assert.Empty(t, missing)
}

// TestNoDatabase: a root without an rpm database reports ErrNoDatabase from
// every query, so that callers fall back to the rpm command.
func TestNoDatabase(t *testing.T) {
	useRoot(t, t.TempDir())

	_, err := ListInstalled(t.Context())
	require.ErrorIs(t, err, ErrNoDatabase)

	_, err = ListInstalledProvides(t.Context())
	require.ErrorIs(t, err, ErrNoDatabase)

	_, err = FilterInstalled(t.Context(), []string{"bash"})
	require.ErrorIs(t, err, ErrNoDatabase)
}

// TestCorruptDatabase: a file that is a database but not a readable one is an
// error, not "no database".
func TestCorruptDatabase(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "var", "lib", "rpm")
	require.NoError(t, os.MkdirAll(dir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "rpmdb.sqlite"), []byte("SQLite format 3\x00 not really"), 0o600))
	useRoot(t, root)

	_, err := ListInstalled(t.Context())
	require.Error(t, err)
	assert.NotErrorIs(t, err, ErrNoDatabase)
}
