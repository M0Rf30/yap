// SPDX-FileCopyrightText: 2021-2026 Gianluca Boiano
// SPDX-License-Identifier: GPL-3.0-only

package archive_test

import (
	"archive/tar"
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/klauspost/compress/zstd"

	"github.com/M0Rf30/yap/v2/pkg/archive"
)

// writeTarZst writes a .tar.zst holding one regular file.
func writeTarZst(t *testing.T, path, name, content string) {
	t.Helper()

	out, err := os.Create(path) //nolint:gosec
	if err != nil {
		t.Fatalf("create archive: %v", err)
	}

	defer func() { _ = out.Close() }()

	zw, err := zstd.NewWriter(out)
	if err != nil {
		t.Fatalf("zstd writer: %v", err)
	}

	tw := tar.NewWriter(zw)

	hdr := &tar.Header{Name: name, Mode: 0o644, Size: int64(len(content)), Typeflag: tar.TypeReg}
	if err := tw.WriteHeader(hdr); err != nil {
		t.Fatalf("write header: %v", err)
	}

	if _, err := tw.Write([]byte(content)); err != nil {
		t.Fatalf("write body: %v", err)
	}

	if err := tw.Close(); err != nil {
		t.Fatalf("close tar: %v", err)
	}

	if err := zw.Close(); err != nil {
		t.Fatalf("close zstd: %v", err)
	}
}

func TestExtract(t *testing.T) {
	tempDir := t.TempDir()
	archiveFile := filepath.Join(tempDir, "test.tar.zst")
	extractDir := filepath.Join(tempDir, "extract")

	writeTarZst(t, archiveFile, "test.txt", "test content")

	if err := os.MkdirAll(extractDir, 0o755); err != nil {
		t.Fatalf("Failed to create extract directory: %v", err)
	}

	if err := archive.Extract(context.Background(), archiveFile, extractDir); err != nil {
		t.Fatalf("Extract failed: %v", err)
	}

	content, err := os.ReadFile(filepath.Join(extractDir, "test.txt")) //nolint:gosec
	if err != nil {
		t.Fatalf("Failed to read extracted file: %v", err)
	}

	if string(content) != "test content" {
		t.Fatalf("Extracted file content mismatch. Got: %s, Expected: test content", string(content))
	}
}

func TestExtractInvalidArchive(t *testing.T) {
	extractDir := filepath.Join(t.TempDir(), "extract")

	if err := os.MkdirAll(extractDir, 0o755); err != nil {
		t.Fatalf("Failed to create extract directory: %v", err)
	}

	err := archive.Extract(context.Background(), "/non/existent/archive.tar.zst", extractDir)
	if err == nil {
		t.Fatal("Expected error for invalid archive file, got nil")
	}
}
