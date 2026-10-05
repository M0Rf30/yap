package safepath_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/M0Rf30/yap/v2/pkg/safepath"
)

func newRoot(t *testing.T) string {
	t.Helper()

	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}

	return root
}

func mustSymlink(t *testing.T, target, link string) {
	t.Helper()

	if err := os.MkdirAll(filepath.Dir(link), 0o755); err != nil {
		t.Fatal(err)
	}

	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
}

func assertInside(t *testing.T, root, got string) {
	t.Helper()

	if got != root && !strings.HasPrefix(got, root+string(filepath.Separator)) {
		t.Fatalf("result %q escapes root %q", got, root)
	}
}

func TestResolveInRootAbsoluteSymlinkReinterpreted(t *testing.T) {
	root := newRoot(t)
	mustSymlink(t, "/etc", filepath.Join(root, "link"))

	got, err := safepath.ResolveInRoot(root, "link/passwd")
	if err != nil {
		t.Fatal(err)
	}

	assertInside(t, root, got)

	if want := filepath.Join(root, "etc", "passwd"); got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestResolveInRootDotDotChainClamped(t *testing.T) {
	root := newRoot(t)
	mustSymlink(t, "../..", filepath.Join(root, "a", "b"))

	got, err := safepath.ResolveInRoot(root, "a/b/etc/shadow")
	if err != nil {
		t.Fatal(err)
	}

	assertInside(t, root, got)

	if want := filepath.Join(root, "etc", "shadow"); got != want {
		t.Fatalf("got %q, want %q", got, want)
	}

	got, err = safepath.ResolveInRoot(root, "a/b/../../../../../x")
	if err != nil {
		t.Fatal(err)
	}

	assertInside(t, root, got)
}

func TestResolveInRootUsrMerge(t *testing.T) {
	root := newRoot(t)
	mustSymlink(t, "usr/lib", filepath.Join(root, "lib"))

	if err := os.MkdirAll(filepath.Join(root, "usr", "lib"), 0o755); err != nil {
		t.Fatal(err)
	}

	got, err := safepath.ResolveInRoot(root, "/lib/modules/6.1/foo.ko")
	if err != nil {
		t.Fatal(err)
	}

	if want := filepath.Join(root, "usr", "lib", "modules", "6.1", "foo.ko"); got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestResolveInRootNonexistentTail(t *testing.T) {
	root := newRoot(t)

	got, err := safepath.ResolveInRoot(root, "does/not/exist.txt")
	if err != nil {
		t.Fatal(err)
	}

	if want := filepath.Join(root, "does", "not", "exist.txt"); got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestResolveInRootLexicalTraversal(t *testing.T) {
	root := newRoot(t)

	got, err := safepath.ResolveInRoot(root, "../../etc/passwd")
	if err != nil {
		t.Fatal(err)
	}

	assertInside(t, root, got)
}

func TestResolveInRootSlashRoot(t *testing.T) {
	got, err := safepath.ResolveInRoot("/", "/does-not-exist-yap/x/../y")
	if err != nil {
		t.Fatal(err)
	}

	if want := "/does-not-exist-yap/y"; got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestResolveInRootSymlinkLoopErrors(t *testing.T) {
	root := newRoot(t)
	mustSymlink(t, "b", filepath.Join(root, "a"))
	mustSymlink(t, "a", filepath.Join(root, "b"))

	if _, err := safepath.ResolveInRoot(root, "a/x"); err == nil {
		t.Fatal("expected error for symlink loop")
	}
}
