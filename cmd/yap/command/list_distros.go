package command

import (
	"github.com/M0Rf30/yap/pkg/constants"
	"github.com/pterm/pterm"
	"github.com/spf13/cobra"
)

// listDistrosCmd represents the listDistros command.
var listDistrosCmd = &cobra.Command{
	Use:   "list-distros",
	Short: "List a bunch of available distros to build for",
	Run: func(_ *cobra.Command, _ []string) {
		ListDistros()
	},
}

func ListDistros() {
	for _, release := range constants.Releases {
		pterm.Println(release)
	}
}

func init() {
	rootCmd.AddCommand(listDistrosCmd)
}
