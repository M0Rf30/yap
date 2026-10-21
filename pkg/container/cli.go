// SPDX-FileCopyrightText: 2021-2026 Gianluca Boiano
// SPDX-License-Identifier: GPL-3.0-only

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
// into out. When out is nil the output is streamed to the default sinks, but
// env and ctx are still honoured. env is
// forwarded via value-less `-e KEY` flags, with the values set in the
// container CLI's own environment, so secrets do not appear in argv (and
// therefore not in `ps`).
func (r *cliRuntime) RunShellCapture(ctx context.Context, distro, workDir, shellCmd string,
	env map[string]string, out io.Writer,
) error {
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

	childEnv := envPairs(env)

	if out == nil {
		return shell.ExecWithEnv(ctx, childEnv, false, "", r.bin, runArgs...)
	}

	return shell.ExecCaptureEnv(ctx, childEnv, out, "", r.bin, runArgs...)
}

// envPairs renders an env map into sorted KEY=VALUE entries for the child
// process environment, from which docker/podman read the values of the
// value-less `-e KEY` flags.
func envPairs(env map[string]string) []string {
	if len(env) == 0 {
		return nil
	}

	keys := make([]string, 0, len(env))
	for k := range env {
		keys = append(keys, k)
	}

	sort.Strings(keys)

	out := make([]string, 0, len(keys))
	for _, k := range keys {
		out = append(out, k+"="+env[k])
	}

	return out
}

// envFlags renders an env map into a sorted slice of value-less `-e KEY`
// argv fragments. The values never appear in argv (so not in `ps` or
// /proc/<pid>/cmdline); the container CLI reads them from its own
// environment, which RunShellCapture populates via envPairs.
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
		out = append(out, "-e", k)
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
