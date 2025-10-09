// Package loader provides functionality to load and parse project configurations
// for graph generation.
package loader

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
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
			pkgName, _, _ := parsePKGBUILD(projectPath)
			// Use parsed pkgname if available, otherwise fall back to directory name
			if pkgName == "" {
				pkgName = filepath.Base(projectPath)
			}

			return createSinglePackageGraph(pkgName, themeName), nil
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
				Dependencies: nil,
				Level:        0,
			},
		},
		Edges: nil,
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
			Dependencies: nil,
			Level:        0, // All at same level initially
		}
		graphData.Nodes[proj.Name] = node
	}

	// Add dependencies by parsing PKGBUILD files
	addDependenciesFromPKGBUILD(graphData, projects, projectPath)

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

// addDependenciesFromPKGBUILD parses actual dependencies from PKGBUILD files.
func addDependenciesFromPKGBUILD(graphData *graph.Data, projects []struct {
	Name string `json:"name"`
}, projectPath string) {
	// Create a map from pkgname to project name for quick lookup
	pkgnameToProject := make(map[string]string)

	for _, proj := range projects {
		node, exists := graphData.Nodes[proj.Name]
		if exists && node.PkgName != "" {
			pkgnameToProject[node.PkgName] = proj.Name
		}
	}

	// Parse dependencies from each PKGBUILD file
	for _, proj := range projects {
		pkgbuildPath := filepath.Join(projectPath, proj.Name, "PKGBUILD")
		dependencies := parseDependenciesFromPKGBUILD(pkgbuildPath)

		for depType, deps := range dependencies {
			for _, dep := range deps {
				// Clean up dependency name (remove version constraints)
				depName := cleanDependencyName(dep)

				// Find the project name corresponding to the dependency's pkgname
				targetProjectName, isInternal := pkgnameToProject[depName]

				// Create external node if target doesn't exist in project
				if !isInternal && depName != "" {
					// Avoid creating duplicate external nodes
					if _, exists := graphData.Nodes[depName]; !exists {
						nodeWidth := float64(len(depName)*8 + 40)
						if nodeWidth < 100 {
							nodeWidth = 100
						}

						externalNode := &graph.Node{
							Name:         depName,
							PkgName:      depName,
							Version:      "external",
							Release:      "1",
							Width:        nodeWidth,
							Height:       60,
							IsExternal:   true,
							IsPopular:    false,
							Dependencies: nil,
							Level:        1,
						}
						graphData.Nodes[depName] = externalNode
					}

					targetProjectName = depName // Use depName for edge to external node
				}

				// Add the edge if the dependency is not empty
				if depName != "" {
					edge := graph.Edge{
						From: proj.Name,
						To:   targetProjectName,
						Type: depType,
					}
					graphData.Edges = append(graphData.Edges, edge)
				}
			}
		}
	}
}

// parseDependenciesFromPKGBUILD extracts dependency arrays from a PKGBUILD file.
func parseDependenciesFromPKGBUILD(pkgbuildPath string) map[string][]string {
	dependencies := make(map[string][]string)

	content, err := os.ReadFile(filepath.Clean(pkgbuildPath))
	if err != nil {
		return dependencies
	}

	lines := strings.Split(string(content), "\n")
	currentArray := ""

	var currentDeps []string

	inArray := false

	for _, line := range lines {
		line = strings.TrimSpace(line)

		// Skip comments and empty lines
		if strings.HasPrefix(line, "#") || line == "" {
			continue
		}

		currentArray, currentDeps, inArray = processDependencyLine(
			line, currentArray, currentDeps, inArray, dependencies,
		)
	}

	return dependencies
}

// processDependencyLine processes a single line for dependency parsing.
func processDependencyLine(
	line, currentArray string,
	currentDeps []string,
	inArray bool,
	dependencies map[string][]string,
) (newArray string, newDeps []string, newInArray bool) {
	// Regex to match dynamic dependency declarations like depends__{distro}
	re := regexp.MustCompile(`^(depends|makedepends|checkdepends|optdepends)__\w+=\(`)

	switch {
	case strings.HasPrefix(line, "depends=("):
		return handleDependencyArrayStart(line, "depends=(", "runtime", dependencies)
	case strings.HasPrefix(line, "makedepends=("):
		return handleDependencyArrayStart(line, "makedepends=(", "make", dependencies)
	case strings.HasPrefix(line, "checkdepends=("):
		return handleDependencyArrayStart(line, "checkdepends=(", "check", dependencies)
	case strings.HasPrefix(line, "optdepends=("):
		return handleDependencyArrayStart(line, "optdepends=(", "optional", dependencies)
	case re.MatchString(line):
		prefix := re.FindString(line)
		arrayType := strings.Split(prefix, "__")[0]

		return handleDependencyArrayStart(line, prefix, arrayType, dependencies)
	case inArray:
		return handleArrayContinuation(line, currentArray, currentDeps, dependencies)
	default:
		return currentArray, currentDeps, inArray
	}
}

// handleDependencyArrayStart handles the start of a dependency array.
func handleDependencyArrayStart(
	line, prefix, arrayType string,
	dependencies map[string][]string,
) (newArray string, newDeps []string, newInArray bool) {
	currentDeps := parseDependencyArray(line, prefix)
	if !strings.HasSuffix(line, ")") {
		return arrayType, currentDeps, true
	}

	dependencies[arrayType] = currentDeps

	return "", nil, false
}

// handleArrayContinuation handles continuation of multi-line arrays.
func handleArrayContinuation(
	line, currentArray string,
	currentDeps []string,
	dependencies map[string][]string,
) (newArray string, newDeps []string, newInArray bool) {
	if strings.HasSuffix(line, ")") {
		// End of array
		line = strings.TrimSuffix(line, ")")
		if line != "" {
			moreDeps := parseDependencyLine(line)
			currentDeps = append(currentDeps, moreDeps...)
		}

		if currentArray != "" {
			dependencies[currentArray] = currentDeps
		}

		return "", nil, false
	}

	// Continue parsing array items
	moreDeps := parseDependencyLine(line)
	currentDeps = append(currentDeps, moreDeps...)

	return currentArray, currentDeps, true
}

// parseDependencyArray parses a dependency array line like depends=('pkg1' 'pkg2').
func parseDependencyArray(line, prefix string) []string {
	line = strings.TrimPrefix(line, prefix)
	line = strings.TrimSuffix(line, ")")

	return parseDependencyLine(line)
}

// parseDependencyLine parses individual dependency items from a line.
func parseDependencyLine(line string) []string {
	var deps []string

	// Split by spaces but respect quoted strings
	inQuote := false

	var currentDep strings.Builder

	quoteChar := byte(0)

	for i := 0; i < len(line); i++ {
		char := line[i]

		switch {
		case !inQuote && (char == '\'' || char == '"'):
			inQuote = true
			quoteChar = char
		case inQuote && char == quoteChar:
			inQuote = false

			dep := strings.TrimSpace(currentDep.String())
			if dep != "" {
				deps = append(deps, dep)
			}

			currentDep.Reset()

			quoteChar = 0
		case inQuote:
			currentDep.WriteByte(char)
		case char == ' ' || char == '\t':
			// Space outside quotes - potential separator
			dep := strings.TrimSpace(currentDep.String())
			if dep != "" && !inQuote {
				deps = append(deps, dep)

				currentDep.Reset()
			}
		default:
			currentDep.WriteByte(char)
		}
	}

	// Add final dependency if any
	dep := strings.TrimSpace(currentDep.String())
	if dep != "" {
		deps = append(deps, dep)
	}

	return deps
}

// cleanDependencyName removes version constraints from dependency names.
func cleanDependencyName(dep string) string {
	// Remove version constraints like >=1.0, <2.0, etc.
	for _, op := range []string{">=", "<=", "=", ">", "<"} {
		if idx := strings.Index(dep, op); idx != -1 {
			return strings.TrimSpace(dep[:idx])
		}
	}

	return strings.TrimSpace(dep)
}
