//go:build linux

//nolint:testpackage // exercises unexported helpers
package rootless

import (
	"archive/tar"
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func buildTar(t *testing.T, build func(tw *tar.Writer)) *bytes.Reader {
	t.Helper()

	var buf bytes.Buffer

	tw := tar.NewWriter(&buf)
	build(tw)

	if err := tw.Close(); err != nil {
		t.Fatalf("close tar: %v", err)
	}

	return bytes.NewReader(buf.Bytes())
}

// TestExtractTarDoesNotWriteThroughSymlink verifies that an entry written
// below a previously extracted symlink stays inside the rootfs.
func TestExtractTarDoesNotWriteThroughSymlink(t *testing.T) {
	dest := t.TempDir()
	outside := t.TempDir()

	data := []byte("pwned")

	r := buildTar(t, func(tw *tar.Writer) {
		hdrs := []*tar.Header{
			{Name: "link", Typeflag: tar.TypeSymlink, Linkname: outside, Mode: 0o777},
			{Name: "link/evil", Typeflag: tar.TypeReg, Mode: 0o644, Size: int64(len(data))},
		}

		for _, h := range hdrs {
			if err := tw.WriteHeader(h); err != nil {
				t.Fatal(err)
			}

			if h.Size > 0 {
				if _, err := tw.Write(data); err != nil {
					t.Fatal(err)
				}
			}
		}
	})

	if err := extractTar(r, dest); err != nil {
		t.Fatalf("extractTar: %v", err)
	}

	if _, err := os.Stat(filepath.Join(outside, "evil")); err == nil {
		t.Fatal("file was written outside the rootfs through a symlink")
	}

	if _, err := os.Stat(filepath.Join(dest, outside, "evil")); err != nil {
		t.Errorf("expected file contained under rootfs: %v", err)
	}
}

// TestExtractTarRegularFileReplacesSymlink verifies a regular file entry
// replaces an existing symlink rather than writing through it.
func TestExtractTarRegularFileReplacesSymlink(t *testing.T) {
	dest := t.TempDir()
	victim := filepath.Join(t.TempDir(), "victim")

	if err := os.WriteFile(victim, []byte("orig"), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := os.Symlink(victim, filepath.Join(dest, "f")); err != nil {
		t.Fatal(err)
	}

	r := buildTar(t, func(tw *tar.Writer) {
		if err := tw.WriteHeader(&tar.Header{
			Name: "f", Typeflag: tar.TypeReg, Mode: 0o644, Size: 3,
		}); err != nil {
			t.Fatal(err)
		}

		if _, err := tw.Write([]byte("new")); err != nil {
			t.Fatal(err)
		}
	})

	if err := extractTar(r, dest); err != nil {
		t.Fatalf("extractTar: %v", err)
	}

	got, err := os.ReadFile(victim)
	if err != nil {
		t.Fatal(err)
	}

	if string(got) != "orig" {
		t.Errorf("symlink target was overwritten: %q", got)
	}
}

func TestSwapRootfsReplacesExisting(t *testing.T) {
	base := t.TempDir()
	rootfs := filepath.Join(base, "rootfs")
	newDir := filepath.Join(base, "new")

	for dir, marker := range map[string]string{rootfs: "old", newDir: "new"} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}

		if err := os.WriteFile(filepath.Join(dir, marker), nil, 0o600); err != nil {
			t.Fatal(err)
		}
	}

	if err := swapRootfs(newDir, rootfs); err != nil {
		t.Fatalf("swapRootfs: %v", err)
	}

	if _, err := os.Stat(filepath.Join(rootfs, "new")); err != nil {
		t.Errorf("new rootfs not installed: %v", err)
	}

	if _, err := os.Stat(filepath.Join(rootfs, "old")); err == nil {
		t.Error("old rootfs content still present")
	}

	if _, err := os.Stat(newDir + ".old"); err == nil {
		t.Error("backup not cleaned up")
	}
}
