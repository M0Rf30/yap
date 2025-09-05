package graph

import (
	"testing"
)

func TestNode_Creation(t *testing.T) {
	node := &Node{
		Name:         "test-package",
		PkgName:      "test-pkg",
		Version:      "1.0.0",
		Release:      "1",
		X:            100.0,
		Y:            200.0,
		Width:        50.0,
		Height:       30.0,
		IsExternal:   false,
		IsPopular:    true,
		Dependencies: []string{"dep1", "dep2"},
		Level:        2,
	}

	if node.Name != "test-package" {
		t.Fatalf("Expected Name 'test-package', got '%s'", node.Name)
	}

	if node.PkgName != "test-pkg" {
		t.Fatalf("Expected PkgName 'test-pkg', got '%s'", node.PkgName)
	}

	if node.Version != "1.0.0" {
		t.Fatalf("Expected Version '1.0.0', got '%s'", node.Version)
	}

	if node.Release != "1" {
		t.Fatalf("Expected Release '1', got '%s'", node.Release)
	}

	if node.X != 100.0 {
		t.Fatalf("Expected X 100.0, got %f", node.X)
	}

	if node.Y != 200.0 {
		t.Fatalf("Expected Y 200.0, got %f", node.Y)
	}

	if node.Width != 50.0 {
		t.Fatalf("Expected Width 50.0, got %f", node.Width)
	}

	if node.Height != 30.0 {
		t.Fatalf("Expected Height 30.0, got %f", node.Height)
	}

	if node.IsExternal {
		t.Fatal("Expected IsExternal false, got true")
	}

	if !node.IsPopular {
		t.Fatal("Expected IsPopular true, got false")
	}

	if len(node.Dependencies) != 2 {
		t.Fatalf("Expected 2 dependencies, got %d", len(node.Dependencies))
	}

	if node.Level != 2 {
		t.Fatalf("Expected Level 2, got %d", node.Level)
	}
}

func TestEdge_Creation(t *testing.T) {
	edge := Edge{
		From: "package-a",
		To:   "package-b",
		Type: "runtime",
	}

	if edge.From != "package-a" {
		t.Fatalf("Expected From 'package-a', got '%s'", edge.From)
	}

	if edge.To != "package-b" {
		t.Fatalf("Expected To 'package-b', got '%s'", edge.To)
	}

	if edge.Type != "runtime" {
		t.Fatalf("Expected Type 'runtime', got '%s'", edge.Type)
	}
}

func TestEdge_Types(t *testing.T) {
	edgeTypes := []string{"runtime", "make", "check", "opt"}

	for _, edgeType := range edgeTypes {
		edge := Edge{
			From: "source",
			To:   "target",
			Type: edgeType,
		}

		if edge.Type != edgeType {
			t.Fatalf("Expected edge type '%s', got '%s'", edgeType, edge.Type)
		}
	}
}

func TestData_Creation(t *testing.T) {
	node1 := &Node{Name: "node1", PkgName: "pkg1"}
	node2 := &Node{Name: "node2", PkgName: "pkg2"}

	data := &Data{
		Nodes: map[string]*Node{
			"node1": node1,
			"node2": node2,
		},
		Edges: []Edge{
			{From: "node1", To: "node2", Type: "runtime"},
		},
		Theme: Theme{
			Background:   "#ffffff",
			NodeInternal: "#0066cc",
		},
	}

	if len(data.Nodes) != 2 {
		t.Fatalf("Expected 2 nodes, got %d", len(data.Nodes))
	}

	if len(data.Edges) != 1 {
		t.Fatalf("Expected 1 edge, got %d", len(data.Edges))
	}

	if data.Theme.Background != "#ffffff" {
		t.Fatalf("Expected background '#ffffff', got '%s'", data.Theme.Background)
	}
}

func TestBounds_Creation(t *testing.T) {
	bounds := Bounds{
		MinX:    0.0,
		MinY:    0.0,
		MaxX:    800.0,
		MaxY:    600.0,
		Width:   800.0,
		Height:  600.0,
		Padding: 20.0,
	}

	if bounds.MinX != 0.0 {
		t.Fatalf("Expected MinX 0.0, got %f", bounds.MinX)
	}

	if bounds.MaxX != 800.0 {
		t.Fatalf("Expected MaxX 800.0, got %f", bounds.MaxX)
	}

	if bounds.Width != 800.0 {
		t.Fatalf("Expected Width 800.0, got %f", bounds.Width)
	}

	if bounds.Padding != 20.0 {
		t.Fatalf("Expected Padding 20.0, got %f", bounds.Padding)
	}
}

func TestTheme_Creation(t *testing.T) {
	theme := Theme{
		Background:   "#ffffff",
		NodeInternal: "#0066cc",
		NodeExternal: "#ff6600",
		NodePopular:  "#ff0066",
		EdgeRuntime:  "#000000",
		EdgeMake:     "#666666",
		EdgeCheck:    "#999999",
		EdgeOptional: "#cccccc",
		TextColor:    "#333333",
		BorderColor:  "#dddddd",
		GridColor:    "#f0f0f0",
	}

	if theme.Background != "#ffffff" {
		t.Fatalf("Expected Background '#ffffff', got '%s'", theme.Background)
	}

	if theme.NodeInternal != "#0066cc" {
		t.Fatalf("Expected NodeInternal '#0066cc', got '%s'", theme.NodeInternal)
	}

	if theme.NodeExternal != "#ff6600" {
		t.Fatalf("Expected NodeExternal '#ff6600', got '%s'", theme.NodeExternal)
	}

	if theme.EdgeRuntime != "#000000" {
		t.Fatalf("Expected EdgeRuntime '#000000', got '%s'", theme.EdgeRuntime)
	}
}

func TestOptions_Creation(t *testing.T) {
	options := Options{
		Output:       "graph.svg",
		Format:       "svg",
		Theme:        "default",
		ShowExternal: true,
	}

	if options.Output != "graph.svg" {
		t.Fatalf("Expected Output 'graph.svg', got '%s'", options.Output)
	}

	if options.Format != "svg" {
		t.Fatalf("Expected Format 'svg', got '%s'", options.Format)
	}

	if options.Theme != "default" {
		t.Fatalf("Expected Theme 'default', got '%s'", options.Theme)
	}

	if !options.ShowExternal {
		t.Fatal("Expected ShowExternal true, got false")
	}
}

func TestNode_DefaultValues(t *testing.T) {
	node := &Node{}

	// Test zero values
	if node.X != 0.0 {
		t.Fatalf("Expected default X 0.0, got %f", node.X)
	}

	if node.IsExternal {
		t.Fatal("Expected default IsExternal false, got true")
	}

	if node.IsPopular {
		t.Fatal("Expected default IsPopular false, got true")
	}

	if node.Level != 0 {
		t.Fatalf("Expected default Level 0, got %d", node.Level)
	}
}

func TestData_NodeLookup(t *testing.T) {
	node1 := &Node{Name: "test-node", PkgName: "test-pkg"}

	data := &Data{
		Nodes: map[string]*Node{
			"test-node": node1,
		},
		Edges: []Edge{},
	}

	// Test node lookup
	foundNode, exists := data.Nodes["test-node"]
	if !exists {
		t.Fatal("Node should exist in map")
	}

	if foundNode.Name != "test-node" {
		t.Fatalf("Expected found node name 'test-node', got '%s'", foundNode.Name)
	}

	// Test non-existent node
	_, exists = data.Nodes["non-existent"]
	if exists {
		t.Fatal("Non-existent node should not be found")
	}
}

func TestEdge_Directional(t *testing.T) {
	// Test that edges are directional
	edge1 := Edge{From: "A", To: "B", Type: "runtime"}
	edge2 := Edge{From: "B", To: "A", Type: "runtime"}

	// These should be different edges even though they connect the same nodes
	if edge1.From == edge2.From && edge1.To == edge2.To {
		t.Fatal("Edges should be directional - A->B is different from B->A")
	}
}
