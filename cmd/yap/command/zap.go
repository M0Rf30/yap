package command

import (
	"path/filepath"
	"strings"

	"github.com/M0Rf30/yap/pkg/osutils"
	"github.com/M0Rf30/yap/pkg/project"
	"github.com/spf13/cobra"
)

var (
	// zapCmd represents the command to build the entire project.
	zapCmd = &cobra.Command{
		Use:   "zap [distro] [path]",
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

			project.NoMakeDeps = true
			project.SkipSyncDeps = true
			project.Zap = true

			err := mpc.MultiProject(distro, release, fullJSONPath)
			if err != nil {
				osutils.Logger.Fatal("fatal error",
					osutils.Logger.Args("error", err))
			}

			err = mpc.Clean()
			if err != nil {
				osutils.Logger.Fatal("fatal error",
					osutils.Logger.Args("error", err))
			}

			osutils.Logger.Info("zap done", osutils.Logger.Args("distro", distro, "release", release))
		},
	}
)

func init() {
	rootCmd.AddCommand(zapCmd)
}
