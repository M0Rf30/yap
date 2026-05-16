// Package layout provides graph layout algorithms for positioning nodes.
package layout

import (
	"sort"

	"github.com/M0Rf30/yap/v2/pkg/graph"
)

// CalculateOptimizedLayout chooses and applies the best layout algorithm for the graph.
func CalculateOptimizedLayout(graphData *graph.Data) {
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

	// Use hierarchical layout whenever there are real dependencies
	if len(graphData.Edges) > 0 {
		CalculateHierarchicalLayout(graphData)
	} else {
		CalculateGridLayout(graphData)
	}
}

const (
	// maxLevelWidth caps the horizontal extent of a single level row in pixels.
	// Nodes that don't fit are wrapped into additional sub-rows.
	maxLevelWidth = 2400.0
)

// CalculateHierarchicalLayout positions nodes in levels based on dependencies.
// Levels with many nodes are wrapped into sub-rows to keep the canvas width
// bounded to maxLevelWidth.
func CalculateHierarchicalLayout(graphData *graph.Data) {
	// Group nodes by level
	levels := make(map[int][]*graph.Node)
	maxLevel := 0

	for _, node := range graphData.Nodes {
		levels[node.Level] = append(levels[node.Level], node)
		if node.Level > maxLevel {
			maxLevel = node.Level
		}
	}

	// Sort nodes within each level by name for consistency
	for level := range levels {
		nodes := levels[level]
		sort.Slice(nodes, func(i, j int) bool {
			return nodes[i].Name < nodes[j].Name
		})
	}

	// Calculate level spacing based on max node height + padding
	maxNodeHeight := 60.0
	for _, node := range graphData.Nodes {
		if node.Height > maxNodeHeight {
			maxNodeHeight = node.Height
		}
	}

	rowSpacing := maxNodeHeight + 60.0 // vertical gap between rows

	// currentY tracks the running Y offset as we place levels (and sub-rows)
	currentY := 0.0

	for level := 0; level <= maxLevel; level++ {
		nodesInLevel := levels[level]
		if len(nodesInLevel) == 0 {
			continue
		}

		// Calculate horizontal spacing for this level
		maxW := 0.0
		for _, n := range nodesInLevel {
			if n.Width > maxW {
				maxW = n.Width
			}
		}

		hSpacing := maxW + 50.0
		if hSpacing < 160.0 {
			hSpacing = 160.0
		}

		// How many nodes fit in one row within maxLevelWidth?
		nodesPerRow := max(int(maxLevelWidth/hSpacing), 1)

		// Place nodes in sub-rows
		for i, node := range nodesInLevel {
			subRow := i / nodesPerRow
			col := i % nodesPerRow

			// Count nodes in this sub-row for centering
			rowStart := subRow * nodesPerRow

			rowEnd := min(rowStart+nodesPerRow, len(nodesInLevel))

			rowCount := rowEnd - rowStart

			totalRowW := float64(rowCount) * hSpacing
			startX := -totalRowW / 2

			node.X = startX + float64(col)*hSpacing + hSpacing/2
			node.Y = currentY + float64(subRow)*rowSpacing
		}

		// Advance currentY past all sub-rows of this level
		subRows := (len(nodesInLevel) + nodesPerRow - 1) / nodesPerRow
		currentY += float64(subRows)*rowSpacing + 20.0 // extra gap between levels
	}
}

// CalculateGridLayout positions nodes in a grid formation.
func CalculateGridLayout(graphData *graph.Data) {
	nodeCount := len(graphData.Nodes)
	if nodeCount == 0 {
		return
	}

	// Calculate optimal grid dimensions
	cols := calculateOptimalColumns(nodeCount)

	// Convert map to slice for consistent ordering
	nodes := make([]*graph.Node, 0, nodeCount)
	for _, node := range graphData.Nodes {
		nodes = append(nodes, node)
	}

	// Sort nodes by level then name for consistent layout
	sort.Slice(nodes, func(i, j int) bool {
		if nodes[i].Level != nodes[j].Level {
			return nodes[i].Level < nodes[j].Level
		}

		return nodes[i].Name < nodes[j].Name
	})

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
	minPadding := 30.0                           // Minimum padding between nodes
	nodeSpacingX := maxNodeWidth + minPadding*2  // Horizontal spacing based on widest node
	nodeSpacingY := maxNodeHeight + minPadding*2 // Vertical spacing based on tallest node

	// Ensure minimum spacing for readability
	if nodeSpacingX < 160.0 {
		nodeSpacingX = 160.0
	}

	if nodeSpacingY < 100.0 {
		nodeSpacingY = 100.0
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

// calculateOptimalColumns determines the optimal number of columns for grid layout.
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

// CalculateGraphBounds calculates the bounding box for the entire graph.
func CalculateGraphBounds(graphData *graph.Data) *graph.Bounds {
	if len(graphData.Nodes) == 0 {
		return &graph.Bounds{
			MinX: 0, MinY: 0, MaxX: 400, MaxY: 300,
			Width: 400, Height: 300, Padding: 50,
		}
	}

	padding := 60.0 // Optimized padding for better visualization
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

	if width < 500 { // Optimized minimum width
		width = 500
		center := (minX + maxX) / 2
		minX = center - 250
		maxX = center + 250
	}

	if height < 350 { // Optimized minimum height
		height = 350
		center := (minY + maxY) / 2
		minY = center - 175
		maxY = center + 175
	}

	return &graph.Bounds{
		MinX: minX, MinY: minY, MaxX: maxX, MaxY: maxY,
		Width: width, Height: height, Padding: padding,
	}
}
