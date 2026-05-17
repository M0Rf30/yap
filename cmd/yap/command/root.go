package command

import (
	"os"
	"slices"

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
	Use:     commandYap,
	Short:   "Yet Another Packager - Multi-distribution package builder", // Will be set in init()
	Long:    "",                                                          // Will be set in init()
	Example: "",                                                          // Will be set in init()
	PersistentPreRun: func(_ *cobra.Command, _ []string) {
		// Set color preference based on --no-color flag and environment variables
		shouldDisableColor := noColor || os.Getenv("NO_COLOR") != "" || os.Getenv("TERM") == "dumb"
		logger.SetColorDisabled(shouldDisableColor)
	},
	PersistentPostRun: func(cmd *cobra.Command, _ []string) {
		// Show helpful tips after command execution
		ShowCommandTips(cmd)
	},
}

// ParseLanguageFlag scans os.Args for -l / --language before cobra parses
// flags, so that i18n is re-initialised with the correct locale before
// InitializeLocalizedDescriptions() sets all command descriptions — including
// the path taken by --help, which never fires PersistentPreRun.
func ParseLanguageFlag() {
	args := os.Args[1:]
	supported := i18n.SupportedLanguages

	for i, arg := range args {
		var lang string

		switch {
		case arg == "-l" || arg == "--language":
			if i+1 < len(args) {
				lang = args[i+1]
			}
		case len(arg) > 10 && arg[:11] == "--language=":
			lang = arg[11:]
		case len(arg) > 3 && arg[:3] == "-l=":
			lang = arg[3:]
		}

		if lang != "" && slices.Contains(supported, lang) {
			_ = i18n.Init(lang)

			return
		}
	}
}

// Execute adds all child commands to the root command and sets flags
// appropriately. This is called by main.main(). It only needs to happen once to
// the rootCmd.
func Execute() {
	cobra.CheckErr(rootCmd.Execute())
}

// InitializeLocalizedDescriptions sets all localized command descriptions.
// This should be called from main() after ParseLanguageFlag() and after all
// commands are registered.
func InitializeLocalizedDescriptions() {
	// Update root command descriptions
	rootCmd.Short = i18n.T("root.short")
	rootCmd.Example = i18n.T("root.examples")

	// Update global flag descriptions (set to "" at init time; locale now known)
	if f := rootCmd.PersistentFlags().Lookup("verbose"); f != nil {
		f.Usage = i18n.T("flags.verbose")
	}

	if f := rootCmd.PersistentFlags().Lookup("no-color"); f != nil {
		f.Usage = i18n.T("flags.no_color")
	}

	// Update command groups
	for _, group := range rootCmd.Groups() {
		switch group.ID {
		case buildGroup:
			group.Title = i18n.T("groups.build")
		case commandEnvironment:
			group.Title = i18n.T("groups.environment")
		case commandUtility:
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
		case commandInstall:
			subCmd.Short = i18n.T("commands.install.short")
		case commandListDistro:
			subCmd.Short = i18n.T("commands.list_distros.short")
		case prepareCommand:
			subCmd.Short = i18n.T("commands.prepare.short")
		case commandPull:
			subCmd.Short = i18n.T("commands.pull.short")
		case commandStatus:
			subCmd.Short = i18n.T("commands.status.short")
		case commandVersion:
			subCmd.Short = i18n.T("commands.version.short")
		case commandZap:
			subCmd.Short = i18n.T("commands.zap.short")
		case commandHelp:
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
	rootCmd.AddGroup(&cobra.Group{ID: buildGroup, Title: "Build Commands"})
	rootCmd.AddGroup(&cobra.Group{ID: commandEnvironment, Title: "Environment Commands"})
	rootCmd.AddGroup(&cobra.Group{ID: commandUtility, Title: utilityCommandsGroup})

	// Global flags — descriptions are updated after ParseLanguageFlag() in main().
	rootCmd.PersistentFlags().BoolVarP(&verbose, "verbose", "v", false, "")
	rootCmd.PersistentFlags().BoolVar(&noColor, "no-color", false, "")
	rootCmd.PersistentFlags().StringVarP(&language, "language", "l", "",
		"set language (en, it) - defaults to system locale")

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
