package command

import (
	"fmt"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/M0Rf30/yap/v2/pkg/graph/layout"
	"github.com/M0Rf30/yap/v2/pkg/graph/loader"
	"github.com/M0Rf30/yap/v2/pkg/graph/render"
	"github.com/M0Rf30/yap/v2/pkg/osutils"
)

var (
	graphOutput  string
	graphFormat  string
	graphTheme   string
	showExternal bool
)

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
	graphData, err := loader.LoadProjectForGraph(absPath, graphTheme)
	if err != nil {
		return fmt.Errorf("failed to load project: %w", err)
	}

	// Calculate layout
	layout.CalculateOptimizedLayout(graphData)

	// Create visualization
	switch graphFormat {
	case "png":
		return render.GeneratePNGGraph(graphData, graphOutput, showExternal)
	default:
		return render.GenerateSVGGraph(graphData, graphOutput, showExternal)
	}
}
