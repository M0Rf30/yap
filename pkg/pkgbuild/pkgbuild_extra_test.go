// SPDX-FileCopyrightText: 2021-2026 Gianluca Boiano
// SPDX-License-Identifier: GPL-3.0-only

package pkgbuild_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/M0Rf30/yap/v2/pkg/pkgbuild"
)

// ---------------------------------------------------------------------------
// apkInstalledSet
// ---------------------------------------------------------------------------

// writeAPKDB writes content as the apk installed database of a scratch root.
func writeAPKDB(t *testing.T, content string) string {
	t.Helper()

	root := t.TempDir()
	dir := filepath.Join(root, "lib", "apk", "db")

	require.NoError(t, os.MkdirAll(dir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "installed"), []byte(content), 0o600))

	return root
}

func TestApkInstalledSet_NoDatabase(t *testing.T) {
	result, err := pkgbuild.ApkInstalledSetForTesting(t.TempDir())
	require.Error(t, err, "a root without a database is an error, so callers fall back")
	assert.Nil(t, result)
}

func TestApkInstalledSet_EmptyFile(t *testing.T) {
	result, err := pkgbuild.ApkInstalledSetForTesting(writeAPKDB(t, ""))
	require.NoError(t, err)
	assert.NotNil(t, result, "empty file should return empty map, not nil")
	assert.Empty(t, result, "empty file should produce empty map")
}

func TestApkInstalledSet_SinglePackage(t *testing.T) {
	result, err := pkgbuild.ApkInstalledSetForTesting(writeAPKDB(t, "P:musl\nV:1.2.3-r0\nA:x86_64\n\n"))
	require.NoError(t, err)
	assert.True(t, result["musl"], "musl should be in the installed set")
	assert.Len(t, result, 1)
}

func TestApkInstalledSet_MultiplePackages(t *testing.T) {
	content := "P:musl\nV:1.2.3-r0\nA:x86_64\n\nP:busybox\nV:1.35.0-r0\nA:x86_64\n\n" +
		"P:alpine-baselayout\nV:3.4.0-r0\nA:x86_64\n\n"

	result, err := pkgbuild.ApkInstalledSetForTesting(writeAPKDB(t, content))
	require.NoError(t, err)
	assert.True(t, result["musl"])
	assert.True(t, result["busybox"])
	assert.True(t, result["alpine-baselayout"])
	assert.Len(t, result, 3)
}

func TestApkInstalledSet_NoTrailingBlankLine(t *testing.T) {
	// Last package entry has no trailing blank line — should still be included.
	result, err := pkgbuild.ApkInstalledSetForTesting(writeAPKDB(t, "P:musl\nV:1.2.3-r0\nA:x86_64"))
	require.NoError(t, err)
	assert.True(t, result["musl"], "package without trailing blank line should still be included")
}

func TestApkInstalledSet_OnlyBlankLines(t *testing.T) {
	result, err := pkgbuild.ApkInstalledSetForTesting(writeAPKDB(t, "\n\n\n"))
	require.NoError(t, err)
	assert.Empty(t, result)
}

func TestApkInstalledSet_PackageWithHyphenatedName(t *testing.T) {
	result, err := pkgbuild.ApkInstalledSetForTesting(writeAPKDB(t, "P:alpine-baselayout-data\nV:3.4.0-r0\nA:noarch\n\n"))
	require.NoError(t, err)
	assert.True(t, result["alpine-baselayout-data"])
}

func TestApkInstalledSet_VirtualPackageAndFileRecords(t *testing.T) {
	content := "P:.build-deps\nV:20260101.000000\nA:noarch\n\n" +
		"P:busybox\nV:1.37.0-r31\nA:x86_64\nF:bin\nR:busybox\na:0:0:755\nZ:Q1y5VcgN7Y1x2d9E1Gp7BkG5mZYGM=\n\n"

	result, err := pkgbuild.ApkInstalledSetForTesting(writeAPKDB(t, content))
	require.NoError(t, err)
	assert.True(t, result[".build-deps"])
	assert.True(t, result["busybox"])
	assert.Len(t, result, 2)
}

// ---------------------------------------------------------------------------
// pacmanInstalledSet
// ---------------------------------------------------------------------------

// writePacmanDB writes package directories (name of directory → desc) as the
// local database of a scratch root.
func writePacmanDB(t *testing.T, pkgs map[string]string) string {
	t.Helper()

	root := t.TempDir()
	local := filepath.Join(root, "var", "lib", "pacman", "local")

	require.NoError(t, os.MkdirAll(local, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(local, "ALPM_DB_VERSION"), []byte("9\n"), 0o600))

	for dir, desc := range pkgs {
		require.NoError(t, os.MkdirAll(filepath.Join(local, dir), 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(local, dir, "desc"), []byte(desc), 0o600))
	}

	return root
}

func pacmanDesc(name, version string) string {
	return "%NAME%\n" + name + "\n\n%VERSION%\n" + version + "\n\n"
}

func TestPacmanInstalledSet_NoDatabase(t *testing.T) {
	result, err := pkgbuild.PacmanInstalledSetForTesting(t.TempDir())
	require.Error(t, err, "a root without a database is an error, so callers fall back")
	assert.Nil(t, result)
}

func TestPacmanInstalledSet_Empty(t *testing.T) {
	result, err := pkgbuild.PacmanInstalledSetForTesting(writePacmanDB(t, nil))
	require.NoError(t, err)
	assert.NotNil(t, result)
	assert.Empty(t, result)
}

func TestPacmanInstalledSet_Names(t *testing.T) {
	root := writePacmanDB(t, map[string]string{
		"gcc-12.2.0-1":           pacmanDesc("gcc", "12.2.0-1"),
		"linux-headers-6.1.1-1":  pacmanDesc("linux-headers", "6.1.1-1"),
		"lib32-glibc-2.37-1":     pacmanDesc("lib32-glibc", "2.37-1"),
		"python-3.11.3-1":        pacmanDesc("python", "3.11.3-1"),
		"ca-certificates-2024-1": pacmanDesc("ca-certificates", "2024-1"),
	})

	result, err := pkgbuild.PacmanInstalledSetForTesting(root)
	require.NoError(t, err)
	assert.Len(t, result, 5)

	for _, name := range []string{"gcc", "linux-headers", "lib32-glibc", "python", "ca-certificates"} {
		assert.True(t, result[name], name)
	}
}

// The name of a package is the %NAME% of its desc, not a guess from the
// directory name; a directory without a valid desc is not a package.
func TestPacmanInstalledSet_NameFromDesc(t *testing.T) {
	root := writePacmanDB(t, map[string]string{
		"odd-dir-name":   pacmanDesc("real", "1.0-1"),
		"broken-1.0-1":   "%VERSION%\n1.0-1\n\n",
		"empty-name-1-1": "",
	})

	result, err := pkgbuild.PacmanInstalledSetForTesting(root)
	require.NoError(t, err)
	assert.Equal(t, map[string]bool{"real": true}, result)
}

// ---------------------------------------------------------------------------
// stripVersionConstraint
// ---------------------------------------------------------------------------

func TestStripVersionConstraint_GreaterEqual(t *testing.T) {
	assert.Equal(t, "musl-dev", pkgbuild.StripVersionConstraintForTesting("musl-dev>=1.2"))
}

func TestStripVersionConstraint_NotEqual(t *testing.T) {
	assert.Equal(t, "foo", pkgbuild.StripVersionConstraintForTesting("foo!=1.0"))
}

func TestStripVersionConstraint_LessEqual(t *testing.T) {
	assert.Equal(t, "bar", pkgbuild.StripVersionConstraintForTesting("bar<=2.0"))
}

func TestStripVersionConstraint_Equal(t *testing.T) {
	assert.Equal(t, "baz", pkgbuild.StripVersionConstraintForTesting("baz=1.0"))
}

func TestStripVersionConstraint_Greater(t *testing.T) {
	assert.Equal(t, "qux", pkgbuild.StripVersionConstraintForTesting("qux>1.0"))
}

func TestStripVersionConstraint_Less(t *testing.T) {
	assert.Equal(t, "quux", pkgbuild.StripVersionConstraintForTesting("quux<1.0"))
}

func TestStripVersionConstraint_Plain(t *testing.T) {
	assert.Equal(t, "plain", pkgbuild.StripVersionConstraintForTesting("plain"))
}

func TestStripVersionConstraint_Spaces(t *testing.T) {
	assert.Equal(t, "spaced", pkgbuild.StripVersionConstraintForTesting("  spaced  "))
}

func TestStripVersionConstraint_Tilde(t *testing.T) {
	assert.Equal(t, "pkg", pkgbuild.StripVersionConstraintForTesting("pkg~1.0"))
}

func TestStripVersionConstraint_SpacesAroundOperator(t *testing.T) {
	// Spaces before the operator — TrimSpace on the result should still work.
	assert.Equal(t, "foo", pkgbuild.StripVersionConstraintForTesting("foo>=2.0"))
}

// ---------------------------------------------------------------------------
// copySplitOverrideFields / SnapshotSplitOverrides / RestoreSplitOverrides /
// RestoreTopLevelOverrides
// ---------------------------------------------------------------------------

func newTestPKGBUILD() *pkgbuild.PKGBUILD {
	p := &pkgbuild.PKGBUILD{
		PkgDesc:    "A test package",
		URL:        "https://example.com",
		License:    []string{"MIT"},
		Depends:    []string{"glibc", "zlib"},
		OptDepends: []string{"curl: for downloads"},
		Provides:   []string{"virtual-pkg"},
		Conflicts:  []string{"old-pkg"},
		Replaces:   []string{"legacy-pkg"},
		Backup:     []string{"etc/foo.conf"},
		Options:    []string{"!strip"},
		PkgName:    "testpkg",
		PkgVer:     "1.0.0",
		PkgRel:     "1",
		PkgNames:   []string{"testpkg", "testpkg-libs"},
	}
	p.Init()

	return p
}

func TestSnapshotSplitOverrides_CopiesValues(t *testing.T) {
	p := newTestPKGBUILD()

	snap := p.SnapshotSplitOverrides()

	assert.Equal(t, p.PkgDesc, snap.PkgDesc)
	assert.Equal(t, p.URL, snap.URL)
	assert.Equal(t, p.License, snap.License)
	assert.Equal(t, p.Depends, snap.Depends)
	assert.Equal(t, p.OptDepends, snap.OptDepends)
	assert.Equal(t, p.Provides, snap.Provides)
	assert.Equal(t, p.Conflicts, snap.Conflicts)
	assert.Equal(t, p.Replaces, snap.Replaces)
	assert.Equal(t, p.Backup, snap.Backup)
	assert.Equal(t, p.Options, snap.Options)
}

func TestSnapshotSplitOverrides_DeepCopiesSlices(t *testing.T) {
	p := newTestPKGBUILD()

	snap := p.SnapshotSplitOverrides()

	// Mutating the snapshot's slice must not affect the original.
	snap.Depends = append(snap.Depends, "extra-dep")
	assert.NotContains(t, p.Depends, "extra-dep", "snapshot Depends should be independent of original")

	snap.License[0] = "GPL-2.0-only"
	assert.Equal(t, "MIT", p.License[0], "snapshot License should be independent of original")
}

func TestRestoreSplitOverrides_RestoresValues(t *testing.T) {
	p := newTestPKGBUILD()

	snap := p.SnapshotSplitOverrides()

	// Mutate the PKGBUILD to simulate a sub-package override.
	p.PkgDesc = "Overridden description"
	p.URL = "https://override.example.com"
	p.License = []string{"GPL-2.0-only"}
	p.Depends = []string{"override-dep"}

	// Restore from snapshot.
	p.RestoreSplitOverrides(&snap)

	assert.Equal(t, "A test package", p.PkgDesc)
	assert.Equal(t, "https://example.com", p.URL)
	assert.Equal(t, []string{"MIT"}, p.License)
	assert.Equal(t, []string{"glibc", "zlib"}, p.Depends)
}

func TestRestoreSplitOverrides_DeepCopiesSlices(t *testing.T) {
	p := newTestPKGBUILD()

	snap := p.SnapshotSplitOverrides()

	// Override and restore.
	p.Depends = []string{"new-dep"}
	p.RestoreSplitOverrides(&snap)

	// Mutating the restored slice must not affect the snapshot.
	p.Depends = append(p.Depends, "extra")
	assert.NotContains(t, snap.Depends, "extra", "restored slice should be independent of snapshot")
}

func TestRestoreTopLevelOverrides_NoOpWhenSnapNil(t *testing.T) {
	// A non-split package: topLevelSnap is nil after Init/Finalize.
	p := &pkgbuild.PKGBUILD{
		PkgDesc: "single package",
		PkgName: "single",
		PkgVer:  "1.0",
		PkgRel:  "1",
		License: []string{"MIT"},
		Package: "package() { install -Dm755 bin/foo usr/bin/foo; }",
	}
	p.Init()
	p.Finalize() // topLevelSnap stays nil for non-split packages

	// Should not panic and should not change PkgDesc.
	p.PkgDesc = "mutated"
	p.RestoreTopLevelOverrides()
	assert.Equal(t, "mutated", p.PkgDesc, "RestoreTopLevelOverrides should be no-op for non-split packages")
}

func TestRestoreTopLevelOverrides_RestoresAfterFinalize(t *testing.T) {
	p := newTestPKGBUILD()

	// Finalize captures topLevelSnap for split packages.
	p.Finalize()

	originalDesc := p.PkgDesc
	originalDepends := append([]string(nil), p.Depends...)

	// Simulate sub-package override.
	p.PkgDesc = "sub-package description"
	p.Depends = []string{"sub-dep"}

	// Restore via top-level snap.
	p.RestoreTopLevelOverrides()

	assert.Equal(t, originalDesc, p.PkgDesc)
	assert.Equal(t, originalDepends, p.Depends)
}

func TestRestoreTopLevelOverrides_MultipleRestores(t *testing.T) {
	p := newTestPKGBUILD()
	p.Finalize()

	originalDesc := p.PkgDesc

	// First sub-package override + restore.
	p.PkgDesc = "sub1"
	p.RestoreTopLevelOverrides()
	assert.Equal(t, originalDesc, p.PkgDesc)

	// Second sub-package override + restore.
	p.PkgDesc = "sub2"
	p.RestoreTopLevelOverrides()
	assert.Equal(t, originalDesc, p.PkgDesc)
}
