// SPDX-FileCopyrightText: 2021-2026 Gianluca Boiano
// SPDX-License-Identifier: GPL-3.0-only

package pacman

import (
	"compress/gzip"
	"context"
	"io"
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

	mtree := readMtree(t, filepath.Join(packageDir, ".MTREE"))
	if !strings.Contains(mtree, "./.INSTALL ") {
		t.Errorf(".MTREE should list .INSTALL, got:\n%s", mtree)
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

func readMtree(t *testing.T, path string) string {
	t.Helper()

	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("open .MTREE: %v", err)
	}

	defer func() { _ = f.Close() }()

	zr, err := gzip.NewReader(f)
	if err != nil {
		t.Fatalf("gunzip .MTREE: %v", err)
	}

	data, err := io.ReadAll(zr)
	if err != nil {
		t.Fatalf("read .MTREE: %v", err)
	}

	return string(data)
}

func TestMtreeModesAndDotfiles(t *testing.T) {
	pkg, packageDir, artifactsDir := newPrepared(t)

	etcDir := filepath.Join(packageDir, "etc")
	if err := os.MkdirAll(etcDir, 0o755); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(filepath.Join(etcDir, ".bashrc"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := os.Symlink("test.txt", filepath.Join(packageDir, "link")); err != nil {
		t.Fatal(err)
	}

	suid := filepath.Join(packageDir, "suid")
	if err := os.WriteFile(suid, []byte("x"), 0o755); err != nil {
		t.Fatal(err)
	}

	if err := os.Chmod(suid, 0o755|os.ModeSetuid); err != nil {
		t.Fatal(err)
	}

	if err := pkg.PrepareFakeroot(context.Background(), artifactsDir, ""); err != nil {
		t.Fatalf("PrepareFakeroot failed: %v", err)
	}

	mtree := readMtree(t, filepath.Join(packageDir, ".MTREE"))

	for _, want := range []string{
		"./etc time=", "mode=755 type=dir",
		"./etc/.bashrc ",
		"./.PKGINFO ",
		"./.BUILDINFO ",
		"./link ", "mode=777 type=link",
		"./suid time=", "mode=4755 ",
	} {
		if !strings.Contains(mtree, want) {
			t.Errorf(".MTREE missing %q, got:\n%s", want, mtree)
		}
	}

	if strings.Contains(mtree, "./.MTREE") {
		t.Errorf(".MTREE must not list itself, got:\n%s", mtree)
	}

	if strings.Contains(mtree, "20000000") || strings.Contains(mtree, "1000000000") {
		t.Errorf(".MTREE contains Go FileMode bits, got:\n%s", mtree)
	}
}

func TestMtreeMode(t *testing.T) {
	tests := []struct {
		mode os.FileMode
		want string
	}{
		{0o644, "644"},
		{os.ModeDir | 0o755, "755"},
		{os.ModeSymlink | 0o777, "777"},
		{os.ModeSetuid | 0o755, "4755"},
		{os.ModeSetgid | 0o755, "2755"},
		{os.ModeDir | os.ModeSticky | 0o777, "1777"},
		{os.ModeSetuid | os.ModeSetgid | os.ModeSticky | 0o755, "7755"},
	}

	for _, tt := range tests {
		if got := mtreeMode(tt.mode); got != tt.want {
			t.Errorf("mtreeMode(%v) = %q, want %q", tt.mode, got, tt.want)
		}
	}
}
