package common

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWriteCMakeToolchainFilePerTarget(t *testing.T) {
	t.Setenv("TMPDIR", t.TempDir())

	pathA, err := writeCMakeToolchainFile(
		"aarch64", "aarch64-linux-gnu-gcc", "aarch64-linux-gnu-g++", "aarch64-linux-gnu", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	pathB, err := writeCMakeToolchainFile(
		"riscv64", "riscv64-linux-gnu-gcc", "riscv64-linux-gnu-g++", "riscv64-linux-gnu", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if pathA == pathB {
		t.Fatalf("different targets must not share a toolchain file: %s", pathA)
	}

	again, err := writeCMakeToolchainFile(
		"aarch64", "aarch64-linux-gnu-gcc", "aarch64-linux-gnu-g++", "aarch64-linux-gnu", "")
	if err != nil || again != pathA {
		t.Fatalf("identical request should reuse file: got %q err=%v want %q", again, err, pathA)
	}

	data, err := os.ReadFile(pathA) //nolint:gosec // test path
	if err != nil {
		t.Fatalf("read toolchain file: %v", err)
	}

	content := string(data)
	if !strings.Contains(content, "CMAKE_SYSTEM_PROCESSOR aarch64") ||
		!strings.Contains(content, "aarch64-linux-gnu-gcc") {
		t.Errorf("toolchain content missing target info:\n%s", content)
	}

	if strings.Contains(content, "riscv64") {
		t.Errorf("toolchain for aarch64 leaked other target:\n%s", content)
	}

	info, err := os.Stat(filepath.Dir(pathA))
	if err != nil {
		t.Fatalf("stat dir: %v", err)
	}

	if perm := info.Mode().Perm(); perm != 0o700 {
		t.Errorf("toolchain dir perm = %o, want 700", perm)
	}
}

func TestWriteCMakeToolchainFileIgnoresPredictablePath(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("TMPDIR", tmp)

	// A pre-created file at the old predictable path must never be used.
	spoof := filepath.Join(tmp, "yap-cross-.cmake")
	if err := os.WriteFile(spoof, []byte("execute_process(COMMAND evil)"), 0o600); err != nil {
		t.Fatal(err)
	}

	path, err := writeCMakeToolchainFile("s390x", "s390x-linux-gnu-gcc", "s390x-linux-gnu-g++",
		"s390x-linux-gnu", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if path == spoof {
		t.Fatal("used spoofable predictable path")
	}

	data, err := os.ReadFile(path) //nolint:gosec // test path
	if err != nil {
		t.Fatal(err)
	}

	if strings.Contains(string(data), "evil") {
		t.Fatal("toolchain file picked up spoofed content")
	}
}

func TestWriteCMakeToolchainFileCCFlags(t *testing.T) {
	t.Setenv("TMPDIR", t.TempDir())

	path, err := writeCMakeToolchainFile("i686", "gcc", "g++", "i686-linux-gnu", "-m32")
	if err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(path) //nolint:gosec // test path
	if err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(string(data), `CMAKE_C_FLAGS_INIT "-m32"`) {
		t.Errorf("missing -m32 flags:\n%s", data)
	}
}
