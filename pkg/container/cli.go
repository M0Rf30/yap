package container

import (
	"context"
	"io"
	"sort"

	"github.com/M0Rf30/yap/v2/pkg/constants"
	"github.com/M0Rf30/yap/v2/pkg/shell"
)

// Shared CLI flag/path constants used across cliRuntime invocations.
const (
	subRun           = "run"
	flagRm           = "--rm"
	envInContainer   = "YAP_IN_CONTAINER=1"
	containerWorkdir = "/project"
)

// cliRuntime delegates to the system podman or docker binary.
type cliRuntime struct {
	bin string // absolute path to podman or docker
}

// Type implements Runtime.
func (r *cliRuntime) Type() RuntimeType { return RuntimeCLI }

// Pull implements Runtime by running `<bin> pull docker.io/m0rf30/yap-<distro>`.
func (r *cliRuntime) Pull(distro string) error {
	return shell.Exec(context.Background(), false, "",
		r.bin, "pull", constants.DockerOrg+distro)
}

// RunShell implements Runtime by overriding the ENTRYPOINT with /bin/sh -c
// and executing shellCmd. Use this to chain multiple yap commands in one
// container invocation, e.g. "yap prepare ubuntu-jammy && yap build ...".
func (r *cliRuntime) RunShell(distro, workDir, shellCmd string) error {
	runArgs := []string{
		subRun, flagRm,
		"--entrypoint", "/bin/sh",
		"-e", envInContainer,
		"-v", workDir + ":" + containerWorkdir + ":z",
		"-w", containerWorkdir,
		"--user", "root",
		constants.DockerOrg + distro,
		"-c", shellCmd,
	}

	return shell.Exec(context.Background(), false, "", r.bin, runArgs...)
}

// RunShellCapture implements Runtime by tee-ing podman/docker stdout+stderr
// into out. Falls back to the default behaviour when out is nil. env is
// forwarded via `-e KEY=VALUE` flags so secrets do not appear in the shell
// argv (and therefore not in `ps`).
func (r *cliRuntime) RunShellCapture(ctx context.Context, distro, workDir, shellCmd string,
	env map[string]string, out io.Writer,
) error {
	if out == nil {
		return r.RunShell(distro, workDir, shellCmd)
	}

	runArgs := []string{
		subRun, flagRm,
		"--entrypoint", "/bin/sh",
		"-e", envInContainer,
	}
	runArgs = append(runArgs, envFlags(env)...)
	runArgs = append(runArgs,
		"-v", workDir+":"+containerWorkdir+":z",
		"-w", containerWorkdir,
		"--user", "root",
		constants.DockerOrg+distro,
		"-c", shellCmd,
	)

	return shell.ExecCapture(ctx, out, "", r.bin, runArgs...)
}

// envFlags renders an env map into a sorted slice of `-e KEY=VALUE` argv
// fragments. Sorting makes the output stable for tests; values are passed
// verbatim — callers MUST NOT trust them to escape shell metacharacters
// because the env value never goes through the shell, only the container
// runtime CLI.
func envFlags(env map[string]string) []string {
	if len(env) == 0 {
		return nil
	}

	keys := make([]string, 0, len(env))
	for k := range env {
		keys = append(keys, k)
	}

	sort.Strings(keys)

	out := make([]string, 0, 2*len(keys))
	for _, k := range keys {
		out = append(out, "-e", k+"="+env[k])
	}

	return out
}

// Run implements Runtime by running the container with the workspace mounted.
// The workspace directory is bind-mounted to /project inside the container.
// YAP_IN_CONTAINER=1 is injected so the inner yap process knows it is already
// inside the correct environment and must not re-dispatch to a container.
//
// For Podman: --userns=keep-id maps the host UID into the container so the
// mounted workspace is writable by the container process.
// For Docker: --user uid:gid achieves the same effect.
func (r *cliRuntime) Run(distro, workDir string, args []string) error {
	runArgs := []string{
		subRun, flagRm,
		"-e", envInContainer,
		"-v", workDir + ":" + containerWorkdir + ":z",
		"-w", containerWorkdir,
	}

	// Run as root so the container process can write to the bind-mounted
	// workspace and run privileged package manager operations (apt-get, dpkg).
	// The YAP builder images use a restricted sudoers config for the 'yap'
	// user, so running as root is the correct mode for build operations.
	runArgs = append(runArgs, "--user", "root", constants.DockerOrg+distro)
	runArgs = append(runArgs, args...)

	return shell.Exec(context.Background(), false, "", r.bin, runArgs...)
}
