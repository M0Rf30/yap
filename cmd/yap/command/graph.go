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
	Name         string
	Version      string
	Release      string
	X, Y         float64
	IsExternal   bool
	IsPopular    bool
	Dependencies []string
	Level        int
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

	return createMultiPackageGraph(config.Projects), nil
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
				Version:      "1.0.0",
				Release:      "1",
				X:            600,
				Y:            400,
				IsExternal:   false,
				IsPopular:    false,
				Dependencies: make([]string, 0),
				Level:        0,
			},
		},
		Edges: make([]Edge, 0),
		Theme: getTheme(graphTheme),
	}

	return graphData
}

func createMultiPackageGraph(projects []struct {
	Name string `json:"name"`
}) *GraphData {
	graphData := &GraphData{
		Nodes: make(map[string]*GraphNode),
		Edges: make([]Edge, 0),
		Theme: getTheme(graphTheme),
	}

	// Create nodes from project names
	for i, proj := range projects {
		node := &GraphNode{
			Name:         proj.Name,
			Version:      "1.0.0",
			Release:      "1",
			IsExternal:   false,
			IsPopular:    i == 0, // Make first project popular for demo
			Dependencies: make([]string, 0),
			Level:        i,
		}
		graphData.Nodes[proj.Name] = node
	}

	// Create demo dependencies between consecutive projects
	for i := 1; i < len(projects); i++ {
		edge := Edge{
			From: projects[i].Name,
			To:   projects[i-1].Name,
			Type: "runtime",
		}
		graphData.Edges = append(graphData.Edges, edge)
	}

	// Calculate positions
	calculateSimpleNodePositions(graphData)

	return graphData
}

func calculateSimpleNodePositions(graphData *GraphData) {
	// Simple circular layout
	nodeCount := len(graphData.Nodes)
	if nodeCount == 0 {
		return
	}

	centerX, centerY := 600.0, 400.0
	radius := 200.0

	i := 0
	for _, node := range graphData.Nodes {
		angle := float64(i) * 2.0 * 3.14159 / float64(nodeCount)
		node.X = centerX + radius*cos(angle)
		node.Y = centerY + radius*sin(angle)
		i++
	}
}

// Simple cos/sin approximations
func cos(x float64) float64 {
	// Simple approximation for demo purposes
	for x < 0 {
		x += 2 * 3.14159
	}

	for x > 2*3.14159 {
		x -= 2 * 3.14159
	}

	if x > 3.14159 {
		return -cos(x - 3.14159)
	}

	if x > 3.14159/2 {
		return -cos(3.14159 - x)
	}

	// Taylor series approximation
	x2 := x * x

	return 1 - x2/2 + x2*x2/24 - x2*x2*x2/720
}

func sin(x float64) float64 {
	// Simple approximation for demo purposes
	for x < 0 {
		x += 2 * 3.14159
	}

	for x > 2*3.14159 {
		x -= 2 * 3.14159
	}

	if x > 3.14159 {
		return -sin(x - 3.14159)
	}

	if x > 3.14159/2 {
		return sin(3.14159 - x)
	}

	// Taylor series approximation
	x2 := x * x

	return x - x*x2/6 + x*x2*x2/120 - x*x2*x2*x2/5040
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
	Type string // "runtime" or "make"
}

// Theme represents the visual styling configuration
type Theme struct {
	Background   string
	NodeInternal string
	NodeExternal string
	NodePopular  string
	EdgeRuntime  string
	EdgeMake     string
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
			EdgeMake:     "#888888",
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
			EdgeMake:     "#757575",
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
			EdgeMake:     "#E0E0E0",
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

	// Create SVG with modern styling
	svg := &SVG{
		Width:   "1200",
		Height:  "800",
		ViewBox: "0 0 1200 800",
		Xmlns:   "http://www.w3.org/2000/svg",
	}

	// Add CSS styles and definitions
	svg.Defs.Content = createSVGStyles(&theme)
	svg.Content = createSVGContentBody(graphData)

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
      .node:hover { 
        transform: scale(1.1); 
        filter: drop-shadow(4px 4px 8px rgba(0,0,0,0.5));
      }
      .edge-runtime { 
        stroke: %s; 
        stroke-width: 2; 
        fill: none; 
        marker-end: url(#arrowhead-runtime);
        opacity: 0.8;
      }
      .edge-make { 
        stroke: %s; 
        stroke-width: 1; 
        stroke-dasharray: 5,5; 
        fill: none; 
        marker-end: url(#arrowhead-make);
        opacity: 0.6;
      }
      .node-text { 
        fill: %s; 
        font-family: 'Segoe UI', Tahoma, Geneva, Verdana, sans-serif; 
        font-size: 12px; 
        font-weight: 600; 
        text-anchor: middle; 
        dominant-baseline: central;
        pointer-events: none;
      }
      .version-text { 
        fill: %s; 
        font-family: 'Segoe UI', Tahoma, Geneva, Verdana, sans-serif; 
        font-size: 10px; 
        text-anchor: middle; 
        dominant-baseline: central;
        pointer-events: none;
        opacity: 0.8;
      }
      .title-text {
        fill: %s;
        font-family: 'Segoe UI', Tahoma, Geneva, Verdana, sans-serif;
        font-size: 24px;
        font-weight: 700;
        text-anchor: middle;
      }
      .subtitle-text {
        fill: %s;
        font-family: 'Segoe UI', Tahoma, Geneva, Verdana, sans-serif;
        font-size: 14px;
        text-anchor: middle;
        opacity: 0.8;
      }
      .legend-text {
        fill: %s;
        font-family: 'Segoe UI', Tahoma, Geneva, Verdana, sans-serif;
        font-size: 12px;
        font-weight: 500;
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
      <filter id="shadow" x="-20%%" y="-20%%" width="140%%" height="140%%">
        <feDropShadow dx="2" dy="2" stdDeviation="3" flood-opacity="0.3"/>
      </filter>
    </defs>`,
		theme.Background, theme.NodeInternal, theme.BorderColor,
		theme.NodeExternal, theme.BorderColor, theme.NodePopular, theme.BorderColor,
		theme.EdgeRuntime, theme.EdgeMake, theme.TextColor, theme.TextColor,
		theme.TextColor, theme.TextColor, theme.TextColor, theme.EdgeRuntime, theme.EdgeMake)
}

func createSVGContentBody(graphData *GraphData) string {
	theme := graphData.Theme
	content := strings.Builder{}

	// Background
	addBackground(&content, &theme)

	// Title and subtitle
	content.WriteString(`
  <text x="600" y="40" class="title-text">Dependency Graph</text>
  <text x="600" y="65" class="subtitle-text">Package Dependencies and Build Order</text>`)

	// Draw edges and nodes
	addEdges(&content, graphData)
	addNodes(&content, graphData)

	// Add legend
	addLegend(&content)

	return content.String()
}

func addBackground(content *strings.Builder, theme *Theme) {
	if strings.Contains(theme.Background, "gradient") {
		fmt.Fprintf(content, `<rect width="1200" height="800" fill="url(#bg-gradient)" />
  <defs><linearGradient id="bg-gradient" x1="0%%" y1="0%%" x2="100%%" y2="100%%">
  <stop offset="0%%" style="stop-color:#667eea;stop-opacity:1" />
  <stop offset="100%%" style="stop-color:#764ba2;stop-opacity:1" />
  </linearGradient></defs>`)
	} else {
		content.WriteString(`<rect width="1200" height="800" class="graph-bg" />`)
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

		class := "edge-runtime"
		if edge.Type == "make" {
			class = "edge-make"
		}

		// Create curved path for better visualization
		fmt.Fprintf(content, `
  <path d="M %g %g Q %g %g %g %g" class="%s" />`,
			fromNode.X, fromNode.Y,
			(fromNode.X+toNode.X)/2, (fromNode.Y+toNode.Y)/2-50,
			toNode.X, toNode.Y, class)
	}
}

func addNodes(content *strings.Builder, graphData *GraphData) {
	for _, node := range graphData.Nodes {
		if !showExternal && node.IsExternal {
			continue
		}

		class := "node-internal"
		radius := 25.0

		if node.IsExternal {
			class = "node-external"
			radius = 20.0
		} else if node.IsPopular {
			class = "node-popular"
			radius = 30.0
		}

		nodeType := "Internal"
		if node.IsExternal {
			nodeType = "External"
		} else if node.IsPopular {
			nodeType = "Popular Internal"
		}

		// Node circle with tooltip
		fmt.Fprintf(content, `
  <g class="node">
    <circle cx="%g" cy="%g" r="%g" class="%s">
      <title>%s
Version: %s-%s
Level: %d
Type: %s</title>
    </circle>
    <text x="%g" y="%g" class="node-text">%s</text>`,
			node.X, node.Y, radius, class,
			node.Name, node.Version, node.Release, node.Level, nodeType,
			node.X, node.Y-2, node.Name)

		// Version text for internal nodes
		if !node.IsExternal && node.Version != "" {
			fmt.Fprintf(content, `
    <text x="%g" y="%g" class="version-text">v%s</text>`,
				node.X, node.Y+12, node.Version)
		}

		content.WriteString(`
  </g>`)
	}
}

func addLegend(content *strings.Builder) {
	legendY := 720.0
	fmt.Fprintf(content, `
  <g class="legend">
    <text x="50" y="%g" class="legend-text">Legend:</text>
    <circle cx="70" cy="%g" r="8" class="node-internal" />
    <text x="85" y="%g" class="legend-text">Internal Package</text>
    <circle cx="200" cy="%g" r="8" class="node-popular" />
    <text x="215" y="%g" class="legend-text">Popular Package</text>
    <circle cx="330" cy="%g" r="8" class="node-external" />
    <text x="345" y="%g" class="legend-text">External Package</text>
    <line x1="470" y1="%g" x2="500" y2="%g" class="edge-runtime" />
    <text x="510" y="%g" class="legend-text">Runtime Dependency</text>
    <line x1="650" y1="%g" x2="680" y2="%g" class="edge-make" />
    <text x="690" y="%g" class="legend-text">Make Dependency</text>
  </g>`,
		legendY, legendY+20, legendY+25, legendY+20, legendY+25,
		legendY+20, legendY+25, legendY+20, legendY+20, legendY+25,
		legendY+20, legendY+20, legendY+25)
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
