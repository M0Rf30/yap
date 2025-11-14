package loader

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/M0Rf30/yap/v2/pkg/graph"
)

func TestLoadProjectForGraph_MissingFiles(t *testing.T) {
	tempDir := t.TempDir()

	// Test with directory that has neither yap.json nor PKGBUILD
	_, err := LoadProjectForGraph(tempDir, "dark")
	if err == nil {
		t.Fatal("Expected error when no yap.json or PKGBUILD exists")
	}

	if err.Error() != "no yap.json or PKGBUILD found in "+tempDir {
		t.Errorf("Expected specific error message, got: %v", err)
	}
}

func TestLoadProjectForGraph_SinglePKGBUILD(t *testing.T) {
	tempDir := t.TempDir()

	// Create a PKGBUILD file
	pkgbuildContent := `pkgname="test-pkg"
pkgver="1.0.0"
pkgrel="1"
pkgdesc="Test package"
arch=("x86_64")
license=("GPL")
`
	pkgbuildPath := filepath.Join(tempDir, "PKGBUILD")

	err := os.WriteFile(pkgbuildPath, []byte(pkgbuildContent), 0o644)
	if err != nil {
		t.Fatalf("Failed to create PKGBUILD: %v", err)
	}

	// Test loading single package project
	graphData, err := LoadProjectForGraph(tempDir, "dark")
	if err != nil {
		t.Fatalf("Failed to load project: %v", err)
	}

	// Verify the graph data
	if len(graphData.Nodes) != 1 {
		t.Fatalf("Expected 1 node, got %d", len(graphData.Nodes))
	}

	// Since we parse the PKGBUILD, the node name should be the parsed pkgname
	nodeName := "test-pkg"

	node, exists := graphData.Nodes[nodeName]
	if !exists {
		t.Fatalf("Expected node with name %s", nodeName)
	}

	if node.PkgName != "test-pkg" {
		t.Errorf("Expected PkgName 'test-pkg', got '%s'", node.PkgName)
	}

	if node.Version != "1.0.0" {
		t.Errorf("Expected Version '1.0.0', got '%s'", node.Version)
	}

	if node.Release != "1" {
		t.Errorf("Expected Release '1', got '%s'", node.Release)
	}

	if len(graphData.Edges) != 0 {
		t.Errorf("Expected 0 edges for single package, got %d", len(graphData.Edges))
	}
}

func TestLoadProjectForGraph_SinglePKGBUILDNoParsing(t *testing.T) {
	tempDir := t.TempDir()

	// Create a PKGBUILD file with minimal content
	pkgbuildContent := `# Just a comment`
	pkgbuildPath := filepath.Join(tempDir, "PKGBUILD")

	err := os.WriteFile(pkgbuildPath, []byte(pkgbuildContent), 0o644)
	if err != nil {
		t.Fatalf("Failed to create PKGBUILD: %v", err)
	}

	// Test loading single package project with minimal PKGBUILD
	graphData, err := LoadProjectForGraph(tempDir, "classic")
	if err != nil {
		t.Fatalf("Failed to load project: %v", err)
	}

	// Verify the graph data with defaults
	nodeName := filepath.Base(tempDir)

	node, exists := graphData.Nodes[nodeName]
	if !exists {
		t.Fatalf("Expected node with name %s", nodeName)
	}

	// Should use directory name as pkgname when pkgname not found
	if node.PkgName != nodeName {
		t.Errorf("Expected PkgName to be directory name '%s', got '%s'", nodeName, node.PkgName)
	}

	// Should have default version and release
	if node.Version != "1.0.0" {
		t.Errorf("Expected default Version '1.0.0', got '%s'", node.Version)
	}

	if node.Release != "1" {
		t.Errorf("Expected default Release '1', got '%s'", node.Release)
	}
}

func TestLoadProjectForGraph_YapJSON(t *testing.T) {
	tempDir := t.TempDir()

	// Create a yap.json file
	yapJSONContent := `{
		"name": "test-project",
		"description": "Test project",
		"projects": [
			{
				"name": "project1"
			},
			{
				"name": "project2"
			}
		]
	}`
	yapJSONPath := filepath.Join(tempDir, "yap.json")

	err := os.WriteFile(yapJSONPath, []byte(yapJSONContent), 0o644)
	if err != nil {
		t.Fatalf("Failed to create yap.json: %v", err)
	}

	// Create project directories
	project1Dir := filepath.Join(tempDir, "project1")
	project2Dir := filepath.Join(tempDir, "project2")

	err = os.MkdirAll(project1Dir, 0o755)
	if err != nil {
		t.Fatalf("Failed to create project1 dir: %v", err)
	}

	err = os.MkdirAll(project2Dir, 0o755)
	if err != nil {
		t.Fatalf("Failed to create project2 dir: %v", err)
	}

	// Create PKGBUILD files in project directories
	pkgbuild1Content := `pkgname="pkg1"
pkgver="1.0.0"
pkgrel="1"
`
	pkgbuild2Content := `pkgname="pkg2"
pkgver="2.0.0"
pkgrel="2"
`

	err = os.WriteFile(filepath.Join(project1Dir, "PKGBUILD"), []byte(pkgbuild1Content), 0o644)
	if err != nil {
		t.Fatalf("Failed to create PKGBUILD for project1: %v", err)
	}

	err = os.WriteFile(filepath.Join(project2Dir, "PKGBUILD"), []byte(pkgbuild2Content), 0o644)
	if err != nil {
		t.Fatalf("Failed to create PKGBUILD for project2: %v", err)
	}

	// Test loading multi-package project
	graphData, err := LoadProjectForGraph(tempDir, "gradient")
	if err != nil {
		t.Fatalf("Failed to load project: %v", err)
	}

	// Verify the graph data
	if len(graphData.Nodes) != 2 {
		t.Fatalf("Expected 2 nodes, got %d", len(graphData.Nodes))
	}

	// Check project1 node
	node1, exists := graphData.Nodes["project1"]
	if !exists {
		t.Fatalf("Expected node for project1")
	}

	if node1.PkgName != "pkg1" {
		t.Errorf("Expected project1 PkgName 'pkg1', got '%s'", node1.PkgName)
	}

	if node1.Version != "1.0.0" {
		t.Errorf("Expected project1 Version '1.0.0', got '%s'", node1.Version)
	}

	// Check project2 node
	node2, exists := graphData.Nodes["project2"]
	if !exists {
		t.Fatalf("Expected node for project2")
	}

	if node2.PkgName != "pkg2" {
		t.Errorf("Expected project2 PkgName 'pkg2', got '%s'", node2.PkgName)
	}

	if node2.Version != "2.0.0" {
		t.Errorf("Expected project2 Version '2.0.0', got '%s'", node2.Version)
	}
}

func TestLoadProjectForGraph_YapJSONInvalid(t *testing.T) {
	tempDir := t.TempDir()

	// Create an invalid yap.json file
	yapJSONContent := `{
		"name": "test-project",
		"projects": [
			{
				"name": "project1"
			}
		`
	yapJSONPath := filepath.Join(tempDir, "yap.json")

	err := os.WriteFile(yapJSONPath, []byte(yapJSONContent), 0o644)
	if err != nil {
		t.Fatalf("Failed to create yap.json: %v", err)
	}

	// Test loading invalid yap.json
	_, err = LoadProjectForGraph(tempDir, "dark")
	if err == nil {
		t.Fatal("Expected error when parsing invalid yap.json")
	}
}

func TestLoadProjectForGraph_YapJSONEmpty(t *testing.T) {
	tempDir := t.TempDir()

	// Create an empty yap.json file
	yapJSONPath := filepath.Join(tempDir, "yap.json")

	err := os.WriteFile(yapJSONPath, []byte("{}"), 0o644)
	if err != nil {
		t.Fatalf("Failed to create yap.json: %v", err)
	}

	// Test loading empty yap.json
	graphData, err := LoadProjectForGraph(tempDir, "dark")
	if err != nil {
		t.Fatalf("Failed to load project: %v", err)
	}

	// Should have no nodes since there are no projects defined
	if len(graphData.Nodes) != 0 {
		t.Fatalf("Expected 0 nodes for empty yap.json, got %d", len(graphData.Nodes))
	}
}

func TestFileExists(t *testing.T) {
	tempDir := t.TempDir()

	// Test with existing file
	testFile := filepath.Join(tempDir, "test.txt")

	err := os.WriteFile(testFile, []byte("test"), 0o644)
	if err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	if !fileExists(testFile) {
		t.Errorf("Expected fileExists to return true for existing file")
	}

	// Test with non-existing file
	nonExistent := filepath.Join(tempDir, "non-existent.txt")
	if fileExists(nonExistent) {
		t.Errorf("Expected fileExists to return false for non-existing file")
	}
}

func TestCreateSinglePackageGraph(t *testing.T) {
	graphData := createSinglePackageGraph("test-package", "dark")

	if len(graphData.Nodes) != 1 {
		t.Fatalf("Expected 1 node, got %d", len(graphData.Nodes))
	}

	node, exists := graphData.Nodes["test-package"]
	if !exists {
		t.Fatalf("Expected node 'test-package'")
	}

	if node.Name != "test-package" {
		t.Errorf("Expected Name 'test-package', got '%s'", node.Name)
	}

	if node.PkgName != "test-package" {
		t.Errorf("Expected PkgName 'test-package', got '%s'", node.PkgName)
	}

	if node.Version != "1.0.0" {
		t.Errorf("Expected Version '1.0.0', got '%s'", node.Version)
	}

	if node.Release != "1" {
		t.Errorf("Expected Release '1', got '%s'", node.Release)
	}

	if len(graphData.Edges) != 0 {
		t.Errorf("Expected 0 edges, got %d", len(graphData.Edges))
	}

	// Check that theme is applied
	if graphData.Theme.Background != "#1a1a1a" { // Dark theme background
		t.Errorf("Expected dark theme, got background '%s'", graphData.Theme.Background)
	}
}

func TestCreateMultiPackageGraph(t *testing.T) {
	projects := []struct {
		Name string `json:"name"`
	}{
		{Name: "project1"},
		{Name: "project2"},
	}

	graphData := createMultiPackageGraph(projects, "/tmp", "classic")

	if len(graphData.Nodes) != 2 {
		t.Fatalf("Expected 2 nodes, got %d", len(graphData.Nodes))
	}

	node1, exists := graphData.Nodes["project1"]
	if !exists {
		t.Fatalf("Expected node 'project1'")
	}

	node2, exists := graphData.Nodes["project2"]
	if !exists {
		t.Fatalf("Expected node 'project2'")
	}

	if node1.Name != "project1" {
		t.Errorf("Expected project1 Name 'project1', got '%s'", node1.Name)
	}

	if node2.Name != "project2" {
		t.Errorf("Expected project2 Name 'project2', got '%s'", node2.Name)
	}

	// Check that first 3 projects are marked as popular (in this case both should be)
	if !node1.IsPopular {
		t.Errorf("Expected project1 to be popular")
	}

	if !node2.IsPopular {
		t.Errorf("Expected project2 to be popular")
	}

	// Check theme
	if graphData.Theme.Background != "#ffffff" { // Classic theme background
		t.Errorf("Expected classic theme, got background '%s'", graphData.Theme.Background)
	}
}

func TestCleanDependencyName(t *testing.T) {
	tests := []struct {
		name     string
		dep      string
		expected string
	}{
		{
			name:     "no version constraint",
			dep:      "gcc",
			expected: "gcc",
		},
		{
			name:     "greater than constraint",
			dep:      "gcc>=11.0",
			expected: "gcc",
		},
		{
			name:     "greater than or equal constraint",
			dep:      "gcc>=11.0",
			expected: "gcc",
		},
		{
			name:     "less than or equal constraint",
			dep:      "gcc<=12.0",
			expected: "gcc",
		},
		{
			name:     "equal constraint",
			dep:      "gcc=11.2.0",
			expected: "gcc",
		},
		{
			name:     "greater than constraint",
			dep:      "gcc>10.0",
			expected: "gcc",
		},
		{
			name:     "less than constraint",
			dep:      "gcc<15.0",
			expected: "gcc",
		},
		{
			name:     "whitespace handling",
			dep:      " gcc >= 11.0 ",
			expected: "gcc",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := cleanDependencyName(tt.dep)
			if result != tt.expected {
				t.Errorf("cleanDependencyName(%q) = %q, want %q", tt.dep, result, tt.expected)
			}
		})
	}
}

func TestParseDependencyLine(t *testing.T) {
	tests := []struct {
		name     string
		line     string
		expected []string
	}{
		{
			name:     "single quoted dependency",
			line:     "'dep1'",
			expected: []string{"dep1"},
		},
		{
			name:     "multiple quoted dependencies",
			line:     "'dep1' 'dep2' 'dep3'",
			expected: []string{"dep1", "dep2", "dep3"},
		},
		{
			name:     "mixed quotes",
			line:     "'dep1' \"dep2\" 'dep3'",
			expected: []string{"dep1", "dep2", "dep3"},
		},
		{
			name:     "unquoted dependency",
			line:     "dep1",
			expected: []string{"dep1"},
		},
		{
			name:     "empty line",
			line:     "",
			expected: []string{},
		},
		{
			name:     "whitespace",
			line:     "  'dep1'  'dep2'  ",
			expected: []string{"dep1", "dep2"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parseDependencyLine(tt.line)
			if len(result) != len(tt.expected) {
				t.Errorf("parseDependencyLine(%q) returned %d items, want %d", tt.line, len(result), len(tt.expected))
				t.Logf("Got: %v, Want: %v", result, tt.expected)

				return
			}

			for i, expected := range tt.expected {
				if i < len(result) && result[i] != expected {
					t.Errorf("parseDependencyLine(%q) result[%d] = %q, want %q", tt.line, i, result[i], expected)
				}
			}
		})
	}
}

func TestParseDependenciesFromPKGBUILD(t *testing.T) {
	tempDir := t.TempDir()
	pkgbuildPath := filepath.Join(tempDir, "PKGBUILD")

	// Create a PKGBUILD with various dependency types
	pkgbuildContent := `pkgname="test-pkg"
pkgver="1.0.0"
pkgrel="1"

depends=("dep1" "dep2")
makedepends=("make" "gcc")
checkdepends=("gtest")
optdepends=("optional-dep")
`

	err := os.WriteFile(pkgbuildPath, []byte(pkgbuildContent), 0o644)
	if err != nil {
		t.Fatalf("Failed to create PKGBUILD: %v", err)
	}

	dependencies := parseDependenciesFromPKGBUILD(pkgbuildPath)

	// Check that all dependency types are parsed
	if len(dependencies) == 0 {
		t.Fatal("Expected some dependencies to be parsed")
	}

	// Check runtime dependencies (stored as "runtime" not "depends")
	if runtimeDeps, exists := dependencies["runtime"]; exists {
		if len(runtimeDeps) != 2 || runtimeDeps[0] != "dep1" || runtimeDeps[1] != "dep2" {
			t.Errorf("Expected ['dep1', 'dep2'] for runtime deps, got %v", runtimeDeps)
		}
	} else {
		t.Error("Expected 'runtime' key in dependencies map")
		t.Logf("Available keys: %v", getMapKeys(dependencies))
	}

	// Check make dependencies (stored as "make")
	if makeDeps, exists := dependencies["make"]; exists {
		if len(makeDeps) != 2 || makeDeps[0] != "make" || makeDeps[1] != "gcc" {
			t.Errorf("Expected ['make', 'gcc'] for make deps, got %v", makeDeps)
		}
	} else {
		t.Error("Expected 'make' key in dependencies map")
		t.Logf("Available keys: %v", getMapKeys(dependencies))
	}
}

// Helper function to get map keys for debugging
func getMapKeys(m map[string][]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}

	return keys
}

func TestParsePKGBUILD(t *testing.T) {
	tempDir := t.TempDir()
	pkgbuildPath := filepath.Join(tempDir, "PKGBUILD")

	pkgbuildContent := `# This is a comment
pkgname="test-pkg"
pkgver="1.5.2"
pkgrel="3"
# Another comment
`

	err := os.WriteFile(pkgbuildPath, []byte(pkgbuildContent), 0o644)
	if err != nil {
		t.Fatalf("Failed to create PKGBUILD: %v", err)
	}

	pkgName, version, release := parsePKGBUILD(tempDir)

	if pkgName != "test-pkg" {
		t.Errorf("Expected pkgName 'test-pkg', got '%s'", pkgName)
	}

	if version != "1.5.2" {
		t.Errorf("Expected version '1.5.2', got '%s'", version)
	}

	if release != "3" {
		t.Errorf("Expected release '3', got '%s'", release)
	}
}

func TestParsePKGBUILDMissingValues(t *testing.T) {
	tempDir := t.TempDir()
	pkgbuildPath := filepath.Join(tempDir, "PKGBUILD")

	pkgbuildContent := `# Just comments
# No actual values
`

	err := os.WriteFile(pkgbuildPath, []byte(pkgbuildContent), 0o644)
	if err != nil {
		t.Fatalf("Failed to create PKGBUILD: %v", err)
	}

	pkgName, version, release := parsePKGBUILD(tempDir)

	if pkgName != "" {
		t.Errorf("Expected empty pkgName, got '%s'", pkgName)
	}

	if version != "1.0.0" {
		t.Errorf("Expected default version '1.0.0', got '%s'", version)
	}

	if release != "1" {
		t.Errorf("Expected default release '1', got '%s'", release)
	}
}

func TestGraphPackageImportUsed(t *testing.T) {
	// This function exists to ensure the graph package import is used
	// The import is also used in other functions when working with graphData.Theme
	_ = graph.Data{} // Explicitly use the graph package to ensure import is recognized
}
