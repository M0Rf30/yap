package mcp

import (
	"context"
	"strings"
	"sync"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/M0Rf30/yap/v2/pkg/constants"

	"github.com/M0Rf30/yap/v2/cmd/yap/command"
	"github.com/M0Rf30/yap/v2/pkg/builders/common"
	"github.com/M0Rf30/yap/v2/pkg/container"
	"github.com/M0Rf30/yap/v2/pkg/errors"
	"github.com/M0Rf30/yap/v2/pkg/packer"
	"github.com/M0Rf30/yap/v2/pkg/parser"
	"github.com/M0Rf30/yap/v2/pkg/pkgbuild"
	"github.com/M0Rf30/yap/v2/pkg/project"
	"github.com/M0Rf30/yap/v2/pkg/repo"
	"github.com/M0Rf30/yap/v2/pkg/signing"
)

// toolNameBuild is the MCP tool name for the async build entrypoint.
// Extracted so goconst doesn't flag the multiple references in handler
// strings, log fields, and tests.
const toolNameBuild = "build"

// nativeBuildMu serialises native (in-process) MCP builds. Several yap
// internals (parser.OverridePkgVer/Rel, common.SkipToolchainValidation,
// and the project package's static state) are process-globals: running
// two builds concurrently in the same yap-mcp process would race on those
// values. Container-dispatched builds bypass this lock — each runs in its
// own yap subprocess with its own globals.
//
//nolint:gochecknoglobals // intentional process-singleton; see comment.
var nativeBuildMu sync.Mutex

func registerBuildPipelineTools(srv *mcpsdk.Server) {
	registerPrepare(srv)
	registerPull(srv)
	registerBuildAndStatus(srv)
}

// ----- prepare -------------------------------------------------------

type prepareArgs struct {
	Distro     string `json:"distro,omitempty"     jsonschema:"distribution name; auto-detected when empty"`
	Release    string `json:"release,omitempty"    jsonschema:"release/codename; auto-detected when empty"`
	GoLang     bool   `json:"goLang,omitempty"     jsonschema:"install Go toolchain"`
	TargetArch string `json:"targetArch,omitempty" jsonschema:"target arch; empty for native"`
	SkipSync   bool   `json:"skipSync,omitempty"   jsonschema:"skip package-manager update before prerequisites"`
}

type prepareResult struct {
	Distro  string `json:"distro"`
	Release string `json:"release"`
	OK      bool   `json:"ok"`
	Error   string `json:"error,omitempty"`
}

func registerPrepare(srv *mcpsdk.Server) {
	mcpsdk.AddTool(srv, &mcpsdk.Tool{
		Name:        "prepare",
		Description: "Prepare the host build environment (toolchain + base makedeps) for a distro.",
		Annotations: &mcpsdk.ToolAnnotations{DestructiveHint: hintTrue, OpenWorldHint: hintTrue},
	}, func(ctx context.Context, _ *mcpsdk.CallToolRequest, args prepareArgs,
	) (*mcpsdk.CallToolResult, prepareResult, error) {
		distro, release := command.ResolveDistroRelease(args.Distro, args.Release, "")

		pm, err := packer.GetPackageManager(&pkgbuild.PKGBUILD{}, distro, "", "")
		if err != nil {
			return nil, prepareResult{Distro: distro, Release: release, Error: err.Error()}, nil //nolint:nilerr
		}

		if err := repo.Setup(distro, release, nil); err != nil {
			return nil, prepareResult{Distro: distro, Release: release, Error: err.Error()}, nil //nolint:nilerr
		}

		if !args.SkipSync {
			if err := pm.Update(ctx); err != nil {
				return nil, prepareResult{Distro: distro, Release: release, Error: err.Error()}, nil //nolint:nilerr
			}
		}

		if err := pm.PrepareEnvironment(ctx, args.GoLang, args.TargetArch); err != nil {
			return nil, prepareResult{Distro: distro, Release: release, Error: err.Error()}, nil //nolint:nilerr
		}

		return nil, prepareResult{Distro: distro, Release: release, OK: true}, nil
	})
}

// ----- pull ----------------------------------------------------------

type pullArgs struct {
	Distro string `json:"distro" jsonschema:"distro tag, e.g. 'ubuntu' or 'ubuntu-noble'"`
}

type pullResult struct {
	Distro  string `json:"distro"`
	Runtime string `json:"runtime"`
	OK      bool   `json:"ok"`
	Error   string `json:"error,omitempty"`
}

func registerPull(srv *mcpsdk.Server) {
	mcpsdk.AddTool(srv, &mcpsdk.Tool{
		Name:        "pull",
		Description: "Pull the yap container image for the requested distro.",
		Annotations: &mcpsdk.ToolAnnotations{OpenWorldHint: hintTrue, IdempotentHint: true},
	}, func(_ context.Context, _ *mcpsdk.CallToolRequest, args pullArgs,
	) (*mcpsdk.CallToolResult, pullResult, error) {
		rt, err := container.Detect(command.ContainerRuntimeOverride())
		if err != nil {
			return nil, pullResult{Distro: args.Distro, Error: err.Error()}, nil //nolint:nilerr
		}

		res := pullResult{Distro: args.Distro, Runtime: string(rt.Type())}
		if err := rt.Pull(args.Distro); err != nil {
			res.Error = err.Error()
			return nil, res, nil //nolint:nilerr
		}

		res.OK = true

		return nil, res, nil
	})
}

// ----- build (async) + build_status ------------------------------

type buildArgs struct {
	Path                    string   `json:"path" jsonschema:"yap.json file, PKGBUILD file, or dir with either"`
	Distro                  string   `json:"distro,omitempty" jsonschema:"target distro; auto-detected when empty"`
	Release                 string   `json:"release,omitempty" jsonschema:"target release; auto-detected when empty"`
	TargetArch              string   `json:"targetArch,omitempty" jsonschema:"cross target; arm64/amd64 aliases ok"`
	Parallel                bool     `json:"parallel,omitempty" jsonschema:"run independent build steps in parallel"`
	CleanBuild              bool     `json:"cleanBuild,omitempty" jsonschema:"clean build dirs before starting"`
	NoBuild                 bool     `json:"noBuild,omitempty" jsonschema:"prepare/resolve only, skip the build step"`
	Zap                     bool     `json:"zap,omitempty" jsonschema:"remove the build dir after a successful build"`
	SkipSyncDeps            bool     `json:"skipSyncDeps,omitempty" jsonschema:"skip pkg-manager update before makedeps"`
	NoMakeDeps              bool     `json:"noMakeDeps,omitempty" jsonschema:"skip makedeps installation entirely"`
	SkipHashCheck           bool     `json:"skipHashCheck,omitempty" jsonschema:"disable sha verification of sources"`
	NoCheck                 bool     `json:"noCheck,omitempty" jsonschema:"skip the PKGBUILD check() function"`
	SkipToolchainValidation bool     `json:"skipToolchainValidation,omitempty" jsonschema:"skip cross toolchain checks"`
	SkipDeps                []string `json:"skipDeps,omitempty" jsonschema:"pkgs to omit from makedeps"`
	FromPkgName             string   `json:"fromPkgName,omitempty" jsonschema:"start build at this pkg"`
	ToPkgName               string   `json:"toPkgName,omitempty" jsonschema:"stop build order at this pkg (inclusive)"`
	OnlyPkgNames            string   `json:"onlyPkgNames,omitempty" jsonschema:"csv allowlist of pkg names to build"`
	SkipPkgNames            string   `json:"skipPkgNames,omitempty" jsonschema:"csv denylist of pkg names to skip"`
	DebugDir                string   `json:"debugDir,omitempty" jsonschema:"dir to capture per-step build debug artifacts"`
	OverridePkgVer          string   `json:"overridePkgVer,omitempty" jsonschema:"override PKGBUILD pkgver for all pkgs"`
	OverridePkgRel          string   `json:"overridePkgRel,omitempty" jsonschema:"override PKGBUILD pkgrel for all pkgs"`
	SBOM                    bool     `json:"sbom,omitempty" jsonschema:"emit CycloneDX+SPDX SBOMs per artifact"`
	SBOMFormat              string   `json:"sbomFormat,omitempty" jsonschema:"sbom format: cyclonedx, spdx, or both"`
	CompressionDeb          string   `json:"compressionDeb,omitempty" jsonschema:"deb compression: zstd, gzip, or xz"`
	CompressionRpm          string   `json:"compressionRpm,omitempty" jsonschema:"rpm compression: zstd, gzip, or xz"`
	Sign                    bool     `json:"sign,omitempty" jsonschema:"sign produced artifacts"`
	SignKey                 string   `json:"signKey,omitempty" jsonschema:"signing key path"`
	SignPassphrase          string   `json:"signPassphrase,omitempty" jsonschema:"passphrase for the signing key"`
	SignKeyName             string   `json:"signKeyName,omitempty" jsonschema:"key name embedded in APK signature stream"`
	UnverifiedRepos         bool     `json:"unverifiedRepos,omitempty" jsonschema:"allow apt sources w/o Signed-By"`
	ExtraRepos              []string `json:"extraRepos,omitempty" jsonschema:"extra apt/dnf repo defs (--repo syntax)"`
	Verbose                 bool     `json:"verbose,omitempty" jsonschema:"enable verbose logging for the build"`
}

type buildStartResult struct {
	BuildID     string `json:"buildID" jsonschema:"opaque id; poll via build_status"`
	State       string `json:"state"`
	Distro      string `json:"distro"`
	Release     string `json:"release"`
	Path        string `json:"path"`
	InContainer bool   `json:"inContainer,omitempty"     jsonschema:"true when dispatched into a container image"`
	//nolint:lll // jsonschema text would lose meaning if line-wrapped
	ContainerRuntime string `json:"containerRuntime,omitempty" jsonschema:"backend: cli (podman/docker) or rootless"`
	ContainerImage   string `json:"containerImage,omitempty"   jsonschema:"resolved image tag, e.g. ubuntu-jammy"`
}

func registerBuildAndStatus(srv *mcpsdk.Server) {
	mcpsdk.AddTool(srv, &mcpsdk.Tool{
		Name: toolNameBuild,
		Description: "Start a yap build asynchronously. Returns a buildID; poll via " +
			"build_status. The tool-call context is NOT propagated to the build — " +
			"use build_cancel to stop a running build.",
		Annotations: &mcpsdk.ToolAnnotations{DestructiveHint: hintTrue, OpenWorldHint: hintTrue},
	}, func(_ context.Context, _ *mcpsdk.CallToolRequest, args buildArgs,
	) (*mcpsdk.CallToolResult, buildStartResult, error) {
		abs, err := resolveProjectDir(args.Path)
		if err != nil {
			return nil, buildStartResult{}, errors.Wrap(err, errors.ErrTypeFileSystem,
				"resolve path").WithOperation(toolNameBuild)
		}

		distro, release := command.ResolveDistroRelease(args.Distro, args.Release, "")

		if err := validateBuildCompression(args.CompressionDeb, args.CompressionRpm); err != nil {
			return nil, buildStartResult{}, err
		}

		// Run cross-distro builds inside the matching yap image when invoked
		// from a host shell; container handlers fall through to the native path.
		if !command.IsInsideContainer() {
			if res, ok := dispatchBuildInContainer(&args, abs, distro, release); ok {
				return nil, res, nil
			}
		}

		opts := buildOptionsFromArgs(&args)

		mpc := &project.MultipleProject{
			Opts:           opts,
			CompressionDeb: args.CompressionDeb,
			CompressionRpm: args.CompressionRpm,
			SBOM:           args.SBOM,
			SBOMFormat:     args.SBOMFormat,
		}

		if args.Sign {
			cfg, err := signing.ResolveGeneric(args.SignKey, args.SignPassphrase, "", "")
			if err != nil {
				return nil, buildStartResult{}, errors.Wrap(err, errors.ErrTypeConfiguration,
					"resolve signing").WithOperation(toolNameBuild)
			}

			cfg.Enabled = true
			cfg.KeyName = args.SignKeyName
			mpc.Signing = &cfg
		}

		// Use background-derived ctx so the build outlives the tool-call ctx;
		// cancellation is driven by the registry.
		sess, buildCtx := defaultRegistry.Register(context.Background(), distro, release, abs)

		go runNativeBuild(buildCtx, sess.ID, mpc, distro, release, abs,
			args.OverridePkgVer, args.OverridePkgRel, args.SkipToolchainValidation)

		return nil, buildStartResult{
			BuildID: sess.ID,
			State:   string(BuildStateRunning),
			Distro:  distro,
			Release: release,
			Path:    abs,
		}, nil
	})

	type statusArgs struct {
		BuildID string `json:"buildID" jsonschema:"identifier returned by build"`
	}

	type statusResult struct {
		BuildID          string `json:"buildID"`
		State            string `json:"state"     jsonschema:"running, succeeded, failed, or canceled"`
		Distro           string `json:"distro,omitempty"`
		Release          string `json:"release,omitempty"`
		Path             string `json:"path,omitempty"`
		InContainer      bool   `json:"inContainer,omitempty"      jsonschema:"true when running in a yap image"`
		ContainerRuntime string `json:"containerRuntime,omitempty" jsonschema:"container backend: cli or rootless"`
		ContainerImage   string `json:"containerImage,omitempty"   jsonschema:"resolved image tag for the dispatch"`
		Error            string `json:"error,omitempty"`
		StartedAt        string `json:"startedAt,omitempty"`
		EndedAt          string `json:"endedAt,omitempty"`
		LogBytes         int    `json:"logBytes,omitempty"  jsonschema:"current size of captured log; fetch via build_logs"`
		LogLines         int    `json:"logLines,omitempty"  jsonschema:"current line count of captured log"`
	}

	mcpsdk.AddTool(srv, &mcpsdk.Tool{
		Name:        "build_status",
		Description: "Return the state of a previously-launched yap build.",
		Annotations: &mcpsdk.ToolAnnotations{ReadOnlyHint: true, IdempotentHint: true},
	}, func(_ context.Context, _ *mcpsdk.CallToolRequest, args statusArgs,
	) (*mcpsdk.CallToolResult, statusResult, error) {
		s := defaultRegistry.Get(args.BuildID)
		if s == nil {
			return nil, statusResult{BuildID: args.BuildID, State: buildStateUnknown}, nil
		}

		out := statusResult{
			BuildID:          s.ID,
			State:            string(s.State),
			Distro:           s.Distro,
			Release:          s.Release,
			Path:             s.Path,
			InContainer:      s.InContainer,
			ContainerRuntime: s.ContainerRuntime,
			ContainerImage:   s.ContainerImage,
			Error:            s.Err,
			StartedAt:        s.StartedAt.Format("2006-01-02T15:04:05Z"),
		}

		if !s.EndedAt.IsZero() {
			out.EndedAt = s.EndedAt.Format("2006-01-02T15:04:05Z")
		}

		if s.Log != nil {
			raw := s.Log.String()
			out.LogBytes = len(raw)
			out.LogLines = strings.Count(raw, "\n")
		}

		return nil, out, nil
	})

	type cancelArgs struct {
		BuildID string `json:"buildID"`
	}

	type cancelResult struct {
		Canceled bool `json:"canceled"`
	}

	mcpsdk.AddTool(srv, &mcpsdk.Tool{
		Name: "build_cancel",
		Description: "Cancel a running yap build by buildID. May leave partial artifacts " +
			"or stale package-manager locks inside the build container.",
		Annotations: &mcpsdk.ToolAnnotations{DestructiveHint: hintTrue, IdempotentHint: true},
	}, func(_ context.Context, _ *mcpsdk.CallToolRequest, args cancelArgs,
	) (*mcpsdk.CallToolResult, cancelResult, error) {
		return nil, cancelResult{Canceled: defaultRegistry.Cancel(args.BuildID)}, nil
	})
}

// buildOptionsFromArgs translates the MCP build args struct into the
// project.BuildOptions used by the in-process build path. Split out from the
// tool handler to keep the registration function under cyclo budget.
func buildOptionsFromArgs(args *buildArgs) project.BuildOptions {
	return project.BuildOptions{
		Verbose:                 args.Verbose,
		CleanBuild:              args.CleanBuild,
		NoBuild:                 args.NoBuild,
		NoMakeDeps:              args.NoMakeDeps,
		SkipSyncDeps:            args.SkipSyncDeps,
		SkipDeps:                args.SkipDeps,
		SkipToolchainValidation: args.SkipToolchainValidation,
		SkipHashCheck:           args.SkipHashCheck,
		NoCheck:                 args.NoCheck,
		Zap:                     args.Zap,
		Parallel:                args.Parallel,
		SBOM:                    args.SBOM,
		SBOMFormat:              args.SBOMFormat,
		FromPkgName:             args.FromPkgName,
		ToPkgName:               args.ToPkgName,
		OnlyPkgNames:            args.OnlyPkgNames,
		SkipPkgNames:            args.SkipPkgNames,
		DebugDir:                args.DebugDir,
		TargetArch:              args.TargetArch,
		AllowUnverifiedRepos:    args.UnverifiedRepos,
		ExtraRepos:              args.ExtraRepos,
	}
}

// runNativeBuild drives the actual project lifecycle inside a goroutine and
// updates the registry on completion. It serialises with other native builds
// via nativeBuildMu so the parser/common globals (OverridePkgVer/Rel,
// SkipToolchainValidation) can't race. Container-dispatched builds run in
// their own subprocess and don't touch this lock.
func runNativeBuild(ctx context.Context, buildID string, mpc *project.MultipleProject,
	distro, release, path, pkgVer, pkgRel string, skipToolchain bool,
) {
	nativeBuildMu.Lock()
	defer nativeBuildMu.Unlock()

	// Snapshot globals so we restore them when the build finishes.
	prevVer, prevRel := parser.OverridePkgVer, parser.OverridePkgRel
	prevSkip := common.SkipToolchainValidation

	parser.OverridePkgVer = pkgVer
	parser.OverridePkgRel = pkgRel
	common.SkipToolchainValidation = skipToolchain

	defer func() {
		parser.OverridePkgVer = prevVer
		parser.OverridePkgRel = prevRel
		common.SkipToolchainValidation = prevSkip
	}()

	if err := mpc.MultiProject(distro, release, path); err != nil {
		defaultRegistry.Finish(buildID, BuildStateFailed, err.Error())
		return
	}

	// Cache the resolved output dir so list_artifacts / build_summary
	// don't re-parse the project.
	if mpc.Output != "" {
		defaultRegistry.SetOutputDir(buildID, mpc.Output)
	}

	if err := mpc.BuildAll(ctx); err != nil {
		if ctx.Err() != nil {
			defaultRegistry.Finish(buildID, BuildStateCanceled, ctx.Err().Error())
			return
		}

		defaultRegistry.Finish(buildID, BuildStateFailed, err.Error())

		return
	}

	defaultRegistry.Finish(buildID, BuildStateSucceeded, "")
}

// validateBuildCompression validates the DEB and RPM compression algorithms
// against the canonical set in pkg/constants, shared with the CLI.
func validateBuildCompression(deb, rpm string) error {
	check := func(label, v string) error {
		if constants.IsSupportedCompression(v) {
			return nil
		}

		return errors.New(errors.ErrTypeValidation,
			"unsupported "+label+" compression "+v+" (want "+
				strings.Join(constants.SupportedCompressions, ", ")+")").
			WithOperation(toolNameBuild).
			WithContext("compression", v)
	}

	if err := check("deb", deb); err != nil {
		return err
	}

	return check("rpm", rpm)
}
