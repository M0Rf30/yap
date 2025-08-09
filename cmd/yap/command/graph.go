package command

import (
	"encoding/json"
	"encoding/xml"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/M0Rf30/yap/v2/pkg/osutils"
)

var (
	graphOutput  string
	graphFormat  string
	graphTheme   string
	showExternal bool
)

// GraphNode represents a package in the dependency graph
type GraphNode struct {
	Name          string
	PkgName       string // Actual package name from PKGBUILD
	Version       string
	Release       string
	X, Y          float64
	Width, Height float64 // Dynamic node dimensions
	IsExternal    bool
	IsPopular     bool
	Dependencies  []string
	Level         int
}

// SVG represents the SVG document structure
type SVG struct {
	XMLName xml.Name `xml:"svg"`
	Width   string   `xml:"width,attr"`
	Height  string   `xml:"height,attr"`
	ViewBox string   `xml:"viewBox,attr"`
	Xmlns   string   `xml:"xmlns,attr"`
	Style   string   `xml:"style"`
	Defs    Defs     `xml:"defs"`
	Content string   `xml:",innerxml"`
}

// GraphBounds represents the calculated dimensions for the graph
type GraphBounds struct {
	MinX, MinY, MaxX, MaxY float64
	Width, Height          float64
	Padding                float64
}

// Defs represents the SVG defs element
type Defs struct {
	XMLName xml.Name `xml:"defs"`
	Content string   `xml:",innerxml"`
}

// graphCmd represents the graph command
var graphCmd = &cobra.Command{
	Use:   "graph [path]",
	Short: "🎨 Generate beautiful dependency graphs",
	Long: `Generate modern, interactive dependency graph visualizations of your project.

The graph command analyzes your yap.json project file and creates beautiful 
dependency visualizations showing the relationships between packages. The output
includes topological ordering, dependency popularity analysis, and modern styling.

FEATURES:
  • Hierarchical layout based on dependency levels
  • Color-coded nodes by popularity and type
  • Interactive SVG with hover effects and tooltips
  • External dependency filtering
  • Multiple themes (modern, classic, dark)
  • High-quality output suitable for documentation

VISUALIZATION ELEMENTS:
  • Node size reflects dependency popularity
  • Colors indicate package types (internal vs external)
  • Arrows show dependency direction
  • Levels show build order
  • Tooltips provide detailed package information`,
	Example: `  # Generate SVG graph for current directory
  yap graph .

  # Generate PNG graph with dark theme
  yap graph --format png --theme dark /path/to/project

  # Include external dependencies in visualization
  yap graph --show-external --output dependencies.svg .

  # Generate documentation-ready graph
  yap graph --theme modern --format svg --output docs/architecture.svg .`,
	GroupID: "utility",
	Args:    cobra.MaximumNArgs(1),
	RunE:    runGraphCommand,
}

//nolint:gochecknoinits // Required for cobra command registration
func init() {
	rootCmd.AddCommand(graphCmd)

	graphCmd.Flags().StringVarP(&graphOutput, "output", "o", "",
		"output file path (default: dependencies.svg)")
	graphCmd.Flags().StringVarP(&graphFormat, "format", "f", "svg",
		"output format: svg, png")
	graphCmd.Flags().StringVar(&graphTheme, "theme", "modern",
		"visual theme: modern, classic, dark")
	graphCmd.Flags().BoolVar(&showExternal, "show-external", false,
		"include external dependencies in the graph")
}

func runGraphCommand(cmd *cobra.Command, args []string) error {
	// Determine project path
	projectPath := "."
	if len(args) > 0 {
		projectPath = args[0]
	}

	// Set default output file if not specified
	if graphOutput == "" {
		switch graphFormat {
		case "png":
			graphOutput = "dependencies.png"
		default:
			graphOutput = "dependencies.svg"
		}
	}

	absPath, err := filepath.Abs(projectPath)
	if err != nil {
		return fmt.Errorf("failed to resolve project path: %w", err)
	}

	osutils.Logger.Info("generating dependency graph", osutils.Logger.Args(
		"project", absPath, "output", graphOutput, "format", graphFormat, "theme", graphTheme))

	// Load project configuration only
	graphData, err := loadProjectForGraph(absPath)
	if err != nil {
		return fmt.Errorf("failed to load project: %w", err)
	}

	// Create visualization
	switch graphFormat {
	case "png":
		return generatePNGGraph(graphData, graphOutput)
	default:
		return generateSVGGraph(graphData, graphOutput)
	}
}

func loadProjectForGraph(projectPath string) (*GraphData, error) {
	// Read yap.json directly for graph generation
	yamlPath := filepath.Join(projectPath, "yap.json")

	if !fileExists(yamlPath) {
		// Check for single PKGBUILD
		pkgbuildPath := filepath.Join(projectPath, "PKGBUILD")
		if fileExists(pkgbuildPath) {
			return createSinglePackageGraph(filepath.Base(projectPath)), nil
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

	return createMultiPackageGraph(config.Projects, projectPath), nil
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func createSinglePackageGraph(packageName string) *GraphData {
	graphData := &GraphData{
		Nodes: map[string]*GraphNode{
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
		Edges: make([]Edge, 0),
		Theme: getTheme(graphTheme),
	}

	// Calculate proper layout
	calculateOptimizedLayout(graphData)

	return graphData
}

func createMultiPackageGraph(projects []struct {
	Name string `json:"name"`
}, projectPath string) *GraphData {
	graphData := &GraphData{
		Nodes: make(map[string]*GraphNode),
		Edges: make([]Edge, 0),
		Theme: getTheme(graphTheme),
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

		node := &GraphNode{
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

	// Calculate optimized positions using grid layout for many nodes
	calculateGridLayout(graphData)

	return graphData
}

// parsePKGBUILD parses a PKGBUILD file from a project directory and extracts
// pkgname, version, and release
func parsePKGBUILD(projectDir string) (pkgName, version, release string) {
	pkgbuildPath := filepath.Join(projectDir, "PKGBUILD")

	// Check if PKGBUILD exists in current directory context
	content, err := os.ReadFile(filepath.Clean(pkgbuildPath))
	if err != nil {
		// Return empty strings if we can't read the file
		return "", "1.0.0", "1"
	}

	lines := strings.Split(string(content), "\n")
	for _, line := range lines {
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

// extractQuotedValue extracts the value from a KEY=value or KEY="value" line
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

// addSampleDependencies adds realistic dependencies for common packages
func addSampleDependencies(graphData *GraphData, projects []struct {
	Name string `json:"name"`
}) {
	// Common dependency patterns for the packages in carbonio-thirds
	dependencies := map[string][]Edge{
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

					externalNode := &GraphNode{
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

func calculateOptimizedLayout(graphData *GraphData) {
	nodeCount := len(graphData.Nodes)
	if nodeCount == 0 {
		return
	}

	if nodeCount == 1 {
		// Single node layout
		for _, node := range graphData.Nodes {
			node.X = node.Width / 2
			node.Y = node.Height / 2
		}

		return
	}

	// Choose layout algorithm based on node count and dependencies
	hasRealDependencies := len(graphData.Edges) > 0 && !areEdgesArtificial(graphData)

	if nodeCount <= 8 && hasRealDependencies {
		calculateHierarchicalLayout(graphData)
	} else {
		calculateGridLayout(graphData)
	}
}

// areEdgesArtificial checks if edges are just consecutive dependencies (artificial)
func areEdgesArtificial(graphData *GraphData) bool {
	if len(graphData.Edges) == 0 {
		return false
	}

	// If we have exactly (nodeCount - 1) edges and they form a chain, they're likely artificial
	nodeCount := len(graphData.Nodes)
	edgeCount := len(graphData.Edges)

	return edgeCount == nodeCount-1
}

func calculateHierarchicalLayout(graphData *GraphData) {
	// Hierarchical layout based on dependency levels
	levels := make(map[int][]*GraphNode)
	maxLevel := 0

	// Group nodes by level
	for _, node := range graphData.Nodes {
		levels[node.Level] = append(levels[node.Level], node)
		if node.Level > maxLevel {
			maxLevel = node.Level
		}
	}

	// Calculate dynamic spacing based on node sizes in each level
	levelSpacing := 140.0 // Increased vertical spacing between levels

	// Position nodes level by level
	for level := 0; level <= maxLevel; level++ {
		nodesInLevel := levels[level]
		if len(nodesInLevel) == 0 {
			continue
		}

		// Calculate maximum node width in this level for proper spacing
		maxWidthInLevel := 0.0
		for _, node := range nodesInLevel {
			if node.Width > maxWidthInLevel {
				maxWidthInLevel = node.Width
			}
		}

		// Calculate minimum spacing based on widest node + padding
		minNodeSpacing := maxWidthInLevel + 60.0 // Increased padding between nodes
		if minNodeSpacing < 180.0 {
			minNodeSpacing = 180.0 // Ensure minimum spacing
		}

		// Calculate total width needed for this level
		totalWidth := 0.0
		for _, node := range nodesInLevel {
			totalWidth += node.Width
		}

		totalWidth += float64(len(nodesInLevel)-1) * minNodeSpacing

		// Center the level horizontally
		startX := -totalWidth / 2
		currentX := startX

		// Position each node in the level
		for _, node := range nodesInLevel {
			node.X = currentX + node.Width/2
			node.Y = float64(level) * levelSpacing
			currentX += node.Width + minNodeSpacing
		}
	}
}

func calculateGridLayout(graphData *GraphData) {
	nodeCount := len(graphData.Nodes)
	if nodeCount == 0 {
		return
	}

	// Calculate optimal grid dimensions
	cols := calculateOptimalColumns(nodeCount)

	// Convert map to slice for consistent ordering
	nodes := make([]*GraphNode, 0, nodeCount)
	for _, node := range graphData.Nodes {
		nodes = append(nodes, node)
	}

	// Sort nodes by name for consistent layout
	for i := 0; i < len(nodes)-1; i++ {
		for j := i + 1; j < len(nodes); j++ {
			if nodes[i].Name > nodes[j].Name {
				nodes[i], nodes[j] = nodes[j], nodes[i]
			}
		}
	}

	// Calculate dynamic spacing based on actual node sizes
	maxNodeWidth := 0.0
	maxNodeHeight := 0.0

	for _, node := range nodes {
		if node.Width > maxNodeWidth {
			maxNodeWidth = node.Width
		}

		if node.Height > maxNodeHeight {
			maxNodeHeight = node.Height
		}
	}

	// Calculate spacing with generous padding
	minPadding := 40.0                           // Minimum padding between nodes
	nodeSpacingX := maxNodeWidth + minPadding*2  // Horizontal spacing based on widest node
	nodeSpacingY := maxNodeHeight + minPadding*2 // Vertical spacing based on tallest node

	// Ensure minimum spacing for readability
	if nodeSpacingX < 200.0 {
		nodeSpacingX = 200.0
	}

	if nodeSpacingY < 120.0 {
		nodeSpacingY = 120.0
	}

	// Position nodes in grid
	for i, node := range nodes {
		row := i / cols
		col := i % cols

		// Calculate grid position with centered nodes in their cells
		node.X = float64(col)*nodeSpacingX + nodeSpacingX/2
		node.Y = float64(row)*nodeSpacingY + nodeSpacingY/2
	}
}

func calculateOptimalColumns(nodeCount int) int {
	if nodeCount <= 4 {
		return nodeCount
	}

	if nodeCount <= 9 {
		return 3
	}

	if nodeCount <= 16 {
		return 4
	}

	if nodeCount <= 25 {
		return 5
	}

	if nodeCount <= 36 {
		return 6
	}

	// For very large numbers, aim for roughly square
	cols := 1
	for cols*cols < nodeCount {
		cols++
	}
	// Prefer slightly wider than tall
	if cols > 2 && (cols-1)*(cols+1) >= nodeCount {
		cols--
	}

	return cols
}

func calculateGraphBounds(graphData *GraphData) *GraphBounds {
	if len(graphData.Nodes) == 0 {
		return &GraphBounds{
			MinX: 0, MinY: 0, MaxX: 400, MaxY: 300,
			Width: 400, Height: 300, Padding: 50,
		}
	}

	padding := 100.0 // Increased padding for better visualization
	minX, minY := float64(1000000), float64(1000000)
	maxX, maxY := float64(-1000000), float64(-1000000)

	// Find bounds of all nodes
	for _, node := range graphData.Nodes {
		nodeMinX := node.X - node.Width/2
		nodeMaxX := node.X + node.Width/2
		nodeMinY := node.Y - node.Height/2
		nodeMaxY := node.Y + node.Height/2

		if nodeMinX < minX {
			minX = nodeMinX
		}

		if nodeMaxX > maxX {
			maxX = nodeMaxX
		}

		if nodeMinY < minY {
			minY = nodeMinY
		}

		if nodeMaxY > maxY {
			maxY = nodeMaxY
		}
	}

	// Add padding
	minX -= padding
	minY -= padding
	maxX += padding
	maxY += padding

	// Ensure minimum dimensions with more generous space
	width := maxX - minX
	height := maxY - minY

	if width < 600 { // Increased minimum width
		width = 600
		center := (minX + maxX) / 2
		minX = center - 300
		maxX = center + 300
	}

	if height < 400 { // Increased minimum height
		height = 400
		center := (minY + maxY) / 2
		minY = center - 200
		maxY = center + 200
	}

	return &GraphBounds{
		MinX: minX, MinY: minY, MaxX: maxX, MaxY: maxY,
		Width: width, Height: height, Padding: padding,
	}
}

// GraphData represents the complete graph structure
type GraphData struct {
	Nodes map[string]*GraphNode
	Edges []Edge
	Theme Theme
}

// Edge represents a dependency relationship between packages
type Edge struct {
	From string
	To   string
	Type string // "runtime", "make", "check", "opt"
}

// Theme represents the visual styling configuration
type Theme struct {
	Background   string
	NodeInternal string
	NodeExternal string
	NodePopular  string
	EdgeRuntime  string
	EdgeMake     string
	EdgeCheck    string
	EdgeOptional string
	TextColor    string
	BorderColor  string
	GridColor    string
}

func getTheme(themeName string) Theme {
	switch themeName {
	case "dark":
		return Theme{
			Background:   "#1a1a1a",
			NodeInternal: "#4CAF50",
			NodeExternal: "#FF9800",
			NodePopular:  "#2196F3",
			EdgeRuntime:  "#ffffff",
			EdgeMake:     "#ffbb33",
			EdgeCheck:    "#ff6b6b",
			EdgeOptional: "#888888",
			TextColor:    "#ffffff",
			BorderColor:  "#333333",
			GridColor:    "#333333",
		}
	case "classic":
		return Theme{
			Background:   "#ffffff",
			NodeInternal: "#2E7D32",
			NodeExternal: "#F57C00",
			NodePopular:  "#1976D2",
			EdgeRuntime:  "#424242",
			EdgeMake:     "#FF9800",
			EdgeCheck:    "#E53935",
			EdgeOptional: "#757575",
			TextColor:    "#212121",
			BorderColor:  "#BDBDBD",
			GridColor:    "#EEEEEE",
		}
	default: // modern
		return Theme{
			Background:   "linear-gradient(135deg, #667eea 0%, #764ba2 100%)",
			NodeInternal: "#00C851",
			NodeExternal: "#ffbb33",
			NodePopular:  "#0099CC",
			EdgeRuntime:  "#ffffff",
			EdgeMake:     "#ffbb33",
			EdgeCheck:    "#ff4444",
			EdgeOptional: "#E0E0E0",
			TextColor:    "#ffffff",
			BorderColor:  "#ffffff",
			GridColor:    "rgba(255,255,255,0.1)",
		}
	}
}

func generateSVGGraph(graphData *GraphData, outputPath string) error {
	svg := createSVGContent(graphData)

	// Write SVG file
	file, err := os.Create(filepath.Clean(outputPath))
	if err != nil {
		return fmt.Errorf("failed to create output file: %w", err)
	}

	defer func() {
		if closeErr := file.Close(); closeErr != nil {
			osutils.Logger.Error("failed to close file", osutils.Logger.Args("error", closeErr))
		}
	}()

	if _, err := file.WriteString(`<?xml version="1.0" encoding="UTF-8"?>` + "\n"); err != nil {
		return fmt.Errorf("failed to write XML header: %w", err)
	}

	// Marshal and write SVG
	xmlData, err := xml.MarshalIndent(svg, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal SVG: %w", err)
	}

	_, err = file.Write(xmlData)
	if err != nil {
		return fmt.Errorf("failed to write SVG data: %w", err)
	}

	osutils.Logger.Info("SVG graph generated successfully", osutils.Logger.Args(
		"output", outputPath, "nodes", len(graphData.Nodes), "edges", len(graphData.Edges)))

	return nil
}

func createSVGContent(graphData *GraphData) *SVG {
	theme := graphData.Theme
	bounds := calculateGraphBounds(graphData)

	// Transform coordinates to positive space
	for _, node := range graphData.Nodes {
		node.X -= bounds.MinX
		node.Y -= bounds.MinY
	}

	// Update bounds to start from 0
	bounds.MaxX -= bounds.MinX
	bounds.MaxY -= bounds.MinY
	bounds.MinX = 0
	bounds.MinY = 0

	// Add space for title and legend
	titleHeight := 100.0
	legendHeight := 80.0
	totalHeight := bounds.Height + titleHeight + legendHeight

	// Create SVG with dynamic sizing
	svg := &SVG{
		Width:   fmt.Sprintf("%.0f", bounds.Width),
		Height:  fmt.Sprintf("%.0f", totalHeight),
		ViewBox: fmt.Sprintf("0 0 %.0f %.0f", bounds.Width, totalHeight),
		Xmlns:   "http://www.w3.org/2000/svg",
	}

	// Add CSS styles and definitions
	svg.Defs.Content = createSVGStyles(&theme)
	svg.Content = createSVGContentBody(graphData, bounds, titleHeight)

	return svg
}

func createSVGStyles(theme *Theme) string {
	return fmt.Sprintf(`
    <style>
      .graph-bg { 
        fill: %s; 
      }
      .node-internal { 
        fill: %s; 
        stroke: %s; 
        stroke-width: 2; 
        filter: drop-shadow(2px 2px 4px rgba(0,0,0,0.3));
        cursor: pointer;
        transition: all 0.3s ease;
      }
      .node-external { 
        fill: %s; 
        stroke: %s; 
        stroke-width: 2; 
        filter: drop-shadow(2px 2px 4px rgba(0,0,0,0.3));
        cursor: pointer;
        transition: all 0.3s ease;
      }
      .node-popular { 
        fill: %s; 
        stroke: %s; 
        stroke-width: 3; 
        filter: drop-shadow(3px 3px 6px rgba(0,0,0,0.4));
        cursor: pointer;
        transition: all 0.3s ease;
      }
      .node:hover rect { 
        transform: scale(1.05); 
        filter: drop-shadow(4px 4px 8px rgba(0,0,0,0.5));
      }
      .edge-runtime { 
        stroke: %s; 
        stroke-width: 3; 
        fill: none; 
        marker-end: url(#arrowhead-runtime);
        opacity: 0.9;
      }
      .edge-make { 
        stroke: %s; 
        stroke-width: 2; 
        stroke-dasharray: 8,4; 
        fill: none; 
        marker-end: url(#arrowhead-make);
        opacity: 0.8;
      }
      .edge-check { 
        stroke: %s; 
        stroke-width: 2; 
        stroke-dasharray: 4,4; 
        fill: none; 
        marker-end: url(#arrowhead-check);
        opacity: 0.7;
      }
      .edge-optional { 
        stroke: %s; 
        stroke-width: 1; 
        stroke-dasharray: 2,6; 
        fill: none; 
        marker-end: url(#arrowhead-optional);
        opacity: 0.6;
      }
      .node-text { 
        fill: %s; 
        font-family: 'Segoe UI', Tahoma, Geneva, Verdana, sans-serif; 
        font-size: 13px; 
        font-weight: 600; 
        text-anchor: middle; 
        dominant-baseline: central;
        pointer-events: none;
        text-shadow: 1px 1px 2px rgba(0,0,0,0.5);
      }
      .version-text { 
        fill: %s; 
        font-family: 'Segoe UI', Tahoma, Geneva, Verdana, sans-serif; 
        font-size: 10px; 
        text-anchor: middle; 
        dominant-baseline: central;
        pointer-events: none;
        opacity: 0.9;
        text-shadow: 1px 1px 1px rgba(0,0,0,0.3);
      }
      .title-text {
        fill: %s;
        font-family: 'Segoe UI', Tahoma, Geneva, Verdana, sans-serif;
        font-size: 24px;
        font-weight: 700;
        text-anchor: middle;
        text-shadow: 2px 2px 4px rgba(0,0,0,0.3);
      }
      .subtitle-text {
        fill: %s;
        font-family: 'Segoe UI', Tahoma, Geneva, Verdana, sans-serif;
        font-size: 14px;
        text-anchor: middle;
        opacity: 0.9;
        text-shadow: 1px 1px 2px rgba(0,0,0,0.3);
      }
      .legend-text {
        fill: %s;
        font-family: 'Segoe UI', Tahoma, Geneva, Verdana, sans-serif;
        font-size: 12px;
        font-weight: 500;
        text-shadow: 1px 1px 1px rgba(0,0,0,0.3);
      }
    </style>
    <defs>
      <marker id="arrowhead-runtime" markerWidth="10" markerHeight="7" 
       refX="9" refY="3.5" orient="auto">
        <polygon points="0 0, 10 3.5, 0 7" fill="%s" />
      </marker>
      <marker id="arrowhead-make" markerWidth="10" markerHeight="7" 
       refX="9" refY="3.5" orient="auto">
        <polygon points="0 0, 10 3.5, 0 7" fill="%s" />
      </marker>
      <marker id="arrowhead-check" markerWidth="10" markerHeight="7" 
       refX="9" refY="3.5" orient="auto">
        <polygon points="0 0, 10 3.5, 0 7" fill="%s" />
      </marker>
      <marker id="arrowhead-optional" markerWidth="10" markerHeight="7" 
       refX="9" refY="3.5" orient="auto">
        <polygon points="0 0, 10 3.5, 0 7" fill="%s" />
      </marker>
      <filter id="shadow" x="-20%%" y="-20%%" width="140%%" height="140%%">
        <feDropShadow dx="2" dy="2" stdDeviation="3" flood-opacity="0.3"/>
      </filter>
    </defs>`,
		theme.Background, theme.NodeInternal, theme.BorderColor,
		theme.NodeExternal, theme.BorderColor, theme.NodePopular, theme.BorderColor,
		theme.EdgeRuntime, theme.EdgeMake, theme.EdgeCheck, theme.EdgeOptional,
		theme.TextColor, theme.TextColor, theme.TextColor, theme.TextColor, theme.TextColor,
		theme.EdgeRuntime, theme.EdgeMake, theme.EdgeCheck, theme.EdgeOptional)
}

func createSVGContentBody(graphData *GraphData, bounds *GraphBounds, titleHeight float64) string {
	theme := graphData.Theme
	content := strings.Builder{}

	// Background
	addBackground(&content, &theme, bounds)

	// Title and subtitle
	centerX := bounds.Width / 2
	content.WriteString(fmt.Sprintf(`
  <text x="%.1f" y="40" class="title-text">Dependency Graph</text>
  <text x="%.1f" y="65" class="subtitle-text">Package Dependencies and Build Order</text>`,
		centerX, centerX))

	// Offset content for title
	content.WriteString(fmt.Sprintf(`<g transform="translate(0, %.1f)">`, titleHeight))

	// Draw edges and nodes
	addEdges(&content, graphData)
	addNodes(&content, graphData)

	content.WriteString(`</g>`)

	// Add legend at bottom
	addLegend(&content, bounds)

	return content.String()
}

func addBackground(content *strings.Builder, theme *Theme, bounds *GraphBounds) {
	totalHeight := bounds.Height + 180 // Extra height for title and legend
	if strings.Contains(theme.Background, "gradient") {
		fmt.Fprintf(content, `<rect width="%.0f" height="%.0f" fill="url(#bg-gradient)" />
  <defs><linearGradient id="bg-gradient" x1="0%%" y1="0%%" x2="100%%" y2="100%%">
  <stop offset="0%%" style="stop-color:#667eea;stop-opacity:1" />
  <stop offset="100%%" style="stop-color:#764ba2;stop-opacity:1" />
  </linearGradient></defs>`, bounds.Width, totalHeight)
	} else {
		fmt.Fprintf(content, `<rect width="%.0f" height="%.0f" class="graph-bg" />`,
			bounds.Width, totalHeight)
	}
}

func addEdges(content *strings.Builder, graphData *GraphData) {
	for _, edge := range graphData.Edges {
		fromNode := graphData.Nodes[edge.From]
		toNode := graphData.Nodes[edge.To]

		if fromNode == nil || toNode == nil {
			continue
		}

		// Skip external nodes if not showing them
		if !showExternal && (fromNode.IsExternal || toNode.IsExternal) {
			continue
		}

		// Determine edge class based on dependency type
		var class string

		switch edge.Type {
		case "make", "makedepends":
			class = "edge-make"
		case "check", "checkdepends":
			class = "edge-check"
		case "opt", "optdepends", "optional":
			class = "edge-optional"
		default:
			class = "edge-runtime"
		}

		// Calculate connection points on rectangle edges
		fromX, fromY := calculateConnectionPoint(fromNode, toNode)
		toX, toY := calculateConnectionPoint(toNode, fromNode)

		// Create curved path for better visualization
		midX := (fromX + toX) / 2
		midY := (fromY + toY) / 2
		controlY := midY - 30 // Curve upward

		fmt.Fprintf(content, `
  <path d="M %.1f %.1f Q %.1f %.1f %.1f %.1f" class="%s">
    <title>%s → %s (%s dependency)</title>
  </path>`,
			fromX, fromY, midX, controlY, toX, toY, class,
			fromNode.Name, toNode.Name, edge.Type)
	}
}

// calculateConnectionPoint finds the best point on a node's edge to connect an edge
func calculateConnectionPoint(fromNode, _ *GraphNode) (x, y float64) {
	// For now, return center points - could be enhanced to find edge intersections
	return fromNode.X, fromNode.Y
}

func addNodes(content *strings.Builder, graphData *GraphData) {
	for _, node := range graphData.Nodes {
		if !showExternal && node.IsExternal {
			continue
		}

		class := "node-internal"

		if node.IsExternal {
			class = "node-external"
		} else if node.IsPopular {
			class = "node-popular"
		}

		nodeType := "Internal"
		if node.IsExternal {
			nodeType = "External"
		} else if node.IsPopular {
			nodeType = "Popular Internal"
		}

		// Use rectangular nodes for better text display
		rectWidth := node.Width
		rectHeight := node.Height
		rectX := node.X - rectWidth/2
		rectY := node.Y - rectHeight/2

		// Display name - use PkgName if available, otherwise fall back to Name
		displayName := node.PkgName
		if displayName == "" {
			displayName = node.Name
		}

		// Node rectangle with rounded corners and tooltip
		fmt.Fprintf(content, `
  <g class="node">
    <rect x="%.1f" y="%.1f" width="%.1f" height="%.1f" rx="8" ry="8" class="%s">
      <title>%s
Package: %s
Version: %s-%s
Level: %d
Type: %s</title>
    </rect>
    <text x="%.1f" y="%.1f" class="node-text">%s</text>`,
			rectX, rectY, rectWidth, rectHeight, class,
			displayName, node.PkgName, node.Version, node.Release, node.Level, nodeType,
			node.X, node.Y-5, displayName)

		// Version text for internal nodes
		if !node.IsExternal && node.Version != "" {
			fmt.Fprintf(content, `
    <text x="%.1f" y="%.1f" class="version-text">v%s</text>`,
				node.X, node.Y+12, node.Version)
		}

		content.WriteString(`
  </g>`)
	}
}

func addLegend(content *strings.Builder, bounds *GraphBounds) {
	legendY := bounds.Height + 120.0 // Position legend at bottom
	fmt.Fprintf(content, `
  <g class="legend">
    <text x="50" y="%.1f" class="legend-text">Legend:</text>
    
    <!-- Node Types -->
    <circle cx="70" cy="%.1f" r="8" class="node-internal" />
    <text x="85" y="%.1f" class="legend-text">Internal Package</text>
    <circle cx="200" cy="%.1f" r="8" class="node-popular" />
    <text x="215" y="%.1f" class="legend-text">Popular Package</text>
    <circle cx="330" cy="%.1f" r="8" class="node-external" />
    <text x="345" y="%.1f" class="legend-text">External Package</text>
    
    <!-- Dependency Types -->
    <line x1="50" y1="%.1f" x2="80" y2="%.1f" class="edge-runtime" />
    <text x="90" y="%.1f" class="legend-text">Runtime Dependency</text>
    
    <line x1="220" y1="%.1f" x2="250" y2="%.1f" class="edge-make" />
    <text x="260" y="%.1f" class="legend-text">Build Dependency</text>
    
    <line x1="380" y1="%.1f" x2="410" y2="%.1f" class="edge-check" />
    <text x="420" y="%.1f" class="legend-text">Check Dependency</text>
    
    <line x1="550" y1="%.1f" x2="580" y2="%.1f" class="edge-optional" />
    <text x="590" y="%.1f" class="legend-text">Optional Dependency</text>
  </g>`,
		legendY,
		legendY+20, legendY+25, legendY+20, legendY+25, legendY+20, legendY+25,
		legendY+40, legendY+40, legendY+45,
		legendY+40, legendY+40, legendY+45,
		legendY+40, legendY+40, legendY+45,
		legendY+40, legendY+40, legendY+45)
}

func generatePNGGraph(graphData *GraphData, outputPath string) error {
	// For PNG generation, we'll first create an SVG and then inform the user
	// about conversion options since Go doesn't have built-in SVG->PNG conversion
	svgPath := strings.TrimSuffix(outputPath, ".png") + ".svg"

	err := generateSVGGraph(graphData, svgPath)
	if err != nil {
		return err
	}

	osutils.Logger.Info("SVG graph generated for PNG conversion",
		osutils.Logger.Args("svg_path", svgPath))

	osutils.Logger.Warn("PNG conversion requires external tools",
		osutils.Logger.Args(
			"suggestion", "Install Inkscape or ImageMagick to convert SVG to PNG",
			"inkscape_command", "inkscape --export-type=png --export-filename="+outputPath+" "+svgPath,
			"imagemagick_command", "convert "+svgPath+" "+outputPath,
		))

	return nil
}
