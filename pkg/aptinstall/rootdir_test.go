// SPDX-FileCopyrightText: 2021-2026 Gianluca Boiano
// SPDX-License-Identifier: GPL-3.0-only

package aptinstall_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/M0Rf30/yap/v2/pkg/aptinstall"
)

func TestRootedPaths(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "/var/lib/dpkg/status", aptinstall.RootedForTesting("", "/var/lib/dpkg/status"))
	assert.Equal(t, "/var/lib/dpkg/status", aptinstall.RootedForTesting("/", "/var/lib/dpkg/status"))
	assert.Equal(t, "/r/var/lib/dpkg/status",
		aptinstall.RootedForTesting("/r", "/var/lib/dpkg/status"))
}

// TestNonHostRootStaysInsideRoot verifies dpkg state is created only under a
// non-"/" RootDir.
func TestNonHostRootStaysInsideRoot(t *testing.T) {
	t.Parallel()

	root := t.TempDir()

	require.NoError(t, aptinstall.EnsureDpkgDirsAtForTesting(root))

	lock, err := aptinstall.AcquireDpkgLockAtForTesting(root)
	require.NoError(t, err)

	lock.Release()

	require.DirExists(t, filepath.Join(root, "var", "lib", "dpkg", "info"))
	require.FileExists(t, filepath.Join(root, "var", "lib", "dpkg", "lock"))

	err = aptinstall.WriteDpkgInfoFilesAtForTesting(root, "rootpkg-xyz", "amd64",
		&aptinstall.DebContentsForTesting{
			Control:    "Package: rootpkg-xyz\nVersion: 1\n",
			Files:      []string{"/usr/bin/x"},
			Scriptlets: map[string]string{"postinst": "#!/bin/sh\n"},
		})
	require.NoError(t, err)
	require.FileExists(t, filepath.Join(root, "var", "lib", "dpkg", "info", "rootpkg-xyz.list"))
	require.FileExists(t, filepath.Join(root, "var", "lib", "dpkg", "info", "rootpkg-xyz.postinst"))

	err = aptinstall.UpdateDpkgStatusAtRootForTesting(
		context.Background(), root, "rootpkg-xyz", "amd64",
		"Package: rootpkg-xyz\nVersion: 1\n")
	require.NoError(t, err)
	require.FileExists(t, filepath.Join(root, "var", "lib", "dpkg", "status"))

	// Nothing leaked onto the host.
	_, err = os.Stat("/var/lib/dpkg/info/rootpkg-xyz.list")
	assert.True(t, os.IsNotExist(err))
}
