package dnfinstall //nolint:testpackage

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestFindLDConfig(t *testing.T) {
	t.Run("missing in sandbox root returns empty", func(t *testing.T) {
		root := t.TempDir()
		if got := findLDConfig(root); got != "" {
			t.Fatalf("findLDConfig(empty sandbox) = %q, want \"\"", got)
		}
	})

	t.Run("found in sandbox root returns relative candidate", func(t *testing.T) {
		root := t.TempDir()
		if err := os.MkdirAll(filepath.Join(root, "sbin"), 0o755); err != nil {
			t.Fatal(err)
		}

		if err := os.WriteFile(filepath.Join(root, "sbin", "ldconfig"), []byte("#!/bin/sh\n"), 0o755); err != nil { //nolint:gosec
			t.Fatal(err)
		}

		got := findLDConfig(root)
		if got != "/sbin/ldconfig" {
			t.Fatalf("findLDConfig = %q, want /sbin/ldconfig", got)
		}
	})

	t.Run("prefers /sbin over /usr/sbin", func(t *testing.T) {
		root := t.TempDir()
		for _, d := range []string{"sbin", "usr/sbin"} {
			if err := os.MkdirAll(filepath.Join(root, d), 0o755); err != nil {
				t.Fatal(err)
			}

			if err := os.WriteFile(filepath.Join(root, d, "ldconfig"), []byte("x"), 0o755); err != nil { //nolint:gosec
				t.Fatal(err)
			}
		}

		if got := findLDConfig(root); got != "/sbin/ldconfig" {
			t.Fatalf("findLDConfig = %q, want /sbin/ldconfig", got)
		}
	})
}

func TestRunLDConfig(t *testing.T) {
	t.Run("missing binary is non-fatal", func(t *testing.T) {
		// Empty sandbox root: no ldconfig inside → returns nil (skipped).
		if err := runLDConfig(context.Background(), t.TempDir()); err != nil {
			t.Fatalf("runLDConfig(no binary) = %v, want nil", err)
		}
	})

	t.Run("non-root with sandbox root skips chroot path", func(t *testing.T) {
		if os.Getuid() == 0 {
			t.Skip("running as root; chroot path would be taken")
		}

		root := t.TempDir()
		if err := os.MkdirAll(filepath.Join(root, "sbin"), 0o755); err != nil {
			t.Fatal(err)
		}
		// A real-looking but non-executable file is enough: findLDConfig
		// locates it, then runLDConfig returns early because uid != 0.
		if err := os.WriteFile(filepath.Join(root, "sbin", "ldconfig"), []byte("x"), 0o644); err != nil { //nolint:gosec
			t.Fatal(err)
		}

		if err := runLDConfig(context.Background(), root); err != nil {
			t.Fatalf("runLDConfig(non-root sandbox) = %v, want nil", err)
		}
	})
}

func TestToRPMDBFiles(t *testing.T) {
	in := []installedFile{
		{
			Path:       "/usr/bin/foo",
			Mode:       0o755,
			Size:       42,
			SHA256:     "deadbeef",
			LinkTarget: "",
		},
		{
			Path:       "/usr/bin/bar",
			Mode:       0o777,
			Size:       7,
			LinkTarget: "/usr/bin/foo",
		},
	}

	out := toRPMDBFiles(in)
	if len(out) != len(in) {
		t.Fatalf("toRPMDBFiles len = %d, want %d", len(out), len(in))
	}

	if out[0].Path != "/usr/bin/foo" || out[0].Size != 42 || out[0].SHA256 != "deadbeef" {
		t.Fatalf("file[0] mismatch: %+v", out[0])
	}

	if out[0].Mode != uint32(0o755) {
		t.Fatalf("file[0].Mode = %o, want 0755", out[0].Mode)
	}

	if out[1].LinkTarget != "/usr/bin/foo" {
		t.Fatalf("file[1].LinkTarget = %q, want /usr/bin/foo", out[1].LinkTarget)
	}
}

func TestToRPMDBFilesEmpty(t *testing.T) {
	out := toRPMDBFiles(nil)
	if len(out) != 0 {
		t.Fatalf("toRPMDBFiles(nil) len = %d, want 0", len(out))
	}
}
