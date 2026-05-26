//nolint:testpackage
package dnfcache

import (
	"os"
	"strings"
	"testing"
)

func TestPackageNVRA(t *testing.T) {
	got := packageNVRA("perl", "4", "5.26.3", "422.el8", "x86_64")

	want := "perl-4:5.26.3-422.el8.x86_64"
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}

	// Empty epoch defaults to "0".
	got = packageNVRA("perl", "", "5.26.3", "422.el8", "x86_64")

	want = "perl-0:5.26.3-422.el8.x86_64"
	if got != want {
		t.Fatalf("empty epoch: got %q want %q", got, want)
	}
}

func TestIsModularPackage(t *testing.T) {
	for _, tc := range []struct {
		rel  string
		want bool
	}{
		{"404.module+el8.6.0+882+2fa1e48f", true},
		{"422.el8", false},
		{"", false},
		{".module+", true},
	} {
		if got := isModularPackage(tc.rel); got != tc.want {
			t.Errorf("isModularPackage(%q)=%v want %v", tc.rel, got, tc.want)
		}
	}
}

func TestIsModuleIndex(t *testing.T) {
	for _, tc := range []struct {
		name string
		want bool
	}{
		{"abc-modules.yaml.xz", true},
		{"abc-modules.yaml.gz", true},
		{"modules.yaml", true},
		{"abc-primary.xml.gz", false},
		{"repomd.xml", false},
	} {
		if got := isModuleIndex(tc.name); got != tc.want {
			t.Errorf("isModuleIndex(%q)=%v want %v", tc.name, got, tc.want)
		}
	}
}

func TestParseModulesYAMLBasic(t *testing.T) {
	doc := `---
document: modulemd
version: 2
data:
  name: perl
  stream: '5.24'
  artifacts:
    rpms:
      - perl-0:5.24.4-404.module+el8.6.0+882+2fa1e48f.x86_64
      - perl-libs-4:5.24.4-404.module+el8.6.0+882+2fa1e48f.x86_64
...
---
document: modulemd
version: 2
data:
  name: perl
  stream: '5.26'
  artifacts:
    rpms:
      - perl-0:5.26.3-419.module+el8.5.0+728+2c8a1bd2.x86_64
      - perl-libs-4:5.26.3-419.module+el8.5.0+728+2c8a1bd2.x86_64
...
---
document: modulemd-defaults
version: 1
data:
  module: perl
  stream: '5.26'
...
`
	idx := newModuleIndex()
	parseModulesYAML(strings.NewReader(doc), idx)

	if got := idx.defaultStream["perl"]; got != "5.26" {
		t.Errorf("default stream perl=%q want 5.26", got)
	}

	if !idx.allowedNVRA["perl-0:5.26.3-419.module+el8.5.0+728+2c8a1bd2.x86_64"] {
		t.Errorf("expected perl-5.26 NVRA in allowed set")
	}

	if idx.allowedNVRA["perl-0:5.24.4-404.module+el8.6.0+882+2fa1e48f.x86_64"] {
		t.Errorf("perl-5.24 NVRA must NOT be in allowed set")
	}
}

// TestParseModulesFileRocky8 verifies parsing against the real Rocky 8
// AppStream modules.yaml.xz when /tmp/modtest/modules.yaml.xz is present.
// Skipped in CI; populated by the developer for ground-truth validation.
func TestParseModulesFileRocky8(t *testing.T) {
	path := "/tmp/modtest/modules.yaml.xz"
	if _, err := os.Stat(path); err != nil {
		t.Skip("Rocky 8 modules.yaml.xz fixture not present; skipping")
	}

	idx := newModuleIndex()

	if err := parseModulesFile(path, idx); err != nil {
		t.Fatalf("parseModulesFile: %v", err)
	}

	// Perl default must be 5.26 on Rocky 8.10.
	if got := idx.defaultStream["perl"]; got != "5.26" {
		t.Errorf("perl default stream=%q want 5.26 (got %d total defaults)",
			got, len(idx.defaultStream))
	}

	// The 5.24 perl NVRA from the failing build must NOT be allowed.
	bad := "perl-0:5.24.4-404.module+el8.6.0+882+2fa1e48f.x86_64"
	if idx.allowedNVRA[bad] {
		t.Errorf("perl-5.24 NVRA must NOT be allowed: %s", bad)
	}

	// All non-default perl-libs NVRAs (5.24, 5.30, 5.32) must be blocked.
	// Note: perl 5.26 is the default but ships as non-modular in BaseOS, so
	// its modulemd `artifacts.rpms` is empty — that's fine because
	// addPackage only filters MODULAR packages.
	for _, bad := range []string{
		"perl-libs-4:5.24.4-404.module+el8.6.0+882+2fa1e48f.x86_64",
		"perl-libs-4:5.30.1-452.module+el8.6.0+878+f93dfff7.x86_64",
		"perl-libs-4:5.32.1-473.module+el8.10.0+1616+0d20cc68.x86_64",
	} {
		if idx.allowedNVRA[bad] {
			t.Errorf("non-default perl-libs NVRA must NOT be allowed: %s", bad)
		}
	}

	t.Logf("defaults=%d allowed_nvra=%d", len(idx.defaultStream), len(idx.allowedNVRA))
}
