package parser_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/M0Rf30/yap/v2/pkg/parser"
)

func parseContent(t *testing.T, content string) error {
	t.Helper()

	dir := t.TempDir()

	err := os.WriteFile(filepath.Join(dir, "PKGBUILD"), []byte(content), 0o600)
	if err != nil {
		t.Fatalf("write PKGBUILD: %v", err)
	}

	_, err = parser.ParseFile("ubuntu", "focal", dir, dir, "")

	return err
}

// An error from an early assignment must not be overwritten by later,
// successful assignments.
func TestParseFile_EarlyAssignmentErrorSurvives(t *testing.T) {
	content := `pkgname="demo"
pkgdesc__a__b="invalid directive"
pkgver="1.0.0"
pkgrel="1"
arch=("any")
license=("MIT")
`

	if err := parseContent(t, content); err == nil {
		t.Fatal("ParseFile() expected error for invalid directive, got nil")
	}
}

// Expansion failures (e.g. command substitution) must be reported instead of
// silently producing an empty value.
func TestParseFile_ExpansionErrorPropagates(t *testing.T) {
	scalar := `pkgname="demo"
pkgver=$(date +%s)
pkgrel="1"
pkgdesc="demo"
arch=("any")
license=("MIT")
`

	if err := parseContent(t, scalar); err == nil {
		t.Error("ParseFile() expected error for command substitution in scalar")
	}

	array := `pkgname="demo"
pkgver="1.0.0"
pkgrel="1"
pkgdesc="demo"
arch=("any")
license=("MIT")
source=("$(echo x)")
`

	if err := parseContent(t, array); err == nil {
		t.Error("ParseFile() expected error for command substitution in array")
	}
}
