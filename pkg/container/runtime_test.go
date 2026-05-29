//nolint:testpackage // exercises unexported helpers (newCLIRuntime, cliRuntimeReady, cliRuntime)
package container

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// writeFakeBin creates an executable shell script named bin in dir that exits
// with infoExit when invoked as `<bin> info` and 0 otherwise. It returns
// nothing; callers point PATH at dir.
func writeFakeBin(t *testing.T, dir, bin string, infoExit int) {
	t.Helper()

	script := "#!/bin/sh\n" +
		"if [ \"$1\" = \"info\" ]; then exit " + itoa(infoExit) + "; fi\n" +
		"exit 0\n"

	path := filepath.Join(dir, bin)
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatalf("write fake %s: %v", bin, err)
	}
}

func itoa(i int) string {
	if i == 0 {
		return "0"
	}

	return string(rune('0' + i))
}

func TestDetectUnknownRuntime(t *testing.T) {
	rt, err := Detect("bogus")
	if err == nil {
		t.Fatalf("expected error for unknown runtime, got runtime %v", rt)
	}
}

func TestDetectExplicitDockerNotFound(t *testing.T) {
	// Empty PATH so docker cannot be found.
	t.Setenv("PATH", t.TempDir())

	if _, err := Detect("docker"); err == nil {
		t.Fatal("expected error when docker not in PATH")
	}
}

func TestDetectExplicitDockerFound(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell-script fake bins are POSIX-only")
	}

	dir := t.TempDir()
	writeFakeBin(t, dir, "docker", 0)
	t.Setenv("PATH", dir)

	rt, err := Detect("docker")
	if err != nil {
		t.Fatalf("Detect(docker) failed: %v", err)
	}

	cr, ok := rt.(*cliRuntime)
	if !ok {
		t.Fatalf("expected *cliRuntime, got %T", rt)
	}

	if filepath.Base(cr.bin) != "docker" {
		t.Fatalf("expected docker backend, got %s", cr.bin)
	}
}

// TestNewCLIRuntimeFallsBackWhenPodmanUnreachable reproduces the macOS bug:
// podman binary exists but `podman info` fails (machine stopped). Detection
// must skip podman and select the reachable docker backend.
func TestNewCLIRuntimeFallsBackWhenPodmanUnreachable(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell-script fake bins are POSIX-only")
	}

	dir := t.TempDir()
	writeFakeBin(t, dir, "podman", 1) // info fails -> unreachable
	writeFakeBin(t, dir, "docker", 0) // info ok -> reachable
	t.Setenv("PATH", dir)

	rt, err := newCLIRuntime()
	if err != nil {
		t.Fatalf("newCLIRuntime failed: %v", err)
	}

	cr := rt.(*cliRuntime)
	if filepath.Base(cr.bin) != "docker" {
		t.Fatalf("expected fallback to docker, got %s", cr.bin)
	}
}

func TestNewCLIRuntimeNoneReachable(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell-script fake bins are POSIX-only")
	}

	dir := t.TempDir()
	writeFakeBin(t, dir, "podman", 1)
	writeFakeBin(t, dir, "docker", 1)
	t.Setenv("PATH", dir)

	if _, err := newCLIRuntime(); err == nil {
		t.Fatal("expected error when no CLI backend is reachable")
	}
}

func TestCliRuntimeReady(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell-script fake bins are POSIX-only")
	}

	dir := t.TempDir()
	writeFakeBin(t, dir, "ok", 0)
	writeFakeBin(t, dir, "bad", 1)

	if !cliRuntimeReady(filepath.Join(dir, "ok")) {
		t.Error("expected ready=true for exit 0")
	}

	if cliRuntimeReady(filepath.Join(dir, "bad")) {
		t.Error("expected ready=false for exit 1")
	}
}
