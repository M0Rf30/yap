package command

import (
	"strings"

	"github.com/M0Rf30/yap/pkg/utils"
	"github.com/spf13/cobra"
)

// pullCmd represents the pull command.
var pullCmd = &cobra.Command{
	Use:   "pull [distro]",
	Short: "Pull pre-built container images",
	Args:  cobra.MinimumNArgs(1),
	Run: func(_ *cobra.Command, args []string) {
		split := strings.Split(args[0], "-")

		if len(split) == 1 && split[0] != "alpine" && split[0] != "arch" {
			utils.Logger.Fatal("except for alpine and arch, specify also the codename (i. e. rocky-9, ubuntu-focal)")
		}

		if err := utils.PullContainers(args[0]); err != nil {
			utils.Logger.Fatal("failed to pull image",
				utils.Logger.Args("error", err))
		}
	},
}

func init() {
	rootCmd.AddCommand(pullCmd)
}
