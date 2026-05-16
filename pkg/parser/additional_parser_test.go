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
	pkgBuild, err := parser.ParseFile("ubuntu", "focal", tempDir, tempDir, "")
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

	// Verify functions contain raw (unexpanded) variable references.
	// Variables are resolved at runtime via the script preamble and environment,
	// not pre-expanded at parse time.
	if !strings.Contains(pkgBuild.Build, "PREFIX=") {
		t.Errorf("Build function should reference PREFIX variable, got: %s", pkgBuild.Build)
	}

	if !strings.Contains(pkgBuild.Package, "DESTDIR=") {
		t.Errorf("Package function should reference DESTDIR variable, got: %s", pkgBuild.Package)
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
	pkgBuild, err := parser.ParseFile("ubuntu", "", tempDir, tempDir, "")
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
	_, err := parser.ParseFile("ubuntu", "", tempDir, tempDir, "")
	if err == nil {
		t.Error("ParseFile() should return error for non-existent PKGBUILD")
	}

	// Test invalid home directory
	_, err = parser.ParseFile("ubuntu", "", tempDir, "/non/existent/path", "")
	if err == nil {
		t.Error("ParseFile() should return error for invalid home directory")
	}
}

func TestParseFileWithUpgradeScriptlets(t *testing.T) {
	// Create a temporary directory with a PKGBUILD file containing upgrade scriptlets
	tempDir := t.TempDir()

	pkgbuildContent := `# Test PKGBUILD with upgrade scriptlets
pkgname="test-upgrade"
pkgver="1.0.0"
pkgrel="1"
pkgdesc="A test package with upgrade scriptlets"
arch=("x86_64")
url="https://example.com"
license=("MIT")

preinst() {
    echo "Running pre-install"
}

postinst() {
    echo "Running post-install"
}

pre_upgrade() {
    echo "Running pre-upgrade"
}

post_upgrade() {
    echo "Running post-upgrade"
}

prerm() {
    echo "Running pre-remove"
}

postrm() {
    echo "Running post-remove"
}

package() {
    mkdir -p "${pkgdir}/usr/bin"
    echo "test" > "${pkgdir}/usr/bin/test"
}
`

	pkgbuildPath := filepath.Join(tempDir, "PKGBUILD")

	err := os.WriteFile(pkgbuildPath, []byte(pkgbuildContent), 0o600)
	if err != nil {
		t.Fatalf("Failed to write PKGBUILD: %v", err)
	}

	// Test parsing
	pkgBuild, err := parser.ParseFile("arch", "", tempDir, tempDir, "")
	if err != nil {
		t.Fatalf("ParseFile() error = %v", err)
	}

	// Verify scriptlet fields
	if pkgBuild.PreInst == "" {
		t.Error("PreInst should not be empty")
	}

	if !strings.Contains(pkgBuild.PreInst, "pre-install") {
		t.Errorf("PreInst should contain 'pre-install', got '%s'", pkgBuild.PreInst)
	}

	if pkgBuild.PostInst == "" {
		t.Error("PostInst should not be empty")
	}

	if !strings.Contains(pkgBuild.PostInst, "post-install") {
		t.Errorf("PostInst should contain 'post-install', got '%s'", pkgBuild.PostInst)
	}

	if pkgBuild.PreUpgrade == "" {
		t.Error("PreUpgrade should not be empty")
	}

	if !strings.Contains(pkgBuild.PreUpgrade, "pre-upgrade") {
		t.Errorf("PreUpgrade should contain 'pre-upgrade', got '%s'", pkgBuild.PreUpgrade)
	}

	if pkgBuild.PostUpgrade == "" {
		t.Error("PostUpgrade should not be empty")
	}

	if !strings.Contains(pkgBuild.PostUpgrade, "post-upgrade") {
		t.Errorf("PostUpgrade should contain 'post-upgrade', got '%s'", pkgBuild.PostUpgrade)
	}

	if pkgBuild.PreRm == "" {
		t.Error("PreRm should not be empty")
	}

	if !strings.Contains(pkgBuild.PreRm, "pre-remove") {
		t.Errorf("PreRm should contain 'pre-remove', got '%s'", pkgBuild.PreRm)
	}

	if pkgBuild.PostRm == "" {
		t.Error("PostRm should not be empty")
	}

	if !strings.Contains(pkgBuild.PostRm, "post-remove") {
		t.Errorf("PostRm should contain 'post-remove', got '%s'", pkgBuild.PostRm)
	}
}

// Test 5: ParseFile with TargetArch source selection
func TestPKGBUILD_ParseFile_TargetArchSourceSelection(t *testing.T) {
	// Create a temp dir with a PKGBUILD containing architecture-specific sources
	tempDir := t.TempDir()

	pkgbuildContent := `pkgname="cross-test"
pkgver="1.0.0"
pkgrel="1"
pkgdesc="Cross-compile test"
arch=("x86_64" "aarch64")
license=("MIT")
source_x86_64=("https://example.com/binary-x86_64")
source_aarch64=("https://example.com/binary-aarch64")
sha256sums_x86_64=("aaaa")
sha256sums_aarch64=("bbbb")

package() {
    echo done
}
`

	pkgbuildPath := filepath.Join(tempDir, "PKGBUILD")

	err := os.WriteFile(pkgbuildPath, []byte(pkgbuildContent), 0o600)
	if err != nil {
		t.Fatalf("Failed to write PKGBUILD: %v", err)
	}

	// Parse with target_arch = "aarch64"
	pkgBuild, err := parser.ParseFile("ubuntu", "focal", tempDir, tempDir, "aarch64")
	if err != nil {
		t.Fatalf("ParseFile() error = %v", err)
	}

	// Verify TargetArch is set
	if pkgBuild.TargetArch != "aarch64" {
		t.Errorf("Expected TargetArch 'aarch64', got '%s'", pkgBuild.TargetArch)
	}

	// Verify aarch64 source is selected
	if len(pkgBuild.SourceURI) != 1 || pkgBuild.SourceURI[0] != "https://example.com/binary-aarch64" {
		t.Errorf("Expected SourceURI ['https://example.com/binary-aarch64'], got %v", pkgBuild.SourceURI)
	}

	// Verify aarch64 checksums are selected
	if len(pkgBuild.HashSums) != 1 || pkgBuild.HashSums[0] != "bbbb" {
		t.Errorf("Expected HashSums ['bbbb'], got %v", pkgBuild.HashSums)
	}
}

// TestParseFile_ArchSplitSourceAppended tests that arch-specific source/checksum
// arrays are appended to base arrays rather than replacing them (makepkg behavior).
func TestParseFile_ArchSplitSourceAppended(t *testing.T) {
	// Create a temp dir with a PKGBUILD containing both base and arch-specific sources
	tempDir := t.TempDir()

	pkgbuildContent := `pkgname="arch-split-test"
pkgver="1.0.0"
pkgrel="1"
pkgdesc="Test arch-split source appending"
arch=("x86_64" "aarch64")
license=("MIT")
source=(
  "local-config.conf"
  "local-service.service"
)
source_aarch64=(
  "https://example.com/binary-aarch64"
)
source_x86_64=(
  "https://example.com/binary-x86_64"
)
sha256sums=(
  "aaaa"
  "bbbb"
)
sha256sums_aarch64=(
  "SKIP"
)
sha256sums_x86_64=(
  "SKIP"
)

package() {
  echo done
}
`

	pkgbuildPath := filepath.Join(tempDir, "PKGBUILD")

	err := os.WriteFile(pkgbuildPath, []byte(pkgbuildContent), 0o600)
	if err != nil {
		t.Fatalf("Failed to write PKGBUILD: %v", err)
	}

	// Create the local source files (needed for local file sources)
	err = os.WriteFile(filepath.Join(tempDir, "local-config.conf"), []byte(""), 0o600)
	if err != nil {
		t.Fatalf("Failed to write local-config.conf: %v", err)
	}

	err = os.WriteFile(filepath.Join(tempDir, "local-service.service"), []byte(""), 0o600)
	if err != nil {
		t.Fatalf("Failed to write local-service.service: %v", err)
	}

	// Parse with target_arch = "aarch64"
	pkgBuild, err := parser.ParseFile("ubuntu", "jammy", tempDir, tempDir, "aarch64")
	if err != nil {
		t.Fatalf("ParseFile() error = %v", err)
	}

	// Verify SourceURI has 3 entries: 2 base + 1 aarch64-specific
	if len(pkgBuild.SourceURI) != 3 {
		t.Errorf("Expected 3 SourceURI entries, got %d: %v", len(pkgBuild.SourceURI), pkgBuild.SourceURI)
	}

	expectedSources := []string{"local-config.conf", "local-service.service", "https://example.com/binary-aarch64"}
	for i, expected := range expectedSources {
		if i >= len(pkgBuild.SourceURI) || pkgBuild.SourceURI[i] != expected {
			t.Errorf("SourceURI[%d]: expected %q, got %q", i, expected, pkgBuild.SourceURI[i])
		}
	}

	// Verify HashSums has 3 entries: 2 base + 1 aarch64-specific
	if len(pkgBuild.HashSums) != 3 {
		t.Errorf("Expected 3 HashSums entries, got %d: %v", len(pkgBuild.HashSums), pkgBuild.HashSums)
	}

	expectedHashes := []string{"aaaa", "bbbb", "SKIP"}
	for i, expected := range expectedHashes {
		if i >= len(pkgBuild.HashSums) || pkgBuild.HashSums[i] != expected {
			t.Errorf("HashSums[%d]: expected %q, got %q", i, expected, pkgBuild.HashSums[i])
		}
	}
}
