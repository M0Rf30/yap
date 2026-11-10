// SPDX-FileCopyrightText: 2021-2026 Gianluca Boiano
// SPDX-License-Identifier: GPL-3.0-only

package deb

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	pkgdeb "github.com/M0Rf30/pkgformat/deb"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// buildDeb lays out a payload, runs the builder and parses the result.
func buildDeb(
	t *testing.T, compression string, mutate func(*Package, string),
) (parsed *pkgdeb.DebFile, debPath, packageDir string) {
	t.Helper()

	tempDir := t.TempDir()
	sourceDir := filepath.Join(tempDir, "source")
	packageDir = filepath.Join(tempDir, "package")
	artifactsDir := filepath.Join(tempDir, "artifacts")

	for _, dir := range []string{sourceDir, artifactsDir, filepath.Join(packageDir, "usr", "bin")} {
		require.NoError(t, os.MkdirAll(dir, 0o755))
	}

	require.NoError(t, os.WriteFile(filepath.Join(packageDir, "usr", "bin", "foo"), []byte("#!/bin/sh\n"), 0o755)) //nolint:gosec

	pkg := NewBuilder(createTestPKGBUILD(), compression)
	pkg.PKGBUILD.SourceDir = sourceDir
	pkg.PKGBUILD.PackageDir = packageDir
	pkg.PKGBUILD.Home = tempDir
	pkg.PKGBUILD.StartDir = tempDir
	pkg.PKGBUILD.ArchComputed = "amd64"

	if mutate != nil {
		mutate(pkg, tempDir)
	}

	debPath, err := pkg.BuildPackage(context.Background(), artifactsDir, "")
	require.NoError(t, err)

	f, err := os.Open(debPath) //nolint:gosec
	require.NoError(t, err)
	t.Cleanup(func() { _ = f.Close() })

	st, err := f.Stat()
	require.NoError(t, err)

	parsed, err = pkgdeb.ParseDebFileAt(f, st.Size())
	require.NoError(t, err)

	return parsed, debPath, packageDir
}

// arMemberNames lists the member names of an ar archive.
func arMemberNames(t *testing.T, debPath string) []string {
	t.Helper()

	data, err := os.ReadFile(debPath) //nolint:gosec
	require.NoError(t, err)
	require.True(t, bytes.HasPrefix(data, []byte("!<arch>\n")))

	var names []string

	for off := 8; off+60 <= len(data); {
		names = append(names, strings.TrimRight(strings.TrimSpace(string(data[off:off+16])), "/"))

		size, err := strconv.Atoi(strings.TrimSpace(string(data[off+48 : off+58])))
		require.NoError(t, err)

		off += 60 + size + size%2
	}

	return names
}

// TestBuildPackageMemberNames verifies that the ar member names follow the
// selected compression algorithm, since dpkg picks the decompressor from the
// member extension.
func TestBuildPackageMemberNames(t *testing.T) {
	tests := []struct {
		compression string
		wantControl string
		wantData    string
	}{
		{"", "control.tar.zst", "data.tar.zst"},
		{"zstd", "control.tar.zst", "data.tar.zst"},
		{"gzip", "control.tar.gz", "data.tar.gz"},
		{"xz", "control.tar.xz", "data.tar.xz"},
	}

	for _, tt := range tests {
		t.Run("compression="+tt.compression, func(t *testing.T) {
			_, debPath, _ := buildDeb(t, tt.compression, nil)

			assert.Equal(t, []string{"debian-binary", tt.wantControl, tt.wantData}, arMemberNames(t, debPath))
		})
	}
}

// TestBuildPackageUnknownCompression: a compression the writer does not know
// is an error rather than a silent fallback.
func TestBuildPackageUnknownCompression(t *testing.T) {
	pkg := NewBuilder(createTestPKGBUILD(), "lz5")
	pkg.PKGBUILD.ArchComputed = "amd64"
	pkg.PKGBUILD.PackageDir = t.TempDir()
	pkg.PKGBUILD.SourceDir = t.TempDir()

	_, err := pkg.BuildPackage(context.Background(), t.TempDir(), "")

	require.Error(t, err)
	assert.ErrorIs(t, err, pkgdeb.ErrUnsupportedCompression)
}

// TestBuildPackageControlMetadata checks the control file, the conffiles, the
// copyright file and the maintainer scripts the builder hands to the writer.
func TestBuildPackageControlMetadata(t *testing.T) {
	parsed, _, packageDir := buildDeb(t, "gzip", func(pkg *Package, _ string) {
		pkg.PKGBUILD.Depends = pkg.ProcessDependencies(pkg.PKGBUILD.Depends)
		pkg.PKGBUILD.OptDepends = pkg.ProcessDependencies(pkg.PKGBUILD.OptDepends)
		pkg.PKGBUILD.Section = "utils"
		pkg.PKGBUILD.Priority = "optional"
		pkg.PKGBUILD.URL = "https://example.com"
	})

	cf := parsed.ControlFile
	assert.Equal(t, "test-package", cf.Get("Package"))
	assert.Equal(t, "1.0.0-1", cf.Get("Version"))
	assert.Equal(t, "amd64", cf.Get("Architecture"))
	assert.Equal(t, "utils", cf.Get("Section"))
	assert.Equal(t, "https://example.com", cf.Get("Homepage"))
	assert.Equal(t, "dependency1 (>= 1.0), dependency2 (<< 2.0)", cf.Get("Depends"))
	assert.Equal(t, "optional (>= 1.0)", cf.Get("Recommends"))
	assert.Equal(t, "Test package description", cf.Get("Description"))

	assert.Equal(t, []string{"/etc/config.conf", "/etc/other.conf"}, parsed.Conffiles())
	assert.Contains(t, string(parsed.Control["md5sums"]), "usr/bin/foo")
	assert.Contains(t, string(parsed.Control["copyright"]), "License: MIT")

	assert.Contains(t, string(parsed.Control["preinst"]), "pre-install")
	assert.Contains(t, string(parsed.Control["postinst"]), "post-install")

	for _, name := range []string{"prerm", "postrm"} {
		assert.True(t, strings.HasPrefix(string(parsed.Control[name]), pkgdeb.RemoveScriptHeader),
			"%s must start with the removal guard", name)
	}

	assert.NoDirExists(t, packageDir, "the package directory is removed after the build")
}

// TestBuildPackageChangelogAndDebconf: the changelog lands in the data archive
// and the debconf assets are copied from the PKGBUILD directory.
func TestBuildPackageChangelogAndDebconf(t *testing.T) {
	parsed, _, _ := buildDeb(t, "gzip", func(pkg *Package, home string) {
		require.NoError(t, os.WriteFile(filepath.Join(home, "debian.config"), []byte("#!/bin/sh\n. /usr/share/debconf/confmodule\n"), 0o600))
		require.NoError(t, os.WriteFile(filepath.Join(home, "debian.templates"), []byte("Template: foo/bar\nType: string\n"), 0o600))
		require.NoError(t, os.WriteFile(filepath.Join(home, "CHANGELOG"), []byte("1.0.0: first\n"), 0o600))

		pkg.PKGBUILD.DebConfig = "debian.config"
		pkg.PKGBUILD.DebTemplate = "debian.templates"
		pkg.PKGBUILD.Changelog = "CHANGELOG"
	})

	assert.Contains(t, string(parsed.Control["config"]), "confmodule")
	assert.Contains(t, string(parsed.Control["templates"]), "Template: foo/bar")

	dest := t.TempDir()
	require.NoError(t, parsed.Extract(dest, pkgdeb.ExtractOptions{}))
	assert.FileExists(t, filepath.Join(dest, "usr", "share", "doc", "test-package", "changelog.Debian.gz"))
	assert.FileExists(t, filepath.Join(dest, "usr", "bin", "foo"))
}

// TestBuildPackageMissingDebconfAsset: a debconf asset named by the PKGBUILD
// but absent from its directory fails the build.
func TestBuildPackageMissingDebconfAsset(t *testing.T) {
	pkg := NewBuilder(createTestPKGBUILD(), "gzip")
	pkg.PKGBUILD.ArchComputed = "amd64"
	pkg.PKGBUILD.PackageDir = t.TempDir()
	pkg.PKGBUILD.SourceDir = t.TempDir()
	pkg.PKGBUILD.Home = t.TempDir()
	pkg.PKGBUILD.DebConfig = "missing.config"

	_, err := pkg.BuildPackage(context.Background(), t.TempDir(), "")

	assert.Error(t, err)
}

// TestPrepareFakerootMetadataReflectsOptions verifies that the package written
// after PrepareFakeroot describes the payload with the PKGBUILD options
// applied (here: !docs): md5sums does not list files that were removed and
// Installed-Size does not count them.
func TestPrepareFakerootMetadataReflectsOptions(t *testing.T) {
	tempDir := t.TempDir()
	packageDir := filepath.Join(tempDir, "package")

	docFile := filepath.Join(packageDir, "usr", "share", "doc", "foo", "README")
	binFile := filepath.Join(packageDir, "usr", "bin", "foo")

	for path, content := range map[string]string{
		docFile: strings.Repeat("d", 8192),
		binFile: "#!/bin/sh\n",
	} {
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
		require.NoError(t, os.WriteFile(path, []byte(content), 0o755)) //nolint:gosec
	}

	pkgBuild := createTestPKGBUILD()
	pkgBuild.ArchComputed = "amd64"
	pkgBuild.PackageDir = packageDir
	pkgBuild.SourceDir = tempDir
	pkgBuild.DocsEnabled = false
	pkgBuild.EmptyDirsEnabled = true
	pkgBuild.LibtoolEnabled = true
	pkgBuild.StaticEnabled = true

	pkg := NewBuilder(pkgBuild, "gzip")

	require.NoError(t, pkg.PrepareFakeroot(context.Background(), tempDir, ""))

	_, err := os.Stat(docFile)
	require.True(t, os.IsNotExist(err), "doc file should have been removed by !docs")

	artifactsDir := t.TempDir()

	debPath, err := pkg.BuildPackage(context.Background(), artifactsDir, "")
	require.NoError(t, err)

	f, err := os.Open(debPath) //nolint:gosec
	require.NoError(t, err)

	defer func() { _ = f.Close() }()

	st, err := f.Stat()
	require.NoError(t, err)

	parsed, err := pkgdeb.ParseDebFileAt(f, st.Size())
	require.NoError(t, err)

	md5sums := string(parsed.Control["md5sums"])
	assert.NotContains(t, md5sums, "usr/share/doc/foo/README")
	assert.Contains(t, md5sums, "usr/bin/foo")

	size, err := strconv.Atoi(parsed.ControlFile.Get("Installed-Size"))
	require.NoError(t, err)
	assert.Less(t, size, 8, "Installed-Size must not count the removed docs")
}
