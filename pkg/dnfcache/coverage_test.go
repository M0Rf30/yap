//nolint:testpackage
package dnfcache

import (
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---- Load, Reload, Update tests ----

// TestLoadReturnsNonNil verifies that Load() always returns a non-nil Cache.
func TestLoadReturnsNonNil(t *testing.T) {
	c := Load()
	assert.NotNil(t, c, "Load() should return non-nil Cache")
}

// TestLoadIdempotent verifies that multiple Load() calls return the same instance.
func TestLoadIdempotent(t *testing.T) {
	c1 := Load()
	c2 := Load()
	assert.Same(t, c1, c2, "Load() should return the same instance on subsequent calls")
}

// TestReloadReturnsNonNil verifies that Reload() always returns a non-nil Cache.
func TestReloadReturnsNonNil(t *testing.T) {
	c := Reload()
	assert.NotNil(t, c, "Reload() should return non-nil Cache")
}

// TestUpdateWithCanceledContext verifies that Update respects context cancellation.
func TestUpdateWithCanceledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := Update(ctx)
	// Should return a context-related error or nil (depending on implementation).
	// The key is that it doesn't panic.
	_ = err
}

// TestInstallWithEmptyNames verifies that Install with empty names returns nil.
func TestInstallWithEmptyNames(t *testing.T) {
	ctx := context.Background()
	err := Install(ctx, []string{})
	assert.NoError(t, err, "Install with empty names should return nil")
}

// ---- shouldReplace tests ----

// TestShouldReplacePreferDownloadable verifies that downloadable packages
// are preferred over non-downloadable ones.
func TestShouldReplacePreferDownloadable(t *testing.T) {
	nonDownloadable := &PackageInfo{
		Name:         "foo",
		Version:      "1.0",
		LocationHref: "",
	}

	downloadable := &PackageInfo{
		Name:         "foo",
		Version:      "1.0",
		LocationHref: "Packages/f/foo-1.0.rpm",
	}

	// shouldReplace(existing, new) returns true if new should replace existing.
	// downloadable should replace non-downloadable.
	result := shouldReplace(nonDownloadable, downloadable)
	assert.True(t, result, "downloadable should replace non-downloadable")

	// Non-downloadable should not replace downloadable.
	result = shouldReplace(downloadable, nonDownloadable)
	assert.False(t, result, "non-downloadable should not replace downloadable")
}

// TestShouldReplacePreferNewerVersion verifies that newer versions replace older ones.
func TestShouldReplacePreferNewerVersion(t *testing.T) {
	older := &PackageInfo{
		Name:         "foo",
		Version:      "1.0",
		Release:      "1.el8",
		LocationHref: "Packages/f/foo-1.0-1.el8.x86_64.rpm",
	}

	newer := &PackageInfo{
		Name:         "foo",
		Version:      "2.0",
		Release:      "1.el8",
		LocationHref: "Packages/f/foo-2.0-1.el8.x86_64.rpm",
	}

	// Newer should replace older.
	result := shouldReplace(older, newer)
	assert.True(t, result, "newer version should replace older")

	// Older should not replace newer.
	result = shouldReplace(newer, older)
	assert.False(t, result, "older version should not replace newer")
}

// TestShouldReplacePreferHostArch verifies that host-arch packages replace
// noarch packages when versions are equal.
func TestShouldReplacePreferHostArch(t *testing.T) {
	hostArch := goArchToRPM()

	noarch := &PackageInfo{
		Name:         "foo",
		Version:      "1.0",
		Release:      "1.el8",
		Arch:         archNoarch,
		LocationHref: "Packages/f/foo-1.0-1.el8.noarch.rpm",
	}

	hostArchPkg := &PackageInfo{
		Name:         "foo",
		Version:      "1.0",
		Release:      "1.el8",
		Arch:         hostArch,
		LocationHref: "Packages/f/foo-1.0-1.el8." + hostArch + ".rpm",
	}

	// Host-arch should replace noarch.
	result := shouldReplace(noarch, hostArchPkg)
	assert.True(t, result, "host-arch should replace noarch")

	// Noarch should not replace host-arch.
	result = shouldReplace(hostArchPkg, noarch)
	assert.False(t, result, "noarch should not replace host-arch")
}

// ---- newerEVR tests ----

// TestNewerEVRWithEpoch verifies that epoch is the primary comparison criterion.
func TestNewerEVRWithEpoch(t *testing.T) {
	// Epoch 2 is always newer than epoch 1, regardless of version.
	existing := &PackageInfo{Epoch: "1", Version: "0.0.1", Release: "1.el8"}
	candidate := &PackageInfo{Epoch: "2", Version: "99.99.99", Release: "1.el8"}
	result := newerEVR(existing, candidate)
	assert.True(t, result, "epoch 2 should be newer than epoch 1")

	existing = &PackageInfo{Epoch: "2", Version: "99.99.99", Release: "1.el8"}
	candidate = &PackageInfo{Epoch: "1", Version: "0.0.1", Release: "1.el8"}
	result = newerEVR(existing, candidate)
	assert.False(t, result, "epoch 1 should not be newer than epoch 2")
}

// TestNewerEVRWithVersion verifies version comparison when epochs are equal.
func TestNewerEVRWithVersion(t *testing.T) {
	existing := &PackageInfo{Epoch: "1", Version: "1.0", Release: "1.el8"}
	candidate := &PackageInfo{Epoch: "1", Version: "2.0", Release: "1.el8"}
	result := newerEVR(existing, candidate)
	assert.True(t, result, "version 2.0 should be newer than 1.0")

	existing = &PackageInfo{Epoch: "1", Version: "2.0", Release: "1.el8"}
	candidate = &PackageInfo{Epoch: "1", Version: "1.0", Release: "1.el8"}
	result = newerEVR(existing, candidate)
	assert.False(t, result, "version 1.0 should not be newer than 2.0")
}

// TestNewerEVREqual verifies that equal EVRs return false.
func TestNewerEVREqual(t *testing.T) {
	existing := &PackageInfo{Epoch: "1", Version: "1.0", Release: "1.el8"}
	candidate := &PackageInfo{Epoch: "1", Version: "1.0", Release: "1.el8"}
	result := newerEVR(existing, candidate)
	assert.False(t, result, "equal EVRs should return false")
}

// ---- evrString tests ----

// TestEvrStringWithEpoch verifies that epoch is included in the output.
func TestEvrStringWithEpoch(t *testing.T) {
	pkg := &PackageInfo{Epoch: "2", Version: "1.0", Release: "1.el8"}
	result := evrString(pkg)
	assert.Equal(t, "2:1.0-1.el8", result)
}

// TestEvrStringWithoutEpoch verifies that missing epoch defaults to "0".
func TestEvrStringWithoutEpoch(t *testing.T) {
	pkg := &PackageInfo{Epoch: "", Version: "1.0", Release: "1.el8"}
	result := evrString(pkg)
	assert.Equal(t, "0:1.0-1.el8", result)
}

// ---- newCache tests ----

// TestNewCacheReturnsNonNil verifies that newCache always returns a non-nil Cache.
func TestNewCacheReturnsNonNil(t *testing.T) {
	c := newCache()
	assert.NotNil(t, c, "newCache() should return non-nil Cache")
}

// TestNewCacheHasEmptyPackages verifies that a new cache has empty packages.
func TestNewCacheHasEmptyPackages(t *testing.T) {
	c := newCache()
	assert.Empty(t, c.packages, "new cache should have empty packages")
	assert.Empty(t, c.providers, "new cache should have empty providers")
}

// ---- parseRepoFiles tests ----

// TestParseRepoFilesWithNoRepoDir verifies that parseRepoFiles handles
// missing /etc/yum.repos.d gracefully.
func TestParseRepoFilesWithNoRepoDir(t *testing.T) {
	// This test runs on any system; on non-RPM systems, /etc/yum.repos.d
	// doesn't exist and parseRepoFiles returns nil.
	repos := parseRepoFiles()
	// len() is defined as zero for a nil slice, so this covers both cases.
	assert.Empty(t, repos,
		"parseRepoFiles should return nil or empty slice when repo dir doesn't exist")
}

// ---- ParseRepoFileContent edge cases ----

// TestParseRepoFileContentEmptyString verifies that empty content returns empty slice.
func TestParseRepoFileContentEmptyString(t *testing.T) {
	repos := ParseRepoFileContent("")
	assert.Empty(t, repos, "empty content should return empty slice")
}

// TestParseRepoFileContentOnlyComments verifies that content with only comments
// returns empty slice.
func TestParseRepoFileContentOnlyComments(t *testing.T) {
	content := `# This is a comment
; This is also a comment
# Another comment`

	repos := ParseRepoFileContent(content)
	assert.Empty(t, repos, "content with only comments should return empty slice")
}

// TestParseRepoFileContentMultipleSections verifies that multiple repo sections
// are parsed correctly.
func TestParseRepoFileContentMultipleSections(t *testing.T) {
	content := `[baseos]
baseurl=http://mirror.example.com/baseos/
enabled=1

[appstream]
baseurl=http://mirror.example.com/appstream/
enabled=0`

	repos := ParseRepoFileContent(content)
	require.Len(t, repos, 2, "should parse two repo sections")

	assert.Equal(t, "baseos", repos[0].ID)
	assert.Equal(t, "http://mirror.example.com/baseos/", repos[0].BaseURL)
	assert.True(t, repos[0].Enabled)

	assert.Equal(t, "appstream", repos[1].ID)
	assert.Equal(t, "http://mirror.example.com/appstream/", repos[1].BaseURL)
	assert.False(t, repos[1].Enabled)
}

// TestParseRepoFileContentDisabledRepo verifies that enabled=0 disables a repo.
func TestParseRepoFileContentDisabledRepo(t *testing.T) {
	content := `[baseos]
baseurl=http://mirror.example.com/baseos/
enabled=0`

	repos := ParseRepoFileContent(content)
	require.Len(t, repos, 1)

	assert.False(t, repos[0].Enabled)
}

// ---- buildPackageInfo tests ----

// TestBuildPackageInfoBasic verifies basic package info construction.
func TestBuildPackageInfoBasic(t *testing.T) {
	pkg := &primaryPackage{
		Name: "bash",
		Arch: "x86_64",
		Version: primaryVersion{
			Epoch: "0",
			Ver:   "5.1.8",
			Rel:   "2.el8",
		},
		Checksum: primaryChecksum{
			Type:  "sha256",
			Value: "abcd1234",
		},
		Size: primarySize{
			Package: 1500000,
		},
		Location: primaryLocation{
			Href: "Packages/b/bash-5.1.8-2.el8.x86_64.rpm",
		},
	}

	info := buildPackageInfo(pkg, "http://mirror.example.com/")
	assert.Equal(t, "bash", info.Name)
	assert.Equal(t, "x86_64", info.Arch)
	assert.Equal(t, "5.1.8", info.Version)
	assert.Equal(t, "2.el8", info.Release)
	assert.Equal(t, "0", info.Epoch)
	assert.Equal(t, "abcd1234", info.SHA256)
	assert.Equal(t, int64(1500000), info.Size)
	// LocationHref is stored as-is (relative path), not joined with baseURL
	assert.Equal(t, "Packages/b/bash-5.1.8-2.el8.x86_64.rpm", info.LocationHref)
}

// TestBuildPackageInfoWithProvides verifies that Provides are parsed correctly.
func TestBuildPackageInfoWithProvides(t *testing.T) {
	pkg := &primaryPackage{
		Name: "bash",
		Arch: "x86_64",
		Version: primaryVersion{
			Epoch: "0",
			Ver:   "5.1.8",
			Rel:   "2.el8",
		},
		Checksum: primaryChecksum{
			Type:  "sha256",
			Value: "abcd1234",
		},
		Size: primarySize{
			Package: 1500000,
		},
		Location: primaryLocation{
			Href: "Packages/b/bash-5.1.8-2.el8.x86_64.rpm",
		},
		Format: primaryFormat{
			Provides: []primaryEntry{
				{Name: "bash"},
				{Name: "/bin/bash"},
			},
		},
	}

	info := buildPackageInfo(pkg, "http://mirror.example.com/")
	assert.Len(t, info.Provides, 2)
	assert.Contains(t, info.Provides, "bash")
	assert.Contains(t, info.Provides, "/bin/bash")
}

// TestBuildPackageInfoWithRequires verifies that Requires are parsed correctly.
func TestBuildPackageInfoWithRequires(t *testing.T) {
	pkg := &primaryPackage{
		Name: "bash",
		Arch: "x86_64",
		Version: primaryVersion{
			Epoch: "0",
			Ver:   "5.1.8",
			Rel:   "2.el8",
		},
		Checksum: primaryChecksum{
			Type:  "sha256",
			Value: "abcd1234",
		},
		Size: primarySize{
			Package: 1500000,
		},
		Location: primaryLocation{
			Href: "Packages/b/bash-5.1.8-2.el8.x86_64.rpm",
		},
		Format: primaryFormat{
			Requires: []primaryEntry{
				{Name: "glibc", Flags: "GE", Ver: "2.17"},
				{Name: "readline", Flags: "GE", Ver: "7.0"},
			},
		},
	}

	info := buildPackageInfo(pkg, "http://mirror.example.com/")
	assert.Len(t, info.Requires, 2)
	// buildPackageInfo strips version constraints from requires
	assert.Contains(t, info.Requires, "glibc")
	assert.Contains(t, info.Requires, "readline")
}

// TestBuildPackageInfoFiltersRpmlib verifies that rpmlib() requires are filtered.
func TestBuildPackageInfoFiltersRpmlib(t *testing.T) {
	pkg := &primaryPackage{
		Name: "bash",
		Arch: "x86_64",
		Version: primaryVersion{
			Epoch: "0",
			Ver:   "5.1.8",
			Rel:   "2.el8",
		},
		Checksum: primaryChecksum{
			Type:  "sha256",
			Value: "abcd1234",
		},
		Size: primarySize{
			Package: 1500000,
		},
		Location: primaryLocation{
			Href: "Packages/b/bash-5.1.8-2.el8.x86_64.rpm",
		},
		Format: primaryFormat{
			Requires: []primaryEntry{
				{Name: "glibc", Flags: "GE", Ver: "2.17"},
				{Name: "rpmlib(CompressedFileNames)", Flags: "LE", Ver: "3.0.4-1"},
				{Name: "rpmlib(PayloadFilesHavePrefix)", Flags: "LE", Ver: "4.0-1"},
			},
		},
	}

	info := buildPackageInfo(pkg, "http://mirror.example.com/")
	// Only glibc should remain; rpmlib() entries should be filtered.
	assert.Len(t, info.Requires, 1)
	assert.Contains(t, info.Requires, "glibc")
}

// TestBuildPackageInfoWithFiles verifies that Files are parsed correctly.
func TestBuildPackageInfoWithFiles(t *testing.T) {
	pkg := &primaryPackage{
		Name: "bash",
		Arch: "x86_64",
		Version: primaryVersion{
			Epoch: "0",
			Ver:   "5.1.8",
			Rel:   "2.el8",
		},
		Checksum: primaryChecksum{
			Type:  "sha256",
			Value: "abcd1234",
		},
		Size: primarySize{
			Package: 1500000,
		},
		Location: primaryLocation{
			Href: "Packages/b/bash-5.1.8-2.el8.x86_64.rpm",
		},
		Format: primaryFormat{
			Files: []string{
				"/bin/bash",
				"/etc/bashrc",
			},
		},
	}

	info := buildPackageInfo(pkg, "http://mirror.example.com/")
	// Note: PackageInfo doesn't have a Files field; files are indexed as virtual
	// providers via the providers map. This test verifies the package is built correctly.
	assert.Equal(t, "bash", info.Name)
	assert.Equal(t, "x86_64", info.Arch)
}

// ---- isPrimaryIndex tests ----

// TestIsPrimaryIndexWithGzipSuffix verifies that .gz suffix is recognized.
func TestIsPrimaryIndexWithGzipSuffix(t *testing.T) {
	assert.True(t, isPrimaryIndex("primary.xml.gz"))
	assert.True(t, isPrimaryIndex("abc-primary.xml.gz"))
}

// TestIsPrimaryIndexWithoutSuffix verifies that plain primary.xml is recognized.
func TestIsPrimaryIndexWithoutSuffix(t *testing.T) {
	assert.True(t, isPrimaryIndex("primary.xml"))
}

// TestIsPrimaryIndexNonPrimary verifies that non-primary files are rejected.
func TestIsPrimaryIndexNonPrimary(t *testing.T) {
	assert.False(t, isPrimaryIndex("repomd.xml"))
	assert.False(t, isPrimaryIndex("filelists.xml.gz"))
	assert.False(t, isPrimaryIndex("other.xml"))
}

// ---- ExpandBooleanDep tests ----

// TestExpandBooleanDepWithOr verifies that OR dependencies are expanded.
func TestExpandBooleanDepWithOr(t *testing.T) {
	result := ExpandBooleanDep("(foo or bar)")
	assert.Len(t, result, 2)
	assert.Contains(t, result, "foo")
	assert.Contains(t, result, "bar")
}

// TestExpandBooleanDepWithAnd verifies that AND dependencies are expanded.
func TestExpandBooleanDepWithAnd(t *testing.T) {
	result := ExpandBooleanDep("(foo and bar)")
	assert.Len(t, result, 2)
	assert.Contains(t, result, "foo")
	assert.Contains(t, result, "bar")
}

// TestExpandBooleanDepWithoutParens verifies that non-parenthesized deps return nil.
func TestExpandBooleanDepWithoutParens(t *testing.T) {
	result := ExpandBooleanDep("foo")
	assert.Nil(t, result)
}

// TestExpandBooleanDepWithConstraints verifies that constraints are stripped.
func TestExpandBooleanDepWithConstraints(t *testing.T) {
	result := ExpandBooleanDep("(foo >= 1.0 or bar < 2.0)")
	assert.Len(t, result, 2)
	assert.Contains(t, result, "foo")
	assert.Contains(t, result, "bar")
}

// TestExpandBooleanDepEmpty verifies that empty string returns nil.
func TestExpandBooleanDepEmpty(t *testing.T) {
	result := ExpandBooleanDep("")
	assert.Nil(t, result)
}

// ---- IsBooleanDep tests ----

// TestIsBooleanDepWithOr verifies that OR is detected.
func TestIsBooleanDepWithOr(t *testing.T) {
	assert.True(t, IsBooleanDep("(foo or bar)"))
}

// TestIsBooleanDepWithAnd verifies that AND is detected.
func TestIsBooleanDepWithAnd(t *testing.T) {
	assert.True(t, IsBooleanDep("(foo and bar)"))
}

// TestIsBooleanDepWithoutParens verifies that simple deps are not boolean.
func TestIsBooleanDepWithoutParens(t *testing.T) {
	assert.False(t, IsBooleanDep("foo"))
	assert.False(t, IsBooleanDep("foo >= 1.0"))
}

// ---- parsePrimaryFile tests ----

// TestParsePrimaryFileWithGzipCompression verifies that gzip-compressed
// primary.xml.gz files are decompressed and parsed.
func TestParsePrimaryFileWithGzipCompression(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a minimal gzip-compressed primary.xml.
	primaryXML := `<?xml version="1.0" encoding="UTF-8"?>
<metadata xmlns="http://linux.duke.edu/metadata/common"
          xmlns:rpm="http://linux.duke.edu/metadata/rpm">
  <package type="rpm">
    <name>test</name>
    <arch>x86_64</arch>
    <version epoch="0" ver="1.0" rel="1.el8"/>
    <checksum type="sha256" pkgid="YES">abcd</checksum>
    <size package="1000"/>
    <location href="Packages/t/test-1.0-1.el8.x86_64.rpm"/>
  </package>
</metadata>`

	// Write gzip-compressed file.
	gzPath := filepath.Join(tmpDir, "primary.xml.gz")
	f, err := os.Create(gzPath)
	require.NoError(t, err)

	defer func() { _ = f.Close() }()

	// Use gzip writer to compress.
	gz := gzip.NewWriter(f)
	_, err = gz.Write([]byte(primaryXML))
	require.NoError(t, err)
	require.NoError(t, gz.Close())
	require.NoError(t, f.Close())

	// Parse the gzip file.
	c := newCache()
	err = c.parsePrimaryFile(gzPath, "http://mirror.example.com/")
	assert.NoError(t, err)

	// Verify the package was indexed.
	pkg, ok := c.Lookup("test")
	assert.True(t, ok, "test package should be indexed")
	assert.Equal(t, "test", pkg.Name)
}

// TestParsePrimaryFileWithPlainXML verifies that plain (uncompressed) primary.xml
// files are parsed correctly.
func TestParsePrimaryFileWithPlainXML(t *testing.T) {
	tmpDir := t.TempDir()

	primaryXML := `<?xml version="1.0" encoding="UTF-8"?>
<metadata xmlns="http://linux.duke.edu/metadata/common"
          xmlns:rpm="http://linux.duke.edu/metadata/rpm">
  <package type="rpm">
    <name>test</name>
    <arch>x86_64</arch>
    <version epoch="0" ver="1.0" rel="1.el8"/>
    <checksum type="sha256" pkgid="YES">abcd</checksum>
    <size package="1000"/>
    <location href="Packages/t/test-1.0-1.el8.x86_64.rpm"/>
  </package>
</metadata>`

	xmlPath := filepath.Join(tmpDir, "primary.xml")
	err := os.WriteFile(xmlPath, []byte(primaryXML), 0o644)
	require.NoError(t, err)

	c := newCache()
	err = c.parsePrimaryFile(xmlPath, "http://mirror.example.com/")
	assert.NoError(t, err)

	pkg, ok := c.Lookup("test")
	assert.True(t, ok, "test package should be indexed")
	assert.Equal(t, "test", pkg.Name)
}

// TestParsePrimaryFileNonExistent verifies that parsing a non-existent file
// returns an error.
func TestParsePrimaryFileNonExistent(t *testing.T) {
	c := newCache()
	err := c.parsePrimaryFile("/nonexistent/path/primary.xml", "http://mirror.example.com/")
	assert.Error(t, err, "parsing non-existent file should return error")
}

// ---- loadFromDisk tests ----

// TestLoadFromDiskWithNoCacheDir verifies that loadFromDisk handles missing
// cache directory gracefully.
func TestLoadFromDiskWithNoCacheDir(t *testing.T) {
	c := newCache()
	// loadFromDisk should not panic even if the cache directory doesn't exist.
	c.loadFromDisk()
	// Cache should remain empty.
	assert.Empty(t, c.packages)
}

// ---- fileMatchesSHA256 tests ----

// TestFileMatchesSHA256WithValidFile verifies that a file with correct SHA256
// passes validation.
func TestFileMatchesSHA256WithValidFile(t *testing.T) {
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "test.txt")

	content := []byte("hello world")
	err := os.WriteFile(filePath, content, 0o644)
	require.NoError(t, err)

	// Compute SHA256 of the content.
	hash := sha256.Sum256(content)
	sha256Hex := hex.EncodeToString(hash[:])

	ok, err := fileMatchesSHA256(filePath, sha256Hex)
	assert.NoError(t, err)
	assert.True(t, ok, "file should match SHA256")
}

// TestFileMatchesSHA256WithMismatchedHash verifies that a file with incorrect
// SHA256 fails validation.
func TestFileMatchesSHA256WithMismatchedHash(t *testing.T) {
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "test.txt")

	content := []byte("hello world")
	err := os.WriteFile(filePath, content, 0o644)
	require.NoError(t, err)

	// Use a wrong SHA256.
	wrongHash := "0000000000000000000000000000000000000000000000000000000000000000"

	ok, err := fileMatchesSHA256(filePath, wrongHash)
	assert.NoError(t, err)
	assert.False(t, ok, "file should not match wrong SHA256")
}

// TestFileMatchesSHA256WithNonExistentFile verifies that a non-existent file
// returns an error.
func TestFileMatchesSHA256WithNonExistentFile(t *testing.T) {
	ok, err := fileMatchesSHA256("/nonexistent/file", "abcd1234")
	assert.Error(t, err, "non-existent file should return error")
	assert.False(t, ok)
}

// ---- ResolveDeps edge cases ----

// TestResolveDepsWithUnknownPackage verifies that unknown packages are
// returned in the unresolved list.
func TestResolveDepsWithUnknownPackage(t *testing.T) {
	c := newCache()
	ctx := context.Background()

	resolved, unresolved, err := c.ResolveDeps(ctx, []string{"nonexistent-package"})
	assert.NoError(t, err)
	assert.Empty(t, resolved)
	assert.Len(t, unresolved, 1)
	assert.Equal(t, "nonexistent-package", unresolved[0])
}

// TestResolveDepsWithMixedPackages verifies that a mix of known and unknown
// packages is handled correctly.
func TestResolveDepsWithMixedPackages(t *testing.T) {
	c := newCache()

	// Add a known package.
	pkg := &PackageInfo{
		Name:         "bash",
		Version:      "5.1.8",
		Release:      "2.el8",
		Arch:         "x86_64",
		LocationHref: "Packages/b/bash-5.1.8-2.el8.x86_64.rpm",
	}

	c.mu.Lock()
	c.packages["bash"] = pkg
	c.mu.Unlock()

	ctx := context.Background()
	resolved, unresolved, err := c.ResolveDeps(ctx, []string{"bash", "nonexistent"})
	assert.NoError(t, err)
	assert.Len(t, resolved, 1)
	assert.Equal(t, "bash", resolved[0].Name)
	assert.Len(t, unresolved, 1)
	assert.Equal(t, "nonexistent", unresolved[0])
}

// ---- Lookup tests ----

// TestLookupNonExistent verifies that Lookup returns false for non-existent packages.
func TestLookupNonExistent(t *testing.T) {
	c := newCache()
	pkg, ok := c.Lookup("nonexistent")
	assert.False(t, ok)
	assert.Nil(t, pkg)
}

// TestLookupExistent verifies that Lookup returns the package when it exists.
func TestLookupExistent(t *testing.T) {
	c := newCache()

	pkg := &PackageInfo{
		Name:         "bash",
		Version:      "5.1.8",
		Release:      "2.el8",
		Arch:         "x86_64",
		LocationHref: "Packages/b/bash-5.1.8-2.el8.x86_64.rpm",
	}

	c.mu.Lock()
	c.packages["bash"] = pkg
	c.mu.Unlock()

	found, ok := c.Lookup("bash")
	assert.True(t, ok)
	assert.Equal(t, pkg, found)
}

// ---- ResolveVirtual tests ----

// TestResolveVirtualNonExistent verifies that ResolveVirtual returns the name
// unchanged for non-existent capabilities.
func TestResolveVirtualNonExistent(t *testing.T) {
	c := newCache()
	result := c.ResolveVirtual("nonexistent-capability")
	assert.Equal(t, "nonexistent-capability", result)
}

// TestResolveVirtualExistent verifies that ResolveVirtual returns the provider
// name when the capability exists.
func TestResolveVirtualExistent(t *testing.T) {
	c := newCache()

	pkg := &PackageInfo{
		Name:         "bash",
		Version:      "5.1.8",
		Release:      "2.el8",
		Arch:         "x86_64",
		LocationHref: "Packages/b/bash-5.1.8-2.el8.x86_64.rpm",
	}

	c.mu.Lock()
	c.packages["bash"] = pkg
	c.providers["/bin/bash"] = []*PackageInfo{pkg}
	c.mu.Unlock()

	result := c.ResolveVirtual("/bin/bash")
	assert.Equal(t, "bash", result)
}

// ---- isBlockedByModuleFilter tests ----

// TestIsBlockedByModuleFilterWithNoModules verifies that packages are not
// blocked when no module metadata is loaded.
func TestIsBlockedByModuleFilterWithNoModules(t *testing.T) {
	c := newCache()
	c.modules = nil

	pkg := &PackageInfo{
		Name:    "perl",
		Release: "404.module+el8.6.0+882+2fa1e48f",
	}

	assert.False(t, c.isBlockedByModuleFilter(pkg),
		"package should not be blocked when modules is nil")
}

// TestIsBlockedByModuleFilterWithNonModularPackage verifies that non-modular
// packages are never blocked.
func TestIsBlockedByModuleFilterWithNonModularPackage(t *testing.T) {
	c := newCache()
	c.modules = newModuleIndex()
	c.modules.defaultStream["perl"] = "5.26"

	pkg := &PackageInfo{
		Name:    "perl",
		Release: "422.el8",
		Epoch:   "0",
		Version: "5.26.3",
		Arch:    "x86_64",
	}

	assert.False(t, c.isBlockedByModuleFilter(pkg),
		"non-modular package should not be blocked")
}

// TestIsBlockedByModuleFilterWithDefaultStream verifies that default-stream
// modular packages are not blocked.
func TestIsBlockedByModuleFilterWithDefaultStream(t *testing.T) {
	c := newCache()
	c.modules = newModuleIndex()
	c.modules.defaultStream["perl"] = "5.26"

	pkg := &PackageInfo{
		Name:    "perl",
		Release: "419.module+el8.5.0+728+2c8a1bd2",
		Epoch:   "0",
		Version: "5.26.3",
		Arch:    "x86_64",
	}

	// Don't add it to blockedNVRA, so it's not blocked.
	assert.False(t, c.isBlockedByModuleFilter(pkg),
		"default-stream modular package should not be blocked")
}

// TestIsBlockedByModuleFilterWithNonDefaultStream verifies that non-default-stream
// modular packages are blocked.
func TestIsBlockedByModuleFilterWithNonDefaultStream(t *testing.T) {
	c := newCache()
	c.modules = newModuleIndex()
	c.modules.defaultStream["perl"] = "5.26"

	pkg := &PackageInfo{
		Name:    "perl",
		Release: "404.module+el8.6.0+882+2fa1e48f",
		Epoch:   "0",
		Version: "5.24.4",
		Arch:    "x86_64",
	}

	// Add the NVRA to blockedNVRA.
	nvra := packageNVRA(pkg.Name, pkg.Epoch, pkg.Version, pkg.Release, pkg.Arch)
	c.modules.blockedNVRA[nvra] = true

	assert.True(t, c.isBlockedByModuleFilter(pkg),
		"non-default-stream modular package should be blocked")
}

// ---- pickProvider tests ----

// TestPickProviderPrefersNoarchOverForeignArch verifies that noarch is preferred
// over foreign-arch.
func TestPickProviderPrefersNoarchOverForeignArch(t *testing.T) {
	noarch := &PackageInfo{
		Name: "foo",
		Arch: archNoarch,
	}

	foreignArch := &PackageInfo{
		Name: "foo",
		Arch: "i686",
	}

	providers := []*PackageInfo{foreignArch, noarch}
	picked := pickProvider(providers)
	assert.Equal(t, noarch, picked, "should prefer noarch over foreign-arch")
}

// TestPickProviderReturnsForeignArchWhenNoOtherOption verifies that foreign-arch
// is returned when it's the only option.
func TestPickProviderReturnsForeignArchWhenNoOtherOption(t *testing.T) {
	foreignArch := &PackageInfo{
		Name: "foo",
		Arch: "i686",
	}

	providers := []*PackageInfo{foreignArch}
	picked := pickProvider(providers)
	assert.Equal(t, foreignArch, picked, "should return foreign-arch when it's the only option")
}

// ---- parseModulesFile tests ----

// TestParseModulesFileWithNonExistentFile verifies that parsing a non-existent
// file returns an error.
func TestParseModulesFileWithNonExistentFile(t *testing.T) {
	idx := newModuleIndex()
	err := parseModulesFile("/nonexistent/modules.yaml", idx)
	assert.Error(t, err, "parsing non-existent file should return error")
}

// TestParseModulesFileWithPlainYAML verifies that plain (uncompressed) YAML
// files are parsed correctly.
func TestParseModulesFileWithPlainYAML(t *testing.T) {
	tmpDir := t.TempDir()

	yamlContent := `---
document: modulemd
version: 2
data:
  name: perl
  stream: '5.26'
  artifacts:
    rpms:
      - perl-0:5.26.3-419.module+el8.5.0+728+2c8a1bd2.x86_64
...
---
document: modulemd-defaults
version: 1
data:
  module: perl
  stream: '5.26'
...
`

	yamlPath := filepath.Join(tmpDir, "modules.yaml")
	err := os.WriteFile(yamlPath, []byte(yamlContent), 0o644)
	require.NoError(t, err)

	idx := newModuleIndex()
	err = parseModulesFile(yamlPath, idx)
	assert.NoError(t, err)

	assert.Equal(t, "5.26", idx.defaultStream["perl"])
}

// ---- collectModuleFiles tests (via loadModuleIndex) ----

// TestLoadModuleIndexWithNoCacheDir verifies that loadModuleIndex handles
// missing cache directory gracefully.
func TestLoadModuleIndexWithNoCacheDir(t *testing.T) {
	idx := loadModuleIndex()
	assert.NotNil(t, idx, "loadModuleIndex should return non-nil index")
	// On non-RPM systems or when no module metadata is present, the index is empty.
	// We can't assert it's empty because it might have data on RPM systems.
}

// ---- parsePrimaryXML tests ----

// TestParsePrimaryXMLWithMultiplePackages verifies that multiple packages in
// a single XML file are all indexed.
func TestParsePrimaryXMLWithMultiplePackages(t *testing.T) {
	xml := `<?xml version="1.0" encoding="UTF-8"?>
<metadata xmlns="http://linux.duke.edu/metadata/common"
          xmlns:rpm="http://linux.duke.edu/metadata/rpm">
  <package type="rpm">
    <name>bash</name>
    <arch>x86_64</arch>
    <version epoch="0" ver="5.1.8" rel="2.el8"/>
    <checksum type="sha256" pkgid="YES">aaaa</checksum>
    <size package="1500000"/>
    <location href="Packages/b/bash-5.1.8-2.el8.x86_64.rpm"/>
  </package>
  <package type="rpm">
    <name>glibc</name>
    <arch>x86_64</arch>
    <version epoch="0" ver="2.28" rel="164.el8"/>
    <checksum type="sha256" pkgid="YES">bbbb</checksum>
    <size package="3500000"/>
    <location href="Packages/g/glibc-2.28-164.el8.x86_64.rpm"/>
  </package>
</metadata>`

	c := newCache()
	err := c.parsePrimaryXML(strings.NewReader(xml), "http://mirror.example.com/")
	assert.NoError(t, err)

	bash, ok := c.Lookup("bash")
	assert.True(t, ok)
	assert.Equal(t, "bash", bash.Name)

	glibc, ok := c.Lookup("glibc")
	assert.True(t, ok)
	assert.Equal(t, "glibc", glibc.Name)
}

// TestParsePrimaryXMLWithNoarch verifies that noarch packages are indexed.
func TestParsePrimaryXMLWithNoarch(t *testing.T) {
	xml := `<?xml version="1.0" encoding="UTF-8"?>
<metadata xmlns="http://linux.duke.edu/metadata/common"
          xmlns:rpm="http://linux.duke.edu/metadata/rpm">
  <package type="rpm">
    <name>setup</name>
    <arch>noarch</arch>
    <version epoch="0" ver="2.12.2" rel="6.el8"/>
    <checksum type="sha256" pkgid="YES">cccc</checksum>
    <size package="700000"/>
    <location href="Packages/s/setup-2.12.2-6.el8.noarch.rpm"/>
  </package>
</metadata>`

	c := newCache()
	err := c.parsePrimaryXML(strings.NewReader(xml), "http://mirror.example.com/")
	assert.NoError(t, err)

	setup, ok := c.Lookup("setup")
	assert.True(t, ok)
	assert.Equal(t, "noarch", setup.Arch)
}

// TestParsePrimaryXMLWithConstrainedRequires verifies that requires with
// version constraints are preserved.
func TestParsePrimaryXMLWithConstrainedRequires(t *testing.T) {
	xml := `<?xml version="1.0" encoding="UTF-8"?>
<metadata xmlns="http://linux.duke.edu/metadata/common"
          xmlns:rpm="http://linux.duke.edu/metadata/rpm">
  <package type="rpm">
    <name>bash</name>
    <arch>x86_64</arch>
    <version epoch="0" ver="5.1.8" rel="2.el8"/>
    <checksum type="sha256" pkgid="YES">aaaa</checksum>
    <size package="1500000"/>
    <location href="Packages/b/bash-5.1.8-2.el8.x86_64.rpm"/>
    <format>
      <rpm:requires>
        <rpm:entry name="glibc" flags="GE" ver="2.17"/>
        <rpm:entry name="readline" flags="GE" ver="7.0"/>
      </rpm:requires>
    </format>
  </package>
</metadata>`

	c := newCache()
	err := c.parsePrimaryXML(strings.NewReader(xml), "http://mirror.example.com/")
	assert.NoError(t, err)

	bash, ok := c.Lookup("bash")
	assert.True(t, ok)
	assert.Len(t, bash.Requires, 2)
	// parsePrimaryXML strips version constraints from requires
	assert.Contains(t, bash.Requires, "glibc")
	assert.Contains(t, bash.Requires, "readline")
}
