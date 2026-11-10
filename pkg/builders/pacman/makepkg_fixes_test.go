// SPDX-FileCopyrightText: 2021-2026 Gianluca Boiano
// SPDX-License-Identifier: GPL-3.0-only

package pacman

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/M0Rf30/pkgformat/pacman"
	"github.com/klauspost/compress/zstd"
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

// buildPrepared runs PrepareFakeroot then BuildPackage and returns the
// package path.
func buildPrepared(t *testing.T, pkg *Pkg, artifactsDir, arch string) string {
	t.Helper()

	if err := pkg.PrepareFakeroot(context.Background(), artifactsDir, arch); err != nil {
		t.Fatalf("PrepareFakeroot failed: %v", err)
	}

	path, err := pkg.BuildPackage(context.Background(), artifactsDir, arch)
	if err != nil {
		t.Fatalf("BuildPackage failed: %v", err)
	}

	return path
}

// readMembers returns the content of every regular member of a zstd package,
// keyed by archive name (with directory entries as empty strings).
func readMembers(t *testing.T, path string) map[string]string {
	t.Helper()

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read package: %v", err)
	}

	zr, err := zstd.NewReader(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("zstd: %v", err)
	}

	defer zr.Close()

	members := map[string]string{}
	tr := tar.NewReader(zr)

	for {
		hdr, err := tr.Next()
		if errors.Is(err, io.EOF) {
			return members
		}

		if err != nil {
			t.Fatalf("tar: %v", err)
		}

		body, err := io.ReadAll(tr)
		if err != nil {
			t.Fatalf("read %s: %v", hdr.Name, err)
		}

		members[hdr.Name] = string(body)
	}
}

func gunzip(t *testing.T, s string) string {
	t.Helper()

	zr, err := gzip.NewReader(strings.NewReader(s))
	if err != nil {
		t.Fatalf("gunzip: %v", err)
	}

	data, err := io.ReadAll(zr)
	if err != nil {
		t.Fatalf("gunzip: %v", err)
	}

	return string(data)
}

func TestBuildPackageMetadataRoundTrips(t *testing.T) {
	pkg, packageDir, artifactsDir := newPrepared(t)
	pb := pkg.PKGBUILD
	pb.URL = "https://example.com"
	pb.Group = "tools"
	pb.Replaces = []string{"old"}
	pb.Conflicts = []string{"clash"}
	pb.Provides = []string{"virt"}
	pb.Backup = []string{"etc/test.conf"}
	pb.OptDepends = []string{"opt: extra"}

	path := buildPrepared(t, pkg, artifactsDir, "x86_64")

	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}

	defer func() { _ = f.Close() }()

	parsed, err := pacman.ReadPackage(f)
	if err != nil {
		t.Fatalf("the library must read what the builder wrote: %v", err)
	}

	info := parsed.Info

	check := func(name string, got, want any) {
		t.Helper()

		if !equal(got, want) {
			t.Errorf("%s = %v, want %v", name, got, want)
		}
	}

	check("name", info.Name, "test-package")
	check("version", info.Version, "1:1.0.0-1")
	check("arch", info.Arch, "x86_64")
	check("desc", info.Description, "Test package description")
	check("url", info.URL, "https://example.com")
	check("packager", info.Packager, "test@example.com")
	check("licenses", info.Licenses, []string{"MIT"})
	check("groups", info.Groups, []string{"tools"})
	check("depends", info.Depends, []string{"dependency1"})
	check("makedepends", info.MakeDepends, []string{"make", "gcc"})
	check("optdepends", info.OptDepends, []string{"opt: extra"})
	check("replaces", info.Replaces, []string{"old"})
	check("conflicts", info.Conflicts, []string{"clash"})
	check("provides", info.Provides, []string{"virt"})
	check("backup", info.Backup, []string{"etc/test.conf"})
	check("xdata", info.XData, []string{"pkgtype=pkg"})
	check("compression", parsed.Compression, "zst")
	check("files", parsed.Files, []string{"test.txt"})

	if info.InstalledSz != pb.InstalledSize || info.InstalledSz == 0 {
		t.Errorf("installed size = %d, want %d (>0)", info.InstalledSz, pb.InstalledSize)
	}

	if _, err := os.Stat(filepath.Join(packageDir, ".PKGINFO")); err == nil {
		t.Error(".PKGINFO must not be staged into the package tree")
	}
}

func equal(a, b any) bool {
	switch x := a.(type) {
	case []string:
		y, ok := b.([]string)
		if !ok || len(x) != len(y) {
			return false
		}

		for i := range x {
			if x[i] != y[i] {
				return false
			}
		}

		return true
	default:
		return a == b
	}
}

func TestBuildPackageUsesTargetArchInMetadata(t *testing.T) {
	pkg, _, artifactsDir := newPrepared(t)

	members := readMembers(t, buildPrepared(t, pkg, artifactsDir, "aarch64"))

	if !strings.Contains(members[".PKGINFO"], "arch = aarch64") {
		t.Errorf(".PKGINFO should declare the target arch, got:\n%s", members[".PKGINFO"])
	}

	if !strings.Contains(members[".PKGINFO"], "# Generated by yap ") {
		t.Errorf(".PKGINFO should name yap as generator, got:\n%s", members[".PKGINFO"])
	}

	if !strings.Contains(members[".BUILDINFO"], "pkgarch = aarch64") {
		t.Errorf(".BUILDINFO should declare the target arch, got:\n%s", members[".BUILDINFO"])
	}

	if !strings.Contains(members[".BUILDINFO"], "buildtool = yap") ||
		!strings.Contains(members[".BUILDINFO"], "options = strip") {
		t.Errorf(".BUILDINFO misses the build tool or default options:\n%s", members[".BUILDINFO"])
	}
}

func TestBuildPackageBuildInfoCarriesPKGBUILDChecksum(t *testing.T) {
	pkg, _, artifactsDir := newPrepared(t)

	members := readMembers(t, buildPrepared(t, pkg, artifactsDir, ""))

	want := "pkgbuild_sha256sum = " + pkg.PKGBUILD.Checksum
	if pkg.PKGBUILD.Checksum == "" || !strings.Contains(members[".BUILDINFO"], want) {
		t.Errorf(".BUILDINFO should carry %q, got:\n%s", want, members[".BUILDINFO"])
	}
}

func TestBuildPackageShipsInstallScriptlet(t *testing.T) {
	pkg, packageDir, artifactsDir := newPrepared(t)
	pkg.PKGBUILD.PostInst = "echo installed"
	pkg.PKGBUILD.PreRm = "echo removing"

	members := readMembers(t, buildPrepared(t, pkg, artifactsDir, ""))

	content, ok := members[".INSTALL"]
	if !ok {
		t.Fatal(".INSTALL must be shipped inside the package")
	}

	for _, want := range []string{"post_install()", "echo installed", "pre_remove()", "echo removing"} {
		if !strings.Contains(content, want) {
			t.Errorf(".INSTALL missing %q, got:\n%s", want, content)
		}
	}

	// A hook that only declares install steps runs them on upgrade too.
	if !strings.Contains(content, "post_upgrade()") {
		t.Errorf(".INSTALL should fall back post_install to post_upgrade, got:\n%s", content)
	}

	if !strings.Contains(gunzip(t, members[".MTREE"]), "./.INSTALL ") {
		t.Errorf(".MTREE should list .INSTALL, got:\n%s", gunzip(t, members[".MTREE"]))
	}

	if _, err := os.Stat(filepath.Join(packageDir, ".INSTALL")); err == nil {
		t.Error(".INSTALL must not be staged into the package tree")
	}
}

func TestBuildPackageNoScriptletsNoInstall(t *testing.T) {
	pkg, _, artifactsDir := newPrepared(t)

	members := readMembers(t, buildPrepared(t, pkg, artifactsDir, ""))

	if _, ok := members[".INSTALL"]; ok {
		t.Error(".INSTALL must not be created without scriptlets")
	}

	if _, ok := members[".CHANGELOG"]; ok {
		t.Error(".CHANGELOG must not be created without a changelog")
	}
}

func TestBuildPackageInstallIncludesHelpers(t *testing.T) {
	pkg, _, artifactsDir := newPrepared(t)
	pkg.PKGBUILD.HelperFunctions = map[string]string{
		"_my_helper": "function _my_helper() { echo helper; }\n",
	}
	pkg.PKGBUILD.PostInst = "_my_helper\n"

	members := readMembers(t, buildPrepared(t, pkg, artifactsDir, ""))

	if !strings.Contains(members[".INSTALL"], "function _my_helper()") {
		t.Errorf(".INSTALL should contain the helper preamble, got:\n%s", members[".INSTALL"])
	}
}

func TestBuildPackageMtreeModesAndDotfiles(t *testing.T) {
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

	mtree := gunzip(t, readMembers(t, buildPrepared(t, pkg, artifactsDir, ""))[".MTREE"])

	for _, want := range []string{
		"./etc time=", "mode=755 type=dir",
		"./etc/.bashrc ",
		"./.PKGINFO ",
		"./.BUILDINFO ",
		"./link ", "type=link",
		"./suid time=", "mode=4755 ",
	} {
		if !strings.Contains(mtree, want) {
			t.Errorf(".MTREE missing %q, got:\n%s", want, mtree)
		}
	}

	if strings.Contains(mtree, "./.MTREE") {
		t.Errorf(".MTREE must not list itself, got:\n%s", mtree)
	}
}

func TestBuildPackageErrors(t *testing.T) {
	pkg, _, artifactsDir := newPrepared(t)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if _, err := pkg.BuildPackage(ctx, artifactsDir, ""); err == nil {
		t.Error("a cancelled context must fail the build")
	}

	pkg.PKGBUILD.PackageDir = filepath.Join(artifactsDir, "absent")

	if _, err := pkg.BuildPackage(context.Background(), artifactsDir, ""); err == nil {
		t.Error("a missing package dir must fail the build")
	}

	pkg.PKGBUILD.PackageDir = filepath.Dir(artifactsDir) + "/package"
	pkg.PKGBUILD.PkgName = "Bad Name"

	if _, err := pkg.BuildPackage(context.Background(), artifactsDir, ""); err == nil {
		t.Error("an invalid package name must fail the build")
	}

	if matches, _ := filepath.Glob(filepath.Join(artifactsDir, "*.pkg.tar.*")); len(matches) != 0 {
		t.Errorf("a failed build must leave no package behind, got %v", matches)
	}
}
