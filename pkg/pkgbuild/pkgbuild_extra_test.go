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

func TestApkInstalledSet_NonExistentPath(t *testing.T) {
	result := pkgbuild.ApkInstalledSetForTesting("/nonexistent/path/that/does/not/exist")
	assert.Nil(t, result, "non-existent path should return nil")
}

func TestApkInstalledSet_EmptyFile(t *testing.T) {
	f, err := os.CreateTemp(t.TempDir(), "apk-installed-*")
	require.NoError(t, err)
	require.NoError(t, f.Close())

	result := pkgbuild.ApkInstalledSetForTesting(f.Name())
	assert.NotNil(t, result, "empty file should return empty map, not nil")
	assert.Empty(t, result, "empty file should produce empty map")
}

func TestApkInstalledSet_SinglePackage(t *testing.T) {
	content := "P:musl\nV:1.2.3-r0\nA:x86_64\n\n"
	path := writeTemp(t, content)

	result := pkgbuild.ApkInstalledSetForTesting(path)
	require.NotNil(t, result)
	assert.True(t, result["musl"], "musl should be in the installed set")
	assert.Len(t, result, 1)
}

func TestApkInstalledSet_MultiplePackages(t *testing.T) {
	content := "P:musl\nV:1.2.3-r0\n\nP:busybox\nV:1.35.0-r0\n\nP:alpine-baselayout\nV:3.4.0-r0\n\n"
	path := writeTemp(t, content)

	result := pkgbuild.ApkInstalledSetForTesting(path)
	require.NotNil(t, result)
	assert.True(t, result["musl"])
	assert.True(t, result["busybox"])
	assert.True(t, result["alpine-baselayout"])
	assert.Len(t, result, 3)
}

func TestApkInstalledSet_NoTrailingBlankLine(t *testing.T) {
	// Last package entry has no trailing blank line — should still be included.
	content := "P:musl\nV:1.2.3-r0\nA:x86_64"
	path := writeTemp(t, content)

	result := pkgbuild.ApkInstalledSetForTesting(path)
	require.NotNil(t, result)
	assert.True(t, result["musl"], "package without trailing blank line should still be included")
}

func TestApkInstalledSet_OnlyBlankLines(t *testing.T) {
	content := "\n\n\n"
	path := writeTemp(t, content)

	result := pkgbuild.ApkInstalledSetForTesting(path)
	require.NotNil(t, result)
	assert.Empty(t, result)
}

func TestApkInstalledSet_PackageWithHyphenatedName(t *testing.T) {
	content := "P:alpine-baselayout-data\nV:3.4.0-r0\n\n"
	path := writeTemp(t, content)

	result := pkgbuild.ApkInstalledSetForTesting(path)
	require.NotNil(t, result)
	assert.True(t, result["alpine-baselayout-data"])
}

// ---------------------------------------------------------------------------
// pacmanDirToName
// ---------------------------------------------------------------------------

func TestPacmanDirToName_Standard(t *testing.T) {
	assert.Equal(t, "gcc", pkgbuild.PacmanDirToNameForTesting("gcc-12.2.0-1"))
}

func TestPacmanDirToName_HyphenatedName(t *testing.T) {
	assert.Equal(t, "linux-headers", pkgbuild.PacmanDirToNameForTesting("linux-headers-6.1.1-1"))
}

func TestPacmanDirToName_OnlyOneSegment(t *testing.T) {
	// "no-hyphens" has only one hyphen — stripping last two segments leaves nothing.
	assert.Equal(t, "", pkgbuild.PacmanDirToNameForTesting("no-hyphens"))
}

func TestPacmanDirToName_Empty(t *testing.T) {
	assert.Equal(t, "", pkgbuild.PacmanDirToNameForTesting(""))
}

func TestPacmanDirToName_ThreeSegments(t *testing.T) {
	// "a-b-c": strip last → "a-b", strip last → "a"
	assert.Equal(t, "a", pkgbuild.PacmanDirToNameForTesting("a-b-c"))
}

func TestPacmanDirToName_MultiHyphenName(t *testing.T) {
	// "lib32-glibc-2.37-1": strip last → "lib32-glibc-2.37", strip last → "lib32-glibc"
	assert.Equal(t, "lib32-glibc", pkgbuild.PacmanDirToNameForTesting("lib32-glibc-2.37-1"))
}

func TestPacmanDirToName_NoHyphen(t *testing.T) {
	assert.Equal(t, "", pkgbuild.PacmanDirToNameForTesting("nohyphen"))
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

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

// writeTemp writes content to a temp file and returns its path.
func writeTemp(t *testing.T, content string) string {
	t.Helper()

	dir := t.TempDir()
	path := filepath.Join(dir, "db")

	require.NoError(t, os.WriteFile(path, []byte(content), 0o600))

	return path
}
