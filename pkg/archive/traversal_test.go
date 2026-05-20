package archive_test

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	stderrors "errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/M0Rf30/yap/v2/pkg/archive"
)

// TestExtract_UnrecognizedArchive verifies that Extract returns the typed
// sentinel ErrUnrecognizedArchive (not silent nil) when the input file is not
// a recognizable archive. This is the regression guard for the silent-no-op
// class of bug that has bitten RPM and APK extraction.
func TestExtract_UnrecognizedArchive(t *testing.T) {
	tmp := t.TempDir()
	notAnArchive := filepath.Join(tmp, "patch.diff")

	if err := os.WriteFile(notAnArchive, []byte("--- a/foo\n+++ b/foo\n"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	dest := filepath.Join(tmp, "dest")
	if err := os.MkdirAll(dest, 0o755); err != nil {
		t.Fatalf("mkdir dest: %v", err)
	}

	err := archive.Extract(context.Background(), notAnArchive, dest)
	if err == nil {
		t.Fatal("Extract on non-archive returned nil; expected ErrUnrecognizedArchive")
	}

	if !stderrors.Is(err, archive.ErrUnrecognizedArchive) {
		t.Fatalf("Extract returned %v; expected ErrUnrecognizedArchive", err)
	}
}

// buildEvilTarGz returns a gzipped tar containing exactly one entry whose
// header name is supplied by the caller. Used to construct zip-slip /
// traversal payloads.
func buildEvilTarGz(t *testing.T, entryName, content string) []byte {
	t.Helper()

	var buf bytes.Buffer

	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)

	body := []byte(content)
	if err := tw.WriteHeader(&tar.Header{
		Name:     entryName,
		Mode:     0o644,
		Size:     int64(len(body)),
		Typeflag: tar.TypeReg,
	}); err != nil {
		t.Fatalf("WriteHeader: %v", err)
	}

	if _, err := tw.Write(body); err != nil {
		t.Fatalf("Write: %v", err)
	}

	if err := tw.Close(); err != nil {
		t.Fatalf("tar.Close: %v", err)
	}

	if err := gz.Close(); err != nil {
		t.Fatalf("gz.Close: %v", err)
	}

	return buf.Bytes()
}

// buildEvilSymlinkTarGz produces a tar.gz with a single symlink entry whose
// linkname is the supplied (potentially-malicious) target.
func buildEvilSymlinkTarGz(t *testing.T, entryName, target string) []byte {
	t.Helper()

	var buf bytes.Buffer

	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)

	if err := tw.WriteHeader(&tar.Header{
		Name:     entryName,
		Linkname: target,
		Mode:     0o777,
		Typeflag: tar.TypeSymlink,
	}); err != nil {
		t.Fatalf("WriteHeader symlink: %v", err)
	}

	if err := tw.Close(); err != nil {
		t.Fatalf("tar.Close: %v", err)
	}

	if err := gz.Close(); err != nil {
		t.Fatalf("gz.Close: %v", err)
	}

	return buf.Bytes()
}

// TestExtract_RejectsTraversal is the regression guard for the zip-slip class
// of bug. A tar entry whose name contains "../" must be refused, and no file
// outside the destination directory may be created.
func TestExtract_RejectsTraversal(t *testing.T) {
	cases := []struct {
		name      string
		entryName string
	}{
		{"parent traversal", "../escape"},
		{"deep traversal", "../../etc/passwd"},
		{"nested traversal", "sub/../../escape"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tmp := t.TempDir()
			tarPath := filepath.Join(tmp, "evil.tar.gz")
			dest := filepath.Join(tmp, "dest")

			if err := os.MkdirAll(dest, 0o755); err != nil {
				t.Fatalf("mkdir dest: %v", err)
			}

			payload := buildEvilTarGz(t, tc.entryName, "pwned")
			if err := os.WriteFile(tarPath, payload, 0o644); err != nil {
				t.Fatalf("write tar: %v", err)
			}

			err := archive.Extract(context.Background(), tarPath, dest)
			if err == nil {
				t.Fatalf("Extract(%q) returned nil, expected traversal rejection",
					tc.entryName)
			}

			// Sanity: nothing escaped tmp.
			escape := filepath.Join(tmp, "escape")
			if _, statErr := os.Stat(escape); statErr == nil {
				t.Fatalf("traversal succeeded: %s was created", escape)
			}
		})
	}
}

// TestExtract_AllowsAbsoluteSymlinkTarget verifies that absolute symlink
// targets are accepted: real packages (e.g. glibc) routinely ship absolute
// symlinks like /usr/lib64/libfoo.so.1 -> /usr/lib/libfoo.so.1, and
// dpkg/rpm/apk accept them.
//
// The traversal-through-symlink attack (link foo -> /etc + write foo/passwd)
// is blocked by safeJoin on every entry's *own write path*, not the symlink
// target. Creating the link itself is harmless: no follow-up write happens
// inside the extraction loop that resolves through the link.
func TestExtract_AllowsAbsoluteSymlinkTarget(t *testing.T) {
	tmp := t.TempDir()
	tarPath := filepath.Join(tmp, "abs-sym.tar.gz")
	dest := filepath.Join(tmp, "dest")

	if err := os.MkdirAll(dest, 0o755); err != nil {
		t.Fatalf("mkdir dest: %v", err)
	}

	payload := buildEvilSymlinkTarGz(t, "innocent-name", "/etc/passwd")
	if err := os.WriteFile(tarPath, payload, 0o644); err != nil {
		t.Fatalf("write tar: %v", err)
	}

	if err := archive.Extract(context.Background(), tarPath, dest); err != nil {
		t.Fatalf("Extract should accept symlink with absolute target: %v", err)
	}

	linkPath := filepath.Join(dest, "innocent-name")

	info, err := os.Lstat(linkPath)
	if err != nil {
		t.Fatalf("expected symlink at %s: %v", linkPath, err)
	}

	if info.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("expected %s to be a symlink, got mode %v", linkPath, info.Mode())
	}

	target, err := os.Readlink(linkPath)
	if err != nil {
		t.Fatalf("readlink: %v", err)
	}

	if target != "/etc/passwd" {
		t.Fatalf("unexpected symlink target: got %q want /etc/passwd", target)
	}
}

// TestExtract_RejectsTraversalSymlinkTarget guards against symlink targets
// that escape via "..".
func TestExtract_RejectsTraversalSymlinkTarget(t *testing.T) {
	tmp := t.TempDir()
	tarPath := filepath.Join(tmp, "evil-sym.tar.gz")
	dest := filepath.Join(tmp, "dest")

	if err := os.MkdirAll(dest, 0o755); err != nil {
		t.Fatalf("mkdir dest: %v", err)
	}

	payload := buildEvilSymlinkTarGz(t, "innocent-name", "../../escape")
	if err := os.WriteFile(tarPath, payload, 0o644); err != nil {
		t.Fatalf("write tar: %v", err)
	}

	err := archive.Extract(context.Background(), tarPath, dest)
	if err == nil {
		t.Fatal("Extract should reject symlink with .. target")
	}
}

// TestExtract_AllowsLegitimateArchive sanity-checks that the traversal guard
// does not break legitimate archives.
func TestExtract_AllowsLegitimateArchive(t *testing.T) {
	tmp := t.TempDir()
	tarPath := filepath.Join(tmp, "good.tar.gz")
	dest := filepath.Join(tmp, "dest")

	if err := os.MkdirAll(dest, 0o755); err != nil {
		t.Fatalf("mkdir dest: %v", err)
	}

	payload := buildEvilTarGz(t, "sub/dir/legit.txt", "ok")
	if err := os.WriteFile(tarPath, payload, 0o644); err != nil {
		t.Fatalf("write tar: %v", err)
	}

	if err := archive.Extract(context.Background(), tarPath, dest); err != nil {
		t.Fatalf("Extract of legitimate archive failed: %v", err)
	}

	got, err := os.ReadFile(filepath.Join(dest, "sub", "dir", "legit.txt"))
	if err != nil {
		t.Fatalf("expected file missing: %v", err)
	}

	if string(got) != "ok" {
		t.Fatalf("content mismatch: %q", string(got))
	}
}
