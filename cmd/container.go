package cmd

import (
	"log"

	"github.com/M0Rf30/yap/utils"
	"github.com/spf13/cobra"
)

// containerCmd represents the container command.
var containerCmd = &cobra.Command{
	Use:   "container [target]",
	Short: "Pull the built images",
	Args:  cobra.MinimumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		err := utils.PullContainers(args[0])
		log.Fatal(err)
	},
}

func init() {
	rootCmd.AddCommand(containerCmd)
}
