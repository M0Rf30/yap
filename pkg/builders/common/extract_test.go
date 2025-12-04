package common

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/M0Rf30/yap/v2/pkg/constants"
	"github.com/M0Rf30/yap/v2/pkg/pkgbuild"
)

// createTestDEB creates a test DEB package with actual content
func createTestDEB(t *testing.T, tmpDir string) string {
	t.Helper()

	// Create package structure
	pkgDir := filepath.Join(tmpDir, "test-pkg")
	debianDir := filepath.Join(pkgDir, "DEBIAN")
	contentDir := filepath.Join(pkgDir, "opt", "test")

	if err := os.MkdirAll(debianDir, 0o755); err != nil {
		t.Fatalf("Failed to create DEBIAN dir: %v", err)
	}

	if err := os.MkdirAll(filepath.Join(contentDir, "lib"), 0o755); err != nil {
		t.Fatalf("Failed to create lib dir: %v", err)
	}

	if err := os.MkdirAll(filepath.Join(contentDir, "include"), 0o755); err != nil {
		t.Fatalf("Failed to create include dir: %v", err)
	}

	// Create control file
	control := `Package: test-package
Version: 1.0.0
Architecture: amd64
Maintainer: Test <test@test.com>
Description: Test package for extraction
`

	if err := os.WriteFile(filepath.Join(debianDir, "control"), []byte(control), 0o644); err != nil {
		t.Fatalf("Failed to write control file: %v", err)
	}

	// Create test files
	libPath := filepath.Join(contentDir, "lib", "libtest.so")
	if err := os.WriteFile(libPath, []byte("test library"), 0o644); err != nil {
		t.Fatalf("Failed to write library file: %v", err)
	}

	includePath := filepath.Join(contentDir, "include", "test.h")
	if err := os.WriteFile(includePath, []byte("test header"), 0o644); err != nil {
		t.Fatalf("Failed to write header file: %v", err)
	}

	// Build DEB package
	debPath := filepath.Join(tmpDir, "test-package_1.0.0_amd64.deb")
	cmd := exec.CommandContext(context.Background(), "dpkg-deb", "--build", pkgDir, debPath)

	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("Failed to build DEB: %v\nOutput: %s", err, output)
	}

	return debPath
}

func TestExtractToStaging(t *testing.T) {
	// Create temporary directory
	tmpDir := t.TempDir()

	// Create test DEB package
	debPath := createTestDEB(t, tmpDir)

	// Create staging directory
	stagingDir := filepath.Join(tmpDir, "staging")

	// Create BaseBuilder
	pkg := &pkgbuild.PKGBUILD{
		PkgName:      "test-package",
		StartDir:     tmpDir,
		ArchComputed: "x86_64",
	}

	bb := &BaseBuilder{
		PKGBUILD: pkg,
		Format:   constants.FormatDEB,
	}

	// Test extraction
	err := bb.ExtractToStaging(debPath, stagingDir)
	if err != nil {
		t.Fatalf("ExtractToStaging failed: %v", err)
	}

	// Verify extracted files
	expectedFiles := []string{
		filepath.Join(stagingDir, "opt", "test", "lib", "libtest.so"),
		filepath.Join(stagingDir, "opt", "test", "include", "test.h"),
	}

	for _, expectedFile := range expectedFiles {
		if _, err := os.Stat(expectedFile); os.IsNotExist(err) {
			t.Errorf("Expected file not found: %s", expectedFile)
		}
	}
}

func TestExtractToStaging_UnsupportedFormat(t *testing.T) {
	tmpDir := t.TempDir()

	pkg := &pkgbuild.PKGBUILD{
		StartDir:     tmpDir,
		ArchComputed: "x86_64",
	}

	bb := &BaseBuilder{
		PKGBUILD: pkg,
		Format:   "unsupported",
	}

	err := bb.ExtractToStaging("/fake/path.deb", tmpDir)
	if err == nil {
		t.Error("Expected error for unsupported format, got nil")
	}
}

func TestGetStagingRoot(t *testing.T) {
	buildDir := "/tmp/test-build"
	expected := filepath.Join(buildDir, "yap-cross-staging")

	result := GetStagingRoot(buildDir)

	if result != expected {
		t.Errorf("GetStagingRoot() = %s, want %s", result, expected)
	}
}

func TestCleanupStaging(t *testing.T) {
	tmpDir := t.TempDir()
	stagingDir := GetStagingRoot(tmpDir)

	// Create staging directory with files
	testDir := filepath.Join(stagingDir, "opt", "test")
	if err := os.MkdirAll(testDir, 0o755); err != nil {
		t.Fatalf("Failed to create staging dir: %v", err)
	}

	testFile := filepath.Join(testDir, "file.txt")
	if err := os.WriteFile(testFile, []byte("test"), 0o644); err != nil {
		t.Fatalf("Failed to write test file: %v", err)
	}

	// Cleanup
	err := CleanupStaging(tmpDir)
	if err != nil {
		t.Fatalf("CleanupStaging failed: %v", err)
	}

	// Verify staging directory is removed
	if _, err := os.Stat(stagingDir); !os.IsNotExist(err) {
		t.Error("Staging directory should be removed")
	}

	// Cleanup again should not error
	err = CleanupStaging(tmpDir)
	if err != nil {
		t.Errorf("CleanupStaging on non-existent dir should not error: %v", err)
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

	stagingDir := filepath.Join(tmpDir, "staging")

	err := extractDEB(invalidDEB, stagingDir)
	if err == nil {
		t.Error("Expected error for invalid DEB, got nil")
	}
}
