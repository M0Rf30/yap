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

// TestSafeJoinRejectsTraversal is the regression test for C-3. The legacy
// `strings.HasPrefix(fullPath, destDir)` guard was a no-op when destDir="/"
// (every absolute path begins with "/") and was vulnerable to prefix
// aliasing for non-root destDirs.
func TestSafeJoinRejectsTraversal(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name, dest, entry string
		wantErr           bool
	}{
		{"plain", "/tmp/sandbox", "etc/hosts", false},
		{"absolute_normalised", "/tmp/sandbox", "/etc/hosts", false},
		{"dotdot_escape", "/tmp/sandbox", "../etc/passwd", true},
		{"deep_escape", "/tmp/sandbox", "a/b/../../../etc/passwd", true},
		{"prefix_alias", "/tmp/sandbox", "../sandboxX/evil", true},
		// Hardened: a member literally named "../etc/passwd" is hostile even
		// when "/" clamping would keep it under root — dpkg/rpm reject it too.
		{"root_with_dotdot", "/", "../etc/passwd", true},
		{"root_plain", "/", "etc/hosts", false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			_, err := aptinstall.SafeJoinForTesting(tc.dest, tc.entry)

			if tc.wantErr && err == nil {
				t.Fatalf("expected error for entry=%q dest=%q", tc.entry, tc.dest)
			}

			if !tc.wantErr && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

// TestSafeSymlinkTargetRejectsEscape is the regression test for H-2.
// A symlink with a relative target containing ".." that resolves outside
// destDir must be refused.
func TestSafeSymlinkTargetRejectsEscape(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name, dest, link, target string
		wantErr                  bool
	}{
		{"abs_target_ok", "/tmp/sandbox", "/tmp/sandbox/lib/libfoo.so", "/usr/lib/libfoo.so.1", false},
		{"rel_inside", "/tmp/sandbox", "/tmp/sandbox/usr/bin/foo", "../share/foo", false},
		{"rel_escape", "/tmp/sandbox", "/tmp/sandbox/usr/bin/foo", "../../../../etc/passwd", true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			err := aptinstall.SafeSymlinkTargetForTesting(tc.dest, tc.link, tc.target)
			if tc.wantErr && err == nil {
				t.Fatalf("expected error for link=%q target=%q", tc.link, tc.target)
			}

			if !tc.wantErr && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

func keysOf(m map[string]map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}

	return keys
}
