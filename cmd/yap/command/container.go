package command

import (
	"strings"

	"github.com/M0Rf30/yap/pkg/utils"
	"github.com/spf13/cobra"
)

// containerCmd represents the container command.
var containerCmd = &cobra.Command{
	Use:   "container [target]",
	Short: "Pull the built images",
	Args:  cobra.MinimumNArgs(1),
	Run: func(_ *cobra.Command, args []string) {
		split := strings.Split(args[0], "-")
		if len(split) <= 1 {
			utils.Logger.Fatal("for pre-built container images specify also the codename (i. e. rocky-9, ubuntu-focal)")
		}

		err := utils.PullContainers(args[0])
		utils.Logger.Fatal("failed to pull containers",
			utils.Logger.Args("error", err))
	},
}

func init() {
	rootCmd.AddCommand(containerCmd)
}
