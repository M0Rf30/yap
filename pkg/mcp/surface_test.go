package mcp_test

import (
	"context"
	"sort"
	"testing"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"

	yapmcp "github.com/M0Rf30/yap/v2/pkg/mcp"
)

// The MCP surface is contract-level: clients, the SKILL card
// (skills/yap/SKILL.md), and the AGENTS.md "MCP surface" section all enumerate
// these names. When you add or remove a tool/prompt/resource, update the
// expected set below AND those docs in the same change. A drift here fails CI
// on purpose so the docs cannot silently rot.

var wantTools = []string{
	"build",
	"build_cancel",
	"build_logs",
	"build_status",
	"build_summary",
	"build_wait",
	"graph",
	"inspect",
	"install",
	"list_artifacts",
	"list_distros",
	"list_images",
	"parse_pkgbuild",
	"prepare",
	"pull",
	"resolve_distro",
	"status",
	"validate",
	"zap",
}

var wantPrompts = []string{
	"build_single_pkg",
	"cross_compile",
}

var wantResources = []string{
	"yap://distros",
}

func connectClient(t *testing.T) (cs *mcpsdk.ClientSession, cleanup func()) {
	t.Helper()

	ctx := context.Background()
	srv := yapmcp.NewServer()
	client := mcpsdk.NewClient(&mcpsdk.Implementation{Name: "test", Version: "0"}, nil)

	srvT, clientT := mcpsdk.NewInMemoryTransports()

	srvSess, err := srv.Connect(ctx, srvT, nil)
	if err != nil {
		t.Fatalf("server connect: %v", err)
	}

	cs, err = client.Connect(ctx, clientT, nil)
	if err != nil {
		_ = srvSess.Close()

		t.Fatalf("client connect: %v", err)
	}

	return cs, func() {
		if cerr := cs.Close(); cerr != nil {
			t.Errorf("client session close: %v", cerr)
		}

		if cerr := srvSess.Close(); cerr != nil {
			t.Errorf("server session close: %v", cerr)
		}
	}
}

func assertExactSet(t *testing.T, kind string, got, want []string) {
	t.Helper()

	gotSet := map[string]bool{}
	for _, g := range got {
		gotSet[g] = true
	}

	wantSet := map[string]bool{}
	for _, w := range want {
		wantSet[w] = true
	}

	var missing, extra []string

	for _, w := range want {
		if !gotSet[w] {
			missing = append(missing, w)
		}
	}

	for _, g := range got {
		if !wantSet[g] {
			extra = append(extra, g)
		}
	}

	sort.Strings(missing)
	sort.Strings(extra)

	if len(missing) > 0 {
		t.Errorf("%s missing from server (declared in test/docs but not registered): %v",
			kind, missing)
	}

	if len(extra) > 0 {
		t.Errorf("%s registered but not in expected set — add it to wantTools/wantPrompts/"+
			"wantResources AND to skills/yap/SKILL.md + AGENTS.md: %v", kind, extra)
	}
}

// TestToolSurfaceExact asserts the registered tool set matches the documented
// set exactly (no missing, no extra).
func TestToolSurfaceExact(t *testing.T) {
	cs, cleanup := connectClient(t)
	defer cleanup()

	lt, err := cs.ListTools(context.Background(), &mcpsdk.ListToolsParams{})
	if err != nil {
		t.Fatalf("ListTools: %v", err)
	}

	got := make([]string, 0, len(lt.Tools))
	for _, tool := range lt.Tools {
		got = append(got, tool.Name)
	}

	assertExactSet(t, "tool", got, wantTools)
}

// TestPromptSurfaceExact asserts the registered prompt set matches the docs.
func TestPromptSurfaceExact(t *testing.T) {
	cs, cleanup := connectClient(t)
	defer cleanup()

	lp, err := cs.ListPrompts(context.Background(), &mcpsdk.ListPromptsParams{})
	if err != nil {
		t.Fatalf("ListPrompts: %v", err)
	}

	got := make([]string, 0, len(lp.Prompts))
	for _, p := range lp.Prompts {
		got = append(got, p.Name)
	}

	assertExactSet(t, "prompt", got, wantPrompts)
}

// TestResourceSurfaceExact asserts the registered resource URIs match the docs.
func TestResourceSurfaceExact(t *testing.T) {
	cs, cleanup := connectClient(t)
	defer cleanup()

	lr, err := cs.ListResources(context.Background(), &mcpsdk.ListResourcesParams{})
	if err != nil {
		t.Fatalf("ListResources: %v", err)
	}

	got := make([]string, 0, len(lr.Resources))
	for _, r := range lr.Resources {
		got = append(got, r.URI)
	}

	assertExactSet(t, "resource", got, wantResources)
}
