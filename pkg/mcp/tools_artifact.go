package mcp

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/M0Rf30/yap/v2/cmd/yap/command"
	"github.com/M0Rf30/yap/v2/pkg/dnfinstall"
	yaperrors "github.com/M0Rf30/yap/v2/pkg/errors"
	"github.com/M0Rf30/yap/v2/pkg/project"
	"github.com/M0Rf30/yap/v2/pkg/shell"
)

// artifactFormatUnknown is the sentinel returned by detectArtifactFormat for
// paths that don't end in a recognised package extension. Used widely enough
// that goconst flags string-literal repeats.
const artifactFormatUnknown = "unknown"

// buildStateUnknown is reported by build_status / build_logs / build_wait
// when the buildID isn't in the registry.
const buildStateUnknown = "unknown"

// Artifact format identifiers returned by detectArtifactFormat and consumed
// by the install switch. Extracted as constants so goconst doesn't flag the
// repeated literals across switch/case bodies.
const (
	artifactFormatDeb = "deb"
	artifactFormatRPM = "rpm"
	artifactFormatAPK = "apk"
	artifactFormatPkg = "pkg"
)

func registerArtifactTools(srv *mcpsdk.Server) {
	registerInspect(srv)
	registerInstall(srv)
	registerZap(srv)
}

// hintTrue is a reusable *bool=true address used by ToolAnnotations fields
// like DestructiveHint and OpenWorldHint. Kept as a global rather than a
// hintTrue helper so the modernize linter doesn't flag every call site.
//
//nolint:gochecknoglobals // module-level reusable bool address
var hintTrue = func() *bool { v := true; return &v }()

// errNotConfirmed is returned (as a structured payload) when destructive
// tools are called without the `confirm: true` opt-in.
var errNotConfirmed = errors.New("destructive operation requires confirm: true")

// ----- inspect -------------------------------------------------------

type inspectArgs struct {
	Artifact string `json:"artifact" jsonschema:"path to a built package (.deb/.rpm/.apk/.pkg.tar.*)"`
}

type inspectResult struct {
	Artifact     string `json:"artifact"`
	Format       string `json:"format"`
	SizeBytes    int64  `json:"sizeBytes"`
	HasCycloneDX bool   `json:"hasCycloneDX"`
	HasSPDX      bool   `json:"hasSPDX"`
	HasSig       bool   `json:"hasSig"`
}

func registerInspect(srv *mcpsdk.Server) {
	mcpsdk.AddTool(srv, &mcpsdk.Tool{
		Name: "inspect",
		Description: "Inspect an artifact: format, size, sibling SBOM/signature presence. " +
			"Accepts any host path the server can stat — there is no project sandboxing; " +
			"clients should pass paths returned by list_artifacts or known build outputs.",
		Annotations: &mcpsdk.ToolAnnotations{ReadOnlyHint: true, IdempotentHint: true},
	}, func(_ context.Context, _ *mcpsdk.CallToolRequest, args inspectArgs,
	) (*mcpsdk.CallToolResult, inspectResult, error) {
		abs, err := filepath.Abs(args.Artifact)
		if err != nil {
			return nil, inspectResult{}, yaperrors.Wrap(err, yaperrors.ErrTypeFileSystem,
				"resolve path")
		}

		st, err := os.Stat(abs)
		if err != nil {
			return nil, inspectResult{}, yaperrors.Wrap(err, yaperrors.ErrTypeFileSystem,
				"stat artifact").WithContext("path", abs)
		}

		res := inspectResult{
			Artifact:  abs,
			Format:    detectArtifactFormat(abs),
			SizeBytes: st.Size(),
		}

		if _, err := os.Stat(abs + ".cdx.json"); err == nil {
			res.HasCycloneDX = true
		}

		if _, err := os.Stat(abs + ".spdx.json"); err == nil {
			res.HasSPDX = true
		}

		for _, suf := range []string{".asc", ".sig"} {
			if _, err := os.Stat(abs + suf); err == nil {
				res.HasSig = true
				break
			}
		}

		return nil, res, nil
	})
}

func detectArtifactFormat(path string) string {
	lower := strings.ToLower(filepath.Base(path))

	switch {
	case strings.HasSuffix(lower, ".deb"):
		return artifactFormatDeb
	case strings.HasSuffix(lower, ".rpm"):
		return artifactFormatRPM
	case strings.HasSuffix(lower, ".apk"):
		return artifactFormatAPK
	case strings.Contains(lower, ".pkg.tar."):
		return artifactFormatPkg
	default:
		return artifactFormatUnknown
	}
}

// ----- install -------------------------------------------------------

type installToolArgs struct {
	Artifact string `json:"artifact" jsonschema:"path to a built .deb/.rpm/.apk/.pkg.tar.* artifact"`
	Confirm  bool   `json:"confirm"  jsonschema:"must be true to perform the install; default refuses"`
}

type installToolResult struct {
	Artifact string `json:"artifact"`
	Format   string `json:"format"`
	OK       bool   `json:"ok"`
	Error    string `json:"error,omitempty"`
}

func registerInstall(srv *mcpsdk.Server) {
	mcpsdk.AddTool(srv, &mcpsdk.Tool{
		Name:        "install",
		Description: "Install a built package artifact on the host. Requires confirm: true.",
		Annotations: &mcpsdk.ToolAnnotations{DestructiveHint: hintTrue, OpenWorldHint: hintTrue},
	}, func(ctx context.Context, _ *mcpsdk.CallToolRequest, args installToolArgs,
	) (*mcpsdk.CallToolResult, installToolResult, error) {
		if !args.Confirm {
			return nil, installToolResult{Artifact: args.Artifact, Error: errNotConfirmed.Error()}, nil
		}

		abs, err := filepath.Abs(args.Artifact)
		if err != nil {
			return nil, installToolResult{Artifact: args.Artifact, Error: err.Error()}, nil //nolint:nilerr
		}

		if _, err := os.Stat(abs); err != nil {
			return nil, installToolResult{Artifact: abs, Error: err.Error()}, nil //nolint:nilerr
		}

		format := detectArtifactFormat(abs)

		out := installToolResult{Artifact: abs, Format: format}

		switch format {
		case artifactFormatRPM:
			err = dnfinstall.InstallFile(ctx, abs, dnfinstall.Options{
				RootDir:             "/",
				AllowRootInstall:    true,
				AllowUnverifiedRPMs: false,
				RunLDConfig:         true,
			})
		case artifactFormatDeb:
			err = shell.Exec(ctx, false, "", "apt-get",
				"--allow-downgrades", "--assume-yes", "install", abs)
		case artifactFormatAPK:
			err = shell.Exec(ctx, false, "", "apk", "add", "--allow-untrusted", abs)
		case artifactFormatPkg:
			err = shell.Exec(ctx, false, "", "pacman", "-U", "--noconfirm", abs)
		default:
			err = yaperrors.New(yaperrors.ErrTypeValidation,
				"unsupported artifact format").WithContext("path", abs)
		}

		if err != nil {
			out.Error = err.Error()
			return nil, out, nil //nolint:nilerr
		}

		out.OK = true

		return nil, out, nil
	})
}

// ----- zap -----------------------------------------------------------

type zapToolArgs struct {
	Path        string `json:"path"                  jsonschema:"yap.json file, PKGBUILD file, or dir with either"`
	Distro      string `json:"distro,omitempty"      jsonschema:"distro context; auto-detected when empty"`
	Release     string `json:"release,omitempty"     jsonschema:"release context"`
	FromPkgName string `json:"fromPkgName,omitempty" jsonschema:"only clean from this package onward"`
	ToPkgName   string `json:"toPkgName,omitempty"   jsonschema:"only clean up to this package"`
	Confirm     bool   `json:"confirm"               jsonschema:"must be true; zap removes build state and artifacts"`
}

type zapToolResult struct {
	Path    string `json:"path"`
	Distro  string `json:"distro"`
	Release string `json:"release"`
	OK      bool   `json:"ok"`
	Error   string `json:"error,omitempty"`
}

func registerZap(srv *mcpsdk.Server) {
	mcpsdk.AddTool(srv, &mcpsdk.Tool{
		Name:        "zap",
		Description: "Deeply clean a yap project's build env and artifacts. Requires confirm: true.",
		Annotations: &mcpsdk.ToolAnnotations{DestructiveHint: hintTrue},
	}, func(_ context.Context, _ *mcpsdk.CallToolRequest, args zapToolArgs,
	) (*mcpsdk.CallToolResult, zapToolResult, error) {
		if !args.Confirm {
			return nil, zapToolResult{Path: args.Path, Error: errNotConfirmed.Error()}, nil
		}

		abs, err := resolveProjectDir(args.Path)
		if err != nil {
			return nil, zapToolResult{Path: args.Path, Error: err.Error()}, nil //nolint:nilerr
		}

		distro, release := command.ResolveDistroRelease(args.Distro, args.Release, "")

		mpc := project.MultipleProject{
			Opts: project.BuildOptions{
				NoMakeDeps:   true,
				SkipSyncDeps: true,
				Zap:          true,
				FromPkgName:  args.FromPkgName,
				ToPkgName:    args.ToPkgName,
			},
		}

		out := zapToolResult{Path: abs, Distro: distro, Release: release}
		if err := mpc.MultiProject(distro, release, abs); err != nil {
			out.Error = err.Error()
			return nil, out, nil //nolint:nilerr
		}

		if err := mpc.Clean(); err != nil {
			out.Error = err.Error()
			return nil, out, nil //nolint:nilerr
		}

		out.OK = true

		return nil, out, nil
	})
}
