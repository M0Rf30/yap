//nolint:testpackage // exercises unexported helpers (filterLogNumbered, etc.)
package mcp

import (
	"strings"
	"testing"
)

func TestDetectArtifactFormat(t *testing.T) {
	cases := map[string]string{
		"foo.deb":                "deb",
		"foo.RPM":                "rpm",
		"foo.apk":                "apk",
		"foo-1.0-1.pkg.tar.zst":  "pkg",
		"foo-1.0-1.pkg.tar.xz":   "pkg",
		"foo.tar.gz":             "unknown",
		"":                       "unknown",
		"/path/to/foo.deb":       "deb",
		"/path/foo.deb.cdx.json": "unknown",
	}

	for in, want := range cases {
		if got := detectArtifactFormat(in); got != want {
			t.Errorf("detectArtifactFormat(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestSplitTag(t *testing.T) {
	cases := []struct {
		in, distro, release string
	}{
		{"ubuntu-noble", "ubuntu", "noble"},
		{"arch", "arch", ""},
		{"alpine", "alpine", ""},
		{"opensuse-leap-15", "opensuse", "leap-15"},
	}

	for _, c := range cases {
		d, r := splitTag(c.in)
		if d != c.distro || r != c.release {
			t.Errorf("splitTag(%q) = (%q,%q), want (%q,%q)",
				c.in, d, r, c.distro, c.release)
		}
	}
}

func TestValidateBuildCompression(t *testing.T) {
	for _, ok := range []string{"", "zstd", "gzip", "xz"} {
		if err := validateBuildCompression(ok, ok); err != nil {
			t.Errorf("validateBuildCompression(%q) unexpected error: %v", ok, err)
		}
	}

	if err := validateBuildCompression("bzip2", ""); err == nil {
		t.Error("expected error for unsupported deb compression")
	}

	if err := validateBuildCompression("", "bzip2"); err == nil {
		t.Error("expected error for unsupported rpm compression")
	}
}

func TestBuildOptionsFromArgsPassthrough(t *testing.T) {
	args := &buildArgs{
		Verbose:         true,
		CleanBuild:      true,
		Parallel:        true,
		SBOM:            true,
		SBOMFormat:      "spdx",
		FromPkgName:     "a",
		ToPkgName:       "z",
		TargetArch:      "aarch64",
		UnverifiedRepos: true,
		ExtraRepos:      []string{"r"},
		SkipDeps:        []string{"d"},
	}

	opts := buildOptionsFromArgs(args)

	if !opts.Verbose || !opts.CleanBuild || !opts.Parallel || !opts.SBOM {
		t.Errorf("bool fields not propagated: %+v", opts)
	}

	if opts.SBOMFormat != "spdx" || opts.FromPkgName != "a" || opts.ToPkgName != "z" ||
		opts.TargetArch != "aarch64" || !opts.AllowUnverifiedRepos {
		t.Errorf("string fields not propagated: %+v", opts)
	}

	if len(opts.ExtraRepos) != 1 || len(opts.SkipDeps) != 1 {
		t.Errorf("slice fields not propagated: %+v", opts)
	}
}

func TestFilterLogNumbered(t *testing.T) {
	raw := "line one\nERROR boom\nline three\nFAILED step\nline five"

	// Tail only.
	out, _ := filterLogNumbered(raw, 2, "", 0)
	if len(out) != 2 || out[len(out)-1].Text != "line five" {
		t.Errorf("tail=2 got %+v", out)
	}

	// Grep without context.
	out, invalid := filterLogNumbered(raw, 0, "ERROR|FAIL", 0)
	if invalid {
		t.Error("valid regexp flagged invalid")
	}

	if len(out) != 2 {
		t.Errorf("grep matches = %d, want 2", len(out))
	}

	// Grep with context.
	out, _ = filterLogNumbered(raw, 0, "ERROR", 1)
	if len(out) != 3 {
		t.Errorf("grep+ctx=1 lines = %d, want 3", len(out))
	}

	// Invalid regexp.
	_, invalid = filterLogNumbered(raw, 0, "[", 0)
	if !invalid {
		t.Error("invalid regexp not flagged")
	}

	// Empty input.
	out, _ = filterLogNumbered("", 0, "", 0)
	if out != nil {
		t.Errorf("empty raw returned %v", out)
	}
}

func TestLastErrorLineAndPhase(t *testing.T) {
	raw := "starting build\nbuild step running\nERROR something failed\nfakeroot step\nFAILED late"
	last, phase := lastErrorLine(raw)

	if !strings.Contains(last, "FAILED late") {
		t.Errorf("last error = %q", last)
	}

	if phase == "" {
		t.Error("phase tag empty")
	}

	if got, _ := lastErrorLine(""); got != "" {
		t.Errorf("empty input got %q", got)
	}
}

func TestInferHintsRecognisesKeywords(t *testing.T) {
	raw := "linker error: undefined reference to `foo'\nPerl is required"
	hints := inferHints(raw)

	if len(hints) < 2 {
		t.Errorf("expected at least 2 hints, got %d: %v", len(hints), hints)
	}

	if got := inferHints(""); got != nil {
		t.Errorf("empty raw hints = %v", got)
	}
}

func TestOutputDirForSession(t *testing.T) {
	if got := outputDirForSession(nil); got != "" {
		t.Errorf("nil session got %q", got)
	}

	s := &BuildSession{Path: "/tmp/proj"}
	if got := outputDirForSession(s); got != "/tmp/proj" {
		t.Errorf("fallback to Path got %q", got)
	}

	s.OutputDir = "/tmp/out"
	if got := outputDirForSession(s); got != "/tmp/out" {
		t.Errorf("OutputDir got %q, want /tmp/out", got)
	}
}
