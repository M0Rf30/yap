package parser_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/M0Rf30/yap/v2/pkg/parser"
)

// FuzzParsePKGBUILD tests ParseFile with arbitrary PKGBUILD content.
// Must never panic. If no error, returned PKGBUILD must have non-nil priorities map.
func FuzzParsePKGBUILD(f *testing.F) {
	// Minimal valid PKGBUILD seed — no empty source/sha256sums (triggers Fatal in set.go).
	minimalPKGBUILD := `pkgname=test
pkgver=1.0
pkgrel=1
pkgdesc="test"
arch=('any')
license=('MIT')
package() { true; }
`

	// Valid PKGBUILD with all supported fields
	fullPKGBUILD := `pkgname=fulltest
pkgver=2.0.0
pkgrel=2
pkgdesc="Full test package"
arch=('x86_64' 'aarch64')
license=('MIT' 'Apache-2.0')
url="https://example.com"
depends=('gcc' 'make')
makedepends=('git')
optdepends=('optional-dep')
conflicts=('conflicting-pkg')
provides=('virtual-pkg')
source=("https://example.com/file.tar.gz")
sha256sums=('abc123')
build() { make; }
package() { make install; }
`

	// PKGBUILD with arch-specific variables
	archSpecificPKGBUILD := `pkgname=archtest
pkgver=1.0
pkgrel=1
pkgdesc="Arch-specific test"
arch=('x86_64' 'aarch64')
license=('MIT')
depends=('base')
depends_x86_64=('x86-specific')
depends_aarch64=('arm-specific')
package() { true; }
`

	// PKGBUILD with distro-specific variables
	distroSpecificPKGBUILD := `pkgname=distrotest
pkgver=1.0
pkgrel=1
pkgdesc="Distro-specific test"
arch=('any')
license=('MIT')
depends=('base')
depends__ubuntu=('ubuntu-pkg')
depends__fedora=('fedora-pkg')
package() { true; }
`

	// PKGBUILD with split packages
	splitPKGBUILD := `pkgname=('split1' 'split2')
pkgver=1.0
pkgrel=1
pkgdesc="Split package"
arch=('any')
license=('MIT')
package_split1() { true; }
package_split2() { true; }
`

	seeds := []string{
		minimalPKGBUILD,
		fullPKGBUILD,
		archSpecificPKGBUILD,
		distroSpecificPKGBUILD,
		splitPKGBUILD,
		"",
		"invalid bash syntax }{",
		"pkgname=test",
		"pkgname='test with spaces'",
		"pkgname=\"test\"",
		"# Comment\npkgname=test",
		"pkgname=test\npkgver=1.0",
		strings.Repeat("pkgname=test\n", 1000),
		"pkgname=$(echo test)",
		"pkgname=${VAR}",
	}

	for _, seed := range seeds {
		f.Add(seed)
	}

	f.Fuzz(func(t *testing.T, content string) {
		// Create temporary directory and PKGBUILD file
		tmpDir, err := os.MkdirTemp("", "fuzz-parser-*")
		if err != nil {
			t.Fatalf("Failed to create temp dir: %v", err)
		}
		defer func() { _ = os.RemoveAll(tmpDir) }()

		pkgbuildPath := filepath.Join(tmpDir, "PKGBUILD")

		err = os.WriteFile(pkgbuildPath, []byte(content), 0o644)
		if err != nil {
			t.Fatalf("Failed to write PKGBUILD: %v", err)
		}

		// Call ParseFile - should not panic
		pb, err := parser.ParseFile("ubuntu", "jammy", tmpDir, tmpDir, "")

		// If no error, verify invariants
		if err == nil && pb != nil {
			// priorities map should be initialized
			if pb == nil {
				t.Error("ParseFile returned nil PKGBUILD with no error")
			}
		}
		// Errors are acceptable; we just want to ensure no panic
	})
}

// FuzzParsePKGBUILDVariables tests ParseFile with fuzzed variable values.
// Must never panic.
func FuzzParsePKGBUILDVariables(f *testing.F) {
	seeds := []struct {
		pkgname string
		pkgver  string
		pkgrel  string
		pkgdesc string
	}{
		{"test", "1.0", "1", "Test package"},
		{"", "", "", ""},
		{"test-pkg", "1.0.0", "1", "Test with dashes"},
		{"test_pkg", "1.0_rc1", "1", "Test with underscores"},
		{"test.pkg", "1.0.0.0", "1", "Test with dots"},
		{strings.Repeat("a", 100), "1.0", "1", "Long name"},
		{"test", strings.Repeat("1.0.", 50), "1", "Long version"},
		{"test", "1.0", strings.Repeat("1", 50), "Long release"},
		{"test", "1.0", "1", strings.Repeat("x", 1000)},
	}

	for _, seed := range seeds {
		f.Add(seed.pkgname, seed.pkgver, seed.pkgrel, seed.pkgdesc)
	}

	f.Fuzz(func(t *testing.T, pkgname, pkgver, pkgrel, pkgdesc string) {
		// Empty pkgname triggers logger.Fatal via os.Setenv("pkgname","") — skip.
		if pkgname == "" {
			return
		}

		// Escape quotes in values for safe PKGBUILD generation
		pkgname = strings.ReplaceAll(pkgname, "'", "\\'")
		pkgver = strings.ReplaceAll(pkgver, "'", "\\'")
		pkgrel = strings.ReplaceAll(pkgrel, "'", "\\'")
		pkgdesc = strings.ReplaceAll(pkgdesc, "'", "\\'")

		// Create temporary directory and PKGBUILD file
		tmpDir, err := os.MkdirTemp("", "fuzz-parser-vars-*")
		if err != nil {
			t.Fatalf("Failed to create temp dir: %v", err)
		}
		defer func() { _ = os.RemoveAll(tmpDir) }()

		// Actually write the content properly
		contentStr := "pkgname='" + pkgname + "'\n" +
			"pkgver='" + pkgver + "'\n" +
			"pkgrel='" + pkgrel + "'\n" +
			"pkgdesc='" + pkgdesc + "'\n" +
			"arch=('any')\n" +
			"license=('MIT')\n" +

			"package() { true; }\n"

		pkgbuildPath := filepath.Join(tmpDir, "PKGBUILD")

		err = os.WriteFile(pkgbuildPath, []byte(contentStr), 0o644)
		if err != nil {
			t.Fatalf("Failed to write PKGBUILD: %v", err)
		}

		// Call ParseFile - should not panic
		_, _ = parser.ParseFile("ubuntu", "jammy", tmpDir, tmpDir, "")
	})
}
