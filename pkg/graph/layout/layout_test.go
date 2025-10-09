package layout

import (
	"testing"

	"github.com/M0Rf30/yap/v2/pkg/graph"
)

func TestCalculateOptimizedLayout(t *testing.T) {
	tests := []struct {
		name      string
		graphData *graph.Data
	}{
		{
			name: "Empty graph",
			graphData: &graph.Data{
				Nodes: make(map[string]*graph.Node),
				Edges: []graph.Edge{},
			},
		},
		{
			name: "Single node",
			graphData: &graph.Data{
				Nodes: map[string]*graph.Node{
					"node1": {Name: "node1", Width: 100, Height: 50},
				},
				Edges: []graph.Edge{},
			},
		},
		{
			name: "Multiple nodes with dependencies",
			graphData: &graph.Data{
				Nodes: map[string]*graph.Node{
					"node1": {Name: "node1", Width: 100, Height: 50, Level: 0},
					"node2": {Name: "node2", Width: 100, Height: 50, Level: 1},
					"node3": {Name: "node3", Width: 100, Height: 50, Level: 1},
				},
				Edges: []graph.Edge{
					{From: "node1", To: "node2"},
					{From: "node1", To: "node3"},
				},
			},
		},
		{
			name: "Large graph for grid layout",
			graphData: &graph.Data{
				Nodes: map[string]*graph.Node{
					"node1": {Name: "node1", Width: 100, Height: 50, Level: 0},
					"node2": {Name: "node2", Width: 100, Height: 50, Level: 0},
					"node3": {Name: "node3", Width: 100, Height: 50, Level: 0},
					"node4": {Name: "node4", Width: 100, Height: 50, Level: 0},
					"node5": {Name: "node5", Width: 100, Height: 50, Level: 0},
					"node6": {Name: "node6", Width: 100, Height: 50, Level: 0},
					"node7": {Name: "node7", Width: 100, Height: 50, Level: 0},
					"node8": {Name: "node8", Width: 100, Height: 50, Level: 0},
					"node9": {Name: "node9", Width: 100, Height: 50, Level: 0},
				},
				Edges: []graph.Edge{},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			CalculateOptimizedLayout(tt.graphData)

			// Verify that single node is positioned correctly
			if tt.name == "Single node" {
				for _, node := range tt.graphData.Nodes {
					if node.X != node.Width/2 || node.Y != node.Height/2 {
						t.Errorf("Single node not positioned correctly: X=%f, Y=%f", node.X, node.Y)
					}
				}
			}

			// For non-empty graphs, verify all nodes have positions set
			if len(tt.graphData.Nodes) > 0 {
				for _, node := range tt.graphData.Nodes {
					if node.X == 0 && node.Y == 0 && len(tt.graphData.Nodes) > 1 {
						t.Errorf("Node %s not positioned", node.Name)
					}
				}
			}
		})
	}
}

func TestCalculateHierarchicalLayout(t *testing.T) {
	graphData := &graph.Data{
		Nodes: map[string]*graph.Node{
			"level0a": {Name: "level0a", Width: 100, Height: 50, Level: 0},
			"level0b": {Name: "level0b", Width: 120, Height: 50, Level: 0},
			"level1a": {Name: "level1a", Width: 100, Height: 50, Level: 1},
			"level2a": {Name: "level2a", Width: 100, Height: 50, Level: 2},
		},
		Edges: []graph.Edge{},
	}

	CalculateHierarchicalLayout(graphData)

	// Verify nodes are positioned in different Y levels
	level0Y := graphData.Nodes["level0a"].Y
	level1Y := graphData.Nodes["level1a"].Y
	level2Y := graphData.Nodes["level2a"].Y

	if level0Y >= level1Y || level1Y >= level2Y {
		t.Error("Nodes not positioned in hierarchical levels")
	}

	// Verify nodes at same level have same Y coordinate
	if graphData.Nodes["level0a"].Y != graphData.Nodes["level0b"].Y {
		t.Error("Nodes at same level should have same Y coordinate")
	}
}

func TestCalculateGridLayout(t *testing.T) {
	tests := []struct {
		name      string
		nodeCount int
	}{
		{"Empty", 0},
		{"Single node", 1},
		{"Four nodes", 4},
		{"Nine nodes", 9},
		{"Large grid", 16},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			graphData := &graph.Data{
				Nodes: make(map[string]*graph.Node),
				Edges: []graph.Edge{},
			}

			// Create nodes
			for i := 0; i < tt.nodeCount; i++ {
				nodeName := string(rune('a' + i))
				graphData.Nodes[nodeName] = &graph.Node{
					Name:   nodeName,
					Width:  100,
					Height: 50,
				}
			}

			CalculateGridLayout(graphData)

			if tt.nodeCount == 0 {
				return
			}

			// Verify all nodes have positions
			for _, node := range graphData.Nodes {
				if node.X <= 0 || node.Y <= 0 {
					t.Errorf("Node %s not positioned correctly: X=%f, Y=%f", node.Name, node.X, node.Y)
				}
			}
		})
	}
}

func TestAreEdgesArtificial(t *testing.T) {
	tests := []struct {
		name      string
		nodeCount int
		edgeCount int
		expected  bool
	}{
		{"No edges", 3, 0, false},
		{"Artificial chain", 3, 2, true},
		{"Real dependencies", 3, 3, false},
		{"Single node", 1, 0, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			graphData := &graph.Data{
				Nodes: make(map[string]*graph.Node),
				Edges: []graph.Edge{},
			}

			// Create nodes
			for i := 0; i < tt.nodeCount; i++ {
				nodeName := string(rune('a' + i))
				graphData.Nodes[nodeName] = &graph.Node{Name: nodeName}
			}

			// Create edges
			for i := 0; i < tt.edgeCount; i++ {
				graphData.Edges = append(graphData.Edges, graph.Edge{From: "from", To: "to"})
			}

			result := areEdgesArtificial(graphData)
			if result != tt.expected {
				t.Errorf("areEdgesArtificial() = %v, expected %v", result, tt.expected)
			}
		})
	}
}

func TestCalculateOptimalColumns(t *testing.T) {
	tests := []struct {
		nodeCount int
		expected  int
	}{
		{1, 1},
		{2, 2},
		{4, 4},
		{5, 3},
		{9, 3},
		{10, 4},
		{16, 4},
		{17, 5},
		{25, 5},
		{26, 6},
		{36, 6},
		{37, 6},
		{100, 10},
	}

	for _, tt := range tests {
		t.Run("", func(t *testing.T) {
			result := calculateOptimalColumns(tt.nodeCount)
			if result != tt.expected {
				t.Errorf("calculateOptimalColumns(%d) = %d, expected %d", tt.nodeCount, result, tt.expected)
			}
		})
	}
}

func TestCalculateGraphBounds(t *testing.T) {
	tests := []struct {
		name      string
		graphData *graph.Data
	}{
		{
			name: "Empty graph",
			graphData: &graph.Data{
				Nodes: make(map[string]*graph.Node),
				Edges: []graph.Edge{},
			},
		},
		{
			name: "Single node",
			graphData: &graph.Data{
				Nodes: map[string]*graph.Node{
					"node1": {Name: "node1", X: 100, Y: 50, Width: 80, Height: 40},
				},
				Edges: []graph.Edge{},
			},
		},
		{
			name: "Multiple nodes",
			graphData: &graph.Data{
				Nodes: map[string]*graph.Node{
					"node1": {Name: "node1", X: 0, Y: 0, Width: 80, Height: 40},
					"node2": {Name: "node2", X: 200, Y: 100, Width: 80, Height: 40},
					"node3": {Name: "node3", X: -50, Y: -50, Width: 80, Height: 40},
				},
				Edges: []graph.Edge{},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			bounds := CalculateGraphBounds(tt.graphData)

			if bounds == nil {
				t.Fatal("CalculateGraphBounds returned nil")
			}

			// Verify bounds make sense
			if bounds.Width <= 0 || bounds.Height <= 0 {
				t.Error("Bounds width and height should be positive")
			}

			if bounds.MaxX <= bounds.MinX || bounds.MaxY <= bounds.MinY {
				t.Error("Max coordinates should be greater than min coordinates")
			}

			// For empty graph, should return default bounds
			if len(tt.graphData.Nodes) == 0 {
				if bounds.Width != 400 || bounds.Height != 300 {
					t.Errorf("Empty graph should have default bounds, got width=%f, height=%f", bounds.Width, bounds.Height)
				}
			}
		})
	}
}
