// SPDX-FileCopyrightText: 2021-2026 Gianluca Boiano
// SPDX-License-Identifier: GPL-3.0-only

package aptinstall_test

import (
	"strings"
	"testing"

	"github.com/M0Rf30/yap/v2/pkg/aptinstall"
)

// TestStatusRoundTripPreservesLastField is the regression test for C-1.
// Before the fix, flushDpkgStatusEntry was called WITHOUT first flushing
// the in-flight (currentField, currentValue) pair, so the last field of
// every stanza was silently dropped on re-parse — corrupting the dpkg
// database after a single round-trip.
func TestStatusRoundTripPreservesLastField(t *testing.T) {
	t.Parallel()

	const stanza = "Package: hello\n" +
		"Architecture: amd64\n" +
		"Status: install ok installed\n" +
		"Version: 2.10-2\n" +
		"Description: greet the world\n" +
		" Long description spread over multiple lines.\n" +
		"\n"

	got, err := aptinstall.ReadDpkgStatusFromStringForTesting(stanza)
	if err != nil {
		t.Fatal(err)
	}

	entry, ok := got["hello:amd64"]
	if !ok {
		t.Fatalf("missing hello:amd64 entry; got keys %v", keysOf(got))
	}

	if entry["Description"] == "" {
		t.Fatalf("Description field dropped! entry=%v", entry)
	}

	if !strings.Contains(entry["Description"], "Long description") {
		t.Fatalf("multi-line continuation lost; got %q", entry["Description"])
	}

	if entry["Version"] != "2.10-2" {
		t.Fatalf("Version wrong: %q", entry["Version"])
	}
}

// TestStatusLastFieldNoTrailingBlankLine covers the EOF flush path:
// a stanza terminated by EOF rather than a blank line must still publish
// every field, including the last one.
func TestStatusLastFieldNoTrailingBlankLine(t *testing.T) {
	t.Parallel()

	const stanza = "Package: foo\n" +
		"Architecture: amd64\n" +
		"Status: install ok installed\n" +
		"Description: trailing"

	got, err := aptinstall.ReadDpkgStatusFromStringForTesting(stanza)
	if err != nil {
		t.Fatal(err)
	}

	entry := got["foo:amd64"]
	if entry["Description"] != "trailing" {
		t.Fatalf("Description lost on EOF flush; entry=%v", entry)
	}
}

func keysOf(m map[string]map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}

	return keys
}
