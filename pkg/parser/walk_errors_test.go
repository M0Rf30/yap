// SPDX-FileCopyrightText: 2021-2026 Gianluca Boiano
// SPDX-License-Identifier: GPL-3.0-only

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

// Expansion failures (e.g. command substitution) are non-fatal: they are
// logged as warnings and parsing continues, preserving historical behavior.
func TestParseFile_ExpansionErrorIsNonFatal(t *testing.T) {
	array := `pkgname="demo"
pkgver="1.0.0"
pkgrel="1"
pkgdesc="demo"
arch=("any")
license=("MIT")
_helper=$(date +%s)
source=("$(echo x)")
`

	if err := parseContent(t, array); err != nil {
		t.Errorf("ParseFile() unexpected error for command substitution: %v", err)
	}
}
