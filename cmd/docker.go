package cmd

import (
	"log"

	"github.com/packagefoundation/yap/utils"
	"github.com/spf13/cobra"
)

// dockerCmd represents the docker command.
var dockerCmd = &cobra.Command{
	Use:   "docker",
	Short: "Pull the built images",
	Run: func(cmd *cobra.Command, args []string) {
		err := utils.PullContainers()
		log.Fatal(err)
	},
}

func init() {
	rootCmd.AddCommand(dockerCmd)
}
