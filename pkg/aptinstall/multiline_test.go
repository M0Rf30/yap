package aptinstall_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/M0Rf30/yap/v2/pkg/aptinstall"
)

// TestMultilineFieldRoundTrip is the regression test for the
// "field name 'These' must be followed by colon" failure observed when
// installing python3-minimal on Ubuntu Noble.
//
// The real Ubuntu /var/lib/dpkg/status carries a multi-line Description
// for apt that looks like:
//
//	Description: commandline package manager
//	 This package provides commandline tools...
//	 .
//	 These include:
//	  * apt-get for retrieval of packages
//
// The parser stores this in-memory as "synopsis\nline1\n.\nThese include:\n..."
// (leading space stripped on each continuation line). On re-emit, every
// embedded newline MUST be followed by a leading space — without it,
// dpkg parses each continuation as a new field header, sees "These" with
// no colon, and refuses the entire status file. After that point every
// other postinst that shells out to dpkg-query fails (e.g. py3compile in
// python3-minimal's postinst).
func TestMultilineFieldRoundTrip(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "out")

	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}

	value := "commandline package manager\n" +
		"This package provides commandline tools.\n" +
		".\n" +
		"These include:\n" +
		" * apt-get"

	if err := aptinstall.WriteDeb822FieldForTesting(f, "Description", value); err != nil {
		t.Fatal(err)
	}

	if err := f.Close(); err != nil {
		t.Fatal(err)
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	want := "Description: commandline package manager\n" +
		" This package provides commandline tools.\n" +
		" .\n" +
		" These include:\n" +
		"  * apt-get\n"

	if string(got) != want {
		t.Fatalf("multi-line emit wrong:\n--- want ---\n%s--- got ---\n%s", want, got)
	}
}

// TestSingleLineFieldUnchanged confirms the single-line fast path still
// emits the conventional "Field: value\n" form.
func TestSingleLineFieldUnchanged(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "out")

	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}

	if err := aptinstall.WriteDeb822FieldForTesting(f, "Package", "hello"); err != nil {
		t.Fatal(err)
	}

	_ = f.Close()

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	if string(got) != "Package: hello\n" {
		t.Fatalf("single-line emit wrong: %q", got)
	}
}

// TestEmptyValueEmitsColonOnly matches dpkg's own output for fields like
// `Conffiles:` when a package ships no conffiles. dpkg's own tooling
// reads this either way, but matching the canonical form keeps diffs
// against the original status quiet.
func TestEmptyValueEmitsColonOnly(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "out")

	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}

	if err := aptinstall.WriteDeb822FieldForTesting(f, "Conffiles", ""); err != nil {
		t.Fatal(err)
	}

	_ = f.Close()

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	if !strings.HasPrefix(string(got), "Conffiles:") {
		t.Fatalf("empty value should start with 'Conffiles:', got %q", got)
	}
}
