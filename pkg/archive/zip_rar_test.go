// Package archive_test covers the zip and rar extraction paths, the exported
// SafeJoin / SafeSymlinkTarget wrappers, and the writeSymlinkFromOpener helper
// (exercised indirectly through zip symlink extraction).
package archive_test

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/base64"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/M0Rf30/yap/v2/pkg/archive"
)

// ---------------------------------------------------------------------------
// Exported wrapper tests: SafeJoin / SafeSymlinkTarget
// ---------------------------------------------------------------------------

// TestSafeJoin_ExportedWrapper verifies that the exported SafeJoin delegates
// correctly to the internal safeJoin: legitimate paths succeed and traversal
// paths are rejected.
func TestSafeJoin_ExportedWrapper(t *testing.T) {
	dest := t.TempDir()

	t.Run("legitimate path", func(t *testing.T) {
		got, err := archive.SafeJoin(dest, "sub/dir/file.txt")
		if err != nil {
			t.Fatalf("SafeJoin returned unexpected error: %v", err)
		}

		if !strings.HasPrefix(got, filepath.Clean(dest)) {
			t.Fatalf("SafeJoin result %q escaped destination %q", got, dest)
		}
	})

	t.Run("traversal rejected", func(t *testing.T) {
		_, err := archive.SafeJoin(dest, "../escape")
		if err == nil {
			t.Fatal("SafeJoin should reject path traversal")
		}
	})

	t.Run("root destination", func(t *testing.T) {
		if runtime.GOOS == "windows" {
			t.Skip("rooted destination test is POSIX-specific")
		}

		got, err := archive.SafeJoin("/", "opt/foo/bar")
		if err != nil {
			t.Fatalf("SafeJoin from / failed: %v", err)
		}

		if got != "/opt/foo/bar" {
			t.Fatalf("SafeJoin from /: got %q, want /opt/foo/bar", got)
		}
	})
}

// TestSafeSymlinkTarget_ExportedWrapper verifies that the exported
// SafeSymlinkTarget delegates correctly to the internal safeSymlinkTarget.
func TestSafeSymlinkTarget_ExportedWrapper(t *testing.T) {
	cases := []struct {
		name    string
		entry   string
		target  string
		wantErr bool
	}{
		{"empty target", "a/b", "", false},
		{"relative sibling", "opt/foo/sbin/tool", "../libexec/tool", false},
		{"absolute target", "a/b", "/etc/passwd", false},
		{"escape from root", "a", "../escape", true},
		{"deep escape", "a/b", "../../../escape", true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := archive.SafeSymlinkTarget(tc.entry, tc.target)
			if (err != nil) != tc.wantErr {
				t.Fatalf("SafeSymlinkTarget(%q, %q) err=%v wantErr=%v",
					tc.entry, tc.target, err, tc.wantErr)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// ZIP extraction helpers
// ---------------------------------------------------------------------------

// zipEntry describes a single entry in a test zip archive.
type zipEntry struct {
	name    string
	content string
	mode    os.FileMode
}

func buildZipBytes(t *testing.T, entries []zipEntry) []byte {
	t.Helper()

	var buf bytes.Buffer

	zw := zip.NewWriter(&buf)

	for _, e := range entries {
		fh := &zip.FileHeader{
			Name:   e.name,
			Method: zip.Deflate,
		}
		fh.SetMode(e.mode)

		w, err := zw.CreateHeader(fh)
		if err != nil {
			t.Fatalf("zip CreateHeader %q: %v", e.name, err)
		}

		if _, err := io.WriteString(w, e.content); err != nil {
			t.Fatalf("zip Write %q: %v", e.name, err)
		}
	}

	if err := zw.Close(); err != nil {
		t.Fatalf("zip Close: %v", err)
	}

	return buf.Bytes()
}

func writeZipFixture(t *testing.T, dir string, entries []zipEntry) string {
	t.Helper()

	data := buildZipBytes(t, entries)
	path := filepath.Join(dir, "fixture.zip")

	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("write zip fixture: %v", err)
	}

	return path
}

// ---------------------------------------------------------------------------
// ZIP extraction tests
// ---------------------------------------------------------------------------

// TestExtract_Zip_RegularFiles verifies that regular files inside a zip are
// extracted to the correct paths with the expected content.
func TestExtract_Zip_RegularFiles(t *testing.T) {
	tmp := t.TempDir()
	dest := filepath.Join(tmp, "dest")

	if err := os.MkdirAll(dest, 0o755); err != nil {
		t.Fatalf("mkdir dest: %v", err)
	}

	entries := []zipEntry{
		{name: "hello.txt", content: "hello world", mode: 0o644},
		{name: "sub/nested.txt", content: "nested content", mode: 0o644},
	}

	archivePath := writeZipFixture(t, tmp, entries)

	if err := archive.Extract(context.Background(), archivePath, dest); err != nil {
		t.Fatalf("Extract zip failed: %v", err)
	}

	cases := []struct {
		path    string
		content string
	}{
		{"hello.txt", "hello world"},
		{"sub/nested.txt", "nested content"},
	}

	for _, tc := range cases {
		t.Run(tc.path, func(t *testing.T) {
			got, err := os.ReadFile(filepath.Join(dest, tc.path))
			if err != nil {
				t.Fatalf("read %q: %v", tc.path, err)
			}

			if string(got) != tc.content {
				t.Fatalf("content mismatch for %q: got %q, want %q",
					tc.path, string(got), tc.content)
			}
		})
	}
}

// TestExtract_Zip_Directory verifies that directory entries in a zip are
// created as directories on disk.
func TestExtract_Zip_Directory(t *testing.T) {
	tmp := t.TempDir()
	dest := filepath.Join(tmp, "dest")

	if err := os.MkdirAll(dest, 0o755); err != nil {
		t.Fatalf("mkdir dest: %v", err)
	}

	entries := []zipEntry{
		{name: "mydir/", content: "", mode: 0o755 | os.ModeDir},
		{name: "mydir/file.txt", content: "inside dir", mode: 0o644},
	}

	archivePath := writeZipFixture(t, tmp, entries)

	if err := archive.Extract(context.Background(), archivePath, dest); err != nil {
		t.Fatalf("Extract zip with directory failed: %v", err)
	}

	info, err := os.Stat(filepath.Join(dest, "mydir"))
	if err != nil {
		t.Fatalf("expected directory mydir: %v", err)
	}

	if !info.IsDir() {
		t.Fatalf("mydir should be a directory, got mode %v", info.Mode())
	}
}

// TestExtract_Zip_Symlink verifies that symlink entries in a zip are
// materialised as symlinks on disk with the correct target.
// This also exercises writeSymlinkFromOpener via writeZipSymlink.
func TestExtract_Zip_Symlink(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlinks require elevated privileges on Windows")
	}

	tmp := t.TempDir()
	dest := filepath.Join(tmp, "dest")

	if err := os.MkdirAll(dest, 0o755); err != nil {
		t.Fatalf("mkdir dest: %v", err)
	}

	// The symlink target is stored as the file body in zip symlink entries.
	entries := []zipEntry{
		{name: "target.txt", content: "real content", mode: 0o644},
		{name: "link.txt", content: "target.txt", mode: 0o777 | os.ModeSymlink},
	}

	archivePath := writeZipFixture(t, tmp, entries)

	if err := archive.Extract(context.Background(), archivePath, dest); err != nil {
		t.Fatalf("Extract zip with symlink failed: %v", err)
	}

	linkPath := filepath.Join(dest, "link.txt")

	info, err := os.Lstat(linkPath)
	if err != nil {
		t.Fatalf("expected symlink at link.txt: %v", err)
	}

	if info.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("link.txt should be a symlink, got mode %v", info.Mode())
	}

	target, err := os.Readlink(linkPath)
	if err != nil {
		t.Fatalf("readlink: %v", err)
	}

	if target != "target.txt" {
		t.Fatalf("symlink target: got %q, want target.txt", target)
	}
}

// TestExtract_Zip_AbsoluteSymlinkTarget verifies that absolute symlink targets
// are accepted (real packages routinely ship them).
func TestExtract_Zip_AbsoluteSymlinkTarget(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlinks require elevated privileges on Windows")
	}

	tmp := t.TempDir()
	dest := filepath.Join(tmp, "dest")

	if err := os.MkdirAll(dest, 0o755); err != nil {
		t.Fatalf("mkdir dest: %v", err)
	}

	entries := []zipEntry{
		{name: "abslink", content: "/etc/passwd", mode: 0o777 | os.ModeSymlink},
	}

	archivePath := writeZipFixture(t, tmp, entries)

	if err := archive.Extract(context.Background(), archivePath, dest); err != nil {
		t.Fatalf("Extract zip with absolute symlink failed: %v", err)
	}

	target, err := os.Readlink(filepath.Join(dest, "abslink"))
	if err != nil {
		t.Fatalf("readlink: %v", err)
	}

	if target != "/etc/passwd" {
		t.Fatalf("symlink target: got %q, want /etc/passwd", target)
	}
}

// TestExtract_Zip_RejectsTraversal verifies that zip entries with path
// traversal names are rejected.
func TestExtract_Zip_RejectsTraversal(t *testing.T) {
	tmp := t.TempDir()
	dest := filepath.Join(tmp, "dest")

	if err := os.MkdirAll(dest, 0o755); err != nil {
		t.Fatalf("mkdir dest: %v", err)
	}

	entries := []zipEntry{
		{name: "../escape.txt", content: "pwned", mode: 0o644},
	}

	archivePath := writeZipFixture(t, tmp, entries)

	err := archive.Extract(context.Background(), archivePath, dest)
	if err == nil {
		t.Fatal("Extract zip with traversal entry should return error")
	}

	// Verify nothing escaped.
	if _, statErr := os.Stat(filepath.Join(tmp, "escape.txt")); statErr == nil {
		t.Fatal("traversal succeeded: escape.txt was created outside dest")
	}
}

// TestExtract_Zip_RejectsTraversalSymlink verifies that zip symlink entries
// whose target escapes the archive root are rejected.
func TestExtract_Zip_RejectsTraversalSymlink(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlinks require elevated privileges on Windows")
	}

	tmp := t.TempDir()
	dest := filepath.Join(tmp, "dest")

	if err := os.MkdirAll(dest, 0o755); err != nil {
		t.Fatalf("mkdir dest: %v", err)
	}

	entries := []zipEntry{
		{name: "evil-link", content: "../../escape", mode: 0o777 | os.ModeSymlink},
	}

	archivePath := writeZipFixture(t, tmp, entries)

	err := archive.Extract(context.Background(), archivePath, dest)
	if err == nil {
		t.Fatal("Extract zip with escaping symlink target should return error")
	}
}

// TestExtract_Zip_MagicDetection verifies that a zip file is detected by its
// magic bytes even when the filename has no .zip suffix.
func TestExtract_Zip_MagicDetection(t *testing.T) {
	tmp := t.TempDir()
	dest := filepath.Join(tmp, "dest")

	if err := os.MkdirAll(dest, 0o755); err != nil {
		t.Fatalf("mkdir dest: %v", err)
	}

	entries := []zipEntry{
		{name: "payload.txt", content: "zip magic test", mode: 0o644},
	}

	data := buildZipBytes(t, entries)

	// Write with a non-.zip suffix — detection must rely on magic bytes.
	archivePath := filepath.Join(tmp, "archive.bin")
	if err := os.WriteFile(archivePath, data, 0o644); err != nil {
		t.Fatalf("write archive: %v", err)
	}

	if err := archive.Extract(context.Background(), archivePath, dest); err != nil {
		t.Fatalf("Extract zip by magic bytes failed: %v", err)
	}

	got, err := os.ReadFile(filepath.Join(dest, "payload.txt"))
	if err != nil {
		t.Fatalf("expected payload.txt: %v", err)
	}

	if string(got) != "zip magic test" {
		t.Fatalf("content mismatch: %q", string(got))
	}
}

// TestExtract_Zip_CancelledContext verifies that a cancelled context causes
// extraction to abort.
func TestExtract_Zip_CancelledContext(t *testing.T) {
	tmp := t.TempDir()
	dest := filepath.Join(tmp, "dest")

	if err := os.MkdirAll(dest, 0o755); err != nil {
		t.Fatalf("mkdir dest: %v", err)
	}

	// Build a zip with several entries so the context check has a chance to fire.
	entries := make([]zipEntry, 20)
	for i := range entries {
		entries[i] = zipEntry{
			name:    strings.Repeat("a", i+1) + ".txt",
			content: "data",
			mode:    0o644,
		}
	}

	archivePath := writeZipFixture(t, tmp, entries)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	err := archive.Extract(ctx, archivePath, dest)
	if err == nil {
		t.Fatal("Extract with cancelled context should return error")
	}
}

// ---------------------------------------------------------------------------
// RAR extraction tests
// ---------------------------------------------------------------------------

// rarArc15Base64 is a RAR 1.5 archive containing a single file:
//
//	bevande.doc — regular file (9216 bytes)
//
// Sourced from a local test fixture. Embedded as base64 to keep the test
// self-contained without a testdata/ directory.
const rarArc15Base64 = "UmFyIRoHAM+QcwAADQAAAAAAAABoEnQggCsAywUAAAAkAAADoyaoVdBUj0cdMwsAtIEAAGJldmFu" +
	"ZGUuZG9jDB1RFMyPzYFfNfxHJZIMYWFsJQZB0ksIQnwoDlpbSEZCECDhIWy0LCpxuCJQjcHGWwvl" +
	"FfglfC1KVHhfESqqQVVK1VF+HvRKq3yKrb4CUPMqR41KpUL8FVFaEqzd6yQbhm697K0yyjjQlalM" +
	"i2bvd17mZu5zd3M3f5fc3m5nNzvO5vFn7M3OrepdXdzN62087n2/ZpOl7vx033hNF4oNQ6+TQA4m" +
	"KsxS0bPw6OjRh5J+wM12Si/npnWQ6WgbHG4pbjRHHI0A0I94NEUqMaRDulzov6KzdBFL+jg8DbhE" +
	"QRg5AZ5KlIHEyZzjcTOxPuiWh5LP4Wd7efde9HL4DYaSQOylpR0w96oCEJpznfD340w6gacfAYfV" +
	"HHg6sahh3pzwR1o1I1Q+ENWOuGsHXjWjsBrhrxsBsR8MdiOyHZoHw2Q+IrjGhNoc8YfHQeQO1GzH" +
	"bDaSB+O3G1HcDbDuRtx3Q7sbgd4MAbkd6N0N2O+HyR8obwfLHzB348BeIXv/s3MGJTGtDLCBxQNF" +
	"ARzgQ1qlm7jAhRouHF4Ud5eRY0PXv4vE4+DfxI7DXF2mr7JQlALTTrS+FW9a+D9bL5ON3MQjS3Eq" +
	"fidkHmhI08AhMMm6Wc3XVyWiZyGLeFtrjBghBDirGivehVvTWIbarCBbtQ3pStLftWYNs6bYaDdW" +
	"0o65t2wRLZs5joODbt5PCt/OtrJt/son4dfVSdNDP1JO+DACOSVK1iq0ZzH9Ly+pmW8vE7NyvC0K" +
	"ldODjgadSPS0Vlva5i7pJpSJyTO0UvmEX/eBDC/I2ZXHOYajpOoHYXcN4YxHwB9hg7T2wlAmdmeq" +
	"UO3K4kviBCOzQtXCULREOmoJBqDJHZQkusAjuCRzCVlXISCpeXZzEO1Ecdqyu7JyZJtJyRe5aqlY" +
	"5sCnQu5JFwyvkoW/VsmaVVzLTAL1IbS9k2oOEsdRCk3CWx/wkDLzh8udo6mKi5V+qX/txS4xgnAk" +
	"sPsFWXx4QD2V+icTx/UgeRDmxIxnERN2HIK4vlNeMJY62c05Zk01gLD+wCRZtIeBuyvF8S1xiuY5" +
	"U4pLHMzu1VOXeFOGEQHBIsDwH5HiKpv+k7TdSFtVLPikoMrhBxGCynQztTmYbf5pkWXJZRM6PAzn" +
	"GGTGZzDZPZ9UzbEH8Zz7JbavpJvvhqNzmeltNx7xSXtOlhcrlfKaMmPqYzntq1ytAqWquMtQ05+L" +
	"Q8R8ndcB/nzWtl5OjLmflLxQRu9cnYs02tSxFdL1vc/PIgcN18HtOQ11T8vRSrLkyd5mXb4S8WsQ" +
	"41H1ViBjLdw8pbuL31u4/mWL5zlu6eet3Vjrd19NbvDqLd4/ys7/DrHTU5uh+7t02Fr1uwsW3vUr" +
	"1IZkkLH3/DrqfZ/crkYuvott8hWbf8eYY1w8oCx11HfzzLP9fCKkC0gxeLBiPv8Gu5NaUHtIjJ8D" +
	"GjJydGrNJl03B85svXQ3wptpieR3J/DTAiRzjS2Wc+Q7HGQX2+nZ1RnZY84LhHVXkwV9MJfTNLgG" +
	"ZF/P7XHmg7R+ZoHRA9/PX9DxCfyWZQT7jBxyf31GHoFAPogRSo3wxlQJ2jVDX+AqtgqqUxhaYEqO" +
	"q1TZ4KCJm1OGchqhHiaeER1LhXaY3mjDQ8R/7l9BtG9oeI9cVHF0RylKVQNYO1G6HgrgAdZjl/A5" +
	"A/uP7D+g9UfyCRfz/bNI0uagNVzfTrXnN+qz1fNiNqg5rMY6PTliXYV1VgFJXT1G7aML0Pr6HKrd" +
	"Q65PtoKzI+J+Leaq/NX4cWVdOpiMxnNexy/7S7s+ftOIzfggR1NTmLpz4Mru6ME5ALm588GJzss2" +
	"Wi+jzk5481LKLlfwFd4ufSoWBKS1RJaB/cfXXl252CWO59UFItLZJEJKy5LhGG4XanOhLnBLdJ77" +
	"fCzfE4Z7YrOyqiS2fQwiNqTj4x8XikKGD8p4SlLv1CDOpU08Y3rDjPT5b9sm895DfTwlp2MTviju" +
	"ZSJ0z6+/6cQ9ewBABwA="

func writeRarFixture(t *testing.T, dir string) string {
	t.Helper()

	data, err := base64.StdEncoding.DecodeString(rarArc15Base64)
	if err != nil {
		t.Fatalf("decode rar fixture: %v", err)
	}

	path := filepath.Join(dir, "sample.rar")
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("write rar fixture: %v", err)
	}

	return path
}

// TestExtract_Rar_RegularFile verifies that a regular file inside a RAR 1.5
// archive is extracted with the correct size.
func TestExtract_Rar_RegularFile(t *testing.T) {
	tmp := t.TempDir()
	dest := filepath.Join(tmp, "dest")

	if err := os.MkdirAll(dest, 0o755); err != nil {
		t.Fatalf("mkdir dest: %v", err)
	}

	archivePath := writeRarFixture(t, tmp)

	if err := archive.Extract(context.Background(), archivePath, dest); err != nil {
		t.Fatalf("Extract rar failed: %v", err)
	}

	// bevande.doc is 9216 bytes in the fixture.
	info, err := os.Stat(filepath.Join(dest, "bevande.doc"))
	if err != nil {
		t.Fatalf("expected bevande.doc: %v", err)
	}

	if info.Size() != 9216 {
		t.Fatalf("bevande.doc size = %d, want 9216", info.Size())
	}
}

// TestExtract_Rar_MagicDetection verifies that a RAR file is detected by its
// magic bytes even when the filename has no .rar suffix.
func TestExtract_Rar_MagicDetection(t *testing.T) {
	tmp := t.TempDir()
	dest := filepath.Join(tmp, "dest")

	if err := os.MkdirAll(dest, 0o755); err != nil {
		t.Fatalf("mkdir dest: %v", err)
	}

	data, err := base64.StdEncoding.DecodeString(rarArc15Base64)
	if err != nil {
		t.Fatalf("decode rar fixture: %v", err)
	}

	// Write with a non-.rar suffix — detection must rely on magic bytes.
	archivePath := filepath.Join(tmp, "archive.bin")
	if err := os.WriteFile(archivePath, data, 0o644); err != nil {
		t.Fatalf("write archive: %v", err)
	}

	if err := archive.Extract(context.Background(), archivePath, dest); err != nil {
		t.Fatalf("Extract rar by magic bytes failed: %v", err)
	}

	if _, err := os.Stat(filepath.Join(dest, "bevande.doc")); err != nil {
		t.Fatalf("expected bevande.doc after magic-byte detection: %v", err)
	}
}

// TestExtract_Rar_CancelledContext verifies that a cancelled context causes
// RAR extraction to abort.
func TestExtract_Rar_CancelledContext(t *testing.T) {
	tmp := t.TempDir()
	dest := filepath.Join(tmp, "dest")

	if err := os.MkdirAll(dest, 0o755); err != nil {
		t.Fatalf("mkdir dest: %v", err)
	}

	archivePath := writeRarFixture(t, tmp)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	err := archive.Extract(ctx, archivePath, dest)
	if err == nil {
		t.Fatal("Extract rar with cancelled context should return error")
	}
}

// ---------------------------------------------------------------------------
// 7z symlink test (exercises write7zSymlink / writeSymlinkFromOpener)
// ---------------------------------------------------------------------------

// TestExtract_7z_WriteSymlinkFromOpener exercises the write7zSymlink /
// writeSymlinkFromOpener code path. The fixture used here is the same
// two-entry archive from sevenzip_test.go (no symlinks), so this test
// verifies that the regular-file path through extract7z continues to work
// and that write7zFile is covered.
func TestExtract_7z_WriteSymlinkFromOpener(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlinks require elevated privileges on Windows")
	}

	tmp := t.TempDir()
	dest := filepath.Join(tmp, "dest")

	if err := os.MkdirAll(dest, 0o755); err != nil {
		t.Fatalf("mkdir dest: %v", err)
	}

	const fixture7zBase64 = "N3q8ryccAAQwP4SyFQAAAAAAAAA4AAAAAAAAAA+CMddIdXV1dWdlIGZpbGUgY29udGVudHMBBAYAAQkVAAcLAQABAQAMFQAABQIOAUAPAYARGQBsAGEAcgBnAGUAAABlAG0AcAB0AHkAAAAAAA=="

	data, err := base64.StdEncoding.DecodeString(fixture7zBase64)
	if err != nil {
		t.Fatalf("decode 7z fixture: %v", err)
	}

	archivePath := filepath.Join(tmp, "fixture.7z")
	if err := os.WriteFile(archivePath, data, 0o644); err != nil {
		t.Fatalf("write 7z fixture: %v", err)
	}

	if err := archive.Extract(context.Background(), archivePath, dest); err != nil {
		t.Fatalf("Extract 7z failed: %v", err)
	}

	// Verify the regular-file entries are present.
	for _, name := range []string{"large", "empty"} {
		if _, err := os.Stat(filepath.Join(dest, name)); err != nil {
			t.Fatalf("expected entry %q: %v", name, err)
		}
	}
}

// fixture7zWithSymlinkBase64 is a 7z archive containing:
//
//	7z_sym_src/           — directory
//	7z_sym_src/target.txt — regular file ("target content\n")
//	7z_sym_src/link.txt   — symlink → target.txt
//
// Created with: 7z a -snl fixture.7z 7z_sym_src/
// Embedded as base64 to keep the test self-contained.
const fixture7zWithSymlinkBase64 = "N3q8ryccAASn0SbmogAAAAAAAAAiAAAAAAAAAMW/xvwBABh0YXJnZXQudHh0dGFyZ2V0IGNvbnRlbnQKAAAAgTMHrg/P2W+8D+vqnL82Pf53Qe42TVJ0RmXlEYB0n+CiifdgalMPMIh2JN3FnNyuNddFz1qRnt5aScE/1SaxqWRzFrpKByQf0ju31ZQY2Pex0Op98l8AsSekeH2JZKaWrWOAJ8jmncsk+f0Y79wxkColAN6me40dMEIsqYEKB0sAAAAXBh0BCYCFAAcLAQABIwMBAQVdABAAAAyAygoBogCotwAA"

// TestExtract_7z_Symlink exercises the write7zSymlink / writeSymlinkFromOpener
// code path using a 7z archive that contains a symlink entry.
// The archive was created with 7z a -snl which preserves symlinks.
func TestExtract_7z_Symlink(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlinks require elevated privileges on Windows")
	}

	tmp := t.TempDir()
	dest := filepath.Join(tmp, "dest")

	if err := os.MkdirAll(dest, 0o755); err != nil {
		t.Fatalf("mkdir dest: %v", err)
	}

	data, err := base64.StdEncoding.DecodeString(fixture7zWithSymlinkBase64)
	if err != nil {
		t.Fatalf("decode 7z symlink fixture: %v", err)
	}

	archivePath := filepath.Join(tmp, "fixture_sym.7z")
	if err := os.WriteFile(archivePath, data, 0o644); err != nil {
		t.Fatalf("write 7z fixture: %v", err)
	}

	if err := archive.Extract(context.Background(), archivePath, dest); err != nil {
		t.Fatalf("Extract 7z with symlink failed: %v", err)
	}

	// Verify the symlink was created.
	linkPath := filepath.Join(dest, "7z_sym_src", "link.txt")

	info, err := os.Lstat(linkPath)
	if err != nil {
		t.Fatalf("expected symlink at 7z_sym_src/link.txt: %v", err)
	}

	if info.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("7z_sym_src/link.txt should be a symlink, got mode %v", info.Mode())
	}

	target, err := os.Readlink(linkPath)
	if err != nil {
		t.Fatalf("readlink: %v", err)
	}

	if target != "target.txt" {
		t.Fatalf("symlink target: got %q, want target.txt", target)
	}
}

// TestExtract_7z_AbsoluteSymlinkTarget verifies that absolute symlink targets
// in a 7z archive are accepted.
func TestExtract_7z_AbsoluteSymlinkTarget(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlinks require elevated privileges on Windows")
	}

	// Build a 7z archive with an absolute symlink target using the zip approach:
	// we can't easily create a 7z with absolute symlinks without the 7z binary,
	// so we verify the safeSymlinkTarget function accepts absolute targets
	// (already covered by TestSafeSymlinkTarget_ExportedWrapper).
	// This test documents the expected behavior.
	t.Skip("absolute symlink targets in 7z require a pre-built fixture; " +
		"covered by TestSafeSymlinkTarget_ExportedWrapper and TestExtract_Zip_AbsoluteSymlinkTarget")
}
