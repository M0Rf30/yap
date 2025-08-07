package parser_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/M0Rf30/yap/v2/pkg/parser"
)

func TestOverrideVariables(t *testing.T) {
	// Test override functionality
	originalPkgRel := parser.OverridePkgRel
	originalPkgVer := parser.OverridePkgVer

	defer func() {
		parser.OverridePkgRel = originalPkgRel
		parser.OverridePkgVer = originalPkgVer
	}()

	// Test that overrides can be set
	parser.OverridePkgRel = "2"
	parser.OverridePkgVer = "2.0.0"

	if parser.OverridePkgRel != "2" {
		t.Errorf("Expected OverridePkgRel '2', got '%s'", parser.OverridePkgRel)
	}

	if parser.OverridePkgVer != "2.0.0" {
		t.Errorf("Expected OverridePkgVer '2.0.0', got '%s'", parser.OverridePkgVer)
	}
}

func TestParseFileIntegration(t *testing.T) {
	// Create a temporary directory with a PKGBUILD file
	tempDir := t.TempDir()

	pkgbuildContent := `# Test PKGBUILD
pkgname="test-package"
pkgver="1.0.0"
pkgrel="1"
pkgdesc="A test package for parsing"
arch=("x86_64" "any")
url="https://example.com"
license=("MIT")
depends=("glibc" "gcc")
makedepends=("make" "cmake")
source=("https://example.com/source.tar.gz")
sha256sums=("abcd1234efgh5678")

# Custom variables
prefix="/usr"
bindir="${prefix}/bin"
datadir="${prefix}/share"

build() {
    make PREFIX="${prefix}"
}

package() {
    make install PREFIX="${prefix}" DESTDIR="${pkgdir}"
    install -Dm644 config.conf "${pkgdir}${datadir}/${pkgname}/"
}
`

	pkgbuildPath := filepath.Join(tempDir, "PKGBUILD")

	err := os.WriteFile(pkgbuildPath, []byte(pkgbuildContent), 0o600)
	if err != nil {
		t.Fatalf("Failed to write PKGBUILD: %v", err)
	}

	// Test parsing
	pkgBuild, err := parser.ParseFile("ubuntu", "focal", tempDir, tempDir)
	if err != nil {
		t.Fatalf("ParseFile() error = %v", err)
	}

	// Verify basic fields
	if pkgBuild.PkgName != "test-package" {
		t.Errorf("Expected PkgName 'test-package', got '%s'", pkgBuild.PkgName)
	}

	if pkgBuild.PkgVer != "1.0.0" {
		t.Errorf("Expected PkgVer '1.0.0', got '%s'", pkgBuild.PkgVer)
	}

	if pkgBuild.PkgRel != "1" {
		t.Errorf("Expected PkgRel '1', got '%s'", pkgBuild.PkgRel)
	}

	if pkgBuild.PkgDesc != "A test package for parsing" {
		t.Errorf("Expected PkgDesc 'A test package for parsing', got '%s'", pkgBuild.PkgDesc)
	}

	// Verify arrays
	if len(pkgBuild.Arch) != 2 || pkgBuild.Arch[0] != "x86_64" || pkgBuild.Arch[1] != "any" {
		t.Errorf("Expected Arch ['x86_64', 'any'], got %v", pkgBuild.Arch)
	}

	if len(pkgBuild.Depends) != 2 || pkgBuild.Depends[0] != "glibc" || pkgBuild.Depends[1] != "gcc" {
		t.Errorf("Expected Depends ['glibc', 'gcc'], got %v", pkgBuild.Depends)
	}

	if len(pkgBuild.License) != 1 || pkgBuild.License[0] != "MIT" {
		t.Errorf("Expected License ['MIT'], got %v", pkgBuild.License)
	}

	// Verify functions contain expanded variables
	if !strings.Contains(pkgBuild.Build, "PREFIX=\"/usr\"") {
		t.Errorf("Build function should contain expanded prefix variable, got: %s", pkgBuild.Build)
	}

	if !strings.Contains(pkgBuild.Package, "DESTDIR=\"") {
		t.Errorf("Package function should contain expanded pkgdir, got: %s", pkgBuild.Package)
	}

	// Verify source directory setup
	expectedSourceDir := filepath.Join(tempDir, "src")
	if pkgBuild.SourceDir != expectedSourceDir {
		t.Errorf("Expected SourceDir '%s', got '%s'", expectedSourceDir, pkgBuild.SourceDir)
	}
}

func TestParseFileWithOverrides(t *testing.T) {
	// Create a temporary directory with a PKGBUILD file
	tempDir := t.TempDir()

	pkgbuildContent := `pkgname="test-override"
pkgver="1.0.0"
pkgrel="1"
pkgdesc="Test override functionality"
arch=("any")
license=("MIT")
source=("source.tar.gz")
sha256sums=("12345")

package() {
    echo "Building package"
}
`

	pkgbuildPath := filepath.Join(tempDir, "PKGBUILD")

	err := os.WriteFile(pkgbuildPath, []byte(pkgbuildContent), 0o600)
	if err != nil {
		t.Fatalf("Failed to write PKGBUILD: %v", err)
	}

	// Set overrides
	originalPkgRel := parser.OverridePkgRel
	originalPkgVer := parser.OverridePkgVer

	defer func() {
		parser.OverridePkgRel = originalPkgRel
		parser.OverridePkgVer = originalPkgVer
	}()

	parser.OverridePkgRel = "2"
	parser.OverridePkgVer = "2.0.0"

	// Test parsing with overrides
	pkgBuild, err := parser.ParseFile("ubuntu", "", tempDir, tempDir)
	if err != nil {
		t.Fatalf("ParseFile() error = %v", err)
	}

	// Verify overrides were applied
	if pkgBuild.PkgVer != "2.0.0" {
		t.Errorf("Expected overridden PkgVer '2.0.0', got '%s'", pkgBuild.PkgVer)
	}

	if pkgBuild.PkgRel != "2" {
		t.Errorf("Expected overridden PkgRel '2', got '%s'", pkgBuild.PkgRel)
	}

	// Verify other fields remain unchanged
	if pkgBuild.PkgName != "test-override" {
		t.Errorf("Expected PkgName 'test-override', got '%s'", pkgBuild.PkgName)
	}
}

func TestParseFileErrors(t *testing.T) {
	tempDir := t.TempDir()

	// Test non-existent PKGBUILD
	_, err := parser.ParseFile("ubuntu", "", tempDir, tempDir)
	if err == nil {
		t.Error("ParseFile() should return error for non-existent PKGBUILD")
	}

	// Test invalid home directory
	_, err = parser.ParseFile("ubuntu", "", tempDir, "/non/existent/path")
	if err == nil {
		t.Error("ParseFile() should return error for invalid home directory")
	}
}
