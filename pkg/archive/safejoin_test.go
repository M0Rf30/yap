//nolint:testpackage // tests cover unexported safeJoin / safeSymlinkTarget helpers
package archive

import (
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestSafeJoin_AllowsLegitimatePaths(t *testing.T) {
	dest := t.TempDir()

	cases := []string{
		"foo.txt",
		"sub/dir/file.txt",
		"./relative.txt",
		"a/b/c/d/e.txt",
	}

	for _, name := range cases {
		t.Run(name, func(t *testing.T) {
			got, err := safeJoin(dest, name)
			if err != nil {
				t.Fatalf("safeJoin(%q) returned error: %v", name, err)
			}

			if !strings.HasPrefix(got, filepath.Clean(dest)) {
				t.Fatalf("safeJoin(%q) escaped destination: %s", name, got)
			}
		})
	}
}

func TestSafeJoin_RejectsTraversal(t *testing.T) {
	dest := t.TempDir()

	cases := []string{
		"../escape",
		"../../etc/passwd",
		"foo/../../escape",
		"a/b/../../../escape",
		"./../escape",
		"sub/../../escape",
	}

	for _, name := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := safeJoin(dest, name)
			if err == nil {
				t.Fatalf("safeJoin(%q) should have rejected traversal", name)
			}
		})
	}
}

func TestSafeJoin_AbsolutePathInsideDest(t *testing.T) {
	// filepath.Join("/tmp/dest", "/foo") -> "/tmp/dest/foo" on Unix.
	// We accept that because it stays inside dest.
	if runtime.GOOS == "windows" {
		t.Skip("absolute path semantics differ on Windows")
	}

	dest := t.TempDir()

	got, err := safeJoin(dest, "/legit/path")
	if err != nil {
		t.Fatalf("safeJoin with absolute child name failed: %v", err)
	}

	if !strings.HasPrefix(got, filepath.Clean(dest)+string(filepath.Separator)) {
		t.Fatalf("absolute child path escaped destination: %s", got)
	}
}

func TestSafeJoin_RootDestination(t *testing.T) {
	// Extraction to "/" must still work — this is the production path for
	// ExtractToRoot on APK/Pacman packages.
	if runtime.GOOS == "windows" {
		t.Skip("rooted destination test is POSIX-specific")
	}

	got, err := safeJoin("/", "opt/zextras/common/lib/libfoo.so")
	if err != nil {
		t.Fatalf("safeJoin from / failed: %v", err)
	}

	want := "/opt/zextras/common/lib/libfoo.so"
	if got != want {
		t.Fatalf("safeJoin from /: got %q, want %q", got, want)
	}

	// And it must still reject traversal from /.
	if _, err := safeJoin("/", "../etc/passwd"); err == nil {
		t.Fatal("safeJoin from / should reject ../ traversal")
	}
}

func TestSafeSymlinkTarget(t *testing.T) {
	cases := []struct {
		name    string // descriptive label
		entry   string // path of the symlink itself within the archive
		target  string // target as stored in the archive
		wantErr bool
	}{
		{"empty", "any/where", "", false},
		{"plain relative", "a/b", "relative.txt", false},
		{"nested relative", "a/b", "sub/dir/target.txt", false},
		{"dot-relative", "a/b", "./local", false},

		// Absolute targets are accepted: real packages routinely ship absolute
		// symlinks. The traversal-through-symlink attack is blocked by safeJoin
		// on every entry's write path, not the link target.
		{"absolute etc", "any/where", "/etc/passwd", false},
		{"absolute lib", "any/where", "/absolute/danger", false},

		// Legitimate sibling-directory relative symlinks. This is the case
		// that previously broke real-world Debian packages (e.g. openldap
		// ships opt/.../sbin/slapacl -> ../libexec/slapd).
		{
			"sibling via parent dir",
			"opt/zextras/common/sbin/slapacl",
			"../libexec/slapd",
			false,
		},
		{
			"sibling two levels up",
			"a/b/c/d", "../../sibling/file",
			false,
		},

		// Targets that resolve above the archive root must be rejected.
		{"escape from root", "a", "../escape", true},
		{"escape via double-dot chain", "a/b", "../../../escape", true},
		{"escape mid-path", "a/b", "foo/../../../escape", true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := safeSymlinkTarget(tc.entry, tc.target)
			if (err != nil) != tc.wantErr {
				t.Fatalf("safeSymlinkTarget(%q, %q) err=%v wantErr=%v",
					tc.entry, tc.target, err, tc.wantErr)
			}
		})
	}
}
