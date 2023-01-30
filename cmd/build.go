package cmd

import (
	"log"
	"os"
	"strings"

	"github.com/M0Rf30/yap/pack"
	"github.com/M0Rf30/yap/project"
	"github.com/spf13/cobra"
)

const argLenght = 2

var NoCache bool

// buildCmd represents the command to build the entire project.
var buildCmd = &cobra.Command{
	Use:   "build [target] [path]",
	Short: "Build multiple PKGBUILD definitions within a yap.json project",
	Args:  cobra.MinimumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		path, err := os.Getwd()
		if len(args) == argLenght {
			path = args[1]
		}
		if err != nil {
			log.Fatal(err)
		}

		split := strings.Split(args[0], "-")
		distro := split[0]
		release := ""
		if len(split) > 1 {
			release = split[1]
		}

		multiplePrj, err := project.MultiProject(distro, release, path)
		if err != nil {
			log.Fatal(err)
		}

		if NoCache {
			if err := multiplePrj.Clean(NoCache); err != nil {
				log.Fatal(err)
			}
		}

		if err := multiplePrj.BuildAll(); err != nil {
			log.Fatal(err)
		}
	},
}

func init() {
	rootCmd.AddCommand(buildCmd)
	buildCmd.Flags().BoolVarP(&NoCache,
		"no-cache", "c", false, "Do not use cache when building the project")
	buildCmd.Flags().BoolVarP(&project.SkipSyncFlag,
		"skip-sync", "s", false, "Skip sync with remotes for package managers")
	buildCmd.PersistentFlags().BoolVarP(&pack.Verbose,
		"verbose", "v", false, "verbose output")
}
