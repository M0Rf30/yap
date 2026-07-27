package mcp

import (
	"context"

	"github.com/M0Rf30/yap/v2/cmd/yap/command"
	"github.com/M0Rf30/yap/v2/pkg/container"
	"github.com/M0Rf30/yap/v2/pkg/shell"
)

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
// Returns (result, true) when dispatch was scheduled or was rejected as a
// failed session; (_, false) when no container runtime is available — the
// caller should then fall back to the native in-process build path.
func dispatchBuildInContainer(args *buildArgs, abs, distro, release string,
) (buildStartResult, bool) {
	rt, err := container.Detect(command.ContainerRuntimeOverride())
	if err != nil || rt == nil {
		return buildStartResult{}, false
	}

	distroTag := innerDistroTag(distro, release)

	image, err := command.ResolveContainerImage(distro, release)
	if err != nil {
		return containerImageResolutionFailure(distro, release, abs, err), true
	}

	cliArgs := buildCLIArgsFromArgs(args, distroTag)
	skipPrepare := args.SkipSyncDeps || args.NoMakeDeps

	// Secrets (passphrase) travel via env, never as CLI args — argv is
	// visible to other processes on the host via `ps`.
	envVars := buildEnvFromArgs(args)

	sess, ctx := defaultRegistry.Register(context.Background(), distro, release, abs)
	defaultRegistry.UpdateContainer(sess.ID, string(rt.Type()), image)

	go func() {
		shellCmd := "yap " + shell.Join(cliArgs)
		if !skipPrepare {
			shellCmd = "yap prepare " + distroTag + " && " + shellCmd
		}

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
	}, true
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
// container. Currently used only to keep the signing passphrase off the
// argv — yap's signing.Resolve* helpers already read YAP_SIGN_PASSPHRASE
// from the environment.
func buildEnvFromArgs(args *buildArgs) map[string]string {
	if !args.Sign || args.SignPassphrase == "" {
		return nil
	}

	return map[string]string{"YAP_SIGN_PASSPHRASE": args.SignPassphrase}
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
		{a.SkipSyncDeps, "--skip-sync-deps"},
		{a.NoMakeDeps, "--no-make-deps"},
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
