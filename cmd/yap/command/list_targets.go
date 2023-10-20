package command

import (
	"fmt"
	"strings"

	"github.com/M0Rf30/yap/pkg/constants"
	"github.com/spf13/cobra"
)

// listTargetsCmd represents the listTargets command.
var listTargetsCmd = &cobra.Command{
	Use:   "list-targets",
	Short: "List a bunch of available build targets",
	Run: func(cmd *cobra.Command, args []string) {
		ListTargets()
	},
}

func ListTargets() {
	for _, release := range constants.Releases {
		fmt.Println(strings.ReplaceAll(release, "_", "-"))
	}
}

func init() {
	rootCmd.AddCommand(listTargetsCmd)
}
