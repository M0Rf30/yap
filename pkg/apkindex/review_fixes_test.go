// SPDX-FileCopyrightText: 2021-2026 Gianluca Boiano
// SPDX-License-Identifier: GPL-3.0-only

package apkindex_test

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/M0Rf30/yap/v2/pkg/apkindex"
)

func TestResolveDepsDedupesVirtualProvider(t *testing.T) {
	const input = `P:musl
V:1.0-r0
A:x86_64
p:so:libc.musl-x86_64.so.1

P:app
V:1.0-r0
A:x86_64
D:musl so:libc.musl-x86_64.so.1

`

	idx := apkindex.NewIndex()
	require.NoError(t, idx.ParseIndex(strings.NewReader(input), "https://x"))

	resolved, err := idx.ResolveDeps([]string{"app"})
	require.NoError(t, err)
	assert.Len(t, resolved, 2)
}

func TestRegisterInstalledKeepsEarlierPackages(t *testing.T) {
	dir := t.TempDir()
	pkgInfo := "pkgname = a\npkgver = 1\n"

	require.NoError(t, apkindex.ExportRegisterInstalled(dir,
		&apkindex.Package{Name: "a", Version: "1", Arch: "x86_64", InstSize: 1234}, pkgInfo))
	require.NoError(t, apkindex.ExportRegisterInstalled(dir,
		&apkindex.Package{Name: "b", Version: "2", Arch: "x86_64"}, ""))

	data, err := os.ReadFile(filepath.Join(dir, "lib", "apk", "db", "installed"))
	require.NoError(t, err)
	assert.Contains(t, string(data), "P:a\n")
	assert.Contains(t, string(data), "P:b\n")
	assert.Contains(t, string(data), "I:1234\n")
}

func TestBuildDownloadRejectsUnsafeName(t *testing.T) {
	const input = `P:evil
V:../../x
A:x86_64

`

	idx := apkindex.NewIndex()
	require.NoError(t, idx.ParseIndex(strings.NewReader(input), "https://x"))

	_, _, err := apkindex.ExportBuildAPKDownloadRequests(
		context.Background(), idx, t.TempDir(), []string{"evil"})
	require.Error(t, err)
}

func apkDataTar(t *testing.T, entries []tar.Header, bodies map[string]string) *bytes.Buffer {
	t.Helper()

	var raw bytes.Buffer

	tw := tar.NewWriter(&raw)

	for i := range entries {
		h := entries[i]
		h.Size = int64(len(bodies[h.Name]))

		require.NoError(t, tw.WriteHeader(&h))

		_, err := tw.Write([]byte(bodies[h.Name]))
		require.NoError(t, err)
	}

	require.NoError(t, tw.Close())

	var gz bytes.Buffer

	zw := gzip.NewWriter(&gz)
	_, err := zw.Write(raw.Bytes())
	require.NoError(t, err)
	require.NoError(t, zw.Close())

	return &gz
}

func TestExtractAPKSymlinkEscapeContained(t *testing.T) {
	base := t.TempDir()
	root := filepath.Join(base, "root")
	outside := filepath.Join(base, "outside")

	require.NoError(t, os.MkdirAll(root, 0o755))
	require.NoError(t, os.MkdirAll(outside, 0o755))

	buf := apkDataTar(t, []tar.Header{
		{Name: "link", Typeflag: tar.TypeSymlink, Linkname: outside, Mode: 0o777},
		{Name: "link/x", Typeflag: tar.TypeReg, Mode: 0o644},
		{Name: "usr/lib/", Typeflag: tar.TypeDir, Mode: 0o755},
		{Name: "lib", Typeflag: tar.TypeSymlink, Linkname: "usr/lib", Mode: 0o777},
		{Name: "lib/y", Typeflag: tar.TypeReg, Mode: 0o4755},
	}, map[string]string{"link/x": "pwn", "lib/y": "ok"})

	require.NoError(t, apkindex.ExportExtractAPKDataTo(buf, root))

	_, err := os.Stat(filepath.Join(outside, "x"))
	assert.True(t, os.IsNotExist(err), "write escaped root")

	fi, err := os.Stat(filepath.Join(root, "usr", "lib", "y"))
	require.NoError(t, err)
	assert.NotZero(t, fi.Mode()&os.ModeSetuid, "setuid bit preserved")
}
