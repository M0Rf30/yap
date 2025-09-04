package command

import (
	"github.com/pterm/pterm"
	"github.com/spf13/cobra"

	"github.com/M0Rf30/yap/v2/pkg/constants"
	"github.com/M0Rf30/yap/v2/pkg/i18n"
)

// listDistrosCmd represents the listDistros command.
var listDistrosCmd = &cobra.Command{
	Use:     "list-distros",
	GroupID: "utility",
	Aliases: []string{"list", "distros"},
	Short:   "📋 List all supported distributions and releases", // Will be set in init()
	Long:    "",                                                // Will be set in init()
	Example: "",                                                // Will be set in init()
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

// InitializeListDistrosDescriptions sets the localized descriptions for the list-distros command.
// This must be called after i18n is initialized.
func InitializeListDistrosDescriptions() {
	listDistrosCmd.Short = i18n.T("commands.list_distros.short")
	listDistrosCmd.Long = i18n.T("commands.list_distros.long")
	listDistrosCmd.Example = i18n.T("commands.list_distros.examples")
}

//nolint:gochecknoinits // Required for cobra command registration
func init() {
	rootCmd.AddCommand(listDistrosCmd)
}
