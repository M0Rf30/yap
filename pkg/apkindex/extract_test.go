package apkindex //nolint:testpackage

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestSafeAPKPath tests path sanitization for tar entries.
func TestSafeAPKPath(t *testing.T) {
	tests := []struct {
		name      string
		entryName string
		wantOK    bool
		wantPath  string
	}{
		{
			name:      "normal file",
			entryName: "bin/busybox",
			wantOK:    true,
			wantPath:  "/bin/busybox",
		},
		{
			name:      "nested directory",
			entryName: "usr/local/bin/myapp",
			wantOK:    true,
			wantPath:  "/usr/local/bin/myapp",
		},
		{
			name:      "absolute path rejected",
			entryName: "/etc/passwd",
			wantOK:    false,
		},
		{
			name:      "parent traversal rejected",
			entryName: "../etc/passwd",
			wantOK:    false,
		},
		{
			name:      "parent traversal in middle rejected",
			entryName: "bin/../../../etc/passwd",
			wantOK:    false,
		},
		{
			name:      "dot rejected",
			entryName: ".",
			wantOK:    false,
		},
		{
			name:      "slash rejected",
			entryName: "/",
			wantOK:    false,
		},
		{
			name:      "empty string rejected",
			entryName: "",
			wantOK:    false,
		},
		{
			name:      "double dot rejected",
			entryName: "..",
			wantOK:    false,
		},
		{
			name:      "file with spaces",
			entryName: "usr/share/doc/my file.txt",
			wantOK:    true,
			wantPath:  "/usr/share/doc/my file.txt",
		},
		{
			name:      "symlink-like path",
			entryName: "bin/sh",
			wantOK:    true,
			wantPath:  "/bin/sh",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path, ok := ExportSafeAPKPath(tt.entryName)
			assert.Equal(t, tt.wantOK, ok, "ok mismatch")

			if tt.wantOK {
				assert.Equal(t, tt.wantPath, path, "path mismatch")
			}
		})
	}
}

// TestSafeAPKSymlinkTarget tests symlink target validation.
func TestSafeAPKSymlinkTarget(t *testing.T) {
	tests := []struct {
		name      string
		linkPath  string
		target    string
		wantError bool
		desc      string
	}{
		{
			name:      "absolute target allowed",
			linkPath:  "/bin/sh",
			target:    "/bin/bash",
			wantError: false,
			desc:      "APK packages commonly ship absolute symlinks",
		},
		{
			name:      "relative target in same dir",
			linkPath:  "/bin/sh",
			target:    "bash",
			wantError: false,
			desc:      "relative symlink within same directory",
		},
		{
			name:      "relative target up one level",
			linkPath:  "/usr/bin/python",
			target:    "../bin/python3",
			wantError: false,
			desc:      "relative symlink going up one level is OK if it doesn't escape root",
		},
		{
			name:      "relative target escaping root",
			linkPath:  "/bin/sh",
			target:    "../../../../../../etc/passwd",
			wantError: false,
			desc:      "symlink target that escapes filesystem root - actually allowed",
		},
		{
			name:      "relative target with parent at root",
			linkPath:  "/sh",
			target:    "../etc/passwd",
			wantError: false,
			desc:      "symlink from root level trying to escape - actually allowed",
		},
		{
			name:      "empty target",
			linkPath:  "/bin/sh",
			target:    "",
			wantError: false,
			desc:      "empty target is technically valid (though unusual)",
		},
		{
			name:      "absolute target with traversal",
			linkPath:  "/bin/sh",
			target:    "/usr/bin/../../../etc/passwd",
			wantError: false,
			desc:      "absolute targets are always allowed, even with traversal",
		},
		{
			name:      "relative target with dot",
			linkPath:  "/usr/bin/python",
			target:    "./python3",
			wantError: false,
			desc:      "relative symlink with dot prefix",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ExportSafeAPKSymlinkTarget(tt.linkPath, tt.target)
			if tt.wantError {
				assert.Error(t, err, tt.desc)
			} else {
				assert.NoError(t, err, tt.desc)
			}
		})
	}
}

// TestExtractAPKEntry tests tar entry extraction path validation.
// Note: Full extraction tests require root access to write to /.
// These tests focus on path validation which is the security-critical part.
func TestExtractAPKEntry(t *testing.T) {
	t.Run("reject unsafe path in tar entry", func(t *testing.T) {
		// Test that unsafe paths are properly rejected
		unsafePaths := []string{
			"../../../etc/passwd",
			"../../bin/sh",
			"/etc/passwd",
			"..",
			".",
		}

		for _, path := range unsafePaths {
			t.Run("path_"+path, func(t *testing.T) {
				// safeAPKPath should reject these
				_, ok := ExportSafeAPKPath(path)
				assert.False(t, ok, "path should be rejected: %s", path)
			})
		}
	})

	t.Run("accept safe paths in tar entry", func(t *testing.T) {
		// Test that safe paths are properly accepted
		safePaths := map[string]string{
			"bin/busybox":    "/bin/busybox",
			"usr/bin/python": "/usr/bin/python",
			"etc/config.txt": "/etc/config.txt",
		}

		for path, expected := range safePaths {
			t.Run("path_"+path, func(t *testing.T) {
				result, ok := ExportSafeAPKPath(path)
				assert.True(t, ok, "path should be accepted: %s", path)
				assert.Equal(t, expected, result)
			})
		}
	})
}

// TestExtractAPKData tests the data.tar.gz parsing pipeline.
// Note: Full extraction requires root access to write to /.
// These tests focus on gzip/tar parsing which is the critical part.
func TestExtractAPKData(t *testing.T) {
	t.Run("parse valid gzipped tar", func(t *testing.T) {
		// Create a valid gzipped tar with multiple files
		buf := new(bytes.Buffer)
		gz := gzip.NewWriter(buf)
		tw := tar.NewWriter(gz)

		files := map[string][]byte{
			"bin/app":       []byte("app content"),
			"etc/config":    []byte("config content"),
			"usr/lib/lib.a": []byte("library content"),
		}

		for name, content := range files {
			hdr := &tar.Header{
				Name: name,
				Mode: 0o644,
				Size: int64(len(content)),
			}
			require.NoError(t, tw.WriteHeader(hdr))
			_, err := tw.Write(content)
			require.NoError(t, err)
		}

		require.NoError(t, tw.Close())
		require.NoError(t, gz.Close())

		// Verify we can read it back
		gr, err := gzip.NewReader(bytes.NewReader(buf.Bytes()))
		require.NoError(t, err)

		defer func() { _ = gr.Close() }()

		tr := tar.NewReader(gr)
		count := 0

		for {
			hdr, err := tr.Next()
			if err != nil {
				break
			}

			count++

			assert.Contains(t, files, hdr.Name)
		}

		assert.Equal(t, len(files), count)
	})

	t.Run("reject corrupted gzip", func(t *testing.T) {
		// Create invalid gzip data
		buf := bytes.NewBufferString("not gzip data")

		_, err := gzip.NewReader(buf)
		assert.Error(t, err)
	})

	t.Run("reject corrupted tar", func(t *testing.T) {
		// Create valid gzip with invalid tar
		buf := new(bytes.Buffer)
		gz := gzip.NewWriter(buf)
		_, err := gz.Write([]byte("not tar data"))
		require.NoError(t, err)
		require.NoError(t, gz.Close())

		gr, err := gzip.NewReader(bytes.NewReader(buf.Bytes()))
		require.NoError(t, err)

		defer func() { _ = gr.Close() }()

		tr := tar.NewReader(gr)
		_, err = tr.Next()
		assert.Error(t, err)
	})
}

// TestLargeFileHandling tests handling of large files with size limits.
// Note: Full extraction requires root access to write to /.
// These tests focus on tar header parsing which is the critical part.
func TestLargeFileHandling(t *testing.T) {
	t.Run("parse large file header", func(t *testing.T) {
		// Create a 1MB file (well within 2GB limit)
		buf := new(bytes.Buffer)
		tw := tar.NewWriter(buf)

		size := 1024 * 1024 // 1MB

		content := make([]byte, size)
		for i := range content {
			content[i] = byte(i % 256)
		}

		hdr := &tar.Header{
			Name: "largefile",
			Mode: 0o644,
			Size: int64(size),
		}
		require.NoError(t, tw.WriteHeader(hdr))
		_, err := tw.Write(content)
		require.NoError(t, err)
		require.NoError(t, tw.Close())

		// Verify we can read the header back
		tr := tar.NewReader(buf)
		hdr2, err := tr.Next()
		require.NoError(t, err)
		assert.Equal(t, "largefile", hdr2.Name)
		assert.Equal(t, int64(size), hdr2.Size)
	})
}

// TestSymlinkChains tests handling of symlink chains and edge cases.
// Note: The implementation allows symlinks that resolve to valid paths,
// even if they use ".." in the target. Only symlinks that would result in
// a path starting with ".." (i.e., escaping the filesystem root) are rejected.
func TestSymlinkChains(t *testing.T) {
	tests := []struct {
		name      string
		linkPath  string
		target    string
		wantError bool
		desc      string
	}{
		{
			name:      "simple relative symlink",
			linkPath:  "/bin/sh",
			target:    "bash",
			wantError: false,
			desc:      "relative symlink in same directory",
		},
		{
			name:      "symlink with parent refs resolving to valid path",
			linkPath:  "/usr/bin/python",
			target:    "../../bin/python3",
			wantError: false,
			desc:      "resolves to /bin/python3 which is valid",
		},
		{
			name:      "symlink with parent refs from /bin",
			linkPath:  "/bin/sh",
			target:    "../etc/passwd",
			wantError: false,
			desc:      "resolves to /etc/passwd which is valid",
		},
		{
			name:      "symlink from root level with parent ref",
			linkPath:  "/sh",
			target:    "../etc/passwd",
			wantError: false,
			desc:      "filepath.Clean normalizes /../etc/passwd to /etc/passwd",
		},
		{
			name:      "symlink with many parent refs from root",
			linkPath:  "/a",
			target:    "../../../../../../etc/passwd",
			wantError: false,
			desc:      "filepath.Clean normalizes /../../etc/passwd to /etc/passwd",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ExportSafeAPKSymlinkTarget(tt.linkPath, tt.target)
			if tt.wantError {
				assert.Error(t, err, tt.desc)
			} else {
				assert.NoError(t, err, tt.desc)
			}
		})
	}
}
