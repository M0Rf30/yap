package command

import (
	"fmt"

	"github.com/spf13/cobra"
)

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print the version number of Yap",
	Long:  `All software has versions. This is Yap's`,
	Run: func(_ *cobra.Command, _ []string) {
		fmt.Println("Yap v1.8")
	},
}

func init() {
	rootCmd.AddCommand(versionCmd)
}
