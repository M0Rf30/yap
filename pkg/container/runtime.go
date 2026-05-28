// Package container provides an abstraction layer for container runtimes,
// allowing YAP to operate with podman, docker, or a built-in rootless runner.
package container

import (
	"context"
	"io"
	"os/exec"

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
// Pass override = "" to use auto-detection; pass "cli" or "rootless" to force.
func Detect(override string) (Runtime, error) {
	switch RuntimeType(override) {
	case RuntimeCLI:
		return newCLIRuntime()
	case RuntimeRootless:
		return NewRootlessRuntime()
	case "":
		// auto-detect
	default:
		return nil, errors.New(errors.ErrTypeConfiguration,
			"unknown runtime: "+override).
			WithOperation("Detect")
	}

	// Try CLI runtimes first.
	if rt, err := newCLIRuntime(); err == nil {
		logger.Debug("container runtime auto-detected", "type", "cli", "backend", rt.(*cliRuntime).bin)

		return rt, nil
	}

	// Fall back to built-in rootless runner.
	logger.Info("no podman/docker found, using built-in rootless runner")

	return NewRootlessRuntime()
}

// newCLIRuntime returns a CLI runtime if podman or docker is available.
func newCLIRuntime() (Runtime, error) {
	for _, bin := range []string{"podman", "docker"} {
		if path, err := exec.LookPath(bin); err == nil {
			return &cliRuntime{bin: path}, nil
		}
	}

	return nil, errors.New(errors.ErrTypeFileSystem,
		"no container CLI found (podman or docker)").
		WithOperation("newCLIRuntime")
}
