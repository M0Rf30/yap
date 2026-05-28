package mcp

import (
	"context"
	"os"
	"path/filepath"
	"sort"
	"strings"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/M0Rf30/yap/v2/cmd/yap/command"
	"github.com/M0Rf30/yap/v2/pkg/errors"
	"github.com/M0Rf30/yap/v2/pkg/graph"
	graphloader "github.com/M0Rf30/yap/v2/pkg/graph/loader"
	"github.com/M0Rf30/yap/v2/pkg/parser"
	"github.com/M0Rf30/yap/v2/pkg/pkgbuild"
)

func registerReadonlyTools(srv *mcpsdk.Server) {
	registerResolveDistro(srv)
	registerParsePkgbuild(srv)
	registerValidatePkgbuild(srv)
	registerGraph(srv)
	registerListImages(srv)
}

// ----- list_images ---------------------------------------------------

type listImagesArgs struct {
	RepoPath string `json:"repoPath,omitempty" jsonschema:"path to yap source tree; defaults to cwd"`
}

type imageInfo struct {
	Tag     string `json:"tag"     jsonschema:"dir name under build/deploy/, usable as 'distro' to build/pull"`
	HasGo   bool   `json:"hasGo"   jsonschema:"true when sibling <tag>-g image exists with Go preinstalled"`
	Distro  string `json:"distro"  jsonschema:"distro family from the tag (e.g. 'ubuntu' from 'ubuntu-noble')"`
	Release string `json:"release" jsonschema:"release/codename component of the tag; empty for single-name"`
}

type listImagesResult struct {
	RepoPath string      `json:"repoPath"`
	Images   []imageInfo `json:"images"`
}

func registerListImages(srv *mcpsdk.Server) {
	mcpsdk.AddTool(srv, &mcpsdk.Tool{
		Name: "list_images",
		Description: "List pre-built yap container image tags from build/deploy/ in the yap " +
			"source tree. Requires running with repoPath pointing at a yap checkout; " +
			"end-user installations should pass distros from list_distros instead.",
		Annotations: &mcpsdk.ToolAnnotations{ReadOnlyHint: true, IdempotentHint: true},
	}, func(_ context.Context, _ *mcpsdk.CallToolRequest, args listImagesArgs,
	) (*mcpsdk.CallToolResult, listImagesResult, error) {
		root := args.RepoPath
		if root == "" {
			cwd, err := os.Getwd()
			if err != nil {
				return nil, listImagesResult{}, err
			}

			root = cwd
		}

		abs, err := filepath.Abs(root)
		if err != nil {
			return nil, listImagesResult{}, err
		}

		deployDir := filepath.Join(abs, "build", "deploy")

		entries, err := os.ReadDir(deployDir)
		if err != nil {
			return nil, listImagesResult{RepoPath: abs}, err
		}

		tagSet := make(map[string]bool)

		for _, e := range entries {
			if !e.IsDir() {
				continue
			}

			tagSet[e.Name()] = true
		}

		images := make([]imageInfo, 0, len(tagSet))

		for tag := range tagSet {
			// Skip -g variants — they're surfaced via HasGo on the base tag.
			if strings.HasSuffix(tag, "-g") {
				continue
			}

			distro, release := splitTag(tag)
			images = append(images, imageInfo{
				Tag:     tag,
				HasGo:   tagSet[tag+"-g"],
				Distro:  distro,
				Release: release,
			})
		}

		// Stable order: lexicographic by tag.
		sort.Slice(images, func(i, j int) bool { return images[i].Tag < images[j].Tag })

		return nil, listImagesResult{RepoPath: abs, Images: images}, nil
	})
}

// splitTag splits "ubuntu-noble" → ("ubuntu", "noble"); single-name tags like
// "arch" or "alpine" return (tag, "").
func splitTag(tag string) (distro, release string) {
	before, after, ok := strings.Cut(tag, "-")
	if !ok {
		return tag, ""
	}

	return before, after
}

// ----- resolve_distro ------------------------------------------------

type resolveDistroArgs struct {
	Distro  string `json:"distro,omitempty"  jsonschema:"distro name; auto-detected from /etc/os-release when empty"`
	Release string `json:"release,omitempty" jsonschema:"release/codename; auto-detected when matching host"`
}

type resolveDistroResult struct {
	Distro  string `json:"distro"  jsonschema:"resolved distribution name"`
	Release string `json:"release" jsonschema:"resolved release / codename"`
}

func registerResolveDistro(srv *mcpsdk.Server) {
	mcpsdk.AddTool(srv, &mcpsdk.Tool{
		Name:        "resolve_distro",
		Description: "Auto-detect distribution and release from /etc/os-release.",
		Annotations: &mcpsdk.ToolAnnotations{ReadOnlyHint: true, IdempotentHint: true},
	}, func(_ context.Context, _ *mcpsdk.CallToolRequest, args resolveDistroArgs,
	) (*mcpsdk.CallToolResult, resolveDistroResult, error) {
		d, r := command.ResolveDistroRelease(args.Distro, args.Release, "")
		return nil, resolveDistroResult{Distro: d, Release: r}, nil
	})
}

// ----- parse_pkgbuild ------------------------------------------------

type parsePkgbuildArgs struct {
	Path       string `json:"path"                 jsonschema:"path to a dir containing a PKGBUILD, or to PKGBUILD itself"`
	Distro     string `json:"distro,omitempty"     jsonschema:"distro context for arch/distro-qualified directives"`
	Release    string `json:"release,omitempty"    jsonschema:"release/codename context; auto-detected when empty"`
	TargetArch string `json:"targetArch,omitempty" jsonschema:"target arch for cross-compile; empty for native"`
}

// pkgbuildSummary is a JSON-friendly projection of pkgbuild.PKGBUILD.
// We export only the fields most useful to MCP clients to keep payloads small.
type pkgbuildSummary struct {
	PkgName     string   `json:"pkgName"`
	PkgBase     string   `json:"pkgBase,omitempty"`
	PkgNames    []string `json:"pkgNames,omitempty"`
	PkgVer      string   `json:"pkgVer"`
	PkgRel      string   `json:"pkgRel"`
	Epoch       string   `json:"epoch,omitempty"`
	PkgDesc     string   `json:"pkgDesc"`
	URL         string   `json:"url,omitempty"`
	License     []string `json:"license,omitempty"`
	Arch        []string `json:"arch,omitempty"`
	Depends     []string `json:"depends,omitempty"`
	MakeDepends []string `json:"makeDepends,omitempty"`
	OptDepends  []string `json:"optDepends,omitempty"`
	Provides    []string `json:"provides,omitempty"`
	Conflicts   []string `json:"conflicts,omitempty"`
	Replaces    []string `json:"replaces,omitempty"`
	Sources     []string `json:"sources,omitempty"`
	HashSums    []string `json:"hashSums,omitempty"`
	Distro      string   `json:"distro,omitempty"`
	Codename    string   `json:"codename,omitempty"`
	TargetArch  string   `json:"targetArch,omitempty"`
	IsSplit     bool     `json:"isSplit"`
}

func summarisePkgbuild(p *pkgbuild.PKGBUILD) pkgbuildSummary {
	return pkgbuildSummary{
		PkgName:     p.PkgName,
		PkgBase:     p.PkgBase,
		PkgNames:    p.PkgNames,
		PkgVer:      p.PkgVer,
		PkgRel:      p.PkgRel,
		Epoch:       p.Epoch,
		PkgDesc:     p.PkgDesc,
		URL:         p.URL,
		License:     p.License,
		Arch:        p.Arch,
		Depends:     p.Depends,
		MakeDepends: p.MakeDepends,
		OptDepends:  p.OptDepends,
		Provides:    p.Provides,
		Conflicts:   p.Conflicts,
		Replaces:    p.Replaces,
		Sources:     p.SourceURI,
		HashSums:    p.HashSums,
		Distro:      p.Distro,
		Codename:    p.Codename,
		TargetArch:  p.TargetArch,
		IsSplit:     p.IsSplitPackage(),
	}
}

// resolvePkgbuildDir accepts a path pointing to either a directory containing
// PKGBUILD or directly to the PKGBUILD file, and returns the directory.
func resolvePkgbuildDir(path string) (string, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", errors.Wrap(err, errors.ErrTypeFileSystem, "resolve path")
	}

	if filepath.Base(abs) == "PKGBUILD" {
		return filepath.Dir(abs), nil
	}

	return abs, nil
}

// resolveProjectDir accepts a path pointing to either a yap.json file, a
// PKGBUILD file, or the directory containing one of them. It returns the
// directory yap's build pipeline expects (yap.json multi-project root or
// single-package PKGBUILD dir).
func resolveProjectDir(path string) (string, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", errors.Wrap(err, errors.ErrTypeFileSystem, "resolve path")
	}

	switch filepath.Base(abs) {
	case "yap.json", "PKGBUILD":
		return filepath.Dir(abs), nil
	}

	return abs, nil
}

func registerParsePkgbuild(srv *mcpsdk.Server) {
	mcpsdk.AddTool(srv, &mcpsdk.Tool{
		Name:        "parse_pkgbuild",
		Description: "Parse a PKGBUILD into structured JSON (name, version, deps, sources, ...).",
		Annotations: &mcpsdk.ToolAnnotations{ReadOnlyHint: true, IdempotentHint: true},
	}, func(_ context.Context, _ *mcpsdk.CallToolRequest, args parsePkgbuildArgs,
	) (*mcpsdk.CallToolResult, pkgbuildSummary, error) {
		dir, err := resolvePkgbuildDir(args.Path)
		if err != nil {
			return nil, pkgbuildSummary{}, err
		}

		distro, release := command.ResolveDistroRelease(args.Distro, args.Release, "")

		p, err := parser.ParseFile(distro, release, dir, dir, args.TargetArch)
		if err != nil {
			return nil, pkgbuildSummary{}, errors.Wrap(err, errors.ErrTypeParser,
				"parse PKGBUILD").WithContext("dir", dir)
		}

		return nil, summarisePkgbuild(p), nil
	})
}

// ----- validate ------------------------------------------------------

type validateArgs struct {
	Path    string `json:"path"              jsonschema:"path to PKGBUILD or its directory"`
	Distro  string `json:"distro,omitempty"  jsonschema:"distro context"`
	Release string `json:"release,omitempty" jsonschema:"release/codename context"`
}

type validateResult struct {
	Valid  bool     `json:"valid"            jsonschema:"true when all mandatory fields are well-formed"`
	Errors []string `json:"errors,omitempty" jsonschema:"validation error messages; empty when valid"`
	Pkg    string   `json:"pkg,omitempty"    jsonschema:"resolved package name"`
}

func registerValidatePkgbuild(srv *mcpsdk.Server) {
	mcpsdk.AddTool(srv, &mcpsdk.Tool{
		Name:        "validate",
		Description: "Validate a PKGBUILD: parse + check mandatory vars + general validation.",
		Annotations: &mcpsdk.ToolAnnotations{ReadOnlyHint: true, IdempotentHint: true},
	}, func(_ context.Context, _ *mcpsdk.CallToolRequest, args validateArgs,
	) (*mcpsdk.CallToolResult, validateResult, error) {
		dir, err := resolvePkgbuildDir(args.Path)
		if err != nil {
			return nil, validateResult{Errors: []string{err.Error()}}, nil //nolint:nilerr // user-facing error
		}

		distro, release := command.ResolveDistroRelease(args.Distro, args.Release, "")

		var errs []string

		p, err := parser.ParseFile(distro, release, dir, dir, "")
		if err != nil {
			errs = append(errs, err.Error())
			return nil, validateResult{Valid: false, Errors: errs}, nil
		}

		if err := p.ValidateMandatoryItems(); err != nil {
			errs = append(errs, err.Error())
		}

		if err := p.ValidateGeneral(); err != nil {
			errs = append(errs, err.Error())
		}

		return nil, validateResult{
			Valid:  len(errs) == 0,
			Errors: errs,
			Pkg:    p.PkgName,
		}, nil
	})
}

// ----- graph ---------------------------------------------------------

type graphArgs struct {
	Path  string `json:"path"            jsonschema:"yap.json file, PKGBUILD file, or dir with either"`
	Theme string `json:"theme,omitempty" jsonschema:"optional theme passed to loader; cosmetic only"`
}

type graphNode struct {
	Name         string   `json:"name"`
	PkgName      string   `json:"pkgName,omitempty"`
	Version      string   `json:"version,omitempty"`
	Release      string   `json:"release,omitempty"`
	IsExternal   bool     `json:"isExternal"`
	Level        int      `json:"level"`
	Dependencies []string `json:"dependencies,omitempty"`
}

type graphEdge struct {
	From string `json:"from"`
	To   string `json:"to"`
	Type string `json:"type" jsonschema:"runtime, make, check, or opt"`
}

type graphResult struct {
	Nodes []graphNode `json:"nodes"`
	Edges []graphEdge `json:"edges"`
}

func registerGraph(srv *mcpsdk.Server) {
	mcpsdk.AddTool(srv, &mcpsdk.Tool{
		Name:        "graph",
		Description: "Build the dependency graph (nodes+edges) for a yap project or single PKGBUILD.",
		Annotations: &mcpsdk.ToolAnnotations{ReadOnlyHint: true, IdempotentHint: true},
	}, func(_ context.Context, _ *mcpsdk.CallToolRequest, args graphArgs,
	) (*mcpsdk.CallToolResult, graphResult, error) {
		abs, err := resolveProjectDir(args.Path)
		if err != nil {
			return nil, graphResult{}, err
		}

		data, err := graphloader.LoadProjectForGraph(abs, args.Theme)
		if err != nil {
			return nil, graphResult{}, err
		}

		return nil, projectGraphToResult(data), nil
	})
}

func projectGraphToResult(data *graph.Data) graphResult {
	res := graphResult{
		Nodes: make([]graphNode, 0, len(data.Nodes)),
		Edges: make([]graphEdge, 0, len(data.Edges)),
	}

	for _, n := range data.Nodes {
		res.Nodes = append(res.Nodes, graphNode{
			Name:         n.Name,
			PkgName:      n.PkgName,
			Version:      n.Version,
			Release:      n.Release,
			IsExternal:   n.IsExternal,
			Level:        n.Level,
			Dependencies: n.Dependencies,
		})
	}

	for _, e := range data.Edges {
		res.Edges = append(res.Edges, graphEdge{From: e.From, To: e.To, Type: e.Type})
	}

	return res
}
