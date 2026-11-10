// SPDX-FileCopyrightText: 2021-2026 Gianluca Boiano
// SPDX-License-Identifier: GPL-3.0-only

package apk

import (
	"archive/tar"
	"bytes"
	"context"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	pkgapk "github.com/M0Rf30/pkgformat/apk"
	"github.com/klauspost/compress/gzip"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/M0Rf30/yap/v2/pkg/pkgbuild"
)

func createTestPKGBUILD() *pkgbuild.PKGBUILD {
	return &pkgbuild.PKGBUILD{
		PkgName:    "test-package",
		PkgVer:     "1.0.0",
		PkgRel:     "1",
		Arch:       []string{"x86_64"},
		PkgDesc:    "Test package description",
		Maintainer: "test@example.com",
	}
}

// stagedBuilder returns a builder whose package dir holds a small tree.
func stagedBuilder(t *testing.T, pkgBuild *pkgbuild.PKGBUILD) (builder *Apk, tempDir string) {
	t.Helper()

	tempDir = t.TempDir()
	pkgDir := filepath.Join(tempDir, "pkg")

	require.NoError(t, os.MkdirAll(filepath.Join(pkgDir, "usr", "bin"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(pkgDir, "usr", "bin", "tool"), []byte("#!/bin/sh\n"), 0o755))
	require.NoError(t, os.Symlink("tool", filepath.Join(pkgDir, "usr", "bin", "alias")))

	builder = NewBuilder(pkgBuild)
	builder.PKGBUILD.PackageDir = pkgDir
	builder.PKGBUILD.ArchComputed = "x86_64"

	return builder, tempDir
}

// readSegments decompresses the gzip members of an .apk into one tar stream
// per member.
func readSegments(t *testing.T, data []byte) [][]byte {
	t.Helper()

	br := bytes.NewReader(data)

	var segments [][]byte

	for br.Len() > 0 {
		gz, err := gzip.NewReader(br)
		require.NoError(t, err)

		gz.Multistream(false)

		seg, err := io.ReadAll(gz)
		require.NoError(t, err)

		segments = append(segments, seg)
	}

	return segments
}

// tarEntries lists the entry headers and bodies of a (possibly unterminated) tar.
func tarEntries(t *testing.T, seg []byte) map[string]string {
	t.Helper()

	entries := map[string]string{}
	tr := tar.NewReader(bytes.NewReader(seg))

	for {
		hdr, err := tr.Next()
		if err != nil {
			break
		}

		body, err := io.ReadAll(tr)
		require.NoError(t, err)

		entries[hdr.Name] = string(body)
	}

	return entries
}

func TestNewBuilder(t *testing.T) {
	builder := NewBuilder(createTestPKGBUILD())

	require.NotNil(t, builder.BaseBuilder)
}

func TestBuildPackage(t *testing.T) {
	pkgBuild := createTestPKGBUILD()
	pkgBuild.Depends = []string{"musl", "busybox>=1.36"}
	pkgBuild.Provides = []string{"cmd:tool"}
	pkgBuild.Replaces = []string{"old-tool"}
	pkgBuild.License = []string{"MIT", "Apache-2.0"}
	pkgBuild.URL = "https://example.com"
	builder, tempDir := stagedBuilder(t, pkgBuild)

	require.NoError(t, builder.PrepareFakeroot(context.Background(), tempDir, ""))

	pkgPath, err := builder.BuildPackage(context.Background(), tempDir, "")
	require.NoError(t, err)

	// APK format is: name-version-release.arch.apk
	assert.Equal(t, filepath.Join(tempDir, "test-package-1.0.0-1.x86_64.apk"), pkgPath)

	data, err := os.ReadFile(pkgPath)
	require.NoError(t, err)

	pkg, err := pkgapk.ParsePackage(bytes.NewReader(data), int64(len(data)))
	require.NoError(t, err, "the library must accept what the builder wrote")

	assert.Equal(t, "test-package", pkg.Name)
	assert.Equal(t, "1.0.0-r1", pkg.Version)
	assert.Equal(t, "x86_64", pkg.Arch)
	assert.Equal(t, "Test package description", pkg.Description)
	assert.Equal(t, "https://example.com", pkg.URL)
	assert.Equal(t, "MIT Apache-2.0", pkg.License, "licenses are one space-joined line")
	assert.Equal(t, "test@example.com", pkg.Packager)
	assert.Equal(t, "test-package", pkg.Origin)
	assert.Equal(t, []string{"musl", "busybox>=1.36"}, pkg.Depends)
	assert.Equal(t, []string{"cmd:tool"}, pkg.Provides)
	assert.Equal(t, []string{"old-tool"}, pkg.Replaces)
	assert.Positive(t, pkg.BuildTime)
	assert.False(t, pkg.Signed)

	segments := readSegments(t, data)
	require.Len(t, segments, 2, "control and data segments")

	assert.Contains(t, tarEntries(t, segments[0]), ".PKGINFO")

	files := tarEntries(t, segments[1])
	assert.Equal(t, "#!/bin/sh\n", files["usr/bin/tool"])
	assert.Contains(t, files, "usr/", "parent directories are entries of their own")
	assert.Contains(t, files, "usr/bin/")
	assert.Contains(t, files, "usr/bin/alias")
}

func TestBuildPackageIgnoresEpoch(t *testing.T) {
	pkgBuild := createTestPKGBUILD()
	pkgBuild.Epoch = "2"
	builder, tempDir := stagedBuilder(t, pkgBuild)

	require.NoError(t, builder.PrepareFakeroot(context.Background(), tempDir, ""))

	pkgPath, err := builder.BuildPackage(context.Background(), tempDir, "")
	require.NoError(t, err, "Alpine versions have no epoch: it is dropped, not an error")

	data, err := os.ReadFile(pkgPath)
	require.NoError(t, err)

	pkg, err := pkgapk.ParsePackage(bytes.NewReader(data), int64(len(data)))
	require.NoError(t, err)
	assert.Equal(t, "1.0.0-r1", pkg.Version)
}

func TestBuildPackageScripts(t *testing.T) {
	pkgBuild := createTestPKGBUILD()
	pkgBuild.PreInst = "echo 'pre-install'"
	pkgBuild.PostInst = "echo 'post-install'"
	pkgBuild.PreRm = "echo pre-rm"
	pkgBuild.PostRm = "echo post-rm"
	builder, tempDir := stagedBuilder(t, pkgBuild)

	require.NoError(t, builder.PrepareFakeroot(context.Background(), tempDir, ""))

	pkgPath, err := builder.BuildPackage(context.Background(), tempDir, "")
	require.NoError(t, err)

	data, err := os.ReadFile(pkgPath)
	require.NoError(t, err)

	control := tarEntries(t, readSegments(t, data)[0])

	// apk-tools only honours separate .pre-*/.post-* members; a single
	// .install file is never executed. The install hooks double as upgrade hooks.
	for _, name := range []string{
		".pre-install", ".pre-upgrade", ".post-install", ".post-upgrade", ".pre-deinstall", ".post-deinstall",
	} {
		assert.Contains(t, control, name)
		assert.Contains(t, control[name], "#!/bin/sh\n")
	}

	assert.Contains(t, control[".pre-install"], "pre-install")
	assert.Equal(t, control[".pre-install"], control[".pre-upgrade"])
	assert.Equal(t, control[".post-install"], control[".post-upgrade"])
	assert.NotContains(t, control, ".install", "legacy .install file must not be created")
}

func TestBuildPackageWithoutScriptsHasNone(t *testing.T) {
	builder, tempDir := stagedBuilder(t, createTestPKGBUILD())

	require.NoError(t, builder.PrepareFakeroot(context.Background(), tempDir, ""))

	pkgPath, err := builder.BuildPackage(context.Background(), tempDir, "")
	require.NoError(t, err)

	data, err := os.ReadFile(pkgPath)
	require.NoError(t, err)

	control := tarEntries(t, readSegments(t, data)[0])
	assert.Len(t, control, 1, "only .PKGINFO")
}

func TestBuildPackageErrors(t *testing.T) {
	builder, tempDir := stagedBuilder(t, createTestPKGBUILD())

	t.Run("missing package dir", func(t *testing.T) {
		missing := NewBuilder(createTestPKGBUILD())
		missing.PKGBUILD.PackageDir = "/nonexistent/pkg/dir"
		missing.PKGBUILD.ArchComputed = "x86_64"

		_, err := missing.BuildPackage(context.Background(), tempDir, "")
		require.Error(t, err)
	})

	t.Run("cancelled context", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		_, err := builder.BuildPackage(ctx, tempDir, "")
		require.ErrorIs(t, err, context.Canceled)
	})

	t.Run("unwritable output dir", func(t *testing.T) {
		_, err := builder.BuildPackage(context.Background(), filepath.Join(tempDir, "absent"), "")
		require.Error(t, err)
	})

	t.Run("invalid spec leaves no file", func(t *testing.T) {
		bad := createTestPKGBUILD()
		bad.PkgName = "Bad Name!"
		invalid, dir := stagedBuilder(t, bad)

		_, err := invalid.BuildPackage(context.Background(), dir, "")
		require.ErrorIs(t, err, pkgapk.ErrInvalidSpec)

		matches, globErr := filepath.Glob(filepath.Join(dir, "*.apk"))
		require.NoError(t, globErr)
		assert.Empty(t, matches)
	})
}

func TestPrepareFakeroot(t *testing.T) {
	builder, tempDir := stagedBuilder(t, createTestPKGBUILD())

	require.NoError(t, builder.PrepareFakeroot(context.Background(), tempDir, ""))

	assert.NotZero(t, builder.PKGBUILD.BuildDate, "BuildDate was not set")
	assert.Positive(t, builder.PKGBUILD.InstalledSize)
	assert.Equal(t, "test-package", builder.PKGBUILD.Origin)
	assert.Equal(t, tempDir, builder.PKGBUILD.PkgDest)

	// The staged tree is the package payload: nothing is written into it.
	_, err := os.Stat(filepath.Join(builder.PKGBUILD.PackageDir, ".PKGINFO"))
	assert.True(t, os.IsNotExist(err), ".PKGINFO must not be staged into the package tree")
}

// TestPrepareFakerootGetDirSizeError exercises the GetDirSize error path.
func TestPrepareFakerootGetDirSizeError(t *testing.T) {
	builder := NewBuilder(createTestPKGBUILD())
	builder.PKGBUILD.PackageDir = "/nonexistent/pkg/dir"

	require.Error(t, builder.PrepareFakeroot(context.Background(), t.TempDir(), ""))
}

func TestPrepare(t *testing.T) {
	// Skip if not running as root and no sudo available (would prompt for password)
	if os.Geteuid() != 0 && os.Getenv("SUDO_USER") == "" && os.Getenv("CI") == "" {
		t.Skip("Skipping Prepare test - requires sudo privileges or CI environment")
	}

	pkgBuild := createTestPKGBUILD()
	builder := NewBuilder(pkgBuild)

	makeDepends := []string{"make", "gcc"}
	err := builder.Prepare(context.Background(), makeDepends, "")
	// This will likely fail since apk isn't installed, but we test the method call
	if err == nil {
		t.Log("Prepare succeeded (unexpected in test environment)")
	}
}

func TestPrepareEnvironment(t *testing.T) {
	// Skip if not running as root and no sudo available (would prompt for password)
	if os.Geteuid() != 0 && os.Getenv("SUDO_USER") == "" && os.Getenv("CI") == "" {
		t.Skip("Skipping PrepareEnvironment test - requires sudo privileges or CI environment")
	}

	pkgBuild := createTestPKGBUILD()
	builder := NewBuilder(pkgBuild)

	err := builder.PrepareEnvironment(context.Background(), false, "")
	// This will likely fail since apk isn't installed, but we test the method call
	if err == nil {
		t.Log("PrepareEnvironment succeeded (unexpected in test environment)")
	}
}

func TestPrepareEnvironmentWithGolang(t *testing.T) {
	// Skip if not running as root and no sudo available (would prompt for password)
	if os.Geteuid() != 0 && os.Getenv("SUDO_USER") == "" && os.Getenv("CI") == "" {
		t.Skip("Skipping PrepareEnvironmentWithGolang test - requires sudo privileges or CI environment")
	}

	pkgBuild := createTestPKGBUILD()
	builder := NewBuilder(pkgBuild)

	err := builder.PrepareEnvironment(context.Background(), true, "")
	// This will likely fail since apk isn't installed, but we test the method call
	if err == nil {
		t.Log("PrepareEnvironment with golang succeeded (unexpected in test environment)")
	}
}

func TestUpdate(t *testing.T) {
	// Skip if not running as root and no sudo available (would prompt for password)
	if os.Geteuid() != 0 && os.Getenv("SUDO_USER") == "" && os.Getenv("CI") == "" {
		t.Skip("Skipping Update test - requires sudo privileges or CI environment")
	}

	// Check if apk is available
	if _, err := exec.LookPath("apk"); err != nil {
		t.Skip("Skipping Update test - apk not found")
	}

	pkgBuild := createTestPKGBUILD()
	builder := NewBuilder(pkgBuild)

	err := builder.Update(context.Background())
	// This will likely fail since apk isn't installed, but we test the method call
	if err == nil {
		t.Log("Update succeeded (unexpected in test environment)")
	}
}

func TestPrepareFakerootAppliesOptions(t *testing.T) {
	pkgBuild := createTestPKGBUILD()
	pkgBuild.DocsEnabled = false
	pkgBuild.LibtoolEnabled = false
	pkgBuild.EmptyDirsEnabled = true
	pkgBuild.StaticEnabled = true
	builder := NewBuilder(pkgBuild)

	tempDir := t.TempDir()
	pkgDir := filepath.Join(tempDir, "pkg")
	docDir := filepath.Join(pkgDir, "usr", "share", "doc", "test-package")
	libDir := filepath.Join(pkgDir, "usr", "lib")

	if err := os.MkdirAll(docDir, 0o755); err != nil {
		t.Fatal(err)
	}

	if err := os.MkdirAll(libDir, 0o755); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(filepath.Join(docDir, "README"), []byte("docs"), 0o644); err != nil {
		t.Fatal(err)
	}

	laFile := filepath.Join(libDir, "libfoo.la")
	if err := os.WriteFile(laFile, []byte("la"), 0o644); err != nil {
		t.Fatal(err)
	}

	builder.PKGBUILD.PackageDir = pkgDir

	if err := builder.PrepareFakeroot(context.Background(), tempDir, ""); err != nil {
		t.Fatalf("PrepareFakeroot failed: %v", err)
	}

	if _, err := os.Stat(filepath.Join(docDir, "README")); !os.IsNotExist(err) {
		t.Error("docs should be removed when !docs is set")
	}

	if _, err := os.Stat(laFile); !os.IsNotExist(err) {
		t.Error(".la file should be removed when !libtool is set")
	}
}
