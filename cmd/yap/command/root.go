package command

import (
	"os"

	"github.com/pterm/pterm"
	"github.com/spf13/cobra"

	"github.com/M0Rf30/yap/v2/pkg/osutils"
)

var (
	verbose bool
	noColor bool
)

// getLongDescription returns the long description with conditional logo coloring.
func getLongDescription() string {
	// Create stylized YAP logo
	logo := `
	██╗   ██╗ █████╗ ██████╗
	╚██╗ ██╔╝██╔══██╗██╔══██╗
	 ╚████╔╝ ███████║██████╔╝
	  ╚██╔╝  ██╔══██║██╔═══╝
	   ██║   ██║  ██║██║
	   ╚═╝   ╚═╝  ╚═╝╚═╝
	Yet Another Packager
	`

	// Check if colors should be disabled
	var coloredLogo string
	if osutils.IsColorDisabled() {
		coloredLogo = logo
	} else {
		coloredLogo = pterm.FgCyan.Sprint(logo)
	}

	return coloredLogo +
		"\nYAP (Yet Another Packager) is a powerful, container-based package building system" +
		"\nthat creates packages for multiple GNU/Linux distributions from a single PKGBUILD-like" +
		"\nspecification format." +
		"\n\nKEY FEATURES:" +
		"\n  • Multi-format support: RPM, DEB, APK, TAR.ZST packages" +
		"\n  • Container-based isolation for clean, reproducible builds" +
		"\n  • Dependency-aware build orchestration with project management" +
		"\n  • PKGBUILD compatibility with enhanced parsing capabilities" +
		"\n  • Component-aware structured logging system" +
		"\n\nSUPPORTED DISTRIBUTIONS:" +
		"\n  Alpine, Arch, CentOS, Debian, Fedora, OpenSUSE, Rocky, Ubuntu, and more" +
		"\n\nWORKFLOW:" +
		"\n  1. Define packages in yap.json configuration" +
		"\n  2. Create PKGBUILD-compatible build specifications" +
		"\n  3. Run 'yap build' to generate packages for target distributions" +
		"\n  4. Deploy packages using your preferred package management system" +
		"\n\nFor detailed documentation and examples, visit https://github.com/M0Rf30/yap"
}

// rootCmd represents the base command when called without any subcommands.
var rootCmd = &cobra.Command{
	Use:   "yap",
	Short: "🚀 Yet Another Packager - Multi-distribution package builder",
	Long:  getLongDescription(),
	Example: `  # Build all packages in current directory for your current distro
  yap build .

  # Build for specific distribution and release
  yap build ubuntu-jammy /path/to/project

  # Prepare build environment for Rocky Linux 9
  yap prepare rocky-9

  # Clean build artifacts
  yap zap ubuntu-jammy /path/to/project

  # List all supported distributions
  yap list-distros`,
	PersistentPreRun: func(cmd *cobra.Command, _ []string) {
		// Set color preference based on --no-color flag and environment variables
		shouldDisableColor := noColor || os.Getenv("NO_COLOR") != "" || os.Getenv("TERM") == "dumb"
		osutils.SetColorDisabled(shouldDisableColor)

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

//nolint:gochecknoinits // Required for cobra root command initialization
func init() {
	// Check environment variables early for color preference
	if os.Getenv("NO_COLOR") != "" || os.Getenv("TERM") == "dumb" {
		osutils.SetColorDisabled(true)
	}

	// Set up enhanced help formatting
	SetupEnhancedHelp()

	// Add command groups for better organization
	rootCmd.AddGroup(&cobra.Group{ID: "build", Title: "Build Commands"})
	rootCmd.AddGroup(&cobra.Group{ID: "environment", Title: "Environment Commands"})
	rootCmd.AddGroup(&cobra.Group{ID: "utility", Title: "Utility Commands"})

	// Global flags
	rootCmd.PersistentFlags().BoolVarP(&verbose, "verbose", "v", false, "enable verbose output with detailed logging")
	rootCmd.PersistentFlags().BoolVar(&noColor, "no-color", false, "disable colored output")

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
