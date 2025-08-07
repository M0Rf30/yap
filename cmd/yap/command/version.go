package command

import (
	"fmt"
	"runtime"
	"strings"
	"time"

	"github.com/pterm/pterm"
	"github.com/pterm/pterm/putils"
	"github.com/spf13/cobra"

	"github.com/M0Rf30/yap/v2/pkg/constants"
)

var versionCmd = &cobra.Command{
	Use:     "version",
	GroupID: "utility",
	Short:   "📊 Display YAP version and system information",
	Long: `Show detailed version information for YAP including build details,
runtime environment, and key features.

This command provides comprehensive system information useful for:
  • Troubleshooting and bug reports
  • Verifying installation and capabilities  
  • Understanding current environment
  • Feature availability confirmation`,
	Example: `  # Display version information
  yap version

  # Typical output includes:
  # - YAP version and build info
  # - Go runtime and system architecture
  # - Available features and capabilities
  # - Links to documentation and support`,
	Run: func(_ *cobra.Command, _ []string) {
		// Create the gorgeous YAP logo
		logo, _ := pterm.DefaultBigText.WithLetters(
			putils.LettersFromStringWithStyle("Y", pterm.NewStyle(pterm.FgBlue)),
			putils.LettersFromStringWithStyle("A", pterm.NewStyle(pterm.FgLightBlue)),
			putils.LettersFromStringWithStyle("P", pterm.NewStyle(pterm.FgLightCyan))).
			Srender()

		pterm.DefaultCenter.Print(logo)

		// Main header with gradient-like effect
		header := pterm.DefaultHeader.WithFullWidth().WithMargin(15).
			WithBackgroundStyle(pterm.NewStyle(pterm.BgDarkGray)).
			WithTextStyle(pterm.NewStyle(pterm.FgLightCyan, pterm.Bold))
		pterm.DefaultCenter.Print(header.Sprint("🚀 Yet Another Packager 🚀"))

		pterm.Println()

		// Create beautiful info panel with box styling
		versionInfo := pterm.DefaultBox.WithTitle("📦 Version Information").
			WithTitleTopLeft().WithBoxStyle(pterm.NewStyle(pterm.FgCyan))

		versionContent := fmt.Sprintf(`%s %s
%s %s
%s %s
%s %s
%s %s`,
			pterm.FgLightBlue.Sprint("Version:"),
			pterm.NewStyle(pterm.FgWhite, pterm.Bold).Sprint(strings.TrimPrefix(constants.YAPVersion, "v")),
			pterm.FgLightMagenta.Sprint("Runtime:"), pterm.FgWhite.Sprintf("%s %s", runtime.GOOS, runtime.GOARCH),
			pterm.FgLightGreen.Sprint("Go Version:"), pterm.FgWhite.Sprint(runtime.Version()),
			pterm.FgYellow.Sprint("Build Time:"), pterm.FgWhite.Sprint(getCurrentTime()),
			pterm.FgLightRed.Sprint("Platform:"), pterm.FgWhite.Sprintf("%s on %s", runtime.Compiler, runtime.GOOS))

		pterm.DefaultCenter.Print(versionInfo.Sprint(versionContent))

		pterm.Println()

		// Features highlight
		featuresBox := pterm.DefaultBox.WithTitle("✨ Key Features").
			WithTitleTopCenter().WithBoxStyle(pterm.NewStyle(pterm.FgMagenta))

		features := fmt.Sprintf(`%s Multi-format package building (RPM, DEB, APK, TAR.ZST)
%s Container-based isolated builds  
%s Dependency-aware build orchestration
%s PKGBUILD compatibility with enhanced parsing
%s Component-aware logging system`,
			pterm.FgGreen.Sprint("▶"),
			pterm.FgGreen.Sprint("▶"),
			pterm.FgGreen.Sprint("▶"),
			pterm.FgGreen.Sprint("▶"),
			pterm.FgGreen.Sprint("▶"))
		pterm.DefaultCenter.Print(featuresBox.Sprint(features))

		pterm.Println()

		// Credits section with heart and sparkles
		pterm.Println()
		pterm.DefaultCenter.Println(pterm.FgRed.Sprint("❤️") + " Coded with love by " +
			pterm.NewStyle(pterm.FgCyan, pterm.Bold).Sprint("M0Rf30"))
		pterm.DefaultCenter.Println(pterm.FgYellow.Sprint("🌟") + " Open Source • GPL3 Licensed")
		pterm.DefaultCenter.Println(pterm.FgBlue.Sprint("🔗") + " github.com/M0Rf30/yap")

		pterm.Println()

		// Fun footer with animated-like border
		border := pterm.FgCyan.Sprint("════════════════════════════════════════════════")
		pterm.DefaultCenter.Println(border)
		pterm.DefaultCenter.Println(pterm.NewStyle(pterm.FgLightBlue, pterm.Italic).Sprint("Thank you for using YAP! 🎉"))
		pterm.DefaultCenter.Println(border)
	},
}

func getCurrentTime() string {
	return time.Now().Format("2006-01-02 15:04:05 MST")
}

//nolint:gochecknoinits // Required for cobra command registration
func init() {
	rootCmd.AddCommand(versionCmd)

	// Set custom usage function
	// Use template-based help system instead of custom usage function
}
