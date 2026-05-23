package command

import (
	"os"
	"path/filepath"
	"strings"

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

// RunPipelineInContainer runs prepare then build in a single container
// invocation using a shell chain. This ensures makedeps installed by prepare
// are available to build without requiring a persistent container.
//
//   - distro: distribution tag, e.g. "ubuntu-noble"
//   - workDir: host directory to mount as /workspace
//   - buildArgs: arguments for the inner yap build command (distroTag + path)
//   - skipPrepare: if true, skip the prepare step (user passed -s or -d)
//
// Returns true if dispatched, false if caller should proceed natively.
func RunPipelineInContainer(distro, workDir string, buildArgs []string, skipPrepare bool) bool {
	if IsInsideContainer() {
		return false
	}

	abs, err := filepath.Abs(workDir)
	if err == nil {
		workDir = abs
	}

	rt, err := container.Detect(ContainerRuntimeOverride())
	if err != nil {
		logger.Error("failed to detect container runtime", "error", err)

		return false
	}

	logger.Info("dispatching pipeline to container",
		"runtime", string(rt.Type()),
		"distro", distro,
		"workdir", workDir,
		"skip_prepare", skipPrepare)

	// Build the shell command: optionally chain prepare before build.
	// Both commands run inside the same container so makedeps installed
	// by prepare are available to build.
	buildCmd := "yap " + shellJoinArgs(buildArgs)

	var shellCmd string
	if skipPrepare {
		shellCmd = buildCmd
	} else {
		shellCmd = "yap prepare " + distro + " && " + buildCmd
	}

	if err := rt.RunShell(distro, workDir, shellCmd); err != nil {
		logger.Error("container pipeline failed", "error", err)
		os.Exit(1)
	}

	return true
}

// shellJoinArgs joins args into a shell-safe string.
func shellJoinArgs(args []string) string {
	var b strings.Builder

	for i, a := range args {
		if i > 0 {
			b.WriteByte(' ')
		}

		b.WriteString(shellQuoteArg(a))
	}

	return b.String()
}

// shellQuoteArg wraps a single argument in single quotes.
func shellQuoteArg(s string) string {
	var b strings.Builder

	b.WriteByte('\'')

	for _, c := range s {
		if c == '\'' {
			b.WriteString("'\\''")
		} else {
			b.WriteRune(c)
		}
	}

	b.WriteByte('\'')

	return b.String()
}
