//go:build linux

package main

import (
	"os"
	"syscall"

	"golang.org/x/sys/unix"

	"github.com/M0Rf30/yap/v2/pkg/logger"
)

// armParentDeathSignal asks the kernel to deliver SIGTERM to this process if
// its parent dies. This guarantees yap-mcp exits when the MCP client
// disappears even if stdin is never closed (e.g. parent was SIGKILLed and the
// inherited pipe is held open by another descriptor).
//
// PR_SET_PDEATHSIG is Linux-specific; other platforms rely on stdin EOF.
func armParentDeathSignal() {
	if err := unix.Prctl(unix.PR_SET_PDEATHSIG, uintptr(syscall.SIGTERM), 0, 0, 0); err != nil {
		logger.Warn("prctl PR_SET_PDEATHSIG failed", "error", err)
		return
	}

	// Edge case: between fork and prctl the parent may have already died and
	// the kernel reparented us to init. Detect that and self-terminate
	// directly — relying on signal delivery ordering here is fragile.
	if unix.Getppid() == 1 {
		logger.Info("parent already gone; exiting")
		os.Exit(1)
	}
}
