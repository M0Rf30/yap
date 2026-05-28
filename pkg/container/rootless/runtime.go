//go:build linux

package rootless

import (
	"context"
	"io"
	"os"
	"sort"

	"github.com/M0Rf30/yap/v2/pkg/container/internal/runtimetype"
	"github.com/M0Rf30/yap/v2/pkg/logger"
)

// Runtime implements the container.Runtime interface using in-process image
// pull and rootlesskit for user-namespace isolation.
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

// RunShellCapture is a best-effort variant: the rootless runner uses
// rootlesskit's parent.Parent loop which writes directly to the host
// stdio, so we cannot redirect output without rewiring that pipeline.
// We log a warning and fall through to RunShell — callers still get a
// correct success/failure signal, just no captured stream.
//
// env is forwarded to the child via os.Setenv/Unsetenv around the
// rootlesskit invocation; values therefore never appear in the shell
// argv (no `ps`/`/proc/<pid>/cmdline` leak) — the same invariant the CLI
// backend honours via `-e KEY=VAL`. We restore any pre-existing values
// so concurrent callers in the same process don't observe stale state.
//
// ctx is accepted for interface compatibility but is not propagated into
// rootlesskit's parent loop; cancellation is observed only at the next
// process boundary.
func (r *Runtime) RunShellCapture(_ context.Context, distro, workDir, shellCmd string,
	env map[string]string, _ io.Writer,
) error {
	logger.Warn("rootless runtime does not support output capture; output goes to host stderr")

	restore := setEnvOnce(env)
	defer restore()

	return r.RunShell(distro, workDir, shellCmd)
}

// setEnvOnce sets each entry from env via os.Setenv and returns a function
// that restores the original values (Unsetenv when the key was previously
// unset). Keys are processed in deterministic order for predictable logs.
func setEnvOnce(env map[string]string) func() {
	if len(env) == 0 {
		return func() {}
	}

	keys := make([]string, 0, len(env))
	for k := range env {
		keys = append(keys, k)
	}

	sort.Strings(keys)

	type prev struct {
		val   string
		isSet bool
	}

	prevs := make(map[string]prev, len(keys))

	for _, k := range keys {
		v, ok := os.LookupEnv(k)
		prevs[k] = prev{val: v, isSet: ok}

		if err := os.Setenv(k, env[k]); err != nil {
			logger.Warn("rootless: setenv failed", "key", k, "error", err)
		}
	}

	return func() {
		for k, p := range prevs {
			var err error
			if p.isSet {
				err = os.Setenv(k, p.val)
			} else {
				err = os.Unsetenv(k)
			}

			if err != nil {
				logger.Warn("rootless: restore env failed", "key", k, "error", err)
			}
		}
	}
}
