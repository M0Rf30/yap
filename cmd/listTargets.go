package cmd

import (
	"fmt"
	"strings"

	"github.com/M0Rf30/yap/constants"
	"github.com/spf13/cobra"
)

func ListTargets() {
	for _, release := range constants.Releases {
		fmt.Println(strings.ReplaceAll(release, "_", "-"))
	}
}

// listTargetsCmd represents the listTargets command.
var listTargetsCmd = &cobra.Command{
	Use:   "list-targets",
	Short: "List a bunch of available build targets",
	Run: func(cmd *cobra.Command, args []string) {
		ListTargets()
	},
}

func init() {
	rootCmd.AddCommand(listTargetsCmd)
}
