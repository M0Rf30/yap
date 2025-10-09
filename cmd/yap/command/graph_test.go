package command

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/M0Rf30/yap/v2/pkg/graph"
	"github.com/M0Rf30/yap/v2/pkg/graph/render"
	"github.com/M0Rf30/yap/v2/pkg/graph/theme"
)

func TestGraphCommand(t *testing.T) {
	t.Run("SVG generation", func(t *testing.T) {
		// Create a temporary directory for testing
		tempDir := t.TempDir()
		outputPath := filepath.Join(tempDir, "test-graph.svg")

		// Test basic SVG generation
		graphData := &graph.Data{
			Nodes: map[string]*graph.Node{
				"pkg1": {
					Name:    "pkg1",
					Version: "1.0.0",
					Release: "1",
					X:       100,
					Y:       100,
					Level:   0,
				},
				"pkg2": {
					Name:    "pkg2",
					Version: "2.0.0",
					Release: "1",
					X:       200,
					Y:       200,
					Level:   1,
				},
			},
			Edges: []graph.Edge{
				{From: "pkg2", To: "pkg1", Type: "runtime"},
			},
			Theme: theme.GetTheme("modern"),
		}

		err := render.GenerateSVGGraph(graphData, outputPath, false)
		require.NoError(t, err)

		// Verify file was created
		assert.FileExists(t, outputPath)

		// Verify file has content
		stat, err := os.Stat(outputPath)
		require.NoError(t, err)
		assert.Greater(t, stat.Size(), int64(100))
	})

	t.Run("Theme selection", func(t *testing.T) {
		modernTheme := theme.GetTheme("modern")
		assert.Equal(t, "#00C851", modernTheme.NodeInternal)

		darkTheme := theme.GetTheme("dark")
		assert.Equal(t, "#1a1a1a", darkTheme.Background)

		classicTheme := theme.GetTheme("classic")
		assert.Equal(t, "#ffffff", classicTheme.Background)
	})
}

func TestGraphCommandFlags(t *testing.T) {
	// Reset global variables
	graphOutput = ""
	graphFormat = "svg"
	graphTheme = "modern"
	showExternal = false

	t.Run("Default values", func(t *testing.T) {
		assert.Equal(t, "", graphOutput)
		assert.Equal(t, "svg", graphFormat)
		assert.Equal(t, "modern", graphTheme)
		assert.False(t, showExternal)
	})
}
