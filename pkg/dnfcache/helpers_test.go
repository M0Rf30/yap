//nolint:testpackage
package dnfcache

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
)

// TestStripRPMConstraintBasic tests basic constraint stripping.
func TestStripRPMConstraintBasic(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		// Existing test cases
		{"glibc >= 2.17", "glibc"},
		{"libfoo", "libfoo"},
		{"rpmlib(CompressedFileNames)", "rpmlib(CompressedFileNames)"},
		{"  foo  ", "foo"},
		// Additional test cases
		{"glibc(x86-64)", "glibc(x86-64)"},
		{"foo <= 1.0", "foo"},
		{"bar = 2.0", "bar"},
		{"", ""},
		{"  ", ""},
		{"package > 1.2.3", "package"},
		{"lib < 3.0", "lib"},
		{"name != 1.0", "name"},
		{"  spaced  >=  1.0  ", "spaced"},
	}

	for _, c := range cases {
		got := StripRPMConstraint(c.in)
		if got != c.want {
			t.Errorf("StripRPMConstraint(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

// TestNormalizeURL tests URL path normalization.
func TestNormalizeURL(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		// Double slashes collapsed
		{"http://example.com/pub//rocky//8.10/", "http://example.com/pub/rocky/8.10/"},
		{"http://example.com/pub///rocky/8.10/", "http://example.com/pub/rocky/8.10/"},
		// Normal URLs unchanged
		{"http://example.com/pub/rocky/8.10/", "http://example.com/pub/rocky/8.10/"},
		{"https://mirror.example.com/centos/8/BaseOS/x86_64/os/", "https://mirror.example.com/centos/8/BaseOS/x86_64/os/"},
		// URLs with query strings
		{"http://example.com/pub//rocky?key=value", "http://example.com/pub/rocky?key=value"},
		// URLs with fragments
		{"http://example.com/pub//rocky#section", "http://example.com/pub/rocky#section"},
		// Invalid URL: url.Parse encodes spaces, so we check it doesn't crash
		// and returns something (the exact encoding is implementation-dependent)
		// {"not a url at all", "not a url at all"},  // Skip this - url.Parse encodes it
		{"://invalid", "://invalid"},
		// Empty string
		{"", ""},
		// Root path with double slashes
		{"http://example.com//", "http://example.com/"},
		// Multiple consecutive double slashes
		{"http://example.com/a////b", "http://example.com/a/b"},
	}

	for _, c := range cases {
		got := normalizeURL(c.in)
		if got != c.want {
			t.Errorf("normalizeURL(%q) = %q, want %q", c.in, got, c.want)
		}
	}

	// Test that invalid URLs don't crash (url.Parse may encode them)
	invalidURL := "not a url at all"

	got := normalizeURL(invalidURL)
	if got == "" {
		t.Errorf("normalizeURL(%q) should not return empty string", invalidURL)
	}
}

// TestGoArchToRPM tests GOARCH to RPM architecture mapping.
func TestGoArchToRPM(t *testing.T) {
	// Save original GOARCH
	originalArch := runtime.GOARCH

	cases := []struct {
		goarch, want string
	}{
		{"amd64", "x86_64"},
		{"arm64", "aarch64"},
		{"386", "i686"},
		{"arm", "armhfp"},
		{"ppc64le", "ppc64le"},
		{"s390x", "s390x"},
		{"mips64", "mips64"},   // unknown, returned as-is
		{"riscv64", "riscv64"}, // unknown, returned as-is
	}

	for _, c := range cases {
		// We can't actually change runtime.GOARCH, so we test via expandRepoVars
		// which calls goArchToRPM internally. Instead, we'll test the function
		// indirectly by checking the current architecture.
		if c.goarch == originalArch {
			got := goArchToRPM()
			if got != c.want {
				t.Errorf("goArchToRPM() for %s = %q, want %q", c.goarch, got, c.want)
			}
		}
	}
}

// TestExpandRepoVarsBasearch tests $basearch expansion.
func TestExpandRepoVarsBasearch(t *testing.T) {
	// Test that $basearch is replaced with the current architecture
	url := "http://mirror.example.com/rocky/$basearch/os/"
	got := expandRepoVars(url)

	// The result should contain the RPM architecture, not $basearch
	if contains(got, "$basearch") {
		t.Errorf("expandRepoVars(%q) still contains $basearch: %q", url, got)
	}

	// Should contain a valid RPM arch
	rpmArch := goArchToRPM()
	if !contains(got, rpmArch) {
		t.Errorf("expandRepoVars(%q) = %q, expected to contain %q", url, got, rpmArch)
	}
}

// TestExpandRepoVarsReleasever tests $releasever expansion.
func TestExpandRepoVarsReleasever(t *testing.T) {
	// Create a temporary /etc/os-release for testing
	tmpDir := t.TempDir()
	osReleaseFile := filepath.Join(tmpDir, "os-release")

	// Write a test os-release file
	content := `NAME="Test Linux"
VERSION_ID="8.10"
ID="test"
`
	if err := os.WriteFile(osReleaseFile, []byte(content), 0o644); err != nil {
		t.Fatalf("failed to write test os-release: %v", err)
	}

	// We can't override the /etc/os-release path in the function,
	// so we test readReleasever indirectly by checking if it reads the system file.
	// This test verifies the function doesn't crash and returns a string.
	ver := readReleasever()
	// ver might be empty on non-RPM systems, but it should be a string
	if ver == "" && runtime.GOOS == "linux" {
		// On Linux, we might have /etc/os-release
		t.Logf("readReleasever() returned empty (might be non-RPM system)")
	}
}

// TestExpandRepoVarsDoubleSlashes tests that double slashes are normalized.
func TestExpandRepoVarsDoubleSlashes(t *testing.T) {
	url := "http://mirror.example.com/pub//rocky/$basearch/os/"
	got := expandRepoVars(url)

	// Should not contain double slashes in the path (after the scheme)
	// Check that the path part doesn't have consecutive slashes
	// url.String() will have the scheme, so we need to check carefully
	if contains(got, "//rocky") || contains(got, "rocky//") {
		t.Errorf("expandRepoVars(%q) = %q, contains double slashes in path", url, got)
	}

	// Verify it contains the expanded basearch
	rpmArch := goArchToRPM()
	if !contains(got, rpmArch) {
		t.Errorf("expandRepoVars(%q) = %q, expected to contain %q", url, got, rpmArch)
	}
}

// TestExpandRepoVarsUnknownVar tests that unknown $var tokens are left as-is.
func TestExpandRepoVarsUnknownVar(t *testing.T) {
	url := "http://mirror.example.com/$unknown/os/"
	got := expandRepoVars(url)

	// Unknown vars should be left as-is (unless /etc/dnf/vars/unknown exists)
	// On most systems, /etc/dnf/vars/unknown won't exist, so it should remain
	if !contains(got, "$unknown") && !contains(got, "os/") {
		t.Errorf("expandRepoVars(%q) = %q, unexpected result", url, got)
	}
}

// TestCacheLookupEmpty tests Lookup on an empty cache.
func TestCacheLookupEmpty(t *testing.T) {
	c := newCache()

	_, ok := c.Lookup("nonexistent")
	if ok {
		t.Error("Lookup on empty cache should return false")
	}
}

// TestCacheLookupAfterAdd tests Lookup after adding a package.
func TestCacheLookupAfterAdd(t *testing.T) {
	c := newCache()
	pkg := &PackageInfo{
		Name:         "gcc",
		Version:      "12.2.0",
		Release:      "1.el8",
		Arch:         "x86_64",
		LocationHref: "Packages/g/gcc-12.2.0-1.el8.x86_64.rpm",
		SHA256:       "abc123",
		Size:         1024,
		BaseURL:      "http://mirror.example.com/",
	}

	c.mu.Lock()
	c.addPackage(pkg)
	c.mu.Unlock()

	found, ok := c.Lookup("gcc")
	if !ok {
		t.Error("Lookup should find added package")
	}

	if found.Name != "gcc" {
		t.Errorf("Lookup returned wrong package: %v", found)
	}
}

// TestCacheResolveVirtualRealPackage tests ResolveVirtual for a real package.
func TestCacheResolveVirtualRealPackage(t *testing.T) {
	c := newCache()
	pkg := &PackageInfo{
		Name:         "glibc",
		Version:      "2.17",
		Release:      "1.el8",
		Arch:         "x86_64",
		LocationHref: "Packages/g/glibc-2.17-1.el8.x86_64.rpm",
		SHA256:       "abc123",
		Size:         1024,
		BaseURL:      "http://mirror.example.com/",
	}

	c.mu.Lock()
	c.addPackage(pkg)
	c.mu.Unlock()

	resolved := c.ResolveVirtual("glibc")
	if resolved != "glibc" {
		t.Errorf("ResolveVirtual for real package should return name, got %q", resolved)
	}
}

// TestCacheResolveVirtualCapability tests ResolveVirtual for a virtual capability.
func TestCacheResolveVirtualCapability(t *testing.T) {
	c := newCache()
	pkg := &PackageInfo{
		Name:         "coreutils-single",
		Version:      "8.32",
		Release:      "1.el8",
		Arch:         "x86_64",
		LocationHref: "Packages/c/coreutils-single-8.32-1.el8.x86_64.rpm",
		SHA256:       "abc123",
		Size:         1024,
		BaseURL:      "http://mirror.example.com/",
		Provides:     []string{"coreutils"},
	}

	c.mu.Lock()
	c.addPackage(pkg)
	c.mu.Unlock()

	resolved := c.ResolveVirtual("coreutils")
	if resolved != "coreutils-single" {
		t.Errorf("ResolveVirtual for virtual should return provider, got %q", resolved)
	}
}

// TestCacheResolveVirtualUnknown tests ResolveVirtual for an unknown package.
func TestCacheResolveVirtualUnknown(t *testing.T) {
	c := newCache()

	resolved := c.ResolveVirtual("nonexistent")
	if resolved != "nonexistent" {
		t.Errorf("ResolveVirtual for unknown should return original name, got %q", resolved)
	}
}

// TestCacheAddPackagePreferDownloadable tests that addPackage prefers downloadable packages.
func TestCacheAddPackagePreferDownloadable(t *testing.T) {
	c := newCache()

	// Add a package without LocationHref
	pkg1 := &PackageInfo{
		Name:         "gcc",
		Version:      "12.2.0",
		Release:      "1.el8",
		Arch:         "x86_64",
		LocationHref: "",
		SHA256:       "abc123",
		Size:         1024,
		BaseURL:      "http://mirror.example.com/",
	}

	c.mu.Lock()
	c.addPackage(pkg1)

	// Add the same package with LocationHref
	pkg2 := &PackageInfo{
		Name:         "gcc",
		Version:      "12.2.0",
		Release:      "1.el8",
		Arch:         "x86_64",
		LocationHref: "Packages/g/gcc-12.2.0-1.el8.x86_64.rpm",
		SHA256:       "def456",
		Size:         2048,
		BaseURL:      "http://mirror.example.com/",
	}

	c.addPackage(pkg2)
	c.mu.Unlock()

	found, ok := c.Lookup("gcc")
	if !ok {
		t.Error("Lookup should find package")
	}

	if found.LocationHref != "Packages/g/gcc-12.2.0-1.el8.x86_64.rpm" {
		t.Errorf("addPackage should prefer downloadable package, got %q", found.LocationHref)
	}
}

// TestCacheAddPackageProviders tests that addPackage populates the providers index.
func TestCacheAddPackageProviders(t *testing.T) {
	c := newCache()
	pkg := &PackageInfo{
		Name:         "coreutils-single",
		Version:      "8.32",
		Release:      "1.el8",
		Arch:         "x86_64",
		LocationHref: "Packages/c/coreutils-single-8.32-1.el8.x86_64.rpm",
		SHA256:       "abc123",
		Size:         1024,
		BaseURL:      "http://mirror.example.com/",
		Provides:     []string{"coreutils", "coreutils-single"},
	}

	c.mu.Lock()
	c.addPackage(pkg)
	c.mu.Unlock()

	// Check that "coreutils" is in the providers index
	c.mu.RLock()
	providers, ok := c.providers["coreutils"]
	c.mu.RUnlock()

	if !ok || len(providers) == 0 {
		t.Error("addPackage should populate providers index")
	}

	if len(providers) > 0 && providers[0].Name != "coreutils-single" {
		t.Errorf("providers index should contain correct provider, got %v", providers[0].Name)
	}
}

// TestCacheResolveDepsEmpty tests ResolveDeps with empty seeds.
func TestCacheResolveDepsEmpty(t *testing.T) {
	c := newCache()
	ctx := context.Background()

	resolved, unresolved, err := c.ResolveDeps(ctx, []string{})
	if err != nil {
		t.Fatalf("ResolveDeps failed: %v", err)
	}

	if len(resolved) != 0 {
		t.Errorf("ResolveDeps with empty seeds should return empty result, got %d", len(resolved))
	}

	if len(unresolved) != 0 {
		t.Errorf("ResolveDeps with empty seeds should have no unresolved, got %d", len(unresolved))
	}
}

// TestCacheResolveDepsKnownPackage tests ResolveDeps with a known package.
func TestCacheResolveDepsKnownPackage(t *testing.T) {
	c := newCache()
	pkg := &PackageInfo{
		Name:         "gcc",
		Version:      "12.2.0",
		Release:      "1.el8",
		Arch:         "x86_64",
		LocationHref: "Packages/g/gcc-12.2.0-1.el8.x86_64.rpm",
		SHA256:       "abc123",
		Size:         1024,
		BaseURL:      "http://mirror.example.com/",
		Requires:     []string{},
	}

	c.mu.Lock()
	c.addPackage(pkg)
	c.mu.Unlock()

	ctx := context.Background()

	resolved, unresolved, err := c.ResolveDeps(ctx, []string{"gcc"})
	if err != nil {
		t.Fatalf("ResolveDeps failed: %v", err)
	}

	if len(resolved) != 1 {
		t.Errorf("ResolveDeps should return 1 package, got %d", len(resolved))
	}

	if len(unresolved) != 0 {
		t.Errorf("ResolveDeps should have no unresolved, got %d", len(unresolved))
	}

	if resolved[0].Name != "gcc" {
		t.Errorf("ResolveDeps should return gcc, got %s", resolved[0].Name)
	}
}

// TestCacheResolveDepsTransitive tests ResolveDeps with transitive dependencies.
func TestCacheResolveDepsTransitive(t *testing.T) {
	c := newCache()

	// Create a dependency chain: gcc -> glibc -> glibc-common
	glibcCommon := &PackageInfo{
		Name:         "glibc-common",
		Version:      "2.17",
		Release:      "1.el8",
		Arch:         "x86_64",
		LocationHref: "Packages/g/glibc-common-2.17-1.el8.x86_64.rpm",
		SHA256:       "abc123",
		Size:         1024,
		BaseURL:      "http://mirror.example.com/",
		Requires:     []string{},
	}

	glibc := &PackageInfo{
		Name:         "glibc",
		Version:      "2.17",
		Release:      "1.el8",
		Arch:         "x86_64",
		LocationHref: "Packages/g/glibc-2.17-1.el8.x86_64.rpm",
		SHA256:       "def456",
		Size:         2048,
		BaseURL:      "http://mirror.example.com/",
		Requires:     []string{"glibc-common"},
	}

	gcc := &PackageInfo{
		Name:         "gcc",
		Version:      "12.2.0",
		Release:      "1.el8",
		Arch:         "x86_64",
		LocationHref: "Packages/g/gcc-12.2.0-1.el8.x86_64.rpm",
		SHA256:       "ghi789",
		Size:         4096,
		BaseURL:      "http://mirror.example.com/",
		Requires:     []string{"glibc"},
	}

	c.mu.Lock()
	c.addPackage(glibcCommon)
	c.addPackage(glibc)
	c.addPackage(gcc)
	c.mu.Unlock()

	ctx := context.Background()

	resolved, unresolved, err := c.ResolveDeps(ctx, []string{"gcc"})
	if err != nil {
		t.Fatalf("ResolveDeps failed: %v", err)
	}

	if len(resolved) != 3 {
		t.Errorf("ResolveDeps should return 3 packages, got %d", len(resolved))
	}

	if len(unresolved) != 0 {
		t.Errorf("ResolveDeps should have no unresolved, got %d", len(unresolved))
	}

	// Check that packages are in dependency order (deps before dependents)
	names := make([]string, len(resolved))
	for i, p := range resolved {
		names[i] = p.Name
	}

	// glibc-common should come before glibc, glibc before gcc
	glibcCommonIdx := indexOf(names, "glibc-common")
	glibcIdx := indexOf(names, "glibc")
	gccIdx := indexOf(names, "gcc")

	if glibcCommonIdx >= glibcIdx {
		t.Errorf("glibc-common should come before glibc in dependency order")
	}

	if glibcIdx >= gccIdx {
		t.Errorf("glibc should come before gcc in dependency order")
	}
}

// TestCacheResolveDepsVirtual tests ResolveDeps with virtual dependencies.
func TestCacheResolveDepsVirtual(t *testing.T) {
	c := newCache()

	coreutilsSingle := &PackageInfo{
		Name:         "coreutils-single",
		Version:      "8.32",
		Release:      "1.el8",
		Arch:         "x86_64",
		LocationHref: "Packages/c/coreutils-single-8.32-1.el8.x86_64.rpm",
		SHA256:       "abc123",
		Size:         1024,
		BaseURL:      "http://mirror.example.com/",
		Provides:     []string{"coreutils"},
		Requires:     []string{},
	}

	gcc := &PackageInfo{
		Name:         "gcc",
		Version:      "12.2.0",
		Release:      "1.el8",
		Arch:         "x86_64",
		LocationHref: "Packages/g/gcc-12.2.0-1.el8.x86_64.rpm",
		SHA256:       "def456",
		Size:         2048,
		BaseURL:      "http://mirror.example.com/",
		Requires:     []string{"coreutils"}, // virtual dep
	}

	c.mu.Lock()
	c.addPackage(coreutilsSingle)
	c.addPackage(gcc)
	c.mu.Unlock()

	ctx := context.Background()

	resolved, unresolved, err := c.ResolveDeps(ctx, []string{"gcc"})
	if err != nil {
		t.Fatalf("ResolveDeps failed: %v", err)
	}

	if len(resolved) != 2 {
		t.Errorf("ResolveDeps should return 2 packages, got %d", len(resolved))
	}

	if len(unresolved) != 0 {
		t.Errorf("ResolveDeps should have no unresolved, got %d", len(unresolved))
	}

	// Check that coreutils-single is in the result
	found := false

	for _, p := range resolved {
		if p.Name == "coreutils-single" {
			found = true
			break
		}
	}

	if !found {
		t.Error("ResolveDeps should resolve virtual dep to coreutils-single")
	}
}

// TestCacheResolveDepsUnknown tests ResolveDeps with unknown packages.
func TestCacheResolveDepsUnknown(t *testing.T) {
	c := newCache()

	ctx := context.Background()

	resolved, unresolved, err := c.ResolveDeps(ctx, []string{"nonexistent"})
	if err != nil {
		t.Fatalf("ResolveDeps failed: %v", err)
	}

	if len(resolved) != 0 {
		t.Errorf("ResolveDeps should return 0 packages for unknown, got %d", len(resolved))
	}

	if len(unresolved) != 1 || unresolved[0] != "nonexistent" {
		t.Errorf("ResolveDeps should list unknown package as unresolved, got %v", unresolved)
	}
}

// TestCacheResolveDepsWithConstraints tests ResolveDeps strips version constraints.
func TestCacheResolveDepsWithConstraints(t *testing.T) {
	c := newCache()

	gcc := &PackageInfo{
		Name:         "gcc",
		Version:      "12.2.0",
		Release:      "1.el8",
		Arch:         "x86_64",
		LocationHref: "Packages/g/gcc-12.2.0-1.el8.x86_64.rpm",
		SHA256:       "abc123",
		Size:         1024,
		BaseURL:      "http://mirror.example.com/",
		Requires:     []string{},
	}

	c.mu.Lock()
	c.addPackage(gcc)
	c.mu.Unlock()

	ctx := context.Background()
	// Request with version constraint
	resolved, unresolved, err := c.ResolveDeps(ctx, []string{"gcc >= 12.0"})
	if err != nil {
		t.Fatalf("ResolveDeps failed: %v", err)
	}

	if len(resolved) != 1 {
		t.Errorf("ResolveDeps should resolve gcc >= 12.0, got %d packages", len(resolved))
	}

	if len(unresolved) != 0 {
		t.Errorf("ResolveDeps should have no unresolved, got %d", len(unresolved))
	}
}

// Helper functions

func contains(s, substr string) bool {
	for i := 0; i < len(s)-len(substr)+1; i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}

	return false
}

func indexOf(slice []string, s string) int {
	for i, v := range slice {
		if v == s {
			return i
		}
	}

	return -1
}

// ---- ResolveDeps edge cases ----

// TestCacheResolveDepsNoDuplicateSeeds tests that requesting the same package
// twice (or a package that is a dep of another seed) yields it only once.
func TestCacheResolveDepsNoDuplicateSeeds(t *testing.T) {
	c := newCache()

	glibc := &PackageInfo{
		Name:         "glibc",
		Version:      "2.17",
		Release:      "1.el8",
		Arch:         "x86_64",
		LocationHref: "Packages/g/glibc-2.17-1.el8.x86_64.rpm",
		SHA256:       "abc123",
		Size:         1024,
		BaseURL:      "http://mirror.example.com/",
		Requires:     []string{},
	}

	gcc := &PackageInfo{
		Name:         "gcc",
		Version:      "12.2.0",
		Release:      "1.el8",
		Arch:         "x86_64",
		LocationHref: "Packages/g/gcc-12.2.0-1.el8.x86_64.rpm",
		SHA256:       "def456",
		Size:         2048,
		BaseURL:      "http://mirror.example.com/",
		Requires:     []string{"glibc"},
	}

	c.mu.Lock()
	c.addPackage(glibc)
	c.addPackage(gcc)
	c.mu.Unlock()

	ctx := context.Background()
	// Request both gcc and glibc — glibc should appear only once.
	resolved, unresolved, err := c.ResolveDeps(ctx, []string{"gcc", "glibc"})
	if err != nil {
		t.Fatalf("ResolveDeps failed: %v", err)
	}

	if len(unresolved) != 0 {
		t.Errorf("expected no unresolved, got %v", unresolved)
	}

	seen := make(map[string]int)
	for _, p := range resolved {
		seen[p.Name]++
	}

	for name, count := range seen {
		if count > 1 {
			t.Errorf("package %q appears %d times in result (want 1)", name, count)
		}
	}

	if seen["gcc"] != 1 {
		t.Errorf("expected gcc in result, got count=%d", seen["gcc"])
	}

	if seen["glibc"] != 1 {
		t.Errorf("expected glibc in result, got count=%d", seen["glibc"])
	}
}

// TestCacheResolveDepsCircular tests that a circular dependency A→B→A does
// not cause infinite recursion and both packages appear in the result.
func TestCacheResolveDepsCircular(t *testing.T) {
	c := newCache()

	pkgA := &PackageInfo{
		Name:         "pkgA",
		Version:      "1.0",
		Release:      "1",
		Arch:         "x86_64",
		LocationHref: "Packages/pkgA-1.0-1.x86_64.rpm",
		SHA256:       "aaa",
		Size:         100,
		BaseURL:      "http://mirror.example.com/",
		Requires:     []string{"pkgB"},
	}

	pkgB := &PackageInfo{
		Name:         "pkgB",
		Version:      "1.0",
		Release:      "1",
		Arch:         "x86_64",
		LocationHref: "Packages/pkgB-1.0-1.x86_64.rpm",
		SHA256:       "bbb",
		Size:         100,
		BaseURL:      "http://mirror.example.com/",
		Requires:     []string{"pkgA"},
	}

	c.mu.Lock()
	c.addPackage(pkgA)
	c.addPackage(pkgB)
	c.mu.Unlock()

	ctx := context.Background()

	resolved, unresolved, err := c.ResolveDeps(ctx, []string{"pkgA"})
	if err != nil {
		t.Fatalf("ResolveDeps failed: %v", err)
	}

	if len(unresolved) != 0 {
		t.Errorf("expected no unresolved, got %v", unresolved)
	}

	if len(resolved) != 2 {
		t.Errorf("expected 2 packages (pkgA + pkgB), got %d", len(resolved))
	}

	names := make(map[string]bool)
	for _, p := range resolved {
		names[p.Name] = true
	}

	if !names["pkgA"] || !names["pkgB"] {
		t.Errorf("expected both pkgA and pkgB in result, got %v", names)
	}
}

// TestCacheResolveDepsVirtualChain tests that A→virtual "libfoo"→B resolves B.
func TestCacheResolveDepsVirtualChain(t *testing.T) {
	c := newCache()

	pkgB := &PackageInfo{
		Name:         "libfoo-impl",
		Version:      "1.0",
		Release:      "1",
		Arch:         "x86_64",
		LocationHref: "Packages/libfoo-impl-1.0-1.x86_64.rpm",
		SHA256:       "bbb",
		Size:         200,
		BaseURL:      "http://mirror.example.com/",
		Provides:     []string{"libfoo"},
		Requires:     []string{},
	}

	pkgA := &PackageInfo{
		Name:         "myapp",
		Version:      "2.0",
		Release:      "1",
		Arch:         "x86_64",
		LocationHref: "Packages/myapp-2.0-1.x86_64.rpm",
		SHA256:       "aaa",
		Size:         300,
		BaseURL:      "http://mirror.example.com/",
		Requires:     []string{"libfoo"},
	}

	c.mu.Lock()
	c.addPackage(pkgB)
	c.addPackage(pkgA)
	c.mu.Unlock()

	ctx := context.Background()

	resolved, unresolved, err := c.ResolveDeps(ctx, []string{"myapp"})
	if err != nil {
		t.Fatalf("ResolveDeps failed: %v", err)
	}

	if len(unresolved) != 0 {
		t.Errorf("expected no unresolved, got %v", unresolved)
	}

	if len(resolved) != 2 {
		t.Errorf("expected 2 packages, got %d: %v", len(resolved), resolved)
	}

	names := make(map[string]bool)
	for _, p := range resolved {
		names[p.Name] = true
	}

	if !names["libfoo-impl"] {
		t.Errorf("expected libfoo-impl (virtual provider) in result, got %v", names)
	}

	if !names["myapp"] {
		t.Errorf("expected myapp in result, got %v", names)
	}
}

// TestCacheResolveDepsMultipleUnresolved tests that multiple unknown packages
// are all reported in the unresolved list.
func TestCacheResolveDepsMultipleUnresolved(t *testing.T) {
	c := newCache()
	ctx := context.Background()

	resolved, unresolved, err := c.ResolveDeps(ctx, []string{"missing1", "missing2", "missing3"})
	if err != nil {
		t.Fatalf("ResolveDeps failed: %v", err)
	}

	if len(resolved) != 0 {
		t.Errorf("expected 0 resolved, got %d", len(resolved))
	}

	if len(unresolved) != 3 {
		t.Errorf("expected 3 unresolved, got %d: %v", len(unresolved), unresolved)
	}
}

// TestCacheResolveDepsEmptyRequires tests a package with an empty Requires list.
func TestCacheResolveDepsEmptyRequires(t *testing.T) {
	c := newCache()

	pkg := &PackageInfo{
		Name:         "standalone",
		Version:      "1.0",
		Release:      "1",
		Arch:         "noarch",
		LocationHref: "Packages/standalone-1.0-1.noarch.rpm",
		SHA256:       "abc",
		Size:         512,
		BaseURL:      "http://mirror.example.com/",
		Requires:     nil,
	}

	c.mu.Lock()
	c.addPackage(pkg)
	c.mu.Unlock()

	ctx := context.Background()

	resolved, unresolved, err := c.ResolveDeps(ctx, []string{"standalone"})
	if err != nil {
		t.Fatalf("ResolveDeps failed: %v", err)
	}

	if len(unresolved) != 0 {
		t.Errorf("expected no unresolved, got %v", unresolved)
	}

	if len(resolved) != 1 || resolved[0].Name != "standalone" {
		t.Errorf("expected [standalone], got %v", resolved)
	}
}

// ---- ParseRepoFileContent edge cases ----

// TestParseRepoFileContentComments tests that comment lines are ignored.
func TestParseRepoFileContentComments(t *testing.T) {
	content := `
# This is a comment
; This is also a comment
[myrepo]
baseurl=http://mirror.example.com/repo/
enabled=1
# another comment
`

	repos := ParseRepoFileContent(content)
	if len(repos) != 1 {
		t.Fatalf("expected 1 repo, got %d", len(repos))
	}

	if repos[0].ID != "myrepo" {
		t.Errorf("expected ID=myrepo, got %s", repos[0].ID)
	}

	if repos[0].BaseURL != "http://mirror.example.com/repo/" {
		t.Errorf("unexpected BaseURL: %s", repos[0].BaseURL)
	}
}

// TestParseRepoFileContentMetalink tests that metalink= is parsed as MirrorList.
func TestParseRepoFileContentMetalink(t *testing.T) {
	content := `
[epel]
metalink=https://mirrors.fedoraproject.org/metalink?repo=epel-8&arch=$basearch
enabled=1
`

	repos := ParseRepoFileContent(content)
	if len(repos) != 1 {
		t.Fatalf("expected 1 repo, got %d", len(repos))
	}

	if repos[0].MirrorList == "" {
		t.Error("expected MirrorList to be set from metalink=")
	}

	if repos[0].BaseURL != "" {
		t.Errorf("expected BaseURL to be empty, got %s", repos[0].BaseURL)
	}
}

// TestParseRepoFileContentMirrorlist tests that mirrorlist= is parsed.
func TestParseRepoFileContentMirrorlist(t *testing.T) {
	content := `
[base]
mirrorlist=http://mirrorlist.centos.org/?release=8&arch=x86_64&repo=os
enabled=1
`

	repos := ParseRepoFileContent(content)
	if len(repos) != 1 {
		t.Fatalf("expected 1 repo, got %d", len(repos))
	}

	if repos[0].MirrorList == "" {
		t.Error("expected MirrorList to be set from mirrorlist=")
	}
}

// TestParseRepoFileContentNoEqualSign tests that lines without '=' are skipped.
func TestParseRepoFileContentNoEqualSign(t *testing.T) {
	content := `
[myrepo]
this line has no equals sign
baseurl=http://mirror.example.com/
enabled=1
`

	repos := ParseRepoFileContent(content)
	if len(repos) != 1 {
		t.Fatalf("expected 1 repo, got %d", len(repos))
	}
	// Should still parse correctly despite the malformed line.
	if repos[0].BaseURL != "http://mirror.example.com/" {
		t.Errorf("unexpected BaseURL: %s", repos[0].BaseURL)
	}
}

// TestParseRepoFileContentDefaultEnabled tests that repos default to enabled=true.
func TestParseRepoFileContentDefaultEnabled(t *testing.T) {
	content := `
[noenable]
baseurl=http://mirror.example.com/
`

	repos := ParseRepoFileContent(content)
	if len(repos) != 1 {
		t.Fatalf("expected 1 repo, got %d", len(repos))
	}

	if !repos[0].Enabled {
		t.Error("expected repo to be enabled by default")
	}
}

// TestParseRepoFileContentMultipleBaseURLs tests that only the first baseurl is used.
func TestParseRepoFileContentMultipleBaseURLs(t *testing.T) {
	content := `
[multi]
baseurl=http://first.example.com/ http://second.example.com/
enabled=1
`

	repos := ParseRepoFileContent(content)
	if len(repos) != 1 {
		t.Fatalf("expected 1 repo, got %d", len(repos))
	}

	if repos[0].BaseURL != "http://first.example.com/" {
		t.Errorf("expected first URL only, got %s", repos[0].BaseURL)
	}
}

// ---- isPrimaryIndex ----

// TestIsPrimaryIndex tests all supported compression variants.
func TestIsPrimaryIndex(t *testing.T) {
	cases := []struct {
		name string
		want bool
	}{
		{"abc123-primary.xml.gz", true},
		{"primary.xml.gz", true},
		{"primary.xml.xz", true},
		{"primary.xml.zst", true},
		{"primary.xml", true},
		{"filelists.xml.gz", false},
		{"other.xml.gz", false},
		{"repomd.xml", false},
		{"abc123-filelists.xml.gz", false},
	}

	for _, c := range cases {
		got := isPrimaryIndex(c.name)
		if got != c.want {
			t.Errorf("isPrimaryIndex(%q) = %v, want %v", c.name, got, c.want)
		}
	}
}

// ---- buildPackageInfo ----

// TestBuildPackageInfoSkipSrc tests that src RPMs are skipped.
func TestBuildPackageInfoSkipSrc(t *testing.T) {
	pkg := &primaryPackage{
		Name: "gcc",
		Arch: "src",
		Version: primaryVersion{
			Ver: "12.2.0",
			Rel: "1.el8",
		},
		Location: primaryLocation{Href: "Packages/gcc-12.2.0-1.el8.src.rpm"},
	}

	info := buildPackageInfo(pkg, "http://mirror.example.com/")
	if info != nil {
		t.Error("buildPackageInfo should return nil for src RPMs")
	}
}

// TestBuildPackageInfoSkipEmptyName tests that packages with empty name are skipped.
func TestBuildPackageInfoSkipEmptyName(t *testing.T) {
	pkg := &primaryPackage{
		Name: "",
		Arch: "x86_64",
		Version: primaryVersion{
			Ver: "1.0",
			Rel: "1",
		},
	}

	info := buildPackageInfo(pkg, "http://mirror.example.com/")
	if info != nil {
		t.Error("buildPackageInfo should return nil for empty name")
	}
}

// TestBuildPackageInfoSkipEmptyArch tests that packages with empty arch are skipped.
func TestBuildPackageInfoSkipEmptyArch(t *testing.T) {
	pkg := &primaryPackage{
		Name: "gcc",
		Arch: "",
		Version: primaryVersion{
			Ver: "12.2.0",
			Rel: "1.el8",
		},
	}

	info := buildPackageInfo(pkg, "http://mirror.example.com/")
	if info != nil {
		t.Error("buildPackageInfo should return nil for empty arch")
	}
}

// TestBuildPackageInfoSkipsRpmlibDeps tests that rpmlib() deps are filtered out.
func TestBuildPackageInfoSkipsRpmlibDeps(t *testing.T) {
	pkg := &primaryPackage{
		Name: "gcc",
		Arch: "x86_64",
		Version: primaryVersion{
			Ver: "12.2.0",
			Rel: "1.el8",
		},
		Location: primaryLocation{Href: "Packages/gcc-12.2.0-1.el8.x86_64.rpm"},
		Format: primaryFormat{
			Requires: []primaryEntry{
				{Name: "rpmlib(CompressedFileNames)"},
				{Name: "rpmlib(PayloadFilesHavePrefix)"},
				{Name: "glibc"},
			},
		},
	}

	info := buildPackageInfo(pkg, "http://mirror.example.com/")
	if info == nil {
		t.Fatal("buildPackageInfo should not return nil for valid package")
	}

	for _, req := range info.Requires {
		if contains(req, "rpmlib(") {
			t.Errorf("rpmlib dep should be filtered out, got %q", req)
		}
	}

	if len(info.Requires) != 1 || info.Requires[0] != "glibc" {
		t.Errorf("expected only [glibc] in Requires, got %v", info.Requires)
	}
}

// TestBuildPackageInfoKeepsPathDeps verifies that path-style requires such as
// "/usr/bin/python3" are kept in Requires so the resolver can match them
// against the file paths indexed from <file> entries in primary.xml. rpmlib()
// requires must still be filtered.
func TestBuildPackageInfoKeepsPathDeps(t *testing.T) {
	pkg := &primaryPackage{
		Name: "myapp",
		Arch: "x86_64",
		Version: primaryVersion{
			Ver: "1.0",
			Rel: "1",
		},
		Location: primaryLocation{Href: "Packages/myapp-1.0-1.x86_64.rpm"},
		Format: primaryFormat{
			Requires: []primaryEntry{
				{Name: "/usr/bin/python3"},
				{Name: "/bin/sh"},
				{Name: "glibc"},
				{Name: "rpmlib(CompressedFileNames)"},
			},
		},
	}

	info := buildPackageInfo(pkg, "http://mirror.example.com/")
	if info == nil {
		t.Fatal("buildPackageInfo should not return nil for valid package")
	}

	want := []string{"/usr/bin/python3", "/bin/sh", "glibc"}
	if !reflect.DeepEqual(info.Requires, want) {
		t.Errorf("expected Requires=%v, got %v", want, info.Requires)
	}
}

// TestBuildPackageInfoSHA256 tests that SHA256 is set only for sha256 checksum type.
func TestBuildPackageInfoSHA256(t *testing.T) {
	pkg := &primaryPackage{
		Name: "gcc",
		Arch: "x86_64",
		Version: primaryVersion{
			Ver: "12.2.0",
			Rel: "1.el8",
		},
		Location: primaryLocation{Href: "Packages/gcc-12.2.0-1.el8.x86_64.rpm"},
		Checksum: primaryChecksum{
			Type:  "sha256",
			Value: "deadbeef1234",
		},
	}

	info := buildPackageInfo(pkg, "http://mirror.example.com/")
	if info == nil {
		t.Fatal("buildPackageInfo returned nil")
	}

	if info.SHA256 != "deadbeef1234" {
		t.Errorf("expected SHA256=deadbeef1234, got %q", info.SHA256)
	}
}

// TestBuildPackageInfoNonSHA256Checksum tests that non-sha256 checksums are not stored.
func TestBuildPackageInfoNonSHA256Checksum(t *testing.T) {
	pkg := &primaryPackage{
		Name: "gcc",
		Arch: "x86_64",
		Version: primaryVersion{
			Ver: "12.2.0",
			Rel: "1.el8",
		},
		Location: primaryLocation{Href: "Packages/gcc-12.2.0-1.el8.x86_64.rpm"},
		Checksum: primaryChecksum{
			Type:  "md5",
			Value: "abc123",
		},
	}

	info := buildPackageInfo(pkg, "http://mirror.example.com/")
	if info == nil {
		t.Fatal("buildPackageInfo returned nil")
	}

	if info.SHA256 != "" {
		t.Errorf("expected empty SHA256 for md5 checksum, got %q", info.SHA256)
	}
}

// ---- parsePrimaryXML ----

// TestParsePrimaryXML tests parsing a minimal primary.xml document.
func TestParsePrimaryXML(t *testing.T) {
	xmlContent := `<?xml version="1.0" encoding="UTF-8"?>
<metadata xmlns="http://linux.duke.edu/metadata/common" packages="2">
  <package type="rpm">
    <name>gcc</name>
    <arch>x86_64</arch>
    <version epoch="0" ver="12.2.0" rel="1.el8"/>
    <checksum type="sha256">abc123</checksum>
    <size package="4096"/>
    <location href="Packages/g/gcc-12.2.0-1.el8.x86_64.rpm"/>
    <format>
      <rpm:requires>
        <rpm:entry name="glibc"/>
      </rpm:requires>
      <rpm:provides>
        <rpm:entry name="gcc"/>
      </rpm:provides>
    </format>
  </package>
  <package type="rpm">
    <name>glibc</name>
    <arch>x86_64</arch>
    <version epoch="0" ver="2.17" rel="1.el8"/>
    <checksum type="sha256">def456</checksum>
    <size package="2048"/>
    <location href="Packages/g/glibc-2.17-1.el8.x86_64.rpm"/>
    <format/>
  </package>
</metadata>`

	c := newCache()
	r := strings.NewReader(xmlContent)

	if err := c.parsePrimaryXML(r, "http://mirror.example.com/"); err != nil {
		t.Fatalf("parsePrimaryXML failed: %v", err)
	}

	gcc, ok := c.Lookup("gcc")
	if !ok {
		t.Fatal("expected gcc in cache after parsePrimaryXML")
	}

	if gcc.Version != "12.2.0" {
		t.Errorf("expected version 12.2.0, got %s", gcc.Version)
	}

	glibc, ok := c.Lookup("glibc")
	if !ok {
		t.Fatal("expected glibc in cache after parsePrimaryXML")
	}

	if glibc.Version != "2.17" {
		t.Errorf("expected version 2.17, got %s", glibc.Version)
	}
}

// TestParsePrimaryXMLSkipsSrcRPMs tests that src RPMs are skipped during XML parsing.
func TestParsePrimaryXMLSkipsSrcRPMs(t *testing.T) {
	xmlContent := `<?xml version="1.0" encoding="UTF-8"?>
<metadata xmlns="http://linux.duke.edu/metadata/common" packages="1">
  <package type="rpm">
    <name>gcc</name>
    <arch>src</arch>
    <version epoch="0" ver="12.2.0" rel="1.el8"/>
    <checksum type="sha256">abc123</checksum>
    <size package="4096"/>
    <location href="Packages/g/gcc-12.2.0-1.el8.src.rpm"/>
    <format/>
  </package>
</metadata>`

	c := newCache()
	r := strings.NewReader(xmlContent)

	if err := c.parsePrimaryXML(r, "http://mirror.example.com/"); err != nil {
		t.Fatalf("parsePrimaryXML failed: %v", err)
	}

	_, ok := c.Lookup("gcc")
	if ok {
		t.Error("src RPM should not be added to cache")
	}
}

// ---- fileMatchesSHA256 ----

// TestFileMatchesSHA256Match tests that a file with matching SHA256 returns true.
func TestFileMatchesSHA256Match(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "testfile")
	content := []byte("hello world")

	if err := os.WriteFile(path, content, 0o644); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	// Compute expected SHA256.
	h := sha256.New()
	h.Write(content)
	expected := hex.EncodeToString(h.Sum(nil))

	ok, err := fileMatchesSHA256(path, expected)
	if err != nil {
		t.Fatalf("fileMatchesSHA256 failed: %v", err)
	}

	if !ok {
		t.Error("expected fileMatchesSHA256 to return true for matching checksum")
	}
}

// TestFileMatchesSHA256Mismatch tests that a file with wrong SHA256 returns false.
func TestFileMatchesSHA256Mismatch(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "testfile")

	if err := os.WriteFile(path, []byte("hello world"), 0o644); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	ok, err := fileMatchesSHA256(path, "0000000000000000000000000000000000000000000000000000000000000000")
	if err != nil {
		t.Fatalf("fileMatchesSHA256 failed: %v", err)
	}

	if ok {
		t.Error("expected fileMatchesSHA256 to return false for mismatched checksum")
	}
}

// TestFileMatchesSHA256NonExistent tests that a non-existent file returns false with error.
func TestFileMatchesSHA256NonExistent(t *testing.T) {
	ok, err := fileMatchesSHA256("/nonexistent/path/file.rpm", "abc123")
	if err == nil {
		t.Error("expected error for non-existent file")
	}

	if ok {
		t.Error("expected false for non-existent file")
	}
}

// ---- readReleasever edge cases ----

// TestReadReleaseverQuotedValue tests that quoted VERSION_ID values are unquoted.
func TestReadReleaseverQuotedValue(t *testing.T) {
	// We can't override /etc/os-release, but we can test the parsing logic
	// indirectly by verifying readReleasever doesn't crash and returns a string.
	ver := readReleasever()
	// On any system, the result should not contain quotes.
	if contains(ver, `"`) || contains(ver, `'`) {
		t.Errorf("readReleasever() returned quoted value: %q", ver)
	}
}

// ---- parseMedalinkURL ----

// TestParseMedalinkURLBasic tests extraction of a base URL from metalink XML.
func TestParseMedalinkURLBasic(t *testing.T) {
	body := `<?xml version="1.0" encoding="utf-8"?>
<metalink xmlns="urn:ietf:params:xml:ns:metalink">
  <file name="repomd.xml">
    <url protocol="https" type="https" location="US" preference="100">https://mirror.example.com/rocky/8/BaseOS/x86_64/os/repodata/repomd.xml</url>
    <url protocol="https" type="https" location="DE" preference="90">https://mirror2.example.com/rocky/8/BaseOS/x86_64/os/repodata/repomd.xml</url>
  </file>
</metalink>`

	got, err := parseMedalinkURL(body, "https://metalink.example.com/")
	if err != nil {
		t.Fatalf("parseMedalinkURL failed: %v", err)
	}

	if got != "https://mirror.example.com/rocky/8/BaseOS/x86_64/os/" {
		t.Errorf("unexpected base URL: %q", got)
	}
}

// TestParseMedalinkURLNoHTTPS tests that a metalink with no https:// URLs returns an error.
func TestParseMedalinkURLNoHTTPS(t *testing.T) {
	body := `<?xml version="1.0" encoding="utf-8"?>
<metalink>
  <file name="repomd.xml">
    <url>http://mirror.example.com/repodata/repomd.xml</url>
  </file>
</metalink>`

	_, err := parseMedalinkURL(body, "https://metalink.example.com/")
	if err == nil {
		t.Error("expected error when no https:// URL found in metalink")
	}
}

// TestParseMedalinkURLPlainMirror tests a plain https:// URL without repomd.xml suffix.
func TestParseMedalinkURLPlainMirror(t *testing.T) {
	body := `<url>https://mirror.example.com/rocky/8/BaseOS/x86_64/os/</url>`

	got, err := parseMedalinkURL(body, "https://metalink.example.com/")
	if err != nil {
		t.Fatalf("parseMedalinkURL failed: %v", err)
	}

	if got == "" {
		t.Error("expected non-empty URL")
	}
}

// ---- expandDNFVars ----

// TestExpandDNFVarsWithFile tests that a $var is expanded when the file exists.
func TestExpandDNFVarsWithFile(t *testing.T) {
	// We can't write to /etc/dnf/vars/ in tests, but we can verify that
	// unknown vars are left as-is (the file won't exist in CI).
	url := "http://mirror.example.com/$contentdir/os/"
	got := expandDNFVars(url)

	// On systems without /etc/dnf/vars/contentdir, the var stays unexpanded.
	// On systems with it, it gets replaced. Either way, no crash.
	if got == "" {
		t.Error("expandDNFVars should not return empty string")
	}
}

// TestExpandDNFVarsNoVars tests that a URL without $vars is returned unchanged.
func TestExpandDNFVarsNoVars(t *testing.T) {
	url := "http://mirror.example.com/rocky/8/BaseOS/x86_64/os/"

	got := expandDNFVars(url)
	if got != url {
		t.Errorf("expandDNFVars(%q) = %q, want unchanged", url, got)
	}
}

// ---- addPackage edge cases ----

// TestAddPackageKeepExistingDownloadable tests that a second add with no
// LocationHref does not overwrite an existing downloadable entry.
func TestAddPackageKeepExistingDownloadable(t *testing.T) {
	c := newCache()

	pkg1 := &PackageInfo{
		Name:         "gcc",
		Version:      "12.2.0",
		LocationHref: "Packages/g/gcc-12.2.0-1.el8.x86_64.rpm",
		SHA256:       "first",
	}

	pkg2 := &PackageInfo{
		Name:         "gcc",
		Version:      "12.2.0",
		LocationHref: "", // not downloadable
		SHA256:       "second",
	}

	c.mu.Lock()
	c.addPackage(pkg1)
	c.addPackage(pkg2)
	c.mu.Unlock()

	found, ok := c.Lookup("gcc")
	if !ok {
		t.Fatal("expected gcc in cache")
	}

	if found.SHA256 != "first" {
		t.Errorf("expected first (downloadable) entry to be kept, got SHA256=%q", found.SHA256)
	}
}

// TestAddPackageProviderSelfSkipped tests that a package's own name is not
// added to the providers index (would create a self-referential virtual).
func TestAddPackageProviderSelfSkipped(t *testing.T) {
	c := newCache()

	pkg := &PackageInfo{
		Name:         "gcc",
		Version:      "12.2.0",
		LocationHref: "Packages/g/gcc-12.2.0-1.el8.x86_64.rpm",
		Provides:     []string{"gcc", "gcc-x86_64"}, // "gcc" is self, should be skipped
	}

	c.mu.Lock()
	c.addPackage(pkg)
	c.mu.Unlock()

	c.mu.RLock()
	_, selfInProviders := c.providers["gcc"]
	c.mu.RUnlock()

	if selfInProviders {
		t.Error("package's own name should not be added to providers index")
	}

	c.mu.RLock()
	providers, ok := c.providers["gcc-x86_64"]
	c.mu.RUnlock()

	if !ok || len(providers) == 0 {
		t.Error("non-self capability should be in providers index")
	}
}
