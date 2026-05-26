//go:build !linux

// Package shell provides process execution and shell operations.
package shell

import "os/exec"

// applyFakeroot is a no-op on non-Linux platforms. User namespaces with
// CLONE_NEWUSER are Linux-only; on macOS/Windows the fakeroot mechanism is
// simply unavailable and package() scripts must run with real privileges
// (or the build must happen inside a Linux container).
func applyFakeroot(_ *exec.Cmd) {}
