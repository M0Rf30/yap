package command

import (
	"os"
	"path/filepath"

	"github.com/M0Rf30/yap/v2/pkg/container"
	"github.com/M0Rf30/yap/v2/pkg/logger"
)

// inContainerEnv is set by the container runtime when launching yap inside a
// container or rootless namespace. Its presence prevents infinite re-dispatch.
const inContainerEnv = "YAP_IN_CONTAINER"

// IsInsideContainer returns true when the process is already running inside
// a YAP builder container or rootless namespace.
func IsInsideContainer() bool {
	return os.Getenv(inContainerEnv) != ""
}

// RunCommandInContainer re-invokes the given yap sub-command inside the
// appropriate distro container using the configured runtime.
//
//   - distro: distribution tag, e.g. "ubuntu-noble"
//   - workDir: host directory to mount as /workspace (must be absolute)
//   - subArgs: the yap sub-command + arguments to run inside, e.g.
//     ["build", "ubuntu-noble", "/workspace/mypkg"]
//
// Returns true if the command was dispatched (caller should return immediately),
// false if the caller should proceed natively (already inside a container, or
// runtime detection failed non-fatally).
func RunCommandInContainer(distro, workDir string, subArgs []string) bool {
	if IsInsideContainer() {
		return false
	}

	// Ensure workDir is absolute.
	abs, err := filepath.Abs(workDir)
	if err == nil {
		workDir = abs
	}

	rt, err := container.Detect(ContainerRuntimeOverride())
	if err != nil {
		logger.Error("failed to detect container runtime", "error", err)

		return false
	}

	logger.Info("dispatching to container",
		"runtime", string(rt.Type()),
		"distro", distro,
		"workdir", workDir)

	// The runtime injects YAP_IN_CONTAINER=1 so the inner process doesn't loop.
	// Note: the container ENTRYPOINT is already "yap", so subArgs must NOT
	// include the binary name — pass the sub-command and its arguments directly.
	if err := rt.Run(distro, workDir, subArgs); err != nil {
		logger.Error("container run failed", "error", err)
		os.Exit(1)
	}

	return true
}
