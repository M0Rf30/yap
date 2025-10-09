package render

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/M0Rf30/yap/v2/pkg/graph"
)

func TestGenerateSVGGraph(t *testing.T) {
	tests := []struct {
		name         string
		graphData    *graph.Data
		showExternal bool
		expectError  bool
	}{
		{
			name: "Empty graph",
			graphData: &graph.Data{
				Nodes: make(map[string]*graph.Node),
				Edges: []graph.Edge{},
				Theme: graph.Theme{
					Background:   "#ffffff",
					NodeInternal: "#007acc",
					NodeExternal: "#cccccc",
				},
			},
			showExternal: false,
			expectError:  false,
		},
		{
			name: "Single node",
			graphData: &graph.Data{
				Nodes: map[string]*graph.Node{
					"test-node": {
						Name:    "test-node",
						PkgName: "test-pkg",
						Version: "1.0.0",
						X:       100,
						Y:       50,
						Width:   80,
						Height:  40,
					},
				},
				Edges: []graph.Edge{},
				Theme: graph.Theme{
					Background:   "#ffffff",
					NodeInternal: "#007acc",
					NodeExternal: "#cccccc",
					TextColor:    "#000000",
				},
			},
			showExternal: true,
			expectError:  false,
		},
		{
			name: "Multiple nodes with edges",
			graphData: &graph.Data{
				Nodes: map[string]*graph.Node{
					"node1": {
						Name:    "node1",
						PkgName: "pkg1",
						Version: "1.0.0",
						X:       100,
						Y:       50,
						Width:   80,
						Height:  40,
					},
					"node2": {
						Name:       "node2",
						PkgName:    "pkg2",
						Version:    "2.0.0",
						X:          200,
						Y:          150,
						Width:      80,
						Height:     40,
						IsExternal: true,
					},
				},
				Edges: []graph.Edge{
					{From: "node1", To: "node2", Type: "runtime"},
				},
				Theme: graph.Theme{
					Background:   "#ffffff",
					NodeInternal: "#007acc",
					NodeExternal: "#cccccc",
					EdgeRuntime:  "#333333",
					TextColor:    "#000000",
				},
			},
			showExternal: true,
			expectError:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create temporary file
			tmpDir, err := os.MkdirTemp("", "render-test")
			if err != nil {
				t.Fatalf("Failed to create temp dir: %v", err)
			}

			defer func() { _ = os.RemoveAll(tmpDir) }()

			outputPath := filepath.Join(tmpDir, "test.svg")

			err = GenerateSVGGraph(tt.graphData, outputPath, tt.showExternal)

			if tt.expectError && err == nil {
				t.Error("Expected error but got none")
			}

			if !tt.expectError && err != nil {
				t.Errorf("Unexpected error: %v", err)
			}

			if tt.expectError {
				return
			}

			// Verify file was created
			if _, err := os.Stat(outputPath); os.IsNotExist(err) {
				t.Error("SVG file was not created")
			}

			// Read and verify basic SVG structure
			content, err := os.ReadFile(outputPath)
			if err != nil {
				t.Fatalf("Failed to read SVG file: %v", err)
			}

			svgContent := string(content)
			if !strings.Contains(svgContent, "<?xml") {
				t.Error("SVG file missing XML declaration")
			}

			if !strings.Contains(svgContent, "<svg") {
				t.Error("SVG file missing SVG element")
			}
		})
	}
}

func TestGeneratePNGGraph(t *testing.T) {
	graphData := &graph.Data{
		Nodes: map[string]*graph.Node{
			"test-node": {
				Name:    "test-node",
				PkgName: "test-pkg",
				Version: "1.0.0",
				X:       100,
				Y:       50,
				Width:   80,
				Height:  40,
			},
		},
		Edges: []graph.Edge{},
		Theme: graph.Theme{
			Background:   "#ffffff",
			NodeInternal: "#007acc",
			TextColor:    "#000000",
		},
	}

	// Create temporary file
	tmpDir, err := os.MkdirTemp("", "render-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}

	defer func() { _ = os.RemoveAll(tmpDir) }()

	outputPath := filepath.Join(tmpDir, "test.png")

	err = GeneratePNGGraph(graphData, outputPath, true)

	// PNG generation might fail if resvg is not available, which is expected in test environment
	if err != nil {
		// Check if it's the expected "resvg not found" error
		if !strings.Contains(err.Error(), "resvg") && !strings.Contains(err.Error(), "executable file not found") {
			t.Errorf("Unexpected error: %v", err)
		}
	}
}

func TestCreateSVGContent(t *testing.T) {
	graphData := &graph.Data{
		Nodes: map[string]*graph.Node{
			"test-node": {
				Name:    "test-node",
				PkgName: "test-pkg",
				Version: "1.0.0",
				X:       100,
				Y:       50,
				Width:   80,
				Height:  40,
			},
		},
		Edges: []graph.Edge{},
		Theme: graph.Theme{
			Background:   "#ffffff",
			NodeInternal: "#007acc",
			TextColor:    "#000000",
		},
	}

	svg := createSVGContent(graphData, true)

	if svg == nil {
		t.Fatal("createSVGContent returned nil")
	}

	if svg.Xmlns != "http://www.w3.org/2000/svg" {
		t.Error("SVG xmlns not set correctly")
	}

	if svg.Width == "" || svg.Height == "" {
		t.Error("SVG dimensions not set")
	}
}

func TestCreateSVGStyles(t *testing.T) {
	theme := &graph.Theme{
		Background:   "#ffffff",
		NodeInternal: "#007acc",
		NodeExternal: "#cccccc",
		TextColor:    "#000000",
	}

	styles := createSVGStyles(theme)

	if styles == "" {
		t.Error("createSVGStyles returned empty string")
	}

	// Check that styles contain expected elements
	expectedElements := []string{".node", ".edge"}
	for _, element := range expectedElements {
		if !strings.Contains(styles, element) {
			t.Errorf("Styles missing element: %s", element)
		}
	}
}
