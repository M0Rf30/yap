package common

import (
	"testing"

	"github.com/M0Rf30/yap/v2/pkg/aptcache"
)

// ─── isPerlModule ────────────────────────────────────────────────────────────

func TestIsPerlModule(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		in   string
		want bool
	}{
		// Packages ending in "-perl" are Perl modules (host-only build tools).
		{"libxml-parser-perl ends in -perl", "libxml-parser-perl", true},
		{"libdbi-perl ends in -perl", "libdbi-perl", true},
		{"suffix only -perl", "-perl", true},
		// Packages that do NOT end in "-perl".
		{"perl-foo does not end in -perl", "perl-foo", false},
		{"perl-XML-Parser does not end in -perl", "perl-XML-Parser", false},
		{"bare perl is not a module", "perl", false},
		{"libperl-dev is not a perl module", "libperl-dev", false},
		{"empty string is not a perl module", "", false},
		{"contains perl but not suffix", "perl-dev-tools", false},
		{"libperl is not a perl module", "libperl", false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got := IsPerlModule(tc.in)
			if got != tc.want {
				t.Errorf("IsPerlModule(%q) = %v, want %v", tc.in, got, tc.want)
			}
		})
	}
}

// ─── partitionArchAllDeps ────────────────────────────────────────────────────

// makeArchAllPkg returns a PackageInfo with Architecture: all.
func makeArchAllPkg(name string) aptcache.PackageInfo { //nolint:unparam // test helper; name varies by call site
	return aptcache.PackageInfo{Name: name, Architecture: "all"}
}

// makeArchSpecificPkg returns a PackageInfo with Architecture: amd64 and
// Multi-Arch: same (typical dev library — must be qualified per arch).
func makeArchSpecificPkg(name string) aptcache.PackageInfo { //nolint:unparam // test helper; name varies by call site
	return aptcache.PackageInfo{Name: name, Architecture: "amd64", MultiArch: "same"}
}

// makeMultiArchForeignPkg returns a PackageInfo with Multi-Arch: foreign
// (host tool — must NOT be qualified with target arch).
func makeMultiArchForeignPkg(name string) aptcache.PackageInfo { //nolint:unparam // test helper; name varies by call site
	return aptcache.PackageInfo{Name: name, Architecture: "amd64", MultiArch: "foreign"}
}

// makeMultiArchAllowedPkg returns a PackageInfo with Multi-Arch: allowed.
func makeMultiArchAllowedPkg(name string) aptcache.PackageInfo {
	return aptcache.PackageInfo{Name: name, Architecture: "amd64", MultiArch: "allowed"}
}

// makeEssentialPkg returns a PackageInfo with Essential: true.
func makeEssentialPkg(name string) aptcache.PackageInfo {
	return aptcache.PackageInfo{Name: name, Architecture: "amd64", Essential: true}
}

// makeInstalledNoMultiArch returns a PackageInfo that is installed but has no
// Multi-Arch field (absent/no) — should go to archAll when installed.
func makeInstalledNoMultiArch(name string) aptcache.PackageInfo {
	return aptcache.PackageInfo{Name: name, Architecture: "amd64", MultiArch: "", Installed: true}
}

// makeNotInstalledNoMultiArch returns a PackageInfo that is NOT installed and
// has no Multi-Arch field — should go to archSpecific.
func makeNotInstalledNoMultiArch(name string) aptcache.PackageInfo {
	return aptcache.PackageInfo{Name: name, Architecture: "amd64", MultiArch: "", Installed: false}
}

func TestPartitionArchAllDeps_Empty(t *testing.T) {
	MakeTestCache(nil)

	specific, all := PartitionArchAllDeps(nil)

	if len(specific) != 0 {
		t.Errorf("archSpecific: want empty, got %v", specific)
	}

	if len(all) != 0 {
		t.Errorf("archAll: want empty, got %v", all)
	}
}

func TestPartitionArchAllDeps_ArchAllPackage(t *testing.T) {
	MakeTestCache([]aptcache.PackageInfo{
		makeArchAllPkg("python3-six"),
	})

	specific, all := PartitionArchAllDeps([]string{"python3-six"})

	if len(specific) != 0 {
		t.Errorf("archSpecific: want empty, got %v", specific)
	}

	if len(all) != 1 || all[0] != "python3-six" {
		t.Errorf("archAll: want [python3-six], got %v", all)
	}
}

func TestPartitionArchAllDeps_ArchSpecificPackage(t *testing.T) {
	MakeTestCache([]aptcache.PackageInfo{
		makeArchSpecificPkg("libssl-dev"),
	})

	specific, all := PartitionArchAllDeps([]string{"libssl-dev"})

	if len(specific) != 1 || specific[0] != "libssl-dev" {
		t.Errorf("archSpecific: want [libssl-dev], got %v", specific)
	}

	if len(all) != 0 {
		t.Errorf("archAll: want empty, got %v", all)
	}
}

func TestPartitionArchAllDeps_PerlModuleAlwaysArchAll(t *testing.T) {
	// Perl modules go to archAll regardless of their apt metadata.
	MakeTestCache([]aptcache.PackageInfo{
		// Even if the cache says Multi-Arch: same, perl modules are host tools.
		{Name: "libxml-parser-perl", Architecture: "amd64", MultiArch: "same"},
	})

	specific, all := PartitionArchAllDeps([]string{"libxml-parser-perl"})

	if len(specific) != 0 {
		t.Errorf("archSpecific: want empty for perl module, got %v", specific)
	}

	if len(all) != 1 || all[0] != "libxml-parser-perl" {
		t.Errorf("archAll: want [libxml-parser-perl], got %v", all)
	}
}

func TestPartitionArchAllDeps_MultiArchForeign(t *testing.T) {
	MakeTestCache([]aptcache.PackageInfo{
		makeMultiArchForeignPkg("cmake"),
	})

	specific, all := PartitionArchAllDeps([]string{"cmake"})

	if len(specific) != 0 {
		t.Errorf("archSpecific: want empty for Multi-Arch:foreign, got %v", specific)
	}

	if len(all) != 1 || all[0] != "cmake" {
		t.Errorf("archAll: want [cmake], got %v", all)
	}
}

func TestPartitionArchAllDeps_MultiArchAllowed(t *testing.T) {
	MakeTestCache([]aptcache.PackageInfo{
		makeMultiArchAllowedPkg("git"),
	})

	specific, all := PartitionArchAllDeps([]string{"git"})

	if len(specific) != 0 {
		t.Errorf("archSpecific: want empty for Multi-Arch:allowed, got %v", specific)
	}

	if len(all) != 1 || all[0] != "git" {
		t.Errorf("archAll: want [git], got %v", all)
	}
}

func TestPartitionArchAllDeps_EssentialPackage(t *testing.T) {
	MakeTestCache([]aptcache.PackageInfo{
		makeEssentialPkg("bash"),
	})

	specific, all := PartitionArchAllDeps([]string{"bash"})

	if len(specific) != 0 {
		t.Errorf("archSpecific: want empty for essential package, got %v", specific)
	}

	if len(all) != 1 || all[0] != "bash" {
		t.Errorf("archAll: want [bash], got %v", all)
	}
}

func TestPartitionArchAllDeps_InstalledNoMultiArch(t *testing.T) {
	// Installed + no Multi-Arch → archAll (avoid dpkg conflict).
	MakeTestCache([]aptcache.PackageInfo{
		makeInstalledNoMultiArch("zlib1g"),
	})

	specific, all := PartitionArchAllDeps([]string{"zlib1g"})

	if len(specific) != 0 {
		t.Errorf("archSpecific: want empty for installed+no-multiarch, got %v", specific)
	}

	if len(all) != 1 || all[0] != "zlib1g" {
		t.Errorf("archAll: want [zlib1g], got %v", all)
	}
}

func TestPartitionArchAllDeps_NotInstalledNoMultiArch(t *testing.T) {
	// Not installed + no Multi-Arch → archSpecific.
	MakeTestCache([]aptcache.PackageInfo{
		makeNotInstalledNoMultiArch("zlib1g-dev"),
	})

	specific, all := PartitionArchAllDeps([]string{"zlib1g-dev"})

	if len(specific) != 1 || specific[0] != "zlib1g-dev" {
		t.Errorf("archSpecific: want [zlib1g-dev], got %v", specific)
	}

	if len(all) != 0 {
		t.Errorf("archAll: want empty, got %v", all)
	}
}

func TestPartitionArchAllDeps_NotInCache(t *testing.T) {
	// Package not in cache → archSpecific (conservative fallback).
	MakeTestCache(nil)

	specific, all := PartitionArchAllDeps([]string{"custom-repo-pkg"})

	if len(specific) != 1 || specific[0] != "custom-repo-pkg" {
		t.Errorf("archSpecific: want [custom-repo-pkg], got %v", specific)
	}

	if len(all) != 0 {
		t.Errorf("archAll: want empty, got %v", all)
	}
}

func TestPartitionArchAllDeps_VersionConstraintStripped(t *testing.T) {
	// "libssl-dev (>= 1.0)" — version constraint must be stripped for lookup.
	MakeTestCache([]aptcache.PackageInfo{
		makeArchSpecificPkg("libssl-dev"),
	})

	specific, all := PartitionArchAllDeps([]string{"libssl-dev (>= 1.0)"})

	if len(specific) != 1 || specific[0] != "libssl-dev (>= 1.0)" {
		t.Errorf("archSpecific: want [libssl-dev (>= 1.0)], got %v", specific)
	}

	if len(all) != 0 {
		t.Errorf("archAll: want empty, got %v", all)
	}
}

func TestPartitionArchAllDeps_ArchQualifierStripped(t *testing.T) {
	// "libssl-dev:amd64" — arch qualifier must be stripped for lookup.
	MakeTestCache([]aptcache.PackageInfo{
		makeArchSpecificPkg("libssl-dev"),
	})

	specific, all := PartitionArchAllDeps([]string{"libssl-dev:amd64"})

	if len(specific) != 1 || specific[0] != "libssl-dev:amd64" {
		t.Errorf("archSpecific: want [libssl-dev:amd64], got %v", specific)
	}

	if len(all) != 0 {
		t.Errorf("archAll: want empty, got %v", all)
	}
}

func TestPartitionArchAllDeps_MixedDeps(t *testing.T) {
	MakeTestCache([]aptcache.PackageInfo{
		makeArchSpecificPkg("libssl-dev"),
		makeArchAllPkg("python3-six"),
		makeMultiArchForeignPkg("cmake"),
		makeEssentialPkg("bash"),
	})

	deps := []string{"libssl-dev", "python3-six", "cmake", "bash"}
	specific, all := PartitionArchAllDeps(deps)

	if len(specific) != 1 || specific[0] != "libssl-dev" {
		t.Errorf("archSpecific: want [libssl-dev], got %v", specific)
	}

	if len(all) != 3 {
		t.Errorf("archAll: want 3 entries, got %v", all)
	}
}

// ─── partitionArchAllDepsForExtract ──────────────────────────────────────────

func TestPartitionArchAllDepsForExtract_Empty(t *testing.T) {
	MakeTestCache(nil)

	specific, all := PartitionArchAllDepsForExtract(nil)

	if len(specific) != 0 {
		t.Errorf("archSpecific: want empty, got %v", specific)
	}

	if len(all) != 0 {
		t.Errorf("archAll: want empty, got %v", all)
	}
}

func TestPartitionArchAllDepsForExtract_ArchAllPackage(t *testing.T) {
	MakeTestCache([]aptcache.PackageInfo{
		makeArchAllPkg("python3-six"),
	})

	specific, all := PartitionArchAllDepsForExtract([]string{"python3-six"})

	if len(specific) != 0 {
		t.Errorf("archSpecific: want empty, got %v", specific)
	}

	if len(all) != 1 || all[0] != "python3-six" {
		t.Errorf("archAll: want [python3-six], got %v", all)
	}
}

func TestPartitionArchAllDepsForExtract_PerlModule(t *testing.T) {
	MakeTestCache([]aptcache.PackageInfo{
		{Name: "libxml-parser-perl", Architecture: "amd64", MultiArch: "same"},
	})

	specific, all := PartitionArchAllDepsForExtract([]string{"libxml-parser-perl"})

	if len(specific) != 0 {
		t.Errorf("archSpecific: want empty for perl module, got %v", specific)
	}

	if len(all) != 1 || all[0] != "libxml-parser-perl" {
		t.Errorf("archAll: want [libxml-parser-perl], got %v", all)
	}
}

func TestPartitionArchAllDepsForExtract_MultiArchForeign(t *testing.T) {
	MakeTestCache([]aptcache.PackageInfo{
		makeMultiArchForeignPkg("cmake"),
	})

	specific, all := PartitionArchAllDepsForExtract([]string{"cmake"})

	if len(specific) != 0 {
		t.Errorf("archSpecific: want empty for Multi-Arch:foreign, got %v", specific)
	}

	if len(all) != 1 || all[0] != "cmake" {
		t.Errorf("archAll: want [cmake], got %v", all)
	}
}

func TestPartitionArchAllDepsForExtract_EssentialPackage(t *testing.T) {
	MakeTestCache([]aptcache.PackageInfo{
		makeEssentialPkg("bash"),
	})

	specific, all := PartitionArchAllDepsForExtract([]string{"bash"})

	if len(specific) != 0 {
		t.Errorf("archSpecific: want empty for essential, got %v", specific)
	}

	if len(all) != 1 || all[0] != "bash" {
		t.Errorf("archAll: want [bash], got %v", all)
	}
}

func TestPartitionArchAllDepsForExtract_InstalledIsStillArchSpecific(t *testing.T) {
	// Unlike partitionArchAllDeps, the extract variant does NOT check Installed —
	// installed packages still get qualified because extraction overwrites files
	// without dpkg conflict checks.
	MakeTestCache([]aptcache.PackageInfo{
		makeInstalledNoMultiArch("zlib1g"),
	})

	specific, all := PartitionArchAllDepsForExtract([]string{"zlib1g"})

	// zlib1g: installed, no Multi-Arch → archSpecific in the extract variant.
	if len(specific) != 1 || specific[0] != "zlib1g" {
		t.Errorf("archSpecific: want [zlib1g] for extract variant, got %v", specific)
	}

	if len(all) != 0 {
		t.Errorf("archAll: want empty for extract variant, got %v", all)
	}
}

func TestPartitionArchAllDepsForExtract_NotInCache(t *testing.T) {
	MakeTestCache(nil)

	specific, all := PartitionArchAllDepsForExtract([]string{"custom-repo-pkg"})

	if len(specific) != 1 || specific[0] != "custom-repo-pkg" {
		t.Errorf("archSpecific: want [custom-repo-pkg], got %v", specific)
	}

	if len(all) != 0 {
		t.Errorf("archAll: want empty, got %v", all)
	}
}

func TestPartitionArchAllDepsForExtract_MixedDeps(t *testing.T) {
	MakeTestCache([]aptcache.PackageInfo{
		makeArchSpecificPkg("libssl-dev"),
		makeArchAllPkg("python3-six"),
		makeMultiArchForeignPkg("cmake"),
	})

	deps := []string{"libssl-dev", "python3-six", "cmake"}
	specific, all := PartitionArchAllDepsForExtract(deps)

	if len(specific) != 1 || specific[0] != "libssl-dev" {
		t.Errorf("archSpecific: want [libssl-dev], got %v", specific)
	}

	if len(all) != 2 {
		t.Errorf("archAll: want 2 entries, got %v", all)
	}
}

// ─── countDirect ─────────────────────────────────────────────────────────────

func TestCountDirect_EmptyResolved(t *testing.T) {
	t.Parallel()

	n := CountDirect(nil, map[string]bool{"foo": true})
	if n != 0 {
		t.Errorf("want 0, got %d", n)
	}
}

func TestCountDirect_EmptySeedSet(t *testing.T) {
	t.Parallel()

	resolved := []*aptcache.PackageInfo{
		{Name: "libssl-dev"},
		{Name: "cmake"},
	}

	n := CountDirect(resolved, map[string]bool{})
	if n != 0 {
		t.Errorf("want 0, got %d", n)
	}
}

func TestCountDirect_AllInSeedSet(t *testing.T) {
	t.Parallel()

	resolved := []*aptcache.PackageInfo{
		{Name: "libssl-dev"},
		{Name: "cmake"},
	}
	seed := map[string]bool{"libssl-dev": true, "cmake": true}

	n := CountDirect(resolved, seed)
	if n != 2 {
		t.Errorf("want 2, got %d", n)
	}
}

func TestCountDirect_NoneInSeedSet(t *testing.T) {
	t.Parallel()

	resolved := []*aptcache.PackageInfo{
		{Name: "libssl1.1"},
		{Name: "zlib1g"},
	}
	seed := map[string]bool{"libssl-dev": true}

	n := CountDirect(resolved, seed)
	if n != 0 {
		t.Errorf("want 0, got %d", n)
	}
}

func TestCountDirect_MixedSeedSet(t *testing.T) {
	t.Parallel()

	resolved := []*aptcache.PackageInfo{
		{Name: "libssl-dev"}, // direct
		{Name: "libssl1.1"},  // transitive
		{Name: "cmake"},      // direct
		{Name: "zlib1g"},     // transitive
	}
	seed := map[string]bool{"libssl-dev": true, "cmake": true}

	n := CountDirect(resolved, seed)
	if n != 2 {
		t.Errorf("want 2, got %d", n)
	}
}

func TestCountDirect_NilEntrySkipped(t *testing.T) {
	t.Parallel()

	resolved := []*aptcache.PackageInfo{
		{Name: "libssl-dev"},
		nil,
		{Name: "cmake"},
	}
	seed := map[string]bool{"libssl-dev": true, "cmake": true}

	n := CountDirect(resolved, seed)
	if n != 2 {
		t.Errorf("want 2 (nil skipped), got %d", n)
	}
}

// ─── ValidateTargetArch ───────────────────────────────────────────────────────

func TestValidateTargetArch(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		arch    string
		wantErr bool
	}{
		// Empty string → native build, always valid.
		{"empty string is native build", "", false},
		// Known architectures from archTargetTable.
		{"aarch64 is valid", "aarch64", false},
		{"x86_64 is valid", "x86_64", false},
		{"armv7 is valid", "armv7", false},
		{"armv6 is valid", "armv6", false},
		{"i686 is valid", "i686", false},
		{"ppc64le is valid", "ppc64le", false},
		{"s390x is valid", "s390x", false},
		{"riscv64 is valid", "riscv64", false},
		// Invalid / unknown architectures.
		{"invalid-arch returns error", "invalid-arch", true},
		{"arm64 is not a canonical arch", "arm64", true},
		{"amd64 is not a canonical arch", "amd64", true},
		{"AARCH64 uppercase is invalid", "AARCH64", true},
		{"random string is invalid", "mips64el", true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			err := ValidateTargetArch(tc.arch)
			if (err != nil) != tc.wantErr {
				t.Errorf("ValidateTargetArch(%q) error = %v, wantErr %v", tc.arch, err, tc.wantErr)
			}
		})
	}
}

func TestValidateTargetArch_ErrorMessageContainsKnownArches(t *testing.T) {
	t.Parallel()

	err := ValidateTargetArch("bogus")
	if err == nil {
		t.Fatal("expected error for unknown arch")
	}

	msg := err.Error()

	for _, known := range []string{"aarch64", "x86_64", "armv7"} {
		if !containsStr(msg, known) {
			t.Errorf("error message %q does not mention known arch %q", msg, known)
		}
	}
}

// containsStr is a simple substring helper to avoid importing strings in test.
func containsStr(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || s != "" && stringContains(s, sub))
}

func stringContains(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}

	return false
}
