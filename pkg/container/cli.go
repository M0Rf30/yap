package container

import (
	"context"

	"github.com/M0Rf30/yap/v2/pkg/constants"
	"github.com/M0Rf30/yap/v2/pkg/shell"
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
		"run", "--rm",
		"--entrypoint", "/bin/sh",
		"-e", "YAP_IN_CONTAINER=1",
		"-v", workDir + ":/workspace:z",
		"-w", "/workspace",
		"--user", "root",
		constants.DockerOrg + distro,
		"-c", shellCmd,
	}

	return shell.Exec(context.Background(), false, "", r.bin, runArgs...)
}

// Run implements Runtime by running the container with the workspace mounted.
// The workspace directory is bind-mounted to /workspace inside the container.
// YAP_IN_CONTAINER=1 is injected so the inner yap process knows it is already
// inside the correct environment and must not re-dispatch to a container.
//
// For Podman: --userns=keep-id maps the host UID into the container so the
// mounted workspace is writable by the container process.
// For Docker: --user uid:gid achieves the same effect.
func (r *cliRuntime) Run(distro, workDir string, args []string) error {
	runArgs := []string{
		"run", "--rm",
		"-e", "YAP_IN_CONTAINER=1",
		"-v", workDir + ":/workspace:z",
		"-w", "/workspace",
	}

	// Run as root so the container process can write to the bind-mounted
	// workspace and run privileged package manager operations (apt-get, dpkg).
	// The YAP builder images use a restricted sudoers config for the 'yap'
	// user, so running as root is the correct mode for build operations.
	runArgs = append(runArgs, "--user", "root", constants.DockerOrg+distro)
	runArgs = append(runArgs, args...)

	return shell.Exec(context.Background(), false, "", r.bin, runArgs...)
}
