package cmd

import (
	"fmt"
	"log"
	"strings"

	"github.com/M0Rf30/yap/constants"
	"github.com/M0Rf30/yap/packer"
	"github.com/M0Rf30/yap/pkgbuild"
	"github.com/spf13/cobra"
)

var GoLang bool

// prepareCmd represents the listTargets command.
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

		err := packageManager.PrepareEnvironment(GoLang)
		if err != nil {
			log.Fatal(err)
		}

		fmt.Printf("%s🪛 :: %sBasic build environment successfully prepared%s\n",
			string(constants.ColorBlue),
			string(constants.ColorYellow),
			string(constants.ColorWhite))

		if GoLang {
			fmt.Printf("%s🪛 :: %sGO successfully installed%s\n",
				string(constants.ColorBlue),
				string(constants.ColorYellow),
				string(constants.ColorWhite))
		}
	},
}

func init() {
	rootCmd.AddCommand(prepareCmd)
	prepareCmd.Flags().BoolVarP(&GoLang,
		"golang", "g", false, "Additionally install golang")
}
