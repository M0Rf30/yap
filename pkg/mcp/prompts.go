package mcp

import (
	"context"
	"fmt"
	"strings"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

// registerPrompts installs reusable prompt templates for common yap workflows.
// Prompts are discoverable via prompts/list and selected by name; clients
// inline them into the user-facing chat to drive yap-mcp tool sequences.
// argNamePath / argNameDistro / argNameTargetArch / argNameSignKey /
// argNameSignKeyName name common prompt arguments. Extracted so goconst
// doesn't flag the repeats across the three prompt builders.
const (
	argNamePath       = "path"
	argNameDistro     = "distro"
	argNameTargetArch = "targetArch"
)

func registerPrompts(srv *mcpsdk.Server) {
	registerBuildSinglePkgPrompt(srv)
	registerCrossCompilePrompt(srv)
}

// userMessage builds a single user-role PromptMessage with plain text content.
func userMessage(text string) *mcpsdk.PromptMessage {
	return &mcpsdk.PromptMessage{
		Role:    "user",
		Content: &mcpsdk.TextContent{Text: text},
	}
}

// argOr returns args[name] when present, else fallback.
func argOr(args map[string]string, name, fallback string) string {
	if v, ok := args[name]; ok && v != "" {
		return v
	}

	return fallback
}

// ----- build_single_pkg ---------------------------------------------

func registerBuildSinglePkgPrompt(srv *mcpsdk.Server) {
	srv.AddPrompt(&mcpsdk.Prompt{
		Name:        "build_single_pkg",
		Title:       "Build a single package",
		Description: "Validate, build, and inspect one PKGBUILD or yap.json project natively on the host.",
		Arguments: []*mcpsdk.PromptArgument{
			{Name: argNamePath, Description: "yap.json file, PKGBUILD file, or its directory", Required: true},
			{Name: argNameDistro, Description: "optional distro to dispatch into a container (e.g. ubuntu-jammy)"},
		},
	}, func(_ context.Context, req *mcpsdk.GetPromptRequest,
	) (*mcpsdk.GetPromptResult, error) {
		path := argOr(req.Params.Arguments, argNamePath, "<path>")
		distro := argOr(req.Params.Arguments, argNameDistro, "")

		var b strings.Builder

		fmt.Fprintf(&b, "Build the package at %q using yap-mcp.\n\n", path)
		b.WriteString("Steps:\n")
		b.WriteString("  1. Call validate with that path. Abort if errors are non-empty.\n")

		if distro != "" {
			fmt.Fprintf(&b, "  2. Call build with path and distro=%q. Save the buildID.\n", distro)
		} else {
			b.WriteString("  2. Call build with that path (no distro = native host build). Save the buildID.\n")
		}

		b.WriteString("  3. Call build_wait with the buildID and timeoutSec=1800.\n")
		b.WriteString("  4. If state != succeeded, call build_summary(buildID) for a one-shot diagnosis,\n")
		b.WriteString("     then build_logs(buildID, tail=200, grep=\"ERROR|FAIL\", context=3, withLineNo=true) " +
			"only if more detail is needed.\n")
		b.WriteString("  5. On success, call list_artifacts(buildID) and inspect each artifact.\n")
		b.WriteString("  6. Report: artifacts found, formats, sizes, SBOM/signature presence.\n")
		b.WriteString("  Optional: rerun build with sign=true, sbom=true for signed release artifacts.\n")

		return &mcpsdk.GetPromptResult{
			Description: "Validate → build → wait → list_artifacts → inspect",
			Messages:    []*mcpsdk.PromptMessage{userMessage(b.String())},
		}, nil
	})
}

// ----- cross_compile -------------------------------------------------

func registerCrossCompilePrompt(srv *mcpsdk.Server) {
	srv.AddPrompt(&mcpsdk.Prompt{
		Name:        "cross_compile",
		Title:       "Cross-compile a package for another arch",
		Description: "Cross-compile a yap project for arm64/aarch64 (or any supported arch) and report results.",
		Arguments: []*mcpsdk.PromptArgument{
			{Name: argNamePath, Description: "yap.json or PKGBUILD path", Required: true},
			{
				Name: argNameTargetArch, Required: true,
				Description: "target arch: aarch64, x86_64, armv7, ... (arm64/amd64 aliases ok)",
			},
			{
				Name:        argNameDistro,
				Description: "container distro tag (e.g. ubuntu-jammy); recommended for cross builds",
			},
		},
	}, func(_ context.Context, req *mcpsdk.GetPromptRequest,
	) (*mcpsdk.GetPromptResult, error) {
		path := argOr(req.Params.Arguments, argNamePath, "<path>")
		arch := argOr(req.Params.Arguments, argNameTargetArch, "aarch64")
		distro := argOr(req.Params.Arguments, argNameDistro, "")

		var b strings.Builder

		fmt.Fprintf(&b, "Cross-compile %q for targetArch=%q.\n\n", path, arch)
		b.WriteString("Steps:\n")
		b.WriteString("  1. Call validate to catch PKGBUILD errors early.\n")

		if distro != "" {
			fmt.Fprintf(&b, "  2. Call build with path=%q, distro=%q, targetArch=%q.\n",
				path, distro, arch)
		} else {
			fmt.Fprintf(&b,
				"  2. Call build with path=%q, targetArch=%q. "+
					"Note: cross builds usually need a distro arg to dispatch into "+
					"a container with the right cross-toolchain.\n",
				path, arch)
		}

		b.WriteString("  3. Call build_wait with timeoutSec=3600 (cross builds are slower).\n")
		b.WriteString("  4. If state == failed, call build_summary(buildID) first.\n")
		b.WriteString("     If hints don't pinpoint it, call build_logs(buildID, " +
			"grep=\"cross|aarch64|arm|undefined reference|cannot execute\", context=3, withLineNo=true).\n")
		b.WriteString("  5. On success, call list_artifacts(buildID) and confirm " +
			"the produced .deb/.rpm Architecture field matches the target.\n")
		b.WriteString("  6. Note any libraries that fell back to host arch " +
			"(these often indicate Multi-Arch issues).\n")

		return &mcpsdk.GetPromptResult{
			Description: "validate → build (with targetArch) → wait → diagnose cross issues",
			Messages:    []*mcpsdk.PromptMessage{userMessage(b.String())},
		}, nil
	})
}
