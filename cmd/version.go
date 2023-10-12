package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(versionCmd)
}

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print the version number of Yap",
	Long:  `All software has versions. This is Yap's`,
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println("Yap v1.0")
	},
}
