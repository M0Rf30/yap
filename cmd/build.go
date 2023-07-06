package cmd

import (
	"fmt"
	"log"
	"path/filepath"
	"strings"

	"github.com/M0Rf30/yap/constants"
	"github.com/M0Rf30/yap/parser"
	"github.com/M0Rf30/yap/pkgbuild"
	"github.com/M0Rf30/yap/project"
	"github.com/spf13/cobra"
)

var NoCache bool

// buildCmd represents the command to build the entire project.
var buildCmd = &cobra.Command{
	Use:   "build [target] [path]",
	Short: "Build multiple PKGBUILD definitions within a yap.json project",
	Args:  cobra.MinimumNArgs(2),
	Run: func(cmd *cobra.Command, args []string) {
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
			fmt.Printf("%s❌ :: %serror:\n",
				string(constants.ColorBlue),
				string(constants.ColorYellow))
			log.Fatal(err)
		}

		if project.NoCache {
			if err := mpc.Clean(); err != nil {
				fmt.Printf("%s❌ :: %serror:\n",
					string(constants.ColorBlue),
					string(constants.ColorYellow))
				log.Fatal(err)
			}
		}

		if err := mpc.BuildAll(); err != nil {
			fmt.Printf("%s❌ :: %serror:\n",
				string(constants.ColorBlue),
				string(constants.ColorYellow))
			log.Fatal(err)
		}
	},
}

func init() {
	rootCmd.AddCommand(buildCmd)
	buildCmd.AddCommand(listTargetsCmd)
	buildCmd.Flags().BoolVarP(&project.SkipSyncBuildEnvironmentDeps,
		"ignore-makedeps", "d", false, "Ignore make dependencies resolution")
	buildCmd.Flags().BoolVarP(&project.NoCache,
		"no-cache", "c", false, "Do not use cache when building the project")
	buildCmd.PersistentFlags().StringVarP(&parser.OverridePkgver,
		"override-pkgver", "p", "", "Override package version (pkgver)")
	buildCmd.Flags().BoolVarP(&project.SkipSyncFlag,
		"skip-sync", "s", false, "Skip sync with remotes for package managers")
	buildCmd.Flags().StringVarP(&project.UntilPkgName,
		"until", "u", "", "Build until a defined package name")
	buildCmd.PersistentFlags().BoolVarP(&pkgbuild.Verbose,
		"verbose", "v", false, "verbose output")
}
