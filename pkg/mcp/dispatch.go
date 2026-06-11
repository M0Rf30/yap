package mcp

import (
	"context"

	"github.com/M0Rf30/yap/v2/cmd/yap/command"
	"github.com/M0Rf30/yap/v2/pkg/container"
	"github.com/M0Rf30/yap/v2/pkg/shell"
)

// dispatchBuildInContainer mirrors the CLI's RunPipelineInContainer flow for
// the MCP build tool. It runs the build asynchronously (so the tool call
// returns immediately with a buildID) inside the appropriate yap container
// image, forwarding every flag the user passed via the MCP args.
//
// Returns (result, true) when dispatch was scheduled; (_, false) when no
// container runtime is available — the caller should then fall back to the
// native in-process build path.
func dispatchBuildInContainer(args *buildArgs, abs, distro, release string,
) (buildStartResult, bool) {
	rt, err := container.Detect(command.ContainerRuntimeOverride())
	if err != nil || rt == nil {
		return buildStartResult{}, false
	}

	distroTag := distro
	if release != "" {
		distroTag = distro + "-" + release
	}

	cliArgs := buildCLIArgsFromArgs(args, distroTag)
	skipPrepare := args.SkipSyncDeps || args.NoMakeDeps

	// Secrets (passphrase) travel via env, never as CLI args — argv is
	// visible to other processes on the host via `ps`.
	envVars := buildEnvFromArgs(args)

	sess, ctx := defaultRegistry.Register(context.Background(), distro, release, abs)
	defaultRegistry.UpdateContainer(sess.ID, string(rt.Type()), distroTag)

	go func() {
		shellCmd := "yap " + shell.Join(cliArgs)
		if !skipPrepare {
			shellCmd = "yap prepare " + distroTag + " && " + shellCmd
		}

		// Capture container stdout+stderr into the session's bounded log so
		// MCP clients can retrieve it via build_status. Pass the session
		// context so build_cancel can terminate the container.
		if err := rt.RunShellCapture(ctx, distroTag, abs, shellCmd, envVars, sess.Log); err != nil {
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
		ContainerImage:   distroTag,
	}, true
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

// buildCLIArgsFromArgs translates an MCP buildArgs into the yap CLI argv used
// when dispatching the build inside a container. Split into focused helpers
// to keep cyclomatic complexity under the project budget.
func buildCLIArgsFromArgs(args *buildArgs, distroTag string) []string {
	cliArgs := []string{toolNameBuild, distroTag, "/project"}
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
