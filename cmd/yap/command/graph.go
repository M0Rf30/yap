package command

import (
	"fmt"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/M0Rf30/yap/v2/pkg/graph/layout"
	"github.com/M0Rf30/yap/v2/pkg/graph/loader"
	"github.com/M0Rf30/yap/v2/pkg/graph/render"
	"github.com/M0Rf30/yap/v2/pkg/i18n"
	"github.com/M0Rf30/yap/v2/pkg/logger"
)

var (
	graphOutput  string
	graphFormat  string
	graphTheme   string
	showExternal bool
)

// graphCmd represents the graph command
var graphCmd = &cobra.Command{
	Use:     "graph [path]",
	Short:   "🎨 Generate beautiful dependency graphs", // Will be set in init()
	Long:    "",                                       // Will be set in init()
	Example: "",                                       // Will be set in init()
	GroupID: "utility",
	Args:    cobra.MaximumNArgs(1),
	RunE:    runGraphCommand,
}

// InitializeGraphDescriptions sets the localized descriptions for the graph command.
// This must be called after i18n is initialized.
func InitializeGraphDescriptions() {
	graphCmd.Short = i18n.T("commands.graph.short")
	graphCmd.Long = i18n.T("commands.graph.long")
	graphCmd.Example = i18n.T("commands.graph.examples")

	// Update flag descriptions with localized text
	graphCmd.Flag("output").Usage = i18n.T("flags.graph.output")
	graphCmd.Flag("format").Usage = i18n.T("flags.graph.format")
	graphCmd.Flag("theme").Usage = i18n.T("flags.graph.theme")
	graphCmd.Flag("show-external").Usage = i18n.T("flags.graph.show_external")
}

//nolint:gochecknoinits // Required for cobra command registration
func init() {
	rootCmd.AddCommand(graphCmd)

	graphCmd.Flags().StringVarP(&graphOutput, "output", "o", "",
		"")
	graphCmd.Flags().StringVarP(&graphFormat, "format", "f", "svg",
		"")
	graphCmd.Flags().StringVar(&graphTheme, "theme", "modern",
		"")
	graphCmd.Flags().BoolVar(&showExternal, "show-external", false,
		"")
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
		return fmt.Errorf(i18n.T("errors.graph.failed_to_resolve_project_path")+": %w", err)
	}

	logger.Info(i18n.T("logger.rungraphcommand.info.generating_dependency_graph_1"),
		"project", absPath,
		"output", graphOutput,
		"format", graphFormat,
		"theme", graphTheme)

	// Load project configuration only
	graphData, err := loader.LoadProjectForGraph(absPath, graphTheme)
	if err != nil {
		return fmt.Errorf(i18n.T("errors.graph.failed_to_load_project")+": %w", err)
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
