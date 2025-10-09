package command

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"

	"github.com/pterm/pterm"
	"github.com/spf13/cobra"

	"github.com/M0Rf30/yap/v2/pkg/constants"
	"github.com/M0Rf30/yap/v2/pkg/i18n"
	"github.com/M0Rf30/yap/v2/pkg/logger"
	"github.com/M0Rf30/yap/v2/pkg/platform"
)

// statusCmd provides system and environment information.
var statusCmd = &cobra.Command{
	Use:     "status",
	GroupID: "utility",
	Aliases: []string{"info", "env"},
	Short:   "📊 Display system status and environment information",
	Long: `Show comprehensive system information including:
  • Current operating system and architecture
  • YAP installation details and version
  • Available container runtime information
  • Build environment status
  • Configuration locations

This command is useful for troubleshooting and verifying your YAP setup.`,
	Example: `  # Show basic system status
  yap status

  # Show detailed environment info
  yap status --verbose`,
	Run: func(_ *cobra.Command, _ []string) {
		showSystemStatus()
	},
}

func showSystemStatus() {
	// Header with logo
	pterm.DefaultHeader.WithFullWidth().
		WithBackgroundStyle(pterm.NewStyle(pterm.BgDarkGray)).
		WithTextStyle(pterm.NewStyle(pterm.FgLightCyan, pterm.Bold)).
		WithMargin(10).
		Println("🚀 YAP System Status")

	pterm.Println()

	// System Information
	systemInfo := pterm.DefaultSection.WithLevel(2).Sprint("💻 System Information")
	pterm.Println(systemInfo)

	osRelease, err := platform.ParseOSRelease()
	if err == nil {
		pterm.Printf("  %s %s\n",
			pterm.FgGreen.Sprint("Operating System:"),
			pterm.FgWhite.Sprint(osRelease.ID))
	}

	pterm.Printf("  %s %s %s\n",
		pterm.FgGreen.Sprint("Architecture:"),
		pterm.FgWhite.Sprint(runtime.GOARCH),
		pterm.FgGray.Sprintf("(%s)", runtime.GOOS))

	pterm.Printf("  %s %s\n",
		pterm.FgGreen.Sprint("Go Runtime:"),
		pterm.FgWhite.Sprint(runtime.Version()))

	pterm.Println()

	// YAP Information
	yapInfo := pterm.DefaultSection.WithLevel(2).Sprint("📦 YAP Information")
	pterm.Println(yapInfo)

	pterm.Printf("  %s %s\n",
		pterm.FgBlue.Sprint("Version:"),
		pterm.FgWhite.Sprint(constants.YAPVersion))

	// Check for yap.json in current directory
	currentDir, _ := os.Getwd()
	yapJSONPath := filepath.Join(currentDir, "yap.json")

	_, statErr := os.Stat(yapJSONPath)
	if statErr == nil {
		pterm.Printf("  %s %s ✓\n",
			pterm.FgBlue.Sprint("Project Found:"),
			pterm.FgGreen.Sprint("yap.json detected in current directory"))
	} else {
		pterm.Printf("  %s %s\n",
			pterm.FgBlue.Sprint("Project Status:"),
			pterm.FgYellow.Sprint("No yap.json found in current directory"))
	}

	pterm.Println()

	// Supported Distributions
	distroInfo := pterm.DefaultSection.WithLevel(2).Sprint("🌍 Supported Distributions")
	pterm.Println(distroInfo)

	distroCount := len(constants.Releases)
	pterm.Printf("  %s %d distributions supported\n",
		pterm.FgMagenta.Sprint("Total:"),
		distroCount)

	if verbose {
		// Show all distributions in a nice format
		pterm.Printf("  %s\n", pterm.FgMagenta.Sprint("Available:"))

		// Group distributions by family
		families := make(map[string][]string)

		for _, release := range &constants.Releases {
			family := getDistroFamily(release)
			families[family] = append(families[family], release)
		}

		for family, distros := range families {
			pterm.Printf("    %s: %s\n",
				pterm.NewStyle(pterm.FgCyan, pterm.Bold).Sprint(family),
				pterm.FgWhite.Sprint(fmt.Sprintf("%d variants", len(distros))))
		}
	} else {
		logger.Tips(i18n.T("logger.tips.use_verbose"))
	}

	pterm.Println()

	// Environment Status
	envInfo := pterm.DefaultSection.WithLevel(2).Sprint("⚙️  Environment Status")
	pterm.Println(envInfo)

	// Check for container runtime
	containerRuntime := checkContainerRuntime()
	if containerRuntime != "" {
		pterm.Printf("  %s %s ✓\n",
			pterm.FgYellow.Sprint("Container Runtime:"),
			pterm.FgGreen.Sprint(containerRuntime))
	} else {
		pterm.Printf("  %s %s\n",
			pterm.FgYellow.Sprint("Container Runtime:"),
			pterm.FgRed.Sprint("Not detected"))
	}

	pterm.Println()

	// Footer with helpful commands
	pterm.DefaultBox.WithTitle("💡 Quick Commands").
		WithTitleTopLeft().
		WithBoxStyle(pterm.NewStyle(pterm.FgBlue)).
		Println(`yap list-distros    - Show all supported distributions
yap prepare <distro> - Set up build environment
yap version         - Show detailed version information
yap build --help    - Get help with building packages`)
}

func getDistroFamily(release string) string {
	switch {
	case contains(release, "ubuntu") || contains(release, "debian"):
		return "debian"
	case contains(release, "fedora") || contains(release, "rhel") ||
		contains(release, "centos") || contains(release, "rocky"):
		return "redhat"
	case contains(release, "opensuse") || contains(release, "suse"):
		return "suse"
	case contains(release, "arch"):
		return "arch"
	case contains(release, "alpine"):
		return "alpine"
	default:
		return "unknown"
	}
}

func contains(s, substr string) bool {
	if substr == "" {
		return true
	}

	if len(s) < len(substr) {
		return false
	}

	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}

	return false
}

func checkContainerRuntime() string {
	// Check for common container runtimes
	runtimes := []string{"docker", "podman", "containerd"}

	for _, runtime := range runtimes {
		_, err := os.Stat("/usr/bin/" + runtime)
		if err == nil {
			return runtime
		}

		_, err = os.Stat("/usr/local/bin/" + runtime)
		if err == nil {
			return runtime
		}
	}

	return ""
}

//nolint:gochecknoinits // Required for cobra command registration
func init() {
	rootCmd.AddCommand(statusCmd)
}
