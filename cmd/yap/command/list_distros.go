package command

import (
	"github.com/pterm/pterm"
	"github.com/spf13/cobra"

	"github.com/M0Rf30/yap/v2/pkg/constants"
)

// listDistrosCmd represents the listDistros command.
var listDistrosCmd = &cobra.Command{
	Use:     "list-distros",
	GroupID: "utility",
	Aliases: []string{"list", "distros"},
	Short:   "📋 List all supported distributions and releases",
	Long: `Display all supported GNU/Linux distributions and their available releases.

This command shows the complete list of distributions that YAP can build
packages for, including their specific release codenames or versions.

Use these identifiers with other YAP commands like build, prepare, pull, and zap.

OUTPUT FORMAT:
Each line shows a distribution identifier in the format used by YAP commands.
For most distributions, this includes both the base name and release codename.`,
	Example: `  # List all supported distributions
  yap list-distros

  # Use with other commands (examples from output)
  yap build ubuntu-jammy .
  yap prepare fedora-38
  yap pull alpine`,
	Run: func(_ *cobra.Command, _ []string) {
		ListDistros()
	},
}

// ListDistros prints all available distribution releases.
func ListDistros() {
	for _, release := range &constants.Releases {
		pterm.Println(release)
	}
}

//nolint:gochecknoinits // Required for cobra command registration
func init() {
	rootCmd.AddCommand(listDistrosCmd)
}
