// SPDX-FileCopyrightText: 2021-2026 Gianluca Boiano
// SPDX-License-Identifier: GPL-3.0-only

package mcp

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/M0Rf30/yap/v2/cmd/yap/command"
	"github.com/M0Rf30/yap/v2/pkg/container"
	"github.com/M0Rf30/yap/v2/pkg/errors"
	"github.com/M0Rf30/yap/v2/pkg/shell"
)

// containerTagRe restricts distro and release strings forwarded into a
// container argv/image tag to a conservative charset, so they can never
// carry shell metacharacters or option-like values.
var containerTagRe = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*$`)

// validateContainerTag rejects distro/release values that are not plain
// tag-safe identifiers. An empty value is allowed (bare family / no release).
func validateContainerTag(label, v string) error {
	if v == "" || containerTagRe.MatchString(v) {
		return nil
	}

	return errors.New(errors.ErrTypeValidation,
		"invalid "+label+" "+v+": only letters, digits, '.', '_' and '-' are allowed").
		WithOperation(toolNameBuild).
		WithContext(label, v)
}

// dispatchBuildInContainer mirrors the CLI's RunPipelineInContainer flow for
// the MCP build tool. It runs the build asynchronously (so the tool call
// returns immediately with a buildID) inside a yap container image,
// forwarding every flag the user passed via the MCP args.
//
// The container IMAGE and the package IDENTITY are resolved independently:
// distroTag (bare family, or family-release) is the identity forwarded to
// the inner yap argv and therefore drives the release suffix stamped into
// the built package; image is the possibly release-qualified tag used only
// to pick the build environment via command.ResolveContainerImage. A bare
// family with no matching image (e.g. "ubuntu" with no host os-release
// match) fails the build session immediately rather than silently falling
// back to a native host build of the wrong distro.
//
// The caller invokes this only when the user explicitly requested a distro.
// When no container runtime is available that is an error: silently building
// natively would produce a package for the wrong distro.
func dispatchBuildInContainer(args *buildArgs, abs, distro, release string,
) (buildStartResult, error) {
	if err := validateContainerTag("distro", distro); err != nil {
		return buildStartResult{}, err
	}

	if err := validateContainerTag("release", release); err != nil {
		return buildStartResult{}, err
	}

	rt, err := container.Detect(command.ContainerRuntimeOverride())
	if err != nil || rt == nil {
		cause := err
		if cause == nil {
			cause = errors.New(errors.ErrTypeConfiguration, "no container runtime found")
		}

		return buildStartResult{}, errors.Wrap(cause, errors.ErrTypeConfiguration,
			"container runtime unavailable for distro "+distro+
				"; omit distro to build natively on the host").
			WithOperation(toolNameBuild).
			WithContext("distro", distro)
	}

	distroTag := innerDistroTag(distro, release)

	image, err := command.ResolveContainerImage(distro, release)
	if err != nil {
		return containerImageResolutionFailure(distro, release, abs, err), nil
	}

	skipPrepare := args.SkipSyncDeps || args.NoMakeDeps
	shellCmd := containerShellCmd(args, distroTag, skipPrepare)

	// Secrets (passphrase) travel via env, never as CLI args — argv is
	// visible to other processes on the host via `ps`.
	envVars := buildEnvFromArgs(args)

	sess, ctx := defaultRegistry.Register(context.Background(), distro, release, abs)
	defaultRegistry.UpdateContainer(sess.ID, string(rt.Type()), image)

	// Container builds write artifacts to the project's output dir, which for
	// yap.json projects is usually a subdirectory of the mounted project.
	if out := containerOutputDir(abs); out != "" {
		defaultRegistry.SetOutputDir(sess.ID, out)
	}

	go func() {
		// Capture container stdout+stderr into the session's bounded log so
		// MCP clients can retrieve it via build_status. Pass the session
		// context so build_cancel can terminate the container.
		if err := rt.RunShellCapture(ctx, image, abs, shellCmd, envVars, sess.Log); err != nil {
			if ctx.Err() != nil {
				defaultRegistry.Finish(sess.ID, BuildStateCanceled, ctx.Err().Error())
				return
			}

			defaultRegistry.Finish(sess.ID, BuildStateFailed, err.Error())

			return
		}

		defaultRegistry.Finish(sess.ID, BuildStateSucceeded, "")
	}()

	return buildStartResult{
		BuildID:          sess.ID,
		State:            string(BuildStateRunning),
		Distro:           distro,
		Release:          release,
		Path:             abs,
		InContainer:      true,
		ContainerRuntime: string(rt.Type()),
		ContainerImage:   image,
	}, nil
}

// containerShellCmd builds the single shell string run inside the builder
// container: an optional `yap prepare` (so makedeps are installed) chained
// before `yap build`. Every argument of both halves goes through shell.Join,
// so no MCP-supplied string is ever spliced into the shell unquoted.
func containerShellCmd(args *buildArgs, distroTag string, skipPrepare bool) string {
	shellCmd := "yap " + shell.Join(buildCLIArgsFromArgs(args, distroTag))
	if skipPrepare {
		return shellCmd
	}

	return "yap " + shell.Join(prepareCLIArgs(args, distroTag)) + " && " + shellCmd
}

// prepareCLIArgs builds the inner `yap prepare` argv. Like the CLI's
// forwardedPrepareFlags it carries the extra repositories and cross arch so
// makedeps resolve against the same vendor repos and toolchain as the build.
func prepareCLIArgs(args *buildArgs, distroTag string) []string {
	c := []string{"prepare", distroTag}

	for _, r := range args.ExtraRepos {
		c = append(c, "--repo", r)
	}

	if args.TargetArch != "" {
		c = append(c, "--target-arch", args.TargetArch)
	}

	if args.SkipToolchainValidation {
		c = append(c, "--skip-toolchain-validation")
	}

	return c
}

// containerOutputDir returns the HOST path where a container build of the
// project at abs leaves its artifacts, or "" when it cannot be determined.
// PKGBUILD projects output into the project dir itself; yap.json projects use
// the file's "output" field, resolved against the in-container mount point.
func containerOutputDir(abs string) string {
	if _, err := os.Stat(filepath.Join(abs, "PKGBUILD")); err == nil {
		return abs
	}

	data, err := os.ReadFile(filepath.Join(abs, "yap.json")) //nolint:gosec // user-selected project
	if err != nil {
		return ""
	}

	var cfg struct {
		Output string `json:"output"`
	}

	if err := json.Unmarshal(data, &cfg); err != nil || cfg.Output == "" {
		return ""
	}

	if !filepath.IsAbs(cfg.Output) {
		return filepath.Join(abs, cfg.Output)
	}

	// Absolute output: only recoverable when it lives under the mount point.
	rel, err := filepath.Rel(containerProjectDir, cfg.Output)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return ""
	}

	return filepath.Join(abs, rel)
}

// innerDistroTag derives the distro/release identity forwarded to the inner
// yap process argv (both the build subcommand and the chained prepare
// step). It is deliberately independent of the container image tag: a bare
// family (release == "") stays bare so the release suffix stamped into the
// built package remains generic (e.g. "1ubuntu"), never leaking the host or
// container codename.
func innerDistroTag(distro, release string) string {
	if release == "" {
		return distro
	}

	return distro + "-" + release
}

// containerImageResolutionFailure registers and immediately fails a build
// session when the requested distro has no resolvable container image, so
// the MCP client sees an actionable error instead of a silent fallback to a
// native host build of the wrong distro.
func containerImageResolutionFailure(distro, release, abs string, err error) buildStartResult {
	sess, _ := defaultRegistry.Register(context.Background(), distro, release, abs)
	defaultRegistry.Finish(sess.ID, BuildStateFailed, err.Error())

	return buildStartResult{
		BuildID: sess.ID,
		State:   string(BuildStateFailed),
		Distro:  distro,
		Release: release,
		Path:    abs,
	}
}

// buildEnvFromArgs returns extra env vars to forward into the build
// container. It keeps the signing passphrase off the argv — yap's
// signing.Resolve* helpers already read YAP_SIGN_PASSPHRASE from the
// environment — and carries the apt unverified-repo opt-in as
// YAP_ALLOW_UNVERIFIED_REPOS so the chained `yap prepare` step (which has no
// such flag) honours it too.
func buildEnvFromArgs(args *buildArgs) map[string]string {
	var env map[string]string

	set := func(k, v string) {
		if env == nil {
			env = map[string]string{}
		}

		env[k] = v
	}

	if args.Sign && args.SignPassphrase != "" {
		set("YAP_SIGN_PASSPHRASE", args.SignPassphrase)
	}

	if args.UnverifiedRepos {
		set("YAP_ALLOW_UNVERIFIED_REPOS", "1")
	}

	return env
}

// containerProjectDir is where the host project dir is mounted inside every
// dispatched builder container, and therefore the path the inner yap argv
// must reference.
const containerProjectDir = "/project"

// buildCLIArgsFromArgs translates an MCP buildArgs into the yap CLI argv used
// when dispatching the build inside a container. Split into focused helpers
// to keep cyclomatic complexity under the project budget.
func buildCLIArgsFromArgs(args *buildArgs, distroTag string) []string {
	cliArgs := []string{toolNameBuild, distroTag, containerProjectDir}
	cliArgs = appendBoolFlags(cliArgs, args)
	cliArgs = appendStringFlags(cliArgs, args)
	cliArgs = appendListFlags(cliArgs, args)
	cliArgs = appendSigningFlags(cliArgs, args)

	return cliArgs
}

func appendBoolFlags(c []string, a *buildArgs) []string {
	flags := []struct {
		on   bool
		flag string
	}{
		{a.UnverifiedRepos, "--allow-unverified-repos"},
		{a.CleanBuild, "--cleanbuild"},
		{a.SkipSyncDeps, "--skip-sync"},
		{a.NoMakeDeps, "--no-makedeps"},
		{a.NoBuild, "--no-build"},
		{a.SkipHashCheck, "--skip-hash-check"},
		{a.NoCheck, "--nocheck"},
		{a.SkipToolchainValidation, "--skip-toolchain-validation"},
		{a.Zap, "--zap"},
		{a.Parallel, "--parallel"},
		{a.SBOM, "--sbom"},
		{a.Verbose, "--verbose"},
	}

	for _, f := range flags {
		if f.on {
			c = append(c, f.flag)
		}
	}

	return c
}

func appendStringFlags(c []string, a *buildArgs) []string {
	flags := []struct {
		val  string
		flag string
	}{
		{a.TargetArch, "--target-arch"},
		{a.SBOMFormat, "--sbom-format"},
		{a.CompressionDeb, "--compression-deb"},
		{a.CompressionRpm, "--compression-rpm"},
		{a.FromPkgName, "--from"},
		{a.ToPkgName, "--to"},
		{a.OnlyPkgNames, "--only"},
		{a.SkipPkgNames, "--skip"},
		{a.DebugDir, "--debug-dir"},
		{a.OverridePkgVer, "--pkgver"},
		{a.OverridePkgRel, "--pkgrel"},
	}

	for _, f := range flags {
		if f.val != "" {
			c = append(c, f.flag, f.val)
		}
	}

	return c
}

func appendListFlags(c []string, a *buildArgs) []string {
	for _, d := range a.SkipDeps {
		c = append(c, "--skip-deps", d)
	}

	for _, r := range a.ExtraRepos {
		c = append(c, "--repo", r)
	}

	return c
}

func appendSigningFlags(c []string, a *buildArgs) []string {
	if !a.Sign {
		return c
	}

	c = append(c, "--sign")

	if a.SignKey != "" {
		c = append(c, "--sign-key", a.SignKey)
	}

	if a.SignKeyName != "" {
		c = append(c, "--sign-key-name", a.SignKeyName)
	}

	// Passphrase is intentionally NOT added here — it travels via the
	// YAP_SIGN_PASSPHRASE env var injected by dispatchBuildInContainer so
	// it cannot be observed via `ps`.

	return c
}
