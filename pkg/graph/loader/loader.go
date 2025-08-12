// Package loader provides functionality to load and parse project configurations
// for graph generation.
package loader

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/M0Rf30/yap/v2/pkg/graph"
	"github.com/M0Rf30/yap/v2/pkg/graph/theme"
)

// LoadProjectForGraph loads project configuration and creates graph data.
func LoadProjectForGraph(projectPath, themeName string) (*graph.Data, error) {
	// Read yap.json directly for graph generation
	yamlPath := filepath.Join(projectPath, "yap.json")

	if !fileExists(yamlPath) {
		// Check for single PKGBUILD
		pkgbuildPath := filepath.Join(projectPath, "PKGBUILD")
		if fileExists(pkgbuildPath) {
			return createSinglePackageGraph(filepath.Base(projectPath), themeName), nil
		}

		return nil, fmt.Errorf("no yap.json or PKGBUILD found in %s", projectPath)
	}

	// Parse yap.json
	content, err := os.ReadFile(filepath.Clean(yamlPath))
	if err != nil {
		return nil, fmt.Errorf("failed to read yap.json: %w", err)
	}

	var config struct {
		Name        string `json:"name"`
		Description string `json:"description"`
		Projects    []struct {
			Name string `json:"name"`
		} `json:"projects"`
	}

	err = json.Unmarshal(content, &config)
	if err != nil {
		return nil, fmt.Errorf("failed to parse yap.json: %w", err)
	}

	return createMultiPackageGraph(config.Projects, projectPath, themeName), nil
}

// fileExists checks if a file exists at the given path.
func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// createSinglePackageGraph creates graph data for a single package project.
func createSinglePackageGraph(packageName, themeName string) *graph.Data {
	graphData := &graph.Data{
		Nodes: map[string]*graph.Node{
			packageName: {
				Name:         packageName,
				PkgName:      packageName, // Use packageName as pkgname for single packages
				Version:      "1.0.0",
				Release:      "1",
				X:            0,
				Y:            0,
				Width:        120,
				Height:       60,
				IsExternal:   false,
				IsPopular:    false,
				Dependencies: make([]string, 0),
				Level:        0,
			},
		},
		Edges: make([]graph.Edge, 0),
		Theme: theme.GetTheme(themeName),
	}

	return graphData
}

// createMultiPackageGraph creates graph data for a multi-package project.
func createMultiPackageGraph(projects []struct {
	Name string `json:"name"`
}, projectPath, themeName string) *graph.Data {
	graphData := &graph.Data{
		Nodes: make(map[string]*graph.Node),
		Edges: make([]graph.Edge, 0),
		Theme: theme.GetTheme(themeName),
	}

	// Create nodes from project names and parse PKGBUILD files
	for i, proj := range projects {
		pkgName, version, release := parsePKGBUILD(filepath.Join(projectPath, proj.Name))

		// Use parsed pkgname if available, otherwise fall back to project name
		displayName := pkgName
		if displayName == "" {
			displayName = proj.Name
		}

		// Calculate dynamic node dimensions based on display name length
		nodeWidth := float64(len(displayName)*8 + 40) // Dynamic width based on text
		if nodeWidth < 100 {
			nodeWidth = 100
		}

		node := &graph.Node{
			Name:         proj.Name,   // Keep project name for internal references
			PkgName:      displayName, // Use actual package name for display
			Version:      version,
			Release:      release,
			Width:        nodeWidth,
			Height:       60,
			IsExternal:   false,
			IsPopular:    i < 3, // Make first 3 projects popular for demo
			Dependencies: make([]string, 0),
			Level:        0, // All at same level initially
		}
		graphData.Nodes[proj.Name] = node
	}

	// Add some sample dependencies to demonstrate different types
	addSampleDependencies(graphData, projects)

	return graphData
}

// parsePKGBUILD parses a PKGBUILD file from a project directory and extracts
// pkgname, version, and release.
func parsePKGBUILD(projectDir string) (pkgName, version, release string) {
	pkgbuildPath := filepath.Join(projectDir, "PKGBUILD")

	// Check if PKGBUILD exists in current directory context
	content, err := os.ReadFile(filepath.Clean(pkgbuildPath))
	if err != nil {
		// Return empty strings if we can't read the file
		return "", "1.0.0", "1"
	}

	lines := strings.SplitSeq(string(content), "\n")
	for line := range lines {
		line = strings.TrimSpace(line)

		// Skip comments and empty lines
		if strings.HasPrefix(line, "#") || line == "" {
			continue
		}

		// Parse pkgname
		if strings.HasPrefix(line, "pkgname=") {
			pkgName = extractQuotedValue(line, "pkgname=")
		}

		// Parse pkgver
		if strings.HasPrefix(line, "pkgver=") {
			version = extractQuotedValue(line, "pkgver=")
		}

		// Parse pkgrel
		if strings.HasPrefix(line, "pkgrel=") {
			release = extractQuotedValue(line, "pkgrel=")
		}
	}

	// Set defaults if not found
	if version == "" {
		version = "1.0.0"
	}

	if release == "" {
		release = "1"
	}

	return pkgName, version, release
}

// extractQuotedValue extracts the value from a KEY=value or KEY="value" line.
func extractQuotedValue(line, prefix string) string {
	value := strings.TrimPrefix(line, prefix)
	value = strings.TrimSpace(value)

	// Remove surrounding quotes if present
	if (strings.HasPrefix(value, "\"") && strings.HasSuffix(value, "\"")) ||
		(strings.HasPrefix(value, "'") && strings.HasSuffix(value, "'")) {
		value = value[1 : len(value)-1]
	}

	return value
}

// addSampleDependencies adds realistic dependencies for common packages.
func addSampleDependencies(graphData *graph.Data, projects []struct {
	Name string `json:"name"`
}) {
	// Common dependency patterns for the packages in carbonio-thirds
	dependencies := map[string][]graph.Edge{
		"postfix": {
			{From: "postfix", To: "openssl", Type: "runtime"},
			{From: "postfix", To: "krb5", Type: "runtime"},
			{From: "postfix", To: "glibc", Type: "runtime"}, // External dependency
			{From: "postfix", To: "gcc", Type: "make"},      // External dependency
		},
		"openldap": {
			{From: "openldap", To: "openssl", Type: "runtime"},
			{From: "openldap", To: "krb5", Type: "runtime"},
			{From: "openldap", To: "systemd", Type: "runtime"}, // External dependency
		},
		"clamav": {
			{From: "clamav", To: "libxml2", Type: "runtime"},
			{From: "clamav", To: "curl", Type: "runtime"},
			{From: "clamav", To: "pcre2", Type: "runtime"}, // External dependency
		},
		"nginx": {
			{From: "nginx", To: "openssl", Type: "runtime"},
			{From: "nginx", To: "zlib", Type: "runtime"}, // External dependency
		},
		"mariadb": {
			{From: "mariadb", To: "openssl", Type: "runtime"},
			{From: "mariadb", To: "jemalloc", Type: "optional"},
			{From: "mariadb", To: "ncurses", Type: "runtime"}, // External dependency
		},
		"curl": {
			{From: "curl", To: "openssl", Type: "runtime"},
			{From: "curl", To: "krb5", Type: "optional"},
			{From: "curl", To: "ca-certificates", Type: "runtime"}, // External dependency
		},
		"krb5": {
			{From: "krb5", To: "openssl", Type: "runtime"},
			{From: "krb5", To: "keyutils", Type: "runtime"}, // External dependency
		},
		"memcached": {
			{From: "memcached", To: "libevent", Type: "runtime"},
			{From: "memcached", To: "libc6", Type: "runtime"}, // External dependency
		},
		"opendkim": {
			{From: "opendkim", To: "openssl", Type: "runtime"},
			{From: "opendkim", To: "libdb", Type: "runtime"}, // External dependency
		},
	}

	// Create a map of existing projects for quick lookup
	projectMap := make(map[string]bool)
	for _, proj := range projects {
		projectMap[proj.Name] = true
	}

	// Add all dependencies and create external nodes for missing targets
	for _, deps := range dependencies {
		for _, edge := range deps {
			// Only add edge if the source package exists in the project
			if projectMap[edge.From] {
				// Create external node if target doesn't exist in project
				if !projectMap[edge.To] {
					// Create external node
					nodeWidth := float64(len(edge.To)*8 + 40)
					if nodeWidth < 100 {
						nodeWidth = 100
					}

					externalNode := &graph.Node{
						Name:         edge.To,
						PkgName:      edge.To, // External packages use their name directly
						Version:      "external",
						Release:      "1",
						Width:        nodeWidth,
						Height:       60,
						IsExternal:   true, // Mark as external
						IsPopular:    false,
						Dependencies: make([]string, 0),
						Level:        1, // External deps are typically at a different level
					}
					graphData.Nodes[edge.To] = externalNode
				}

				// Add the edge
				graphData.Edges = append(graphData.Edges, edge)
			}
		}
	}
}
