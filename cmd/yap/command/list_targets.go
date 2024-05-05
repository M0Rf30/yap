package command

import (
	"github.com/M0Rf30/yap/pkg/constants"
	"github.com/pterm/pterm"
	"github.com/spf13/cobra"
)

// listTargetsCmd represents the listTargets command.
var listTargetsCmd = &cobra.Command{
	Use:   "list-targets",
	Short: "List a bunch of available build targets",
	Run: func(_ *cobra.Command, _ []string) {
		ListTargets()
	},
}

func ListTargets() {
	for _, release := range constants.Releases {
		pterm.Println(release)
	}
}

func init() {
	rootCmd.AddCommand(listTargetsCmd)
}
