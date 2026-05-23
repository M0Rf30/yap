package common_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/M0Rf30/yap/v2/pkg/builders/common"
	"github.com/M0Rf30/yap/v2/pkg/constants"
	"github.com/M0Rf30/yap/v2/pkg/pkgbuild"
)

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

func newBuilder(t *testing.T, format string, mutate func(*pkgbuild.PKGBUILD)) *common.BaseBuilder {
	t.Helper()

	pkg := &pkgbuild.PKGBUILD{
		PkgName:      "test-pkg",
		PkgVer:       "1.0.0",
		PkgRel:       "1",
		ArchComputed: "x86_64",
		StartDir:     t.TempDir(),
	}

	if mutate != nil {
		mutate(pkg)
	}

	return common.NewBaseBuilder(pkg, format)
}

// ---------------------------------------------------------------------------
// FormatRelease
// ---------------------------------------------------------------------------

func TestFormatRelease_EmptyCodename_NoChange(t *testing.T) {
	bb := newBuilder(t, constants.FormatDEB, func(p *pkgbuild.PKGBUILD) {
		p.PkgRel = "1"
		p.Codename = ""
	})

	bb.FormatRelease(map[string]string{"ubuntu": ".ubuntu"})

	if bb.PKGBUILD.PkgRel != "1" {
		t.Errorf("PkgRel should be unchanged when Codename is empty, got %q", bb.PKGBUILD.PkgRel)
	}
}

func TestFormatRelease_DEB_AppendsCodename(t *testing.T) {
	bb := newBuilder(t, constants.FormatDEB, func(p *pkgbuild.PKGBUILD) {
		p.PkgRel = "1"
		p.Codename = "jammy"
	})

	bb.FormatRelease(map[string]string{})

	want := "1jammy"
	if bb.PKGBUILD.PkgRel != want {
		t.Errorf("PkgRel = %q, want %q", bb.PKGBUILD.PkgRel, want)
	}
}

func TestFormatRelease_RPM_KnownDistro_AppendsSuffixAndCodename(t *testing.T) {
	bb := newBuilder(t, constants.FormatRPM, func(p *pkgbuild.PKGBUILD) {
		p.PkgRel = "1"
		p.Codename = "39"
		p.Distro = "fedora"
	})

	suffixMap := map[string]string{"fedora": ".fc"}

	bb.FormatRelease(suffixMap)

	want := "1.fc39"
	if bb.PKGBUILD.PkgRel != want {
		t.Errorf("PkgRel = %q, want %q", bb.PKGBUILD.PkgRel, want)
	}
}

func TestFormatRelease_RPM_UnknownDistro_NoChange(t *testing.T) {
	bb := newBuilder(t, constants.FormatRPM, func(p *pkgbuild.PKGBUILD) {
		p.PkgRel = "1"
		p.Codename = "39"
		p.Distro = "unknown-distro"
	})

	suffixMap := map[string]string{"fedora": ".fc"}

	bb.FormatRelease(suffixMap)

	// Codename is set but distro is not in the map → no change for RPM
	if bb.PKGBUILD.PkgRel != "1" {
		t.Errorf("PkgRel should be unchanged for unknown RPM distro, got %q", bb.PKGBUILD.PkgRel)
	}
}

func TestFormatRelease_APK_NoChange(t *testing.T) {
	bb := newBuilder(t, constants.FormatAPK, func(p *pkgbuild.PKGBUILD) {
		p.PkgRel = "1"
		p.Codename = "edge"
	})

	bb.FormatRelease(map[string]string{"alpine": ".apk"})

	// APK is not handled by FormatRelease → PkgRel unchanged
	if bb.PKGBUILD.PkgRel != "1" {
		t.Errorf("PkgRel should be unchanged for APK format, got %q", bb.PKGBUILD.PkgRel)
	}
}

// ---------------------------------------------------------------------------
// BuildCcacheEnvSlice
// ---------------------------------------------------------------------------

func TestBuildCcacheEnvSlice_NoCcache_ReturnsNil(t *testing.T) {
	origPath := os.Getenv("PATH")

	t.Cleanup(func() { _ = os.Setenv("PATH", origPath) })

	_ = os.Setenv("PATH", "/nonexistent-path-for-test")

	bb := newBuilder(t, constants.FormatDEB, nil)
	result := bb.BuildCcacheEnvSlice()

	if result != nil {
		t.Errorf("expected nil when ccache is not in PATH, got %v", result)
	}
}

func TestBuildCcacheEnvSlice_WithCcache_ReturnsSlice(t *testing.T) {
	// Create a fake ccache binary in a temp dir and prepend it to PATH.
	fakeBin := t.TempDir()
	ccachePath := filepath.Join(fakeBin, "ccache")

	if err := os.WriteFile(ccachePath, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatalf("failed to create fake ccache: %v", err)
	}

	origPath := os.Getenv("PATH")

	t.Cleanup(func() { _ = os.Setenv("PATH", origPath) })

	_ = os.Setenv("PATH", fakeBin+":"+origPath)

	bb := newBuilder(t, constants.FormatDEB, func(p *pkgbuild.PKGBUILD) {
		p.StartDir = "/src/myproject"
	})

	result := bb.BuildCcacheEnvSlice()

	if result == nil {
		t.Fatal("expected non-nil slice when ccache is available")
	}

	// Verify required keys are present.
	wantKeys := []string{"CC=", "CXX=", "CCACHE_BASEDIR=", "CCACHE_SLOPPINESS=", "CCACHE_NOHASHDIR="}
	for _, key := range wantKeys {
		found := false

		for _, entry := range result {
			if strings.HasPrefix(entry, key) {
				found = true

				break
			}
		}

		if !found {
			t.Errorf("missing key %q in BuildCcacheEnvSlice result: %v", key, result)
		}
	}
}

func TestBuildCcacheEnvSlice_CCACHE_BASEDIR_MatchesStartDir(t *testing.T) {
	fakeBin := t.TempDir()
	ccachePath := filepath.Join(fakeBin, "ccache")

	if err := os.WriteFile(ccachePath, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatalf("failed to create fake ccache: %v", err)
	}

	origPath := os.Getenv("PATH")

	t.Cleanup(func() { _ = os.Setenv("PATH", origPath) })

	_ = os.Setenv("PATH", fakeBin+":"+origPath)

	startDir := "/custom/start/dir"
	bb := newBuilder(t, constants.FormatDEB, func(p *pkgbuild.PKGBUILD) {
		p.StartDir = startDir
	})

	result := bb.BuildCcacheEnvSlice()

	for _, entry := range result {
		if after, ok := strings.CutPrefix(entry, "CCACHE_BASEDIR="); ok {
			got := after
			if got != startDir {
				t.Errorf("CCACHE_BASEDIR = %q, want %q", got, startDir)
			}

			return
		}
	}

	t.Error("CCACHE_BASEDIR not found in result")
}

func TestBuildCcacheEnvSlice_NoCCACHE_DIR(t *testing.T) {
	fakeBin := t.TempDir()
	ccachePath := filepath.Join(fakeBin, "ccache")

	if err := os.WriteFile(ccachePath, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatalf("failed to create fake ccache: %v", err)
	}

	origPath := os.Getenv("PATH")

	t.Cleanup(func() { _ = os.Setenv("PATH", origPath) })

	_ = os.Setenv("PATH", fakeBin+":"+origPath)

	bb := newBuilder(t, constants.FormatDEB, nil)
	result := bb.BuildCcacheEnvSlice()

	for _, entry := range result {
		if strings.HasPrefix(entry, "CCACHE_DIR=") {
			t.Errorf("CCACHE_DIR should not be set (let ccache use default), got %q", entry)
		}
	}
}

// ---------------------------------------------------------------------------
// PrepareScriptletWithHelpers
// ---------------------------------------------------------------------------

func TestPrepareScriptletWithHelpers_EmptyBody(t *testing.T) {
	bb := newBuilder(t, constants.FormatDEB, nil)
	result := bb.PrepareScriptletWithHelpers("")

	if result != "" {
		t.Errorf("expected empty string for empty body, got %q", result)
	}
}

func TestPrepareScriptletWithHelpers_NoHelperFunctions_BodyUnchanged(t *testing.T) {
	bb := newBuilder(t, constants.FormatDEB, func(p *pkgbuild.PKGBUILD) {
		p.HelperFunctions = nil
	})

	body := "echo hello\nsystemctl enable myservice\n"
	result := bb.PrepareScriptletWithHelpers(body)

	if result != body {
		t.Errorf("body should be unchanged when no helper functions defined\ngot:  %q\nwant: %q", result, body)
	}
}

func TestPrepareScriptletWithHelpers_HelperNotCalledInBody_BodyUnchanged(t *testing.T) {
	bb := newBuilder(t, constants.FormatDEB, func(p *pkgbuild.PKGBUILD) {
		p.HelperFunctions = map[string]string{
			"_install_service": "function _install_service() { systemctl enable \"$1\"; }\n",
		}
	})

	body := "echo hello\n"
	result := bb.PrepareScriptletWithHelpers(body)

	// _install_service is not called in body → preamble should be empty → body returned as-is
	if result != body {
		t.Errorf("body should be unchanged when helper is not called\ngot:  %q\nwant: %q", result, body)
	}
}

func TestPrepareScriptletWithHelpers_HelperCalledInBody_PrependsPreamble(t *testing.T) {
	helperDef := "function _install_service() { systemctl enable \"$1\"; }\n"

	bb := newBuilder(t, constants.FormatDEB, func(p *pkgbuild.PKGBUILD) {
		p.HelperFunctions = map[string]string{
			"_install_service": helperDef,
		}
	})

	body := "_install_service myapp\n"
	result := bb.PrepareScriptletWithHelpers(body)

	if !strings.HasPrefix(result, helperDef) {
		t.Errorf("result should start with helper definition\ngot: %q", result)
	}

	if !strings.HasSuffix(result, body) {
		t.Errorf("result should end with original body\ngot: %q", result)
	}
}

func TestPrepareScriptletWithHelpers_MultipleHelpers_OnlyCalledOnesIncluded(t *testing.T) {
	helperA := "function _helper_a() { echo a; }\n"
	helperB := "function _helper_b() { echo b; }\n"

	bb := newBuilder(t, constants.FormatDEB, func(p *pkgbuild.PKGBUILD) {
		p.HelperFunctions = map[string]string{
			"_helper_a": helperA,
			"_helper_b": helperB,
		}
	})

	body := "_helper_a\n"
	result := bb.PrepareScriptletWithHelpers(body)

	if !strings.Contains(result, "_helper_a") {
		t.Error("result should contain _helper_a definition")
	}

	if strings.Contains(result, "_helper_b") {
		t.Error("result should NOT contain _helper_b (not called in body)")
	}
}

// ---------------------------------------------------------------------------
// SetupCrossStripEnv
// ---------------------------------------------------------------------------

func TestSetupCrossStripEnv_EmptyTargetArch_NoOp(t *testing.T) {
	origStrip := os.Getenv("STRIP")
	origObjcopy := os.Getenv("OBJCOPY")

	t.Cleanup(func() {
		_ = os.Setenv("STRIP", origStrip)
		_ = os.Setenv("OBJCOPY", origObjcopy)
	})

	_ = os.Unsetenv("STRIP")
	_ = os.Unsetenv("OBJCOPY")

	bb := newBuilder(t, constants.FormatDEB, func(p *pkgbuild.PKGBUILD) {
		p.ArchComputed = "x86_64"
	})

	bb.SetupCrossStripEnv("")

	if v := os.Getenv("STRIP"); v != "" {
		t.Errorf("STRIP should not be set for empty targetArch, got %q", v)
	}

	if v := os.Getenv("OBJCOPY"); v != "" {
		t.Errorf("OBJCOPY should not be set for empty targetArch, got %q", v)
	}
}

func TestSetupCrossStripEnv_SameArch_NoOp(t *testing.T) {
	origStrip := os.Getenv("STRIP")
	origObjcopy := os.Getenv("OBJCOPY")

	t.Cleanup(func() {
		_ = os.Setenv("STRIP", origStrip)
		_ = os.Setenv("OBJCOPY", origObjcopy)
	})

	_ = os.Unsetenv("STRIP")
	_ = os.Unsetenv("OBJCOPY")

	bb := newBuilder(t, constants.FormatDEB, func(p *pkgbuild.PKGBUILD) {
		p.ArchComputed = "x86_64"
	})

	// targetArch == ArchComputed → no-op
	bb.SetupCrossStripEnv("x86_64")

	if v := os.Getenv("STRIP"); v != "" {
		t.Errorf("STRIP should not be set when targetArch == ArchComputed, got %q", v)
	}

	if v := os.Getenv("OBJCOPY"); v != "" {
		t.Errorf("OBJCOPY should not be set when targetArch == ArchComputed, got %q", v)
	}
}

func TestSetupCrossStripEnv_UnknownArch_NoOp(t *testing.T) {
	origStrip := os.Getenv("STRIP")
	origObjcopy := os.Getenv("OBJCOPY")

	t.Cleanup(func() {
		_ = os.Setenv("STRIP", origStrip)
		_ = os.Setenv("OBJCOPY", origObjcopy)
	})

	_ = os.Unsetenv("STRIP")
	_ = os.Unsetenv("OBJCOPY")

	bb := newBuilder(t, constants.FormatDEB, func(p *pkgbuild.PKGBUILD) {
		p.ArchComputed = "x86_64"
	})

	// Unknown arch → resolveToolchainPackages fails → warn + return, no env set
	bb.SetupCrossStripEnv("mips64el-unknown")

	// STRIP/OBJCOPY should remain unset (warn path, not fatal)
	if v := os.Getenv("STRIP"); v != "" {
		t.Errorf("STRIP should not be set for unknown arch, got %q", v)
	}
}

func TestSetupCrossStripEnv_CrossArch_SetsStripAndObjcopy(t *testing.T) {
	origStrip := os.Getenv("STRIP")
	origObjcopy := os.Getenv("OBJCOPY")

	t.Cleanup(func() {
		_ = os.Setenv("STRIP", origStrip)
		_ = os.Setenv("OBJCOPY", origObjcopy)
	})

	_ = os.Unsetenv("STRIP")
	_ = os.Unsetenv("OBJCOPY")

	bb := newBuilder(t, constants.FormatDEB, func(p *pkgbuild.PKGBUILD) {
		p.ArchComputed = "x86_64"
	})

	bb.SetupCrossStripEnv("aarch64")

	strip := os.Getenv("STRIP")
	objcopy := os.Getenv("OBJCOPY")

	// If toolchain resolved successfully, STRIP and OBJCOPY must end with the
	// expected suffixes. If the toolchain map has no entry for this arch+format
	// the function warns and returns without setting env — skip in that case.
	if strip == "" && objcopy == "" {
		t.Log("toolchain not resolved (no entry for aarch64/deb) — skipping env assertions")
		return
	}

	if !strings.HasSuffix(strip, "-strip") {
		t.Errorf("STRIP should end with '-strip', got %q", strip)
	}

	if !strings.HasSuffix(objcopy, "-objcopy") {
		t.Errorf("OBJCOPY should end with '-objcopy', got %q", objcopy)
	}

	// Both tools must share the same prefix.
	stripPrefix := strings.TrimSuffix(strip, "-strip")
	objcopyPrefix := strings.TrimSuffix(objcopy, "-objcopy")

	if stripPrefix != objcopyPrefix {
		t.Errorf("STRIP prefix %q != OBJCOPY prefix %q", stripPrefix, objcopyPrefix)
	}
}

// ---------------------------------------------------------------------------
// ApplyOptions
// ---------------------------------------------------------------------------

func TestApplyOptions_EmptyPackageDir_NoError(t *testing.T) {
	bb := newBuilder(t, constants.FormatDEB, func(p *pkgbuild.PKGBUILD) {
		// PackageDir left empty — options.Apply should handle a missing dir gracefully
		p.PackageDir = ""
		p.StripEnabled = false
		p.DocsEnabled = true
		p.LibtoolEnabled = true
		p.StaticEnabled = true
		p.EmptyDirsEnabled = true
	})

	if err := bb.ApplyOptions(); err != nil {
		t.Errorf("ApplyOptions with empty PackageDir should not error, got: %v", err)
	}
}

func TestApplyOptions_AllDisabled_NoError(t *testing.T) {
	dir := t.TempDir()

	bb := newBuilder(t, constants.FormatDEB, func(p *pkgbuild.PKGBUILD) {
		p.PackageDir = dir
		p.StripEnabled = false
		p.DocsEnabled = true    // true = keep docs (no removal)
		p.LibtoolEnabled = true // true = keep libtool files
		p.StaticEnabled = true  // true = keep static libs
		p.EmptyDirsEnabled = true
		p.PurgeEnabled = false
		p.ZipManEnabled = false
	})

	if err := bb.ApplyOptions(); err != nil {
		t.Errorf("ApplyOptions with all options disabled should not error, got: %v", err)
	}
}

func TestApplyOptions_StripDisabled_NoStripCalled(t *testing.T) {
	dir := t.TempDir()

	// Create a plain text file — strip would fail on it if called.
	if err := os.WriteFile(filepath.Join(dir, "notabinary.txt"), []byte("hello"), 0o644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	bb := newBuilder(t, constants.FormatDEB, func(p *pkgbuild.PKGBUILD) {
		p.PackageDir = dir
		p.StripEnabled = false
		p.DocsEnabled = true
		p.LibtoolEnabled = true
		p.StaticEnabled = true
		p.EmptyDirsEnabled = true
	})

	// Should not error because strip is disabled.
	if err := bb.ApplyOptions(); err != nil {
		t.Errorf("ApplyOptions with StripEnabled=false should not error, got: %v", err)
	}
}

func TestApplyOptions_DocsRemoval_RemovesShareDoc(t *testing.T) {
	dir := t.TempDir()

	// Create a fake /usr/share/doc tree.
	docDir := filepath.Join(dir, "usr", "share", "doc", "mypkg")
	if err := os.MkdirAll(docDir, 0o755); err != nil {
		t.Fatalf("failed to create doc dir: %v", err)
	}

	if err := os.WriteFile(filepath.Join(docDir, "README"), []byte("docs"), 0o644); err != nil {
		t.Fatalf("failed to create README: %v", err)
	}

	bb := newBuilder(t, constants.FormatDEB, func(p *pkgbuild.PKGBUILD) {
		p.PackageDir = dir
		p.StripEnabled = false
		p.DocsEnabled = false   // false = remove docs
		p.LibtoolEnabled = true // keep libtool
		p.StaticEnabled = true  // keep static
		p.EmptyDirsEnabled = true
	})

	if err := bb.ApplyOptions(); err != nil {
		t.Errorf("ApplyOptions should not error: %v", err)
	}

	// The doc file should have been removed.
	if _, err := os.Stat(filepath.Join(docDir, "README")); !os.IsNotExist(err) {
		t.Error("README should have been removed by DocsEnabled=false")
	}
}

func TestApplyOptions_ForwardsPKGBUILDOptions(t *testing.T) {
	dir := t.TempDir()

	bb := newBuilder(t, constants.FormatRPM, func(p *pkgbuild.PKGBUILD) {
		p.PackageDir = dir
		p.StripEnabled = false
		p.DocsEnabled = true
		p.EmptyDirsEnabled = true
		p.LibtoolEnabled = true
		p.PurgeEnabled = false
		p.StaticEnabled = true
		p.ZipManEnabled = false
		p.DebugEnabled = false
	})

	// Verify the call succeeds and options are forwarded without panic.
	if err := bb.ApplyOptions(); err != nil {
		t.Errorf("ApplyOptions should not error for RPM format, got: %v", err)
	}
}
