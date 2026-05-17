package loader_test

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/M0Rf30/yap/v2/pkg/graph"
	"github.com/M0Rf30/yap/v2/pkg/graph/loader"
)

// FuzzParseDependencyLine tests parseDependencyLine with arbitrary input.
// Must never panic. Result must be a valid (possibly empty) []string with no empty elements.
func FuzzParseDependencyLine(f *testing.F) {
	seeds := []string{
		"gcc",
		"gcc>=11",
		"libssl-dev (>= 1.0)",
		"python3-dev:amd64",
		"",
		">=1.0",
		"<=2.0",
		"=1.0",
		">1.0",
		"<2.0",
		"gcc make binutils",
		"'gcc' 'make'",
		"\"gcc\" \"make\"",
		"gcc 'make' binutils",
		"gcc>=1.0 make<=2.0",
		strings.Repeat("a", 10000),
		"gcc\nmake",
		"gcc\tmake",
		"gcc  make",
		"'gcc",
		"gcc'",
		"'gcc'make",
		"gcc'make'",
		"gcc:amd64:i386",
		"gcc>=1.0<=2.0",
		"gcc (>= 1.0) (< 2.0)",
	}

	for _, seed := range seeds {
		f.Add(seed)
	}

	f.Fuzz(func(t *testing.T, line string) {
		// Call the exported function
		result := loader.ParseDependencyLineExported(line)

		// Verify invariants
		// No empty elements allowed
		for _, dep := range result {
			if dep == "" {
				t.Errorf("Found empty element in result for input: %q", line)
			}
		}
	})
}

// FuzzCleanDependencyName tests cleanDependencyName with arbitrary input.
// Must never panic. Must not contain version operators in the result.
func FuzzCleanDependencyName(f *testing.F) {
	seeds := []string{
		"gcc",
		"gcc>=11",
		"gcc<=11",
		"gcc=11",
		"gcc>11",
		"gcc<11",
		"gcc:amd64",
		"gcc:amd64>=11",
		"libssl-dev (>= 1.0)",
		"python3-dev:amd64",
		"",
		":",
		">=",
		"<=",
		"=",
		">",
		"<",
		"gcc>=1.0<=2.0",
		"gcc:amd64:i386",
		"gcc (>= 1.0)",
		strings.Repeat("a", 10000),
		"gcc\n>=1.0",
		"gcc\t>=1.0",
		"gcc >=1.0",
	}

	for _, seed := range seeds {
		f.Add(seed)
	}

	f.Fuzz(func(t *testing.T, dep string) {
		result := loader.CleanDependencyNameExported(dep)

		// Result must not contain version operators (except in edge cases where input is only operators)
		// For inputs like "gcc>=1.0", result should be "gcc" (no operators)
		// For inputs like ">=", result can be empty (legitimate edge case)
		if dep != "" && !strings.ContainsAny(dep, ":") {
			// Only check for operators if input doesn't start with an operator
			if !strings.HasPrefix(strings.TrimSpace(dep), ">=") &&
				!strings.HasPrefix(strings.TrimSpace(dep), "<=") &&
				!strings.HasPrefix(strings.TrimSpace(dep), ">") &&
				!strings.HasPrefix(strings.TrimSpace(dep), "<") &&
				!strings.HasPrefix(strings.TrimSpace(dep), "=") {
				operators := []string{">=", "<=", "=", ">", "<"}
				for _, op := range operators {
					if strings.Contains(result, op) {
						t.Errorf("Result contains operator %q: %q (from input %q)", op, result, dep)
					}
				}
			}
		}
	})
}

// FuzzParseDependencyArray tests parseDependencyArray with arbitrary input.
// Must never panic.
func FuzzParseDependencyArray(f *testing.F) {
	seeds := []struct {
		line   string
		prefix string
	}{
		{"depends=('gcc' 'make')", "depends=("},
		{"makedepends=('git' 'cmake')", "makedepends=("},
		{"depends=()", "depends=("},
		{"depends=('gcc')", "depends=("},
		{"", ""},
		{"depends=(", "depends=("},
		{"depends=)", "depends=("},
		{"depends=('gcc", "depends=("},
		{"depends=gcc')", "depends=("},
		{strings.Repeat("a", 10000), "prefix"},
		{"depends=('gcc>=1.0' 'make')", "depends=("},
	}

	for _, seed := range seeds {
		f.Add(seed.line, seed.prefix)
	}

	f.Fuzz(func(t *testing.T, line, prefix string) {
		// Call the exported function - should not panic
		_ = loader.ParseDependencyArrayExported(line, prefix)
	})
}

// FuzzLoadProjectForGraph tests LoadProjectForGraph with fuzzed yap.json and PKGBUILD.
// Must never panic.
func FuzzLoadProjectForGraph(f *testing.F) {
	// Minimal valid yap.json
	minimalYapJSON := `{
  "name": "test-project",
  "projects": []
}`

	// yap.json with projects
	projectsYapJSON := `{
  "name": "multi-project",
  "projects": [
    {"name": "pkg1"},
    {"name": "pkg2"}
  ]
}`

	// Minimal valid PKGBUILD — no source/sha256sums to avoid empty-array Fatal.
	minimalPKGBUILD := `pkgname=test
pkgver=1.0
pkgrel=1
pkgdesc="test"
arch=('any')
license=('MIT')
package() { true; }
`

	seeds := []struct {
		yapJSON  string
		pkgbuild string
	}{
		{minimalYapJSON, ""},
		{projectsYapJSON, ""},
		{"", minimalPKGBUILD},
		{"{}", minimalPKGBUILD},
		{"invalid json", minimalPKGBUILD},
		{"", ""},
		{`{"name":"test"}`, ""},
	}

	for _, seed := range seeds {
		f.Add(seed.yapJSON, seed.pkgbuild)
	}

	f.Fuzz(func(t *testing.T, yapJSON, pkgbuild string) {
		// Create temporary directory
		tmpDir, err := os.MkdirTemp("", "fuzz-loader-*")
		if err != nil {
			t.Fatalf("Failed to create temp dir: %v", err)
		}
		defer func() { _ = os.RemoveAll(tmpDir) }()

		// Write yap.json if provided
		if yapJSON != "" {
			yapPath := filepath.Join(tmpDir, "yap.json")

			err = os.WriteFile(yapPath, []byte(yapJSON), 0o644)
			if err != nil {
				t.Fatalf("Failed to write yap.json: %v", err)
			}
		}

		// Write PKGBUILD if provided
		if pkgbuild != "" {
			pkgbuildPath := filepath.Join(tmpDir, "PKGBUILD")

			err = os.WriteFile(pkgbuildPath, []byte(pkgbuild), 0o644)
			if err != nil {
				t.Fatalf("Failed to write PKGBUILD: %v", err)
			}
		}

		// Call LoadProjectForGraph - should not panic
		_, _ = loader.LoadProjectForGraph(tmpDir, "default")
	})
}

// parseGraphString parses a simple graph format: "a->b,b->c" means a depends on b, b depends on c.
func parseGraphString(graphStr string, inDeg map[string]int, adj map[string][]string) {
	if graphStr == "" {
		return
	}

	edges := strings.SplitSeq(graphStr, ",")
	for edge := range edges {
		parts := strings.Split(edge, "->")
		if len(parts) != 2 {
			continue
		}

		from := strings.TrimSpace(parts[0])
		to := strings.TrimSpace(parts[1])

		if from == "" || to == "" {
			continue
		}

		if _, ok := inDeg[from]; !ok {
			inDeg[from] = 0
		}

		if _, ok := inDeg[to]; !ok {
			inDeg[to] = 0
		}

		adj[from] = append(adj[from], to)
		inDeg[to]++
	}
}

// FuzzKahnLongestPath tests kahnLongestPath with arbitrary graph structures.
// Must never panic.
func FuzzKahnLongestPath(f *testing.F) {
	seeds := []string{
		"a->b,b->c",
		"",
		"a->b",
		"a->b,b->a",
		"a->b,a->c,b->c",
		"invalid",
		"->",
		"a->",
		"->b",
	}

	for _, seed := range seeds {
		f.Add(seed)
	}

	f.Fuzz(func(t *testing.T, graphStr string) {
		// Parse simple graph format: "a->b,b->c" means a depends on b, b depends on c
		inDeg := make(map[string]int)
		adj := make(map[string][]string)

		// Simple parsing of graph string
		parseGraphString(graphStr, inDeg, adj)

		// Call the exported function - should not panic
		result := loader.KahnLongestPathExported(inDeg, adj)

		// Verify result is a valid map
		if result == nil {
			t.Error("KahnLongestPath returned nil")
		}

		// All levels should be non-negative
		for _, level := range result {
			if level < 0 {
				t.Errorf("Negative level found: %d", level)
			}
		}
	})
}

// FuzzBuildInternalGraph tests buildInternalGraph with arbitrary graph data.
// Must never panic.
func FuzzBuildInternalGraph(f *testing.F) {
	seeds := []struct {
		nodeCount int32
		edgeStr   string
	}{
		{5, "node0->node1,node1->node2"},
		{0, ""},
		{1, ""},
		{3, "node0->node1"},
		{10, "node0->node1,node1->node2,node2->node3"},
	}

	for _, seed := range seeds {
		f.Add(seed.nodeCount, seed.edgeStr)
	}

	f.Fuzz(func(t *testing.T, nodeCount int32, edgeStr string) {
		// Limit node count to avoid excessive memory usage
		if nodeCount < 0 || nodeCount > 100 {
			return
		}

		nodes := make(map[string]*graph.Node)

		for i := range nodeCount {
			name := fmt.Sprintf("node%d", i)
			nodes[name] = &graph.Node{
				Name:       name,
				IsExternal: i%2 == 0,
			}
		}

		edges := []graph.Edge{}

		if edgeStr != "" {
			edgeParts := strings.SplitSeq(edgeStr, ",")
			for edgePart := range edgeParts {
				parts := strings.Split(edgePart, "->")
				if len(parts) == 2 {
					from := strings.TrimSpace(parts[0])

					to := strings.TrimSpace(parts[1])
					if from != "" && to != "" {
						edges = append(edges, graph.Edge{From: from, To: to, Type: "depends"})
					}
				}
			}
		}

		graphData := &graph.Data{
			Nodes: nodes,
			Edges: edges,
		}

		// Call the exported function - should not panic
		inDeg, adj := loader.BuildInternalGraphExported(graphData)

		// Verify results are valid maps
		if inDeg == nil {
			t.Error("BuildInternalGraph returned nil inDeg")
		}

		if adj == nil {
			t.Error("BuildInternalGraph returned nil adj")
		}

		// All in-degrees should be non-negative
		for _, deg := range inDeg {
			if deg < 0 {
				t.Errorf("Negative in-degree found: %d", deg)
			}
		}
	})
}

// FuzzLoadProjectForGraphWithMultipleProjects tests LoadProjectForGraph with multiple projects.
// Must never panic.
func FuzzLoadProjectForGraphWithMultipleProjects(f *testing.F) {
	seeds := []struct {
		projectCount    int32
		pkgbuildContent string
	}{
		{1, `pkgname=pkg1
pkgver=1.0
pkgrel=1
pkgdesc="Package 1"
arch=('any')
license=('MIT')
package() { true; }
`},
		{2, `pkgname=pkg2
pkgver=2.0
pkgrel=1
pkgdesc="Package 2"
arch=('any')
license=('MIT')
depends=('pkg1')
package() { true; }
`},
		{0, ""},
	}

	for _, seed := range seeds {
		f.Add(seed.projectCount, seed.pkgbuildContent)
	}

	f.Fuzz(func(t *testing.T, projectCount int32, pkgbuildContent string) {
		// Limit project count to avoid excessive file creation
		if projectCount < 0 || projectCount > 20 {
			return
		}

		tmpDir, err := os.MkdirTemp("", "fuzz-loader-multi-*")
		if err != nil {
			t.Fatalf("Failed to create temp dir: %v", err)
		}

		defer func() { _ = os.RemoveAll(tmpDir) }()

		// Create yap.json with projects
		projects := make([]map[string]string, projectCount)

		for i := range projectCount {
			name := fmt.Sprintf("pkg%d", i)
			projects[i] = map[string]string{"name": name}
		}

		yapConfig := map[string]any{
			"name":     "test-project",
			"projects": projects,
		}

		yapJSON, _ := json.Marshal(yapConfig)
		yapPath := filepath.Join(tmpDir, "yap.json")

		err = os.WriteFile(yapPath, yapJSON, 0o644)
		if err != nil {
			t.Fatalf("Failed to write yap.json: %v", err)
		}

		// Create project directories with PKGBUILD files
		for i := range projectCount {
			projDir := filepath.Join(tmpDir, fmt.Sprintf("pkg%d", i))

			err = os.MkdirAll(projDir, 0o755)
			if err != nil {
				t.Fatalf("Failed to create project dir: %v", err)
			}

			if pkgbuildContent != "" {
				pkgbuildPath := filepath.Join(projDir, "PKGBUILD")

				err = os.WriteFile(pkgbuildPath, []byte(pkgbuildContent), 0o644)
				if err != nil {
					t.Fatalf("Failed to write PKGBUILD: %v", err)
				}
			}
		}

		// Call LoadProjectForGraph - should not panic
		_, _ = loader.LoadProjectForGraph(tmpDir, "default")
	})
}
