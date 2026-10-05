package deb

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/m0rf30/ar"
)

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
			tempDir := t.TempDir()
			sourceDir := filepath.Join(tempDir, "source")
			packageDir := filepath.Join(tempDir, "package")
			artifactsDir := filepath.Join(tempDir, "artifacts")

			for _, dir := range []string{sourceDir, filepath.Join(packageDir, "DEBIAN"), artifactsDir} {
				if err := os.MkdirAll(dir, 0o755); err != nil {
					t.Fatalf("mkdir %s: %v", dir, err)
				}
			}

			pkg := NewBuilder(createTestPKGBUILD(), tt.compression)
			pkg.PKGBUILD.SourceDir = sourceDir
			pkg.PKGBUILD.PackageDir = packageDir
			pkg.debDir = filepath.Join(packageDir, "DEBIAN")

			debPath, err := pkg.BuildPackage(context.Background(), artifactsDir, "")
			if err != nil {
				t.Fatalf("BuildPackage failed: %v", err)
			}

			names := arMemberNames(t, debPath)
			want := []string{"debian-binary", tt.wantControl, tt.wantData}

			if len(names) != len(want) {
				t.Fatalf("members = %v, want %v", names, want)
			}

			for i := range want {
				if names[i] != want[i] {
					t.Errorf("member %d = %q, want %q", i, names[i], want[i])
				}
			}
		})
	}
}

func arMemberNames(t *testing.T, debPath string) []string {
	t.Helper()

	f, err := os.Open(filepath.Clean(debPath))
	if err != nil {
		t.Fatalf("open deb: %v", err)
	}

	defer func() { _ = f.Close() }()

	reader, err := ar.NewReader(f)
	if err != nil {
		t.Fatalf("ar.NewReader: %v", err)
	}

	var names []string

	for {
		hdr, err := reader.Next()
		if err != nil {
			break
		}

		names = append(names, strings.TrimSuffix(hdr.Name, "/"))
	}

	return names
}

// TestPrepareFakerootMetadataReflectsOptions verifies that md5sums describes
// the payload after PKGBUILD options have been applied (here: !docs), so it
// does not list files that are removed before packaging.
func TestPrepareFakerootMetadataReflectsOptions(t *testing.T) {
	tempDir := t.TempDir()
	packageDir := filepath.Join(tempDir, "package")

	docFile := filepath.Join(packageDir, "usr", "share", "doc", "foo", "README")
	binFile := filepath.Join(packageDir, "usr", "bin", "foo")

	for path, content := range map[string]string{
		docFile: strings.Repeat("d", 4096),
		binFile: "#!/bin/sh\n",
	} {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}

		if err := os.WriteFile(path, []byte(content), 0o755); err != nil {
			t.Fatalf("write %s: %v", path, err)
		}
	}

	pkgBuild := createTestPKGBUILD()
	pkgBuild.PackageDir = packageDir
	pkgBuild.DocsEnabled = false
	pkgBuild.EmptyDirsEnabled = true
	pkgBuild.LibtoolEnabled = true
	pkgBuild.StaticEnabled = true

	pkg := NewBuilder(pkgBuild, "")

	if err := pkg.PrepareFakeroot(context.Background(), tempDir, ""); err != nil {
		t.Fatalf("PrepareFakeroot failed: %v", err)
	}

	if _, err := os.Stat(docFile); !os.IsNotExist(err) {
		t.Fatalf("doc file should have been removed by !docs, stat err = %v", err)
	}

	md5sums, err := os.ReadFile(filepath.Join(packageDir, "DEBIAN", "md5sums"))
	if err != nil {
		t.Fatalf("read md5sums: %v", err)
	}

	if strings.Contains(string(md5sums), "usr/share/doc/foo/README") {
		t.Errorf("md5sums lists a file removed by options:\n%s", md5sums)
	}

	if !strings.Contains(string(md5sums), "usr/bin/foo") {
		t.Errorf("md5sums misses usr/bin/foo:\n%s", md5sums)
	}

	if pkgBuild.InstalledSize >= 4 {
		t.Errorf("InstalledSize = %d KiB, should not include removed docs", pkgBuild.InstalledSize)
	}
}
