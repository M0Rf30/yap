// SPDX-FileCopyrightText: 2021-2026 Gianluca Boiano
// SPDX-License-Identifier: GPL-3.0-only

package dnfinstall

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	pkgrpm "github.com/M0Rf30/pkgformat/rpm"
	pkgrpmdb "github.com/M0Rf30/pkgformat/rpm/rpmdb"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestHostInstalledNeverFails: whatever the host's rpm database looks like
// (SQLite, BerkeleyDB, none), the resolver gets an answer, possibly empty.
func TestHostInstalledNeverFails(t *testing.T) {
	pkgs, err := hostInstalled{}.InstalledPackages(t.Context())
	require.NoError(t, err)

	if _, statErr := os.Stat("/var/lib/rpm"); statErr != nil {
		assert.Empty(t, pkgs)
	}
}

// TestRPMQueryWithoutRPM: a failing "rpm -qa" yields no packages.
func TestRPMQueryWithoutRPM(t *testing.T) {
	t.Setenv("PATH", t.TempDir())

	assert.Empty(t, rpmQuery(t.Context()))
}

// TestRPMQueryParsesPackagesAndProvides: each package line starts a package and
// the tab-indented lines below it are its provides, versioned ones as equalities.
func TestRPMQueryParsesPackagesAndProvides(t *testing.T) {
	bin := t.TempDir()
	script := "#!/bin/sh\nprintf 'bash\\t0\\t5.2\\t1.fc40\\tx86_64\\n" +
		"\\tbash\\t5.2-1.fc40\\n\\t/bin/sh\\t\\n\\nglibc\\t2\\t2.39\\t3\\ti686\\n" +
		"\\tlibc.so.6\\t\\n\\tglibc\\t2:2.39-3\\n'\n"
	require.NoError(t, os.WriteFile(filepath.Join(bin, "rpm"), []byte(script), 0o755)) //nolint:gosec
	t.Setenv("PATH", bin)

	pkgs := rpmQuery(t.Context())
	require.Len(t, pkgs, 2)

	assert.Equal(t, "bash", pkgs[0].Name)
	assert.Equal(t, "5.2", pkgs[0].Version)
	assert.Equal(t, "1.fc40", pkgs[0].Release)
	assert.Equal(t, "x86_64", pkgs[0].Arch)
	require.Len(t, pkgs[0].Provides, 2)
	assert.Equal(t, pkgrpm.Dependency{Name: "bash", Flags: pkgrpm.SenseEqual, Version: "5.2-1.fc40"},
		pkgs[0].Provides[0])
	assert.Equal(t, pkgrpm.Dependency{Name: "/bin/sh"}, pkgs[0].Provides[1])

	assert.Equal(t, uint32(2), pkgs[1].Epoch)
	require.Len(t, pkgs[1].Provides, 2)
	assert.Equal(t,
		pkgrpm.Dependency{Name: "glibc", Flags: pkgrpm.SenseEqual, Version: "2.39-3", Epoch: 2},
		pkgs[1].Provides[1])
}

func TestNewHostClientUsesHostPaths(t *testing.T) {
	assert.NotNil(t, newHostClient())
}

// TestUpdateWithoutRepos: with no repo files Update succeeds and drops the
// cached index so the next lookup reloads it.
func TestUpdateWithoutRepos(t *testing.T) {
	if _, err := os.Stat(yumRepoDir); err == nil {
		t.Skip("host has " + yumRepoDir)
	}

	_, _, err := hostIndex(t.Context())
	require.NoError(t, err)
	require.NoError(t, Update(t.Context()))

	hostState.mu.Lock()
	defer hostState.mu.Unlock()

	assert.Nil(t, hostState.index)
}

// TestInstalledPackagesFallsBackWithoutDatabase: a host with no rpm database
// is asked through "rpm -qa".
func TestInstalledPackagesFallsBackWithoutDatabase(t *testing.T) {
	bin := t.TempDir()
	script := "#!/bin/sh\nprintf 'bash\\t0\\t5.2\\t1\\tx86_64\\n'\n"
	require.NoError(t, os.WriteFile(filepath.Join(bin, "rpm"), []byte(script), 0o755)) //nolint:gosec
	t.Setenv("PATH", bin)

	pkgs, err := hostInstalled{}.InstalledPackages(t.Context())
	require.NoError(t, err)

	// The test host may have a real rpm database; the fallback only applies
	// without one.
	if _, _, findErr := pkgrpmdb.Find("/"); errors.Is(findErr, pkgrpmdb.ErrNotFound) {
		require.Len(t, pkgs, 1)
		assert.Equal(t, "bash", pkgs[0].Name)
	}
}
