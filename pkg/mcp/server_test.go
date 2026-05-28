package mcp_test

import (
	"context"
	"testing"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"

	yapmcp "github.com/M0Rf30/yap/v2/pkg/mcp"
)

// TestNewServer_RoundTrip wires the server to an in-memory client and
// verifies the registered tools are reachable and return well-typed payloads.
func TestNewServer_RoundTrip(t *testing.T) {
	ctx := context.Background()

	srv := yapmcp.NewServer()
	client := mcpsdk.NewClient(&mcpsdk.Implementation{Name: "test", Version: "0"}, nil)

	srvT, clientT := mcpsdk.NewInMemoryTransports()

	srvSess, err := srv.Connect(ctx, srvT, nil)
	if err != nil {
		t.Fatalf("server connect: %v", err)
	}

	defer func() {
		if cerr := srvSess.Close(); cerr != nil {
			t.Errorf("server session close: %v", cerr)
		}
	}()

	cs, err := client.Connect(ctx, clientT, nil)
	if err != nil {
		t.Fatalf("client connect: %v", err)
	}

	defer func() {
		if cerr := cs.Close(); cerr != nil {
			t.Errorf("client session close: %v", cerr)
		}
	}()

	// tools/list
	lt, err := cs.ListTools(ctx, &mcpsdk.ListToolsParams{})
	if err != nil {
		t.Fatalf("ListTools: %v", err)
	}

	want := map[string]bool{
		"list_distros":   false,
		"status":         false,
		"resolve_distro": false,
		"parse_pkgbuild": false,
		"validate":       false,
		"graph":          false,
		"list_images":    false,
		"prepare":        false,
		"pull":           false,
		"build":          false,
		"build_status":   false,
		"build_cancel":   false,
		"inspect":        false,
		"install":        false,
		"zap":            false,
	}
	for _, tool := range lt.Tools {
		if _, ok := want[tool.Name]; ok {
			want[tool.Name] = true
		}
	}

	for name, found := range want {
		if !found {
			t.Errorf("tool %q not advertised", name)
		}
	}

	// tools/call list_distros
	res, err := cs.CallTool(ctx, &mcpsdk.CallToolParams{Name: "list_distros"})
	if err != nil {
		t.Fatalf("CallTool list_distros: %v", err)
	}

	if res.IsError {
		t.Fatalf("list_distros returned error: %+v", res.Content)
	}

	if len(res.Content) == 0 {
		t.Error("list_distros returned no content")
	}

	// tools/call status
	res, err = cs.CallTool(ctx, &mcpsdk.CallToolParams{Name: "status"})
	if err != nil {
		t.Fatalf("CallTool status: %v", err)
	}

	if res.IsError {
		t.Fatalf("status returned error: %+v", res.Content)
	}
}
