package cmd

import (
	"log"

	"github.com/M0Rf30/yap/utils"
	"github.com/spf13/cobra"
)

// dockerCmd represents the docker command.
var dockerCmd = &cobra.Command{
	Use:   "docker [target]",
	Short: "Pull the built images",
	Args:  cobra.MinimumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		err := utils.PullContainers(args[0])
		log.Fatal(err)
	},
}

func init() {
	rootCmd.AddCommand(dockerCmd)
}
