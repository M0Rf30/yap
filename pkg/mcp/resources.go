package mcp

import (
	"context"
	"encoding/json"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

// mimeJSON is the MIME type used for every JSON resource payload exposed by
// the server.
const mimeJSON = "application/json"

// registerResources installs the static resources offered by the MCP server.
// Parameterised lookups (e.g. PKGBUILD parsing) are exposed as tools instead
// of resource templates because URL-encoded filesystem paths in URIs are
// awkward for LLM clients to construct correctly.
func registerResources(srv *mcpsdk.Server) {
	registerDistrosResource(srv)
}

// ----- yap://distros -----------------------------------------------------

func registerDistrosResource(srv *mcpsdk.Server) {
	srv.AddResource(&mcpsdk.Resource{
		URI:         "yap://distros",
		Name:        "distros",
		Title:       "Supported distributions",
		Description: "JSON document listing every supported distro paired with its package manager.",
		MIMEType:    mimeJSON,
	}, func(_ context.Context, req *mcpsdk.ReadResourceRequest) (*mcpsdk.ReadResourceResult, error) {
		distros, packers := buildDistroCatalog()
		payload := map[string]any{
			"distros": distros,
			"packers": packers,
		}

		body, err := json.MarshalIndent(payload, "", "  ")
		if err != nil {
			return nil, err
		}

		return &mcpsdk.ReadResourceResult{
			Contents: []*mcpsdk.ResourceContents{{
				URI:      req.Params.URI,
				MIMEType: mimeJSON,
				Text:     string(body),
			}},
		}, nil
	})
}
