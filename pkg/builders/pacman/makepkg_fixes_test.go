package pacman

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// newPrepared builds a Pkg with a populated PackageDir ready for PrepareFakeroot.
func newPrepared(t *testing.T) (pkg *Pkg, packageDir, artifactsDir string) {
	t.Helper()

	root := t.TempDir()
	startDir := filepath.Join(root, "start")
	packageDir = filepath.Join(root, "package")
	artifactsDir = filepath.Join(root, "artifacts")

	for _, dir := range []string{startDir, packageDir, artifactsDir} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", dir, err)
		}
	}

	if err := os.WriteFile(filepath.Join(startDir, "PKGBUILD"), []byte("pkgname=x\n"), 0o644); err != nil {
		t.Fatalf("write PKGBUILD: %v", err)
	}

	if err := os.WriteFile(filepath.Join(packageDir, "test.txt"), []byte("hi"), 0o644); err != nil {
		t.Fatalf("write test.txt: %v", err)
	}

	pb := createTestPKGBUILD()
	pb.StartDir = startDir
	pb.PackageDir = packageDir
	pb.Home = startDir

	return NewBuilder(pb), packageDir, artifactsDir
}

func TestPrepareFakerootUsesTargetArchInMetadata(t *testing.T) {
	pkg, packageDir, artifactsDir := newPrepared(t)

	if err := pkg.PrepareFakeroot(context.Background(), artifactsDir, "aarch64"); err != nil {
		t.Fatalf("PrepareFakeroot failed: %v", err)
	}

	pkginfo, err := os.ReadFile(filepath.Join(packageDir, ".PKGINFO"))
	if err != nil {
		t.Fatalf("read .PKGINFO: %v", err)
	}

	if !strings.Contains(string(pkginfo), "arch = aarch64") {
		t.Errorf(".PKGINFO should declare the target arch, got:\n%s", pkginfo)
	}

	buildinfo, err := os.ReadFile(filepath.Join(packageDir, ".BUILDINFO"))
	if err != nil {
		t.Fatalf("read .BUILDINFO: %v", err)
	}

	if !strings.Contains(string(buildinfo), "pkgarch = aarch64") {
		t.Errorf(".BUILDINFO should declare the target arch, got:\n%s", buildinfo)
	}
}

func TestPrepareFakerootShipsInstallScriptlet(t *testing.T) {
	pkg, packageDir, artifactsDir := newPrepared(t)
	pkg.PKGBUILD.PostInst = "echo installed"
	pkg.PKGBUILD.PreRm = "echo removing"

	if err := pkg.PrepareFakeroot(context.Background(), artifactsDir, ""); err != nil {
		t.Fatalf("PrepareFakeroot failed: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(packageDir, ".INSTALL"))
	if err != nil {
		t.Fatalf(".INSTALL must be shipped inside the package payload: %v", err)
	}

	content := string(data)
	for _, want := range []string{"post_install()", "echo installed", "pre_remove()", "echo removing"} {
		if !strings.Contains(content, want) {
			t.Errorf(".INSTALL missing %q, got:\n%s", want, content)
		}
	}
}

func TestPrepareFakerootNoScriptletsNoInstall(t *testing.T) {
	pkg, packageDir, artifactsDir := newPrepared(t)

	if err := pkg.PrepareFakeroot(context.Background(), artifactsDir, ""); err != nil {
		t.Fatalf("PrepareFakeroot failed: %v", err)
	}

	if _, err := os.Stat(filepath.Join(packageDir, ".INSTALL")); err == nil {
		t.Error(".INSTALL must not be created without scriptlets")
	}
}

func TestPrepareFakerootInstallIncludesHelpers(t *testing.T) {
	pkg, packageDir, artifactsDir := newPrepared(t)
	pkg.PKGBUILD.HelperFunctions = map[string]string{
		"_my_helper": "function _my_helper() { echo helper; }\n",
	}
	pkg.PKGBUILD.PostInst = "_my_helper\n"

	if err := pkg.PrepareFakeroot(context.Background(), artifactsDir, ""); err != nil {
		t.Fatalf("PrepareFakeroot failed: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(packageDir, ".INSTALL"))
	if err != nil {
		t.Fatalf("read .INSTALL: %v", err)
	}

	if !strings.Contains(string(data), "function _my_helper()") {
		t.Errorf(".INSTALL should contain the helper preamble, got:\n%s", data)
	}
}
