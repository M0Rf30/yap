//go:build linux

package rootless

import (
	"github.com/M0Rf30/yap/v2/pkg/container/internal/runtimetype"
)

// Runtime implements the container.Runtime interface using pure-Go image pull
// and rootlesskit for user-namespace isolation.
type Runtime struct{}

// NewRuntime returns a new rootless Runtime.
func NewRuntime() *Runtime { return &Runtime{} }

// Type returns the runtime type identifier.
func (r *Runtime) Type() runtimetype.RuntimeType { return runtimetype.Rootless }

// Pull downloads and extracts the YAP builder image for distro.
func (r *Runtime) Pull(distro string) error {
	return PullImage(distro)
}

// Run executes args inside the distro rootfs with workDir bind-mounted as /workspace.
func (r *Runtime) Run(distro, workDir string, args []string) error {
	return RunInRootless(distro, workDir, args)
}

// RunShell executes a shell command string inside the distro rootfs.
func (r *Runtime) RunShell(distro, workDir, shellCmd string) error {
	return RunInRootless(distro, workDir, []string{"/bin/sh", "-c", shellCmd})
}
