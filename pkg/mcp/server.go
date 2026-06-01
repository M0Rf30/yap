// Package mcp wires the yap functionality onto a Model Context Protocol
// server. The package is transport-agnostic; cmd/yap-mcp picks the transport
// (stdio by default).
//
// Public surface:
//
//	NewServer() *mcp.Server  // returns a configured MCP server with all
//	                         // yap tools and resources registered.
//
// The handlers are intentionally thin: they call into existing yap packages
// (pkg/constants, pkg/buildinfo, pkg/parser, pkg/graph, pkg/project, ...) and
// translate results into MCP tool/resource payloads.
package mcp

//go:generate go run ../../cmd/mcp-surface

import (
	"context"
	"runtime"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/M0Rf30/yap/v2/cmd/yap/command"
	"github.com/M0Rf30/yap/v2/pkg/buildinfo"
	"github.com/M0Rf30/yap/v2/pkg/constants"
)

// ServerName is reported to MCP clients via the Implementation handshake.
const ServerName = "yap-mcp"

// Tool names exposed by NewServer. Extracted as constants so goconst doesn't
// flag the multiple references in registration + tests.
const (
	toolNameListDistros = "list_distros"
	toolNameStatus      = "status"
)

// serverInstructions is injected into the MCP initialize response and used by
// clients as a system-prompt-style hint about how to drive yap-mcp.
const serverInstructions = `yap-mcp builds native Linux packages from PKGBUILDs.

Typical flow:
  1. validate(path)           → catch PKGBUILD errors before a long build.
  2. build(path, distro?)     → returns buildID. Builds run async.
  3. build_wait(buildID)      → blocks until terminal state (or timeoutSec).
  4. On failure:              → build_summary(buildID) first (cheap, one-shot
                                 diagnosis with hints + lastErrorLine).
                                 Only fetch build_logs if summary insufficient.
                                 build_status returns lightweight state +
                                 logBytes/logLines; never the log payload.
  5. list_artifacts(buildID)  → discover .deb/.rpm/.apk/.pkg paths.
  6. inspect(artifact)        → check format/SBOM/signature.
  7. install(artifact, confirm=true) — destructive; opt-in only.

Token-efficient log fetching (no defaults — pass these explicitly):
  build_logs(buildID, tail=200, grep="ERROR|FAIL", context=3, withLineNo=true)
  returns numbered lines around matches; avoids paying for the full 256K buffer.

Arch aliases: pass arm64/amd64; yap normalises to aarch64/x86_64.
Paths accept yap.json, PKGBUILD, or the directory containing them.
Omit distro to build natively on host. Provide distro to dispatch into a container.
list_images shows pre-built distro tags you can pass to build(); pull(distro)
fetches one for offline use.

build() returns inContainer/containerRuntime/containerImage so you can tell
whether the work runs natively or inside an image. build_status echoes the
same fields. The signing passphrase (signPassphrase) is forwarded into
container builds via env (YAP_SIGN_PASSPHRASE) — never the argv — so it is
safe to use over the wire.

Named prompts (build_single_pkg, cross_compile) are discoverable via
prompts/list + prompts/get and walk you through full validate→build→inspect
sequences without you having to design them.`

// NewServer constructs an MCP server with the yap tool surface registered.
// The returned server has no transport bound; the caller runs it via
// server.Run(ctx, transport).
func NewServer() *mcpsdk.Server {
	srv := mcpsdk.NewServer(&mcpsdk.Implementation{
		Name:    ServerName,
		Version: buildinfo.Version,
	}, &mcpsdk.ServerOptions{
		Instructions: serverInstructions,
	})

	registerListDistros(srv)
	registerStatus(srv)
	registerReadonlyTools(srv)
	registerResources(srv)
	registerBuildPipelineTools(srv)
	registerBuildExtras(srv)
	registerArtifactTools(srv)
	registerPrompts(srv)

	srv.AddReceivingMiddleware(loggingMiddleware("recv"))
	srv.AddSendingMiddleware(loggingMiddleware("send"))

	return srv
}

// ----- list_distros --------------------------------------------------

type listDistrosArgs struct{}

// distroInfo is one entry in the listDistrosResult.Distros slice. It pairs the
// distribution id (as it appears in /etc/os-release) with the package manager
// yap uses for it (apt / yum / pacman / apk / zypper).
type distroInfo struct {
	Name           string `json:"name"           jsonschema:"distro id from /etc/os-release ID"`
	PackageManager string `json:"packageManager" jsonschema:"package manager: apt, yum, pacman, apk, zypper"`
}

type listDistrosResult struct {
	Distros  []distroInfo `json:"distros"  jsonschema:"distro ids paired with their pkg manager"`
	Packers  []string     `json:"packers"  jsonschema:"package managers yap knows how to drive"`
	Releases []string     `json:"releases" jsonschema:"deprecated alias for distros[*].name"`
}

// buildDistroCatalog returns the distro+packer data shared by the
// list_distros tool and the yap://distros resource.
func buildDistroCatalog() (distros []distroInfo, packers []string) {
	distros = make([]distroInfo, 0, len(constants.Distros))
	for _, name := range constants.Distros {
		distros = append(distros, distroInfo{
			Name:           name,
			PackageManager: constants.DistroPackageManager[name],
		})
	}

	packers = make([]string, 0, len(constants.Packers))
	packers = append(packers, constants.Packers[:]...)

	return distros, packers
}

func registerListDistros(srv *mcpsdk.Server) {
	mcpsdk.AddTool(srv, &mcpsdk.Tool{
		Name:        toolNameListDistros,
		Description: "List supported distributions, their package managers, and known packers.",
		Annotations: &mcpsdk.ToolAnnotations{ReadOnlyHint: true, IdempotentHint: true},
	}, func(_ context.Context, _ *mcpsdk.CallToolRequest, _ listDistrosArgs,
	) (*mcpsdk.CallToolResult, listDistrosResult, error) {
		distros, packers := buildDistroCatalog()

		// Releases kept for backwards compatibility — same content as distro names.
		releases := make([]string, 0, len(constants.Releases))
		releases = append(releases, constants.Releases[:]...)

		return nil, listDistrosResult{
			Distros:  distros,
			Packers:  packers,
			Releases: releases,
		}, nil
	})
}

// ----- status --------------------------------------------------------

type statusArgs struct{}

type statusResult struct {
	Version     string `json:"version"      jsonschema:"yap semantic version"`
	Commit      string `json:"commit"       jsonschema:"git short commit hash"`
	BuildTime   string `json:"buildTime"    jsonschema:"RFC3339 build timestamp"`
	GoVersion   string `json:"goVersion"    jsonschema:"go runtime version"`
	GOOS        string `json:"goos"         jsonschema:"runtime GOOS"`
	GOARCH      string `json:"goarch"       jsonschema:"runtime GOARCH"`
	InContainer bool   `json:"inContainer"  jsonschema:"true when running inside a container"`
}

func registerStatus(srv *mcpsdk.Server) {
	mcpsdk.AddTool(srv, &mcpsdk.Tool{
		Name:        toolNameStatus,
		Description: "Return yap version, build metadata, and runtime/container detection info.",
		Annotations: &mcpsdk.ToolAnnotations{ReadOnlyHint: true, IdempotentHint: true},
	}, func(_ context.Context, _ *mcpsdk.CallToolRequest, _ statusArgs,
	) (*mcpsdk.CallToolResult, statusResult, error) {
		return nil, statusResult{
			Version:     buildinfo.Version,
			Commit:      buildinfo.Commit,
			BuildTime:   buildinfo.BuildTime,
			GoVersion:   runtime.Version(),
			GOOS:        runtime.GOOS,
			GOARCH:      runtime.GOARCH,
			InContainer: command.IsInsideContainer(),
		}, nil
	})
}
