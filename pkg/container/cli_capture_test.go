//nolint:testpackage // exercises unexported cliRuntime
package container

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// TestRunShellCaptureNilOutForwardsEnv verifies that env is forwarded to the
// container CLI even when no output writer is supplied.
func TestRunShellCaptureNilOutForwardsEnv(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell script fake binary")
	}

	dir := t.TempDir()
	argsFile := filepath.Join(dir, "args")
	script := "#!/bin/sh\nprintf '%s\\n' \"$@\" > " + argsFile + "\n"
	bin := filepath.Join(dir, "fakecli")

	if err := os.WriteFile(bin, []byte(script), 0o755); err != nil { //nolint:gosec
		t.Fatalf("write fake bin: %v", err)
	}

	rt := &cliRuntime{bin: bin}

	err := rt.RunShellCapture(context.Background(), "ubuntu-jammy", dir, "true",
		map[string]string{"YAP_SECRET": "s3cret"}, nil)
	if err != nil {
		t.Fatalf("RunShellCapture: %v", err)
	}

	got, err := os.ReadFile(argsFile)
	if err != nil {
		t.Fatalf("read args: %v", err)
	}

	if !strings.Contains(string(got), "YAP_SECRET=s3cret") {
		t.Errorf("env not forwarded with nil out; args:\n%s", got)
	}
}

// TestRunShellCaptureNilOutHonoursContext verifies a cancelled ctx is not
// ignored when out is nil.
func TestRunShellCaptureNilOutHonoursContext(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell script fake binary")
	}

	dir := t.TempDir()
	bin := filepath.Join(dir, "fakecli")

	if err := os.WriteFile(bin, []byte("#!/bin/sh\nsleep 30\n"), 0o755); err != nil { //nolint:gosec
		t.Fatalf("write fake bin: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	rt := &cliRuntime{bin: bin}

	if err := rt.RunShellCapture(ctx, "ubuntu-jammy", dir, "true", nil, nil); err == nil {
		t.Error("expected error from cancelled context, got nil")
	}
}
