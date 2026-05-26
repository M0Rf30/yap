//go:build linux

// Package shell provides process execution and shell operations.
package shell

// fakeroot.go implements a fakeroot mechanism using Linux user namespaces
// (CLONE_NEWUSER). When applied to an exec.Cmd, the subprocess sees itself as UID 0
// (root) while the kernel maps that back to the real caller UID on the host.
// This allows package() scripts to call `install -o root -g root` without actually
// being root. Inspired by lure.sh/fakeroot (https://github.com/lure-sh/lure).

import (
	"os"
	"os/exec"
	"syscall"
)

// applyFakeroot configures cmd to run inside a new user namespace where UID 0
// maps to the current user. This is a no-op when already running as root.
func applyFakeroot(cmd *exec.Cmd) {
	uid := os.Getuid()
	gid := os.Getgid()

	if uid == 0 {
		// Already root — no mapping needed.
		return
	}

	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}

	cmd.SysProcAttr.Cloneflags |= syscall.CLONE_NEWUSER

	cmd.SysProcAttr.UidMappings = append(cmd.SysProcAttr.UidMappings, syscall.SysProcIDMap{
		ContainerID: 0,
		HostID:      uid,
		Size:        1,
	})

	cmd.SysProcAttr.GidMappings = append(cmd.SysProcAttr.GidMappings, syscall.SysProcIDMap{
		ContainerID: 0,
		HostID:      gid,
		Size:        1,
	})
}
