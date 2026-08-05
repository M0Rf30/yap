package parser_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/M0Rf30/yap/v2/pkg/parser"
)

// writeFile is a small test helper that creates a file (and its parent
// directory) with the given content, failing the test on error.
func writeFile(t *testing.T, path, content string) {
	t.Helper()

	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		t.Fatalf("failed to create parent dir for %s: %v", path, err)
	}

	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("failed to write %s: %v", path, err)
	}
}

func TestDetectSpec_PKGBUILDWinsWhenBothExist(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "PKGBUILD"), "pkgname=demo\npkgver=1\npkgrel=1\n")
	writeFile(t, filepath.Join(dir, "nfpm.yaml"), "name: demo\nversion: \"1\"\n")

	kind, specPath, found := parser.DetectSpec(dir)
	if !found {
		t.Fatal("expected DetectSpec to find a spec")
	}

	if kind != parser.SpecPKGBUILD {
		t.Fatalf("expected SpecPKGBUILD to win when both exist, got %q", kind)
	}

	wantPath, err := filepath.Abs(filepath.Join(dir, "PKGBUILD"))
	if err != nil {
		t.Fatalf("filepath.Abs failed: %v", err)
	}

	if specPath != wantPath {
		t.Fatalf("expected spec path %q, got %q", wantPath, specPath)
	}
}

func TestDetectSpec_NfpmOnly(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "nfpm.yaml"), "name: demo\nversion: \"1\"\n")

	kind, specPath, found := parser.DetectSpec(dir)
	if !found {
		t.Fatal("expected DetectSpec to find the nfpm spec")
	}

	if kind != parser.SpecNFPM {
		t.Fatalf("expected SpecNFPM, got %q", kind)
	}

	if specPath == "" {
		t.Fatal("expected a non-empty spec path")
	}
}

func TestDetectSpec_Neither(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()

	kind, specPath, found := parser.DetectSpec(dir)
	if found {
		t.Fatalf("expected DetectSpec to find nothing, got kind=%q path=%q", kind, specPath)
	}

	if kind != "" || specPath != "" {
		t.Fatalf("expected zero values when not found, got kind=%q path=%q", kind, specPath)
	}
}

func TestParseFile_NfpmSpec(t *testing.T) {
	t.Parallel()

	testdataPath := "testdata/nfpm-single"

	pkgBuild, err := parser.ParseFile("ubuntu", "focal", testdataPath, testdataPath, "")
	if err != nil {
		t.Fatalf("ParseFile() error: %v", err)
	}

	if pkgBuild.PkgName != "nfpm-single" {
		t.Errorf("expected PkgName %q, got %q", "nfpm-single", pkgBuild.PkgName)
	}

	if pkgBuild.PkgVer != "1.2.3" {
		t.Errorf("expected PkgVer %q, got %q", "1.2.3", pkgBuild.PkgVer)
	}

	if pkgBuild.PkgRel != "2" {
		t.Errorf("expected PkgRel %q, got %q", "2", pkgBuild.PkgRel)
	}

	if len(pkgBuild.Depends) != 1 || pkgBuild.Depends[0] != "bash" {
		t.Errorf("expected Depends [bash], got %v", pkgBuild.Depends)
	}

	wantBackup := "etc/nfpm-single/nfpm-single.conf"

	found := false

	for _, b := range pkgBuild.Backup {
		if b == wantBackup {
			found = true
		}

		if strings.HasPrefix(b, "/") {
			t.Errorf("Backup entry %q must not have a leading slash", b)
		}
	}

	if !found {
		t.Errorf("expected Backup to contain %q, got %v", wantBackup, pkgBuild.Backup)
	}

	if strings.TrimSpace(pkgBuild.Package) == "" {
		t.Error("expected a non-empty synthesized package() body")
	}
}

func TestParseFile_NfpmSpec_UnsupportedDistro(t *testing.T) {
	t.Parallel()

	testdataPath := "testdata/nfpm-single"

	_, err := parser.ParseFile("totally-unknown-distro", "", testdataPath, testdataPath, "")
	if err == nil {
		t.Fatal("expected an error for a distro whose format has no nfpm packager")
	}

	if !strings.Contains(err.Error(), "totally-unknown-distro") {
		t.Errorf("expected the error to name the distro, got: %v", err)
	}
}

func TestParseFile_NfpmSpec_PipelineContract(t *testing.T) {
	t.Parallel()

	testdataPath := "testdata/nfpm-single"

	pkgBuild, err := parser.ParseFile("ubuntu", "focal", testdataPath, testdataPath, "")
	if err != nil {
		t.Fatalf("ParseFile() error: %v", err)
	}

	if err := pkgBuild.ValidateMandatoryItems(); err != nil {
		t.Errorf("ValidateMandatoryItems() error: %v", err)
	}

	if err := pkgBuild.ValidateGeneral(); err != nil {
		t.Errorf("ValidateGeneral() error: %v", err)
	}

	if err := pkgBuild.ComputeArchitecture(); err != nil {
		t.Errorf("ComputeArchitecture() error: %v", err)
	}
}

func TestParseFile_SpecFilePopulatedForBothDialects(t *testing.T) {
	t.Parallel()

	t.Run("pkgbuild", func(t *testing.T) {
		t.Parallel()

		testdataPath := "testdata/split-package"

		pkgBuild, err := parser.ParseFile("ubuntu", "focal", testdataPath, testdataPath, "")
		if err != nil {
			t.Fatalf("ParseFile() error: %v", err)
		}

		wantPath, err := filepath.Abs(filepath.Join(testdataPath, "PKGBUILD"))
		if err != nil {
			t.Fatalf("filepath.Abs failed: %v", err)
		}

		if pkgBuild.SpecFile != wantPath {
			t.Errorf("SpecFile = %q, want %q", pkgBuild.SpecFile, wantPath)
		}
	})

	t.Run("nfpm", func(t *testing.T) {
		t.Parallel()

		testdataPath := "testdata/nfpm-single"

		pkgBuild, err := parser.ParseFile("ubuntu", "focal", testdataPath, testdataPath, "")
		if err != nil {
			t.Fatalf("ParseFile() error: %v", err)
		}

		wantPath, err := filepath.Abs(filepath.Join(testdataPath, "nfpm.yaml"))
		if err != nil {
			t.Fatalf("filepath.Abs failed: %v", err)
		}

		if pkgBuild.SpecFile != wantPath {
			t.Errorf("SpecFile = %q, want %q", pkgBuild.SpecFile, wantPath)
		}
	})
}
