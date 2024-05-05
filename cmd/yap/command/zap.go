package command

import (
	"path/filepath"
	"strings"

	"github.com/M0Rf30/yap/pkg/project"
	"github.com/M0Rf30/yap/pkg/utils"
	"github.com/spf13/cobra"
)

var (
	// zapCmd represents the command to build the entire project.
	zapCmd = &cobra.Command{
		Use:   "zap [target] [path]",
		Short: "Deeply clean the build environment of a project",
		Args:  cobra.MinimumNArgs(2),
		Run: func(_ *cobra.Command, args []string) {
			fullJSONPath, _ := filepath.Abs(args[1])
			split := strings.Split(args[0], "-")
			distro := split[0]
			release := ""

			if len(split) > 1 {
				release = split[1]
			}

			mpc := project.MultipleProject{}
			err := mpc.MultiProject(distro, release, fullJSONPath)
			if err != nil {
				utils.Logger.Fatal("fatal error",
					utils.Logger.Args("error", err))
			}

			if err := mpc.Clean(); err != nil {
				utils.Logger.Fatal("fatal error",
					utils.Logger.Args("error", err))
			}

			utils.Logger.Info("zap done", utils.Logger.Args("distro", distro, "release", release))
		},
	}
)

func init() {
	rootCmd.AddCommand(zapCmd)

	project.NoMakeDeps = true
	project.SkipSyncDeps = true
	project.Zap = true
}
