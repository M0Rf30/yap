package archive_test

import (
	"context"
	"encoding/base64"
	"os"
	"path/filepath"
	"testing"

	"github.com/M0Rf30/yap/v2/pkg/archive"
)

// Pre-baked 7z fixture: a 109-byte archive containing two entries:
//
//	large  - regular file, 21 bytes ("Huge file contents")
//	empty  - regular file, 0 bytes
//
// Sourced from github.com/bodgit/sevenzip's own testdata (file_and_empty.7z).
// Embedded as base64 to keep the test self-contained without a testdata/ dir.
const fixture7zBase64 = "N3q8ryccAAQwP4SyFQAAAAAAAAA4AAAAAAAAAA+CMddIdXV1dWdlIGZpbGUgY29udGVudHMBBAYAAQkVAAcLAQABAQAMFQAABQIOAUAPAYARGQBsAGEAcgBnAGUAAABlAG0AcAB0AHkAAAAAAA=="

func write7zFixture(t *testing.T, dir string) string {
	t.Helper()

	data, err := base64.StdEncoding.DecodeString(fixture7zBase64)
	if err != nil {
		t.Fatalf("decode fixture: %v", err)
	}

	path := filepath.Join(dir, "fixture.7z")
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	return path
}

// TestExtract_SevenZip verifies the .7z extraction path: format detection
// reaches extract7z, both regular-file entries materialise on disk with their
// expected sizes, and no zip-slip / decoder errors surface.
func TestExtract_SevenZip(t *testing.T) {
	tmp := t.TempDir()
	archivePath := write7zFixture(t, tmp)
	dest := filepath.Join(tmp, "dest")

	if err := os.MkdirAll(dest, 0o755); err != nil {
		t.Fatalf("mkdir dest: %v", err)
	}

	if err := archive.Extract(context.Background(), archivePath, dest); err != nil {
		t.Fatalf("Extract on .7z failed: %v", err)
	}

	cases := []struct {
		name string
		size int64
	}{
		{"large", 21},
		{"empty", 0},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			info, err := os.Stat(filepath.Join(dest, tc.name))
			if err != nil {
				t.Fatalf("expected entry %q missing: %v", tc.name, err)
			}

			if info.Size() != tc.size {
				t.Fatalf("entry %q size = %d, want %d", tc.name, info.Size(), tc.size)
			}
		})
	}
}

// TestExtract_SevenZipMagic guards the magic-byte detection: a file whose
// content begins with the 7z signature must be routed to extract7z rather
// than the tar fallback.
func TestExtract_SevenZipMagic(t *testing.T) {
	tmp := t.TempDir()
	archivePath := write7zFixture(t, tmp)
	dest := filepath.Join(tmp, "dest")

	if err := os.MkdirAll(dest, 0o755); err != nil {
		t.Fatalf("mkdir dest: %v", err)
	}

	// Rename to remove .7z suffix — detection must rely on the 7z magic
	// bytes ("7z" 0xBC 0xAF 0x27 0x1C), not the filename.
	renamed := filepath.Join(tmp, "fixture.bin")
	if err := os.Rename(archivePath, renamed); err != nil {
		t.Fatalf("rename: %v", err)
	}

	if err := archive.Extract(context.Background(), renamed, dest); err != nil {
		t.Fatalf("Extract on suffixless 7z failed: %v", err)
	}

	if _, err := os.Stat(filepath.Join(dest, "large")); err != nil {
		t.Fatalf("payload missing after magic-byte detection: %v", err)
	}
}
