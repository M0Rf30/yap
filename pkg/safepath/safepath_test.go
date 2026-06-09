package safepath_test

import (
	"path/filepath"
	"testing"

	"github.com/M0Rf30/yap/v2/pkg/safepath"
)

// TestJoinContainment tests Join against traversal, aliasing, and
// normalization attacks.
func TestJoinContainment(t *testing.T) {
	cases := []struct {
		name    string
		root    string
		entry   string
		want    string // expected result; "" means expect rejection
		wantErr bool
	}{
		// Benign entries.
		{"plain file", "/dst", "usr/bin/foo", "/dst/usr/bin/foo", false},
		{"dot-slash prefix", "/dst", "./usr/bin/foo", "/dst/usr/bin/foo", false},
		{"root dir entry", "/dst", "./", "/dst", false},
		{"absolute reinterpreted", "/dst", "/usr/bin/foo", "/dst/usr/bin/foo", false},
		{"internal dotdot stays inside", "/dst", "a/../b", "/dst/b", false},
		{"root is slash", "/", "etc/passwd", "/etc/passwd", false},

		// Hostile entries.
		{"plain traversal", "/dst", "../evil", "", true},
		{"deep traversal", "/dst", "a/../../evil", "", true},
		{"many dotdots", "/dst", "../../../../etc/passwd", "", true},
		{"dotdot after slash burn", "/dst", "./../evil", "", true},

		// Absolute names that traverse after reinterpretation.
		{"absolute with traversal", "/dst", "/../evil", "", true},

		// Root "/" — filepath.Clean clamps ".." at the root, so the hostile
		// name must be rejected from the NAME itself, not the join result.
		{"slash root traversal name", "/", "../../../etc/passwd", "", true},
		{"slash root mid traversal", "/", "a/../../bin/sh", "", true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := safepath.Join(tc.root, tc.entry)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("Join(%q, %q) = %q, want rejection", tc.root, tc.entry, got)
				}

				return
			}

			if err != nil {
				t.Fatalf("Join(%q, %q) unexpected error: %v", tc.root, tc.entry, err)
			}

			if got != filepath.Clean(tc.want) {
				t.Errorf("Join(%q, %q) = %q, want %q", tc.root, tc.entry, got, tc.want)
			}
		})
	}
}

// TestJoinNoPrefixAliasing tests the classic prefix-aliasing hole:
// root "/tmp/foo" must not admit a path that resolves to "/tmp/foobar".
func TestJoinNoPrefixAliasing(t *testing.T) {
	if _, err := safepath.Join("/tmp/foo", "../foobar/evil"); err == nil {
		t.Error("expected rejection of prefix-aliasing escape")
	}
}

// TestJoinStrictRejectsRoot tests that JoinStrict refuses entries that
// resolve to the root itself.
func TestJoinStrictRejectsRoot(t *testing.T) {
	for _, entry := range []string{"", ".", "./", "/"} {
		if got, err := safepath.JoinStrict("/dst", entry); err == nil {
			t.Errorf("JoinStrict(%q) = %q, want rejection", entry, got)
		}
	}

	// Normal entries still pass.
	if _, err := safepath.JoinStrict("/dst", "usr/bin/tree"); err != nil {
		t.Errorf("JoinStrict rejected a benign entry: %v", err)
	}
}

// TestSymlinkTarget tests on-disk symlink target validation.
func TestSymlinkTarget(t *testing.T) {
	cases := []struct {
		name     string
		root     string
		linkPath string
		target   string
		wantErr  bool
	}{
		{"absolute target allowed", "/dst", "/dst/usr/bin/foo", "/usr/lib/libfoo.so", false},
		{"empty target allowed", "/dst", "/dst/a", "", false},
		{"sibling relative", "/dst", "/dst/usr/bin/foo", "bar", false},
		{"updir inside root", "/dst", "/dst/opt/foo/sbin/slapacl", "../libexec/slapd", false},
		{"escape via updirs", "/dst", "/dst/a/b", "../../../escape", true},
		{"escape at top level", "/dst", "/dst/bin", "../outside", true},
		{"root slash never escapes", "/", "/usr/bin/foo", "../../../../etc/passwd", false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := safepath.SymlinkTarget(tc.root, tc.linkPath, tc.target)
			if tc.wantErr && err == nil {
				t.Errorf("SymlinkTarget(%q, %q, %q): want rejection", tc.root, tc.linkPath, tc.target)
			}

			if !tc.wantErr && err != nil {
				t.Errorf("SymlinkTarget(%q, %q, %q): unexpected error %v", tc.root, tc.linkPath, tc.target, err)
			}
		})
	}
}

// TestEntrySymlinkTarget tests archive-relative symlink target validation.
func TestEntrySymlinkTarget(t *testing.T) {
	cases := []struct {
		name    string
		entry   string
		target  string
		wantErr bool
	}{
		{"absolute target allowed", "usr/bin/foo", "/usr/lib/libfoo.so.1", false},
		{"sibling", "usr/bin/foo", "bar", false},
		{"updir inside archive", "opt/foo/sbin/slapacl", "../libexec/slapd", false},
		{"top-level entry escape", "a", "../escape", true},
		{"deep escape", "a/b", "../../../escape", true},
		{"exact parent escape", "a/b", "../..", true},
		{"empty target", "a/b", "", false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := safepath.EntrySymlinkTarget(tc.entry, tc.target)
			if tc.wantErr && err == nil {
				t.Errorf("EntrySymlinkTarget(%q, %q): want rejection", tc.entry, tc.target)
			}

			if !tc.wantErr && err != nil {
				t.Errorf("EntrySymlinkTarget(%q, %q): unexpected error %v", tc.entry, tc.target, err)
			}
		})
	}
}
