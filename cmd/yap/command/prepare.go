package command

import (
	"strings"

	"github.com/M0Rf30/yap/pkg/packer"
	"github.com/M0Rf30/yap/pkg/pkgbuild"
	"github.com/M0Rf30/yap/pkg/project"
	"github.com/M0Rf30/yap/pkg/utils"
	"github.com/spf13/cobra"
)

var (
	GoLang bool

	// prepareCmd represents the listDistros command.
	prepareCmd = &cobra.Command{
		Use:   "prepare [distro]",
		Short: "Install base development packages for every supported distro",
		Args:  cobra.MinimumNArgs(1),
		Run: func(_ *cobra.Command, args []string) {
			split := strings.Split(args[0], "-")
			distro := split[0]

			packageManager := packer.GetPackageManager(&pkgbuild.PKGBUILD{}, distro)
			if !project.SkipSyncDeps {
				if err := packageManager.Update(); err != nil {
					utils.Logger.Error(err.Error(),
						utils.Logger.Args("error", err))
				}
			}

			err := packageManager.PrepareEnvironment(GoLang)
			if err != nil {
				utils.Logger.Error(err.Error())
			}

			utils.Logger.Info("basic build environment successfully prepared")

			if GoLang {
				utils.Logger.Info("go successfully installed")
			}
		},
	}
)

func init() {
	rootCmd.AddCommand(prepareCmd)
	prepareCmd.Flags().BoolVarP(&project.SkipSyncDeps,
		"skip-sync", "s", false, "Skip sync with remotes for package managers")
	prepareCmd.Flags().BoolVarP(&GoLang,
		"golang", "g", false, "Additionally install golang")
}
