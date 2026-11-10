// SPDX-FileCopyrightText: 2021-2026 Gianluca Boiano
// SPDX-License-Identifier: GPL-3.0-only

package dnfinstall

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/M0Rf30/yap/v2/pkg/rpmdb"
)

// TestHostInstalledNeverFails: whatever the host's rpm database looks like
// (SQLite, BerkeleyDB, none), the resolver gets an answer, possibly empty.
func TestHostInstalledNeverFails(t *testing.T) {
	names, err := hostInstalled{}.InstalledNames(t.Context())
	require.NoError(t, err)

	provides, err := hostInstalled{}.InstalledProvides(t.Context())
	require.NoError(t, err)

	if _, statErr := os.Stat("/var/lib/rpm"); statErr != nil {
		assert.Empty(t, names)
		assert.Empty(t, provides)
	}
}

// TestRPMQueryWithoutRPM: a failing "rpm -qa" yields no lines.
func TestRPMQueryWithoutRPM(t *testing.T) {
	t.Setenv("PATH", t.TempDir())

	assert.Empty(t, rpmQuery(t.Context(), "%{NAME}\n"))
}

// TestRPMQueryTrimsLines: output lines are trimmed and blanks dropped.
func TestRPMQueryTrimsLines(t *testing.T) {
	bin := t.TempDir()
	script := "#!/bin/sh\nprintf 'bash\\n\\n  glibc  \\n'\n"
	require.NoError(t, os.WriteFile(filepath.Join(bin, "rpm"), []byte(script), 0o755)) //nolint:gosec
	t.Setenv("PATH", bin)

	assert.Equal(t, []string{"bash", "glibc"}, rpmQuery(t.Context(), "%{NAME}\n"))
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

// TestQueryInstalledFallsBackWithoutDatabase: a host with no rpm database is
// asked through "rpm -qa"; any other failure reads as nothing installed.
func TestQueryInstalledFallsBackWithoutDatabase(t *testing.T) {
	bin := t.TempDir()
	script := "#!/bin/sh\nprintf 'bash\\n'\n"
	require.NoError(t, os.WriteFile(filepath.Join(bin, "rpm"), []byte(script), 0o755)) //nolint:gosec
	t.Setenv("PATH", bin)

	got := queryInstalled(t.Context(), func(context.Context) ([]string, error) {
		return nil, rpmdb.ErrNoDatabase
	}, "%{NAME}\n")
	assert.Equal(t, []string{"bash"}, got)

	got = queryInstalled(t.Context(), func(context.Context) ([]string, error) {
		return nil, errors.New("corrupt")
	}, "%{NAME}\n")
	assert.Empty(t, got)

	got = queryInstalled(t.Context(), func(context.Context) ([]string, error) {
		return []string{"glibc"}, nil
	}, "%{NAME}\n")
	assert.Equal(t, []string{"glibc"}, got)
}
