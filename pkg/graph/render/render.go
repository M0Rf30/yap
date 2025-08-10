// Package render provides rendering functionality for dependency graphs.
package render

import (
	"encoding/xml"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/M0Rf30/yap/v2/pkg/graph"
	"github.com/M0Rf30/yap/v2/pkg/graph/layout"
	"github.com/M0Rf30/yap/v2/pkg/osutils"
)

// SVG represents the SVG document structure.
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

// Defs represents the SVG defs element.
type Defs struct {
	XMLName xml.Name `xml:"defs"`
	Content string   `xml:",innerxml"`
}

// GenerateSVGGraph generates an SVG graph visualization.
func GenerateSVGGraph(graphData *graph.Data, outputPath string, showExternal bool) error {
	svg := createSVGContent(graphData, showExternal)

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

// GeneratePNGGraph generates a PNG graph by first creating SVG and providing
// conversion instructions.
func GeneratePNGGraph(graphData *graph.Data, outputPath string, showExternal bool) error {
	// For PNG generation, we'll first create an SVG and then inform the user
	// about conversion options since Go doesn't have built-in SVG->PNG conversion
	svgPath := strings.TrimSuffix(outputPath, ".png") + ".svg"

	err := GenerateSVGGraph(graphData, svgPath, showExternal)
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

// createSVGContent creates the complete SVG content for the graph.
func createSVGContent(graphData *graph.Data, showExternal bool) *SVG {
	theme := graphData.Theme
	bounds := layout.CalculateGraphBounds(graphData)

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
	svg.Content = createSVGContentBody(graphData, bounds, titleHeight, showExternal)

	return svg
}

// createSVGStyles creates the CSS styles for the SVG.
func createSVGStyles(theme *graph.Theme) string {
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

// createSVGContentBody creates the main SVG content body.
func createSVGContentBody(graphData *graph.Data, bounds *graph.Bounds,
	titleHeight float64, showExternal bool) string {
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
	addEdges(&content, graphData, showExternal)
	addNodes(&content, graphData, showExternal)

	content.WriteString(`</g>`)

	// Add legend at bottom
	addLegend(&content, bounds)

	return content.String()
}

// addBackground adds the background rectangle to the SVG.
func addBackground(content *strings.Builder, theme *graph.Theme, bounds *graph.Bounds) {
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

// addEdges adds all edges to the SVG content.
func addEdges(content *strings.Builder, graphData *graph.Data, showExternal bool) {
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

// calculateConnectionPoint finds the best point on a node's edge to connect an edge.
func calculateConnectionPoint(fromNode, _ *graph.Node) (x, y float64) {
	// For now, return center points - could be enhanced to find edge intersections
	return fromNode.X, fromNode.Y
}

// addNodes adds all nodes to the SVG content.
func addNodes(content *strings.Builder, graphData *graph.Data, showExternal bool) {
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

// addLegend adds the legend to the SVG content.
func addLegend(content *strings.Builder, bounds *graph.Bounds) {
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
