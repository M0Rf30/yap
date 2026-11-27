// SPDX-FileCopyrightText: 2021-2026 Gianluca Boiano
// SPDX-License-Identifier: GPL-3.0-only

package aptinstall_test

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/M0Rf30/pkgformat/deb/apt"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/M0Rf30/yap/v2/pkg/aptinstall"
)

// loadPlanCache builds an apt cache for the host architecture from one
// Packages index and one dpkg status file. "@ARCH@" in either is replaced
// by the host architecture.
func loadPlanCache(t *testing.T, index, status string) *apt.Cache {
	t.Helper()

	arch := apt.ArchFromGoarch(runtime.GOARCH)
	dir := t.TempDir()
	list := filepath.Join(dir, "example.org_dists_stable_main_binary-"+arch+"_Packages")
	statusFile := filepath.Join(dir, "status")

	require.NoError(t, os.WriteFile(list, []byte(strings.ReplaceAll(index, "@ARCH@", arch)), 0o600))
	require.NoError(t, os.WriteFile(statusFile,
		[]byte(strings.ReplaceAll(status, "@ARCH@", arch)), 0o600))

	cache, err := apt.Load(apt.LoadOptions{ListsDir: dir, StatusFile: statusFile, HostArch: arch})
	require.NoError(t, err)

	return cache
}

// The index only carries the security update of binutils, and the cross
// binutils pins binutils-common exactly: installing it must upgrade the
// installed binutils-common and libbinutils, as apt-get install does.
const binutilsIndex = `Package: binutils-common
Version: 2.38-4ubuntu2.14
Architecture: @ARCH@
Filename: pool/b/binutils-common_2.38-4ubuntu2.14.deb

Package: libbinutils
Version: 2.38-4ubuntu2.14
Architecture: @ARCH@
Depends: binutils-common (= 2.38-4ubuntu2.14)
Filename: pool/b/libbinutils_2.38-4ubuntu2.14.deb

Package: binutils-aarch64-linux-gnu
Version: 2.38-4ubuntu2.14
Architecture: @ARCH@
Depends: binutils-common (= 2.38-4ubuntu2.14)
Filename: pool/b/binutils-aarch64-linux-gnu_2.38-4ubuntu2.14.deb

`

const binutilsStatus = `Package: binutils-common
Status: install ok installed
Version: 2.38-4ubuntu2.12
Architecture: @ARCH@

Package: libbinutils
Status: install ok installed
Version: 2.38-4ubuntu2.12
Architecture: @ARCH@
Depends: binutils-common (= 2.38-4ubuntu2.12)

`

func TestPlanInstallUpgradesExactDependency(t *testing.T) {
	t.Parallel()

	cache := loadPlanCache(t, binutilsIndex, binutilsStatus)

	pkgs, oldVersions, err := aptinstall.PlanInstallForTesting(
		cache, []string{"binutils-aarch64-linux-gnu"})
	require.NoError(t, err)

	got := make(map[string]string, len(pkgs))
	for _, p := range pkgs {
		got[p.Name] = p.Version
	}

	assert.Equal(t, map[string]string{
		"binutils-common":            "2.38-4ubuntu2.14",
		"libbinutils":                "2.38-4ubuntu2.14",
		"binutils-aarch64-linux-gnu": "2.38-4ubuntu2.14",
	}, got)

	assert.Equal(t, map[string]string{
		"pool/b/binutils-common_2.38-4ubuntu2.14.deb": "2.38-4ubuntu2.12",
		"pool/b/libbinutils_2.38-4ubuntu2.14.deb":     "2.38-4ubuntu2.12",
	}, oldVersions)
}

// A cache with more slots than apt.DefaultPlanSteps, as Ubuntu with a
// foreign architecture has, must not use up the resolver budget.
func TestPlanInstallLargeCache(t *testing.T) {
	t.Parallel()

	var index strings.Builder

	index.WriteString(binutilsIndex)

	for i := range apt.DefaultPlanSteps + 10000 {
		fmt.Fprintf(&index, "Package: filler-%d\nVersion: 1\nArchitecture: @ARCH@\n"+
			"Filename: pool/f/filler-%d_1.deb\n\n", i, i)
	}

	cache := loadPlanCache(t, index.String(), binutilsStatus)

	pkgs, _, err := aptinstall.PlanInstallForTesting(
		cache, []string{"binutils-aarch64-linux-gnu"})
	require.NoError(t, err)
	assert.Len(t, pkgs, 3)
}

func TestPlanInstallRefusesRemoval(t *testing.T) {
	t.Parallel()

	cache := loadPlanCache(t,
		"Package: mta-a\nVersion: 1\nArchitecture: @ARCH@\nConflicts: mta-b\n"+
			"Filename: pool/m/mta-a_1.deb\n\n",
		"Package: mta-b\nStatus: install ok installed\nVersion: 1\nArchitecture: @ARCH@\n\n")

	_, _, err := aptinstall.PlanInstallForTesting(cache, []string{"mta-a"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "would remove [mta-b:")
}

func TestPlanInstallUnresolved(t *testing.T) {
	t.Parallel()

	cache := loadPlanCache(t, binutilsIndex, binutilsStatus)

	_, _, err := aptinstall.PlanInstallForTesting(cache, []string{"no-such-package"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unresolvable packages: [no-such-package]")
}
