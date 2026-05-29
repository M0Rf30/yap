// Package container provides an abstraction layer for container runtimes,
// allowing YAP to operate with podman, docker, or a built-in rootless runner.
package container

import (
	"context"
	"io"
	"os/exec"
	"strings"
	"time"

	"github.com/M0Rf30/yap/v2/pkg/container/internal/runtimetype"
	"github.com/M0Rf30/yap/v2/pkg/errors"
	"github.com/M0Rf30/yap/v2/pkg/logger"
)

// RuntimeType identifies which container backend is in use.
type RuntimeType = runtimetype.RuntimeType

const (
	// RuntimeCLI uses the system podman or docker CLI.
	RuntimeCLI = runtimetype.CLI
	// RuntimeRootless uses the built-in rootless runner (go-containerregistry + rootlesskit).
	RuntimeRootless = runtimetype.Rootless
)

// Supported CLI backend binary names, in auto-detection priority order.
const (
	binPodman = "podman"
	binDocker = "docker"
)

// Runtime is the interface every container backend must satisfy.
type Runtime interface {
	// Pull downloads the container image for the given distribution tag.
	Pull(distro string) error

	// Run executes the given command inside the distro container, mounting
	// workDir as /project inside the container.
	// args are passed directly to the container ENTRYPOINT.
	Run(distro, workDir string, args []string) error

	// RunShell executes a shell command string inside the distro container,
	// overriding the ENTRYPOINT with /bin/sh -c. Use this to chain multiple
	// yap sub-commands in a single container invocation.
	RunShell(distro, workDir, shellCmd string) error

	// RunShellCapture is like RunShell but tees stdout+stderr into out
	// (typically a bounded buffer owned by the caller — e.g. an MCP build
	// session log). Pass nil to fall back to the default sink.
	//
	// env is forwarded to the container as additional environment variables
	// without appearing in the shell argv — use it for secrets like signing
	// passphrases that would otherwise leak via `ps`. A nil/empty map is
	// equivalent to passing no extra vars.
	//
	// ctx is honoured by the CLI backend (podman/docker) so cancelling the
	// context terminates the container. The rootless backend forwards it on
	// a best-effort basis only — rootlesskit's parent loop has no clean
	// cancel hook, so cancellation may be observed only after the next
	// process boundary.
	RunShellCapture(ctx context.Context, distro, workDir, shellCmd string,
		env map[string]string, out io.Writer) error

	// Type returns the runtime identifier.
	Type() RuntimeType
}

// Detect returns the best available Runtime.
// Priority: podman → docker → built-in rootless.
//
// Pass override = "" to use auto-detection. Accepted explicit values:
//   - "cli": force the system podman/docker CLI (auto-pick between them)
//   - "podman" / "docker": force that specific CLI backend
//   - "rootless": force the built-in rootless runner
func Detect(override string) (Runtime, error) {
	switch RuntimeType(override) {
	case RuntimeCLI:
		return newCLIRuntime()
	case RuntimeRootless:
		return NewRootlessRuntime()
	case binPodman, binDocker:
		return newCLIRuntimeFor(override)
	case "":
		// auto-detect
	default:
		return nil, errors.New(errors.ErrTypeConfiguration,
			"unknown runtime: "+override).
			WithOperation("Detect").
			WithContext("accepted", "cli, podman, docker, rootless")
	}

	// Try CLI runtimes first.
	if rt, err := newCLIRuntime(); err == nil {
		logger.Debug("container runtime auto-detected", "type", "cli", "backend", rt.(*cliRuntime).bin)

		return rt, nil
	}

	// Fall back to built-in rootless runner.
	logger.Info("no usable podman/docker found, using built-in rootless runner")

	return NewRootlessRuntime()
}

// newCLIRuntime returns a CLI runtime for the first podman/docker backend that
// is not only installed but actually reachable. A binary present in $PATH is
// not sufficient: on macOS `podman` is a remote client to a VM, so the binary
// exists even when `podman machine` is stopped and every `podman run` fails
// with a socket error. We probe each candidate with `<bin> info` and skip ones
// that cannot connect to their daemon/machine, falling through to the next.
func newCLIRuntime() (Runtime, error) {
	var found []string

	for _, bin := range []string{binPodman, binDocker} {
		path, err := exec.LookPath(bin)
		if err != nil {
			continue
		}

		found = append(found, bin)

		if cliRuntimeReady(path) {
			return &cliRuntime{bin: path}, nil
		}

		logger.Warn("container CLI installed but not reachable, trying next backend",
			"backend", bin, "hint", "daemon/machine not running")
	}

	if len(found) > 0 {
		return nil, errors.New(errors.ErrTypeFileSystem,
			"container CLI found but none are reachable").
			WithOperation("newCLIRuntime").
			WithContext("found", strings.Join(found, ", ")).
			WithContext("hint", "start the docker daemon or run `podman machine start`")
	}

	return nil, errors.New(errors.ErrTypeFileSystem,
		"no container CLI found (podman or docker)").
		WithOperation("newCLIRuntime")
}

// newCLIRuntimeFor forces a specific CLI backend ("podman" or "docker") as
// requested via --runtime. The binary must exist; readiness is not probed so
// the caller sees the backend's real error if its daemon/machine is down.
func newCLIRuntimeFor(bin string) (Runtime, error) {
	path, err := exec.LookPath(bin)
	if err != nil {
		return nil, errors.New(errors.ErrTypeFileSystem,
			"requested container runtime not found: "+bin).
			WithOperation("newCLIRuntimeFor").
			WithContext("runtime", bin)
	}

	return &cliRuntime{bin: path}, nil
}

// cliRuntimeReady reports whether a podman/docker binary can actually reach its
// backend. `<bin> info` returns non-zero quickly when the podman machine or
// docker daemon is not running. Output is discarded; only the exit status
// matters. A short timeout guards against a hung daemon.
func cliRuntimeReady(bin string) bool {
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, bin, "info")
	cmd.Stdout = nil
	cmd.Stderr = nil

	return cmd.Run() == nil
}
