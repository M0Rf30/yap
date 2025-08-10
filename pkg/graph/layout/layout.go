// Package layout provides graph layout algorithms for positioning nodes.
package layout

import "github.com/M0Rf30/yap/v2/pkg/graph"

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

	// Choose layout algorithm based on node count and dependencies
	hasRealDependencies := len(graphData.Edges) > 0 && !areEdgesArtificial(graphData)

	if nodeCount <= 8 && hasRealDependencies {
		CalculateHierarchicalLayout(graphData)
	} else {
		CalculateGridLayout(graphData)
	}
}

// CalculateHierarchicalLayout positions nodes in levels based on dependencies.
func CalculateHierarchicalLayout(graphData *graph.Data) {
	// Hierarchical layout based on dependency levels
	levels := make(map[int][]*graph.Node)
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

// areEdgesArtificial checks if edges are just consecutive dependencies (artificial).
func areEdgesArtificial(graphData *graph.Data) bool {
	if len(graphData.Edges) == 0 {
		return false
	}

	// If we have exactly (nodeCount - 1) edges and they form a chain, they're likely artificial
	nodeCount := len(graphData.Nodes)
	edgeCount := len(graphData.Edges)

	return edgeCount == nodeCount-1
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

	return &graph.Bounds{
		MinX: minX, MinY: minY, MaxX: maxX, MaxY: maxY,
		Width: width, Height: height, Padding: padding,
	}
}
