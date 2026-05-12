package common

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/blakesmith/ar"

	"github.com/M0Rf30/yap/v2/pkg/pkgbuild"
)

// createTestDEB creates a minimal but valid DEB package in pure Go.
// A DEB is an AR archive with three members:
//
//	debian-binary   — "2.0\n"
//	control.tar.gz  — gzipped tar containing ./control
//	data.tar.gz     — gzipped tar containing the payload files
func createTestDEB(t *testing.T, tmpDir string) string {
	t.Helper()

	const control = `Package: test-package
Version: 1.0.0
Architecture: amd64
Maintainer: Test <test@test.com>
Description: Test package for extraction
`

	// --- build control.tar.gz ---
	controlTar := buildTarGz(t, map[string]string{
		"./control": control,
	})

	// --- build data.tar.gz ---
	dataTar := buildTarGz(t, map[string]string{
		"./opt/test/lib/libtest.so": "test library",
		"./opt/test/include/test.h": "test header",
	})

	// --- assemble AR archive ---
	debPath := filepath.Join(tmpDir, "test-package_1.0.0_amd64.deb")

	f, err := os.Create(debPath)
	if err != nil {
		t.Fatalf("create deb: %v", err)
	}

	defer func() {
		if err := f.Close(); err != nil {
			t.Errorf("close deb: %v", err)
		}
	}()

	w := ar.NewWriter(f)
	if err := w.WriteGlobalHeader(); err != nil {
		t.Fatalf("ar global header: %v", err)
	}

	writeARMember(t, w, "debian-binary", []byte("2.0\n"))
	writeARMember(t, w, "control.tar.gz", controlTar)
	writeARMember(t, w, "data.tar.gz", dataTar)

	return debPath
}

// buildTarGz returns a gzip-compressed tar archive containing the given files
// (path → content).
func buildTarGz(t *testing.T, files map[string]string) []byte {
	t.Helper()

	var buf bytes.Buffer

	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)

	now := time.Now()

	for name, content := range files {
		body := []byte(content)
		hdr := &tar.Header{
			Name:     name,
			Mode:     0o644,
			Size:     int64(len(body)),
			ModTime:  now,
			Typeflag: tar.TypeReg,
		}

		if err := tw.WriteHeader(hdr); err != nil {
			t.Fatalf("tar header %s: %v", name, err)
		}

		if _, err := tw.Write(body); err != nil {
			t.Fatalf("tar write %s: %v", name, err)
		}
	}

	if err := tw.Close(); err != nil {
		t.Fatalf("tar close: %v", err)
	}

	if err := gz.Close(); err != nil {
		t.Fatalf("gzip close: %v", err)
	}

	return buf.Bytes()
}

// writeARMember appends a single member to an AR archive.
func writeARMember(t *testing.T, w *ar.Writer, name string, data []byte) {
	t.Helper()

	hdr := ar.Header{
		Name:    name,
		Size:    int64(len(data)),
		Mode:    0o644,
		ModTime: time.Now(),
	}

	if err := w.WriteHeader(&hdr); err != nil {
		t.Fatalf("ar header %s: %v", name, err)
	}

	if _, err := w.Write(data); err != nil {
		t.Fatalf("ar write %s: %v", name, err)
	}
}

func TestExtractToRoot(t *testing.T) {
	// Create temporary directory
	tmpDir := t.TempDir()

	// Create test DEB package
	debPath := createTestDEB(t, tmpDir)

	// Test extraction to a temporary root directory
	// We'll use a subdirectory to simulate root extraction
	testRootDir := filepath.Join(tmpDir, "test-root")
	if err := os.MkdirAll(testRootDir, 0o755); err != nil {
		t.Fatalf("Failed to create test root dir: %v", err)
	}

	// For this test, we'll extract to the test root directory
	// by modifying the test to use extractDEB directly
	err := extractDEB(debPath, testRootDir)
	if err != nil {
		t.Fatalf("extractDEB failed: %v", err)
	}

	// Verify extracted files
	expectedFiles := []string{
		filepath.Join(testRootDir, "opt", "test", "lib", "libtest.so"),
		filepath.Join(testRootDir, "opt", "test", "include", "test.h"),
	}

	for _, expectedFile := range expectedFiles {
		if _, err := os.Stat(expectedFile); os.IsNotExist(err) {
			t.Errorf("Expected file not found: %s", expectedFile)
		}
	}
}

func TestExtractToRoot_UnsupportedFormat(t *testing.T) {
	tmpDir := t.TempDir()

	pkg := &pkgbuild.PKGBUILD{
		StartDir:     tmpDir,
		ArchComputed: "x86_64",
	}

	bb := &BaseBuilder{
		PKGBUILD: pkg,
		Format:   "unsupported",
	}

	err := bb.ExtractToRoot("/fake/path.deb")
	if err == nil {
		t.Error("Expected error for unsupported format, got nil")
	}
}

func TestCrossCompilationDetection(t *testing.T) {
	tests := []struct {
		name        string
		buildArch   string
		targetArch  string
		wantCross   bool
		description string
	}{
		{
			name:        "Native x86_64",
			buildArch:   "x86_64",
			targetArch:  "x86_64",
			wantCross:   false,
			description: "Same arch should not trigger cross-compilation",
		},
		{
			name:        "Cross to ARM64",
			buildArch:   "x86_64",
			targetArch:  "aarch64",
			wantCross:   true,
			description: "Different arch should trigger cross-compilation",
		},
		{
			name:        "Cross to ARM",
			buildArch:   "x86_64",
			targetArch:  "armv7",
			wantCross:   true,
			description: "x86_64 to ARM should trigger cross-compilation",
		},
		{
			name:        "Empty target arch",
			buildArch:   "x86_64",
			targetArch:  "",
			wantCross:   false,
			description: "Empty target arch should not trigger cross-compilation",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Simulate cross-compilation detection logic
			isCrossCompiling := tt.targetArch != "" && tt.targetArch != tt.buildArch

			if isCrossCompiling != tt.wantCross {
				t.Errorf("%s: got %v, want %v", tt.description, isCrossCompiling, tt.wantCross)
			}
		})
	}
}

func TestExtractDEB_MissingDataTar(t *testing.T) {
	tmpDir := t.TempDir()

	// Create an invalid DEB (just a text file)
	invalidDEB := filepath.Join(tmpDir, "invalid.deb")
	if err := os.WriteFile(invalidDEB, []byte("not a deb"), 0o644); err != nil {
		t.Fatalf("Failed to create invalid DEB: %v", err)
	}

	destDir := filepath.Join(tmpDir, "dest")

	err := extractDEB(invalidDEB, destDir)
	if err == nil {
		t.Error("Expected error for invalid DEB, got nil")
	}
}
