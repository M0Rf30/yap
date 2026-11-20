// SPDX-FileCopyrightText: 2021-2026 Gianluca Boiano
// SPDX-License-Identifier: GPL-3.0-only

package dnfinstall //nolint:testpackage

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/M0Rf30/pkgformat/rpm/dnf"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// useEmptyRepos points the process-global client at a host with no
// repositories, so that every requested package is unresolvable, and restores
// the previous state afterwards.
func useEmptyRepos(t *testing.T) {
	t.Helper()

	hostState.mu.Lock()
	prevClient, prevIndex := hostState.client, hostState.index
	hostState.client = dnf.New(dnf.Options{CacheDir: t.TempDir()})
	hostState.index = nil
	hostState.mu.Unlock()

	t.Cleanup(func() {
		hostState.mu.Lock()
		defer hostState.mu.Unlock()

		hostState.client, hostState.index = prevClient, prevIndex
	})
}

func TestUnresolvedErrorNone(t *testing.T) {
	require.NoError(t, unresolvedError("test", nil))
}

func TestUnresolvedErrorNamesPackages(t *testing.T) {
	err := unresolvedError("test", []string{"gcc", "libfoo-devel"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unresolvable packages: [gcc libfoo-devel]")
}

// TestInstallWithRPMFailsOnUnresolvedMakedepends: a makedepends entry that no
// repository provides stops the build, as it does with apt, instead of going on
// to fail later when the missing tool is needed.
func TestInstallWithRPMFailsOnUnresolvedMakedepends(t *testing.T) {
	useEmptyRepos(t)

	err := InstallWithRPM(t.Context(), []string{"gcc"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unresolvable packages: [gcc]")
}

func TestInstallWithOptionsFailsOnUnresolvedMakedepends(t *testing.T) {
	useEmptyRepos(t)

	err := InstallWithOptions(t.Context(), []string{"gcc"}, Options{AllowRootInstall: true})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unresolvable packages: [gcc]")
}

// TestUnavailableRepos: an enabled repository whose metadata is not in the
// cache after a refresh is reported; disabled ones and refreshed ones are not.
func TestUnavailableRepos(t *testing.T) {
	root := t.TempDir()
	repoDir := filepath.Join(root, "repos")
	cacheDir := filepath.Join(root, "cache")

	require.NoError(t, os.MkdirAll(repoDir, 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(cacheDir, "fresh", "repodata"), 0o755))
	require.NoError(t, os.WriteFile(
		filepath.Join(cacheDir, "fresh", "repodata", "repomd.xml"), []byte("<repomd/>"), 0o600))

	repos := "[fresh]\nbaseurl=http://example.invalid/fresh\n" +
		"[gone]\nmirrorlist=http://example.invalid/list\n" +
		"[off]\nbaseurl=http://example.invalid/off\nenabled=0\n"
	require.NoError(t, os.WriteFile(filepath.Join(repoDir, "t.repo"), []byte(repos), 0o600))

	client := dnf.New(dnf.Options{RepoDir: repoDir, CacheDir: cacheDir})

	assert.Equal(t, []string{"gone"}, unavailableRepos(client))
}
