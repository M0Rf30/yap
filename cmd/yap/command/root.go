package command

import (
	"os"

	"github.com/spf13/cobra"

	"github.com/M0Rf30/yap/v2/pkg/i18n"
	"github.com/M0Rf30/yap/v2/pkg/logger"
)

var (
	verbose  bool
	noColor  bool
	language string
)

// rootCmd represents the base command when called without any subcommands.
var rootCmd = &cobra.Command{
	Use:     "yap",
	Short:   "🚀 Yet Another Packager - Multi-distribution package builder", // Will be set in init()
	Long:    "",                                                            // Will be set in init()
	Example: "",                                                            // Will be set in init()
	PersistentPreRun: func(cmd *cobra.Command, _ []string) {
		// Re-initialize i18n with user preference if language flag was provided
		if language != "" {
			_ = i18n.Init(language) // If i18n fails, continue without localization
			// Update only basic command descriptions to avoid circular dependency
			cmd.Root().Short = i18n.T("root.short")
			cmd.Root().Example = i18n.T("root.examples")
			// Update build command descriptions
			InitializeBuildDescriptions()
			// Update zap command descriptions
			InitializeZapDescriptions()
			// Update graph command descriptions
			InitializeGraphDescriptions()
			// Update install command descriptions
			InitializeInstallDescriptions()
			// Update list-distros command descriptions
			InitializeListDistrosDescriptions()
			// Update prepare command descriptions
			InitializePrepareDescriptions()
		}

		// Set color preference based on --no-color flag and environment variables
		shouldDisableColor := noColor || os.Getenv("NO_COLOR") != "" || os.Getenv("TERM") == "dumb"
		logger.SetColorDisabled(shouldDisableColor)

		// Show welcome message for first-time users
		if cmd.Name() == "yap" && len(cmd.Flags().Args()) == 0 {
			ShowWelcomeMessage()
		}
	}, PersistentPostRun: func(cmd *cobra.Command, _ []string) {
		// Show helpful tips after command execution
		ShowCommandTips(cmd)
	},
}

// Execute adds all child commands to the root command and sets flags
// appropriately. This is called by main.main(). It only needs to happen once to
// the rootCmd.
func Execute() {
	cobra.CheckErr(rootCmd.Execute())
}

// InitializeLocalizedDescriptions sets all localized command descriptions.
// This should be called from main() after all commands are registered.
func InitializeLocalizedDescriptions() {
	// Update root command descriptions
	rootCmd.Short = i18n.T("root.short")
	rootCmd.Example = i18n.T("root.examples")

	// Update command groups
	for _, group := range rootCmd.Groups() {
		switch group.ID {
		case "build":
			group.Title = i18n.T("groups.build")
		case "environment":
			group.Title = i18n.T("groups.environment")
		case "utility":
			group.Title = i18n.T("groups.utility")
		}
	}

	// Update build command descriptions
	InitializeBuildDescriptions()

	// Update zap command descriptions
	InitializeZapDescriptions()

	// Update graph command descriptions
	InitializeGraphDescriptions()

	// Update install command descriptions
	InitializeInstallDescriptions()

	// Update list-distros command descriptions
	InitializeListDistrosDescriptions()

	// Update prepare command descriptions
	InitializePrepareDescriptions()

	// Update other command descriptions
	updateOtherCommandDescriptions()
}

// updateOtherCommandDescriptions updates descriptions for all other commands
func updateOtherCommandDescriptions() {
	// This will be called after all commands are registered
	// Find and update command descriptions by walking through all subcommands
	updateCommandShortDescriptions()
}

// updateCommandShortDescriptions updates the short descriptions of commands
func updateCommandShortDescriptions() {
	// Get all commands from the root command and update their descriptions
	updateSubCommandDescriptions(rootCmd)
}

// updateSubCommandDescriptions recursively updates command descriptions
func updateSubCommandDescriptions(cmd *cobra.Command) {
	for _, subCmd := range cmd.Commands() {
		switch subCmd.Name() {
		case "graph":
			subCmd.Short = i18n.T("commands.graph.short")
		case "install":
			subCmd.Short = i18n.T("commands.install.short")
		case "list-distros":
			subCmd.Short = i18n.T("commands.list_distros.short")
		case prepareCommand:
			subCmd.Short = i18n.T("commands.prepare.short")
		case "pull":
			subCmd.Short = i18n.T("commands.pull.short")
		case "status":
			subCmd.Short = i18n.T("commands.status.short")
		case "version":
			subCmd.Short = i18n.T("commands.version.short")
		case "zap":
			subCmd.Short = i18n.T("commands.zap.short")
		case "completion":
			subCmd.Short = i18n.T("commands.completion.short")
		}
	}
}

//nolint:gochecknoinits // Required for cobra root command initialization
func init() {
	// Initialize i18n system first - this needs to be done before any subcommands are created
	_ = i18n.Init("") // If i18n fails, continue without localization

	// Check environment variables early for color preference
	if os.Getenv("NO_COLOR") != "" || os.Getenv("TERM") == "dumb" {
		logger.SetColorDisabled(true)
	}

	// Set up enhanced help formatting
	SetupEnhancedHelp()

	// Add command groups for better organization
	rootCmd.AddGroup(&cobra.Group{ID: "build", Title: "Build Commands"})
	rootCmd.AddGroup(&cobra.Group{ID: "environment", Title: "Environment Commands"})
	rootCmd.AddGroup(&cobra.Group{ID: "utility", Title: "Utility Commands"})

	// Global flags
	rootCmd.PersistentFlags().BoolVarP(&verbose, "verbose", "v", false,
		i18n.T("flags.verbose"))
	rootCmd.PersistentFlags().BoolVar(&noColor, "no-color", false, i18n.T("flags.no_color"))
	rootCmd.PersistentFlags().StringVarP(&language, "language", "l", "",
		"set language (en, it, ru, zh) - defaults to system locale")

	// Configure completion options
	rootCmd.CompletionOptions.DisableDefaultCmd = false
	rootCmd.CompletionOptions.DisableDescriptions = false

	// Set custom error handler with smart suggestions
	rootCmd.SilenceErrors = true
	rootCmd.SilenceUsage = true
	rootCmd.SetFlagErrorFunc(SmartErrorHandler)
}

// IsNoColorEnabled returns true if the --no-color flag was set.
func IsNoColorEnabled() bool {
	return noColor
}
