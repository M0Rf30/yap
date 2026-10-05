package platform

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"
)

func TestParseSHA256Sidecar(t *testing.T) {
	good := hex.EncodeToString(make([]byte, sha256.Size))

	cases := []struct {
		name    string
		body    string
		want    string
		wantErr bool
	}{
		{"bare", good + "\n", good, false},
		{"with filename", good + "  go.tar.gz\n", good, false},
		{"empty", "  \n", "", true},
		{"short", "abcd", "", true},
		{"not hex", "zz" + good[2:], "", true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := parseSHA256Sidecar([]byte(tc.body))
			if (err != nil) != tc.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tc.wantErr)
			}

			if got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}

func TestVerifyFileSHA256(t *testing.T) {
	path := filepath.Join(t.TempDir(), "f")
	content := []byte("hello go toolchain")

	if err := os.WriteFile(path, content, 0o600); err != nil {
		t.Fatal(err)
	}

	sum := sha256.Sum256(content)

	if err := verifyFileSHA256(path, hex.EncodeToString(sum[:])); err != nil {
		t.Errorf("matching digest rejected: %v", err)
	}

	bad := hex.EncodeToString(make([]byte, sha256.Size))
	if err := verifyFileSHA256(path, bad); err == nil {
		t.Error("mismatching digest accepted")
	}

	if err := verifyFileSHA256(path+".missing", bad); err == nil {
		t.Error("missing file accepted")
	}
}

func TestEnsureSymlinkIdempotent(t *testing.T) {
	dir := t.TempDir()
	link := filepath.Join(dir, "go")

	if err := ensureSymlink("/a/go", link); err != nil {
		t.Fatal(err)
	}

	// Same target again: no error.
	if err := ensureSymlink("/a/go", link); err != nil {
		t.Fatalf("repeat with same target failed: %v", err)
	}

	// Different target: replaced.
	if err := ensureSymlink("/b/go", link); err != nil {
		t.Fatalf("repoint failed: %v", err)
	}

	if got, _ := os.Readlink(link); got != "/b/go" {
		t.Errorf("link target = %q, want /b/go", got)
	}

	// Regular file is left untouched.
	regular := filepath.Join(dir, "gofmt")
	if err := os.WriteFile(regular, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := ensureSymlink("/a/gofmt", regular); err != nil {
		t.Fatalf("regular file case failed: %v", err)
	}

	if fi, _ := os.Lstat(regular); fi.Mode()&os.ModeSymlink != 0 {
		t.Error("regular file was replaced by a symlink")
	}
}

// A dangling symlink inside the tree must not make the recursive chown fail or
// follow it (Lchown operates on the link itself).
func TestChownRecursiveDoesNotFollowSymlinks(t *testing.T) {
	dir := t.TempDir()
	outside := filepath.Join(t.TempDir(), "outside")

	if err := os.WriteFile(outside, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := os.Symlink(outside, filepath.Join(dir, "link")); err != nil {
		t.Fatal(err)
	}

	if err := os.Symlink("/nonexistent/target", filepath.Join(dir, "dangling")); err != nil {
		t.Fatal(err)
	}

	user := &OriginalUser{UID: os.Getuid(), GID: os.Getgid(), Name: "self"}
	if err := user.ChownRecursiveToOriginalUser(dir); err != nil {
		t.Fatalf("recursive chown to self failed: %v", err)
	}
}
