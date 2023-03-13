package cmd

import (
	"log"
	"strings"

	"github.com/M0Rf30/yap/packer"
	"github.com/M0Rf30/yap/pkgbuild"
	"github.com/spf13/cobra"
)

// listTargetsCmd represents the listTargets command.
var prepareCmd = &cobra.Command{
	Use:   "prepare [target]",
	Short: "Install base development packages for every supported distro",
	Args:  cobra.MinimumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		split := strings.Split(args[0], "-")
		distro := split[0]
		codeName := ""
		if len(split) > 1 {
			codeName = split[1]
		}

		packageManager := packer.GetPackageManager(&pkgbuild.PKGBUILD{}, distro, codeName)
		err := packageManager.PrepareEnvironment()
		if err != nil {
			log.Fatal(err)
		}
	},
}

func init() {
	rootCmd.AddCommand(prepareCmd)
}
