package command

import (
	"path/filepath"
	"strings"

	"github.com/M0Rf30/yap/pkg/osutils"
	"github.com/M0Rf30/yap/pkg/parser"
	"github.com/M0Rf30/yap/pkg/project"
	"github.com/M0Rf30/yap/pkg/source"
	"github.com/spf13/cobra"
)

var (
	// buildCmd represents the command to build the entire project.
	buildCmd = &cobra.Command{
		Use:   "build [distro] path",
		Short: "Build multiple PKGBUILD definitions within a yap.json project",
		Args:  cobra.RangeArgs(1, 2), // Allow 1 or 2 arguments
		Run: func(_ *cobra.Command, args []string) {
			fullJSONPath, _ := filepath.Abs(args[len(args)-1]) // Always take the last argument as path
			var distro, release string

			if len(args) == 2 {
				split := strings.Split(args[0], "-")
				distro = split[0]

				if len(split) > 1 {
					release = split[1]
				}
			}

			// Use the default distro if none is provided
			if distro == "" {
				osRelease, _ := osutils.ParseOSRelease()
				distro = osRelease.ID
				osutils.Logger.Warn("distro not specified, using detected",
					osutils.Logger.Args("distro", distro))
			}

			mpc := project.MultipleProject{}
			err := mpc.MultiProject(distro, release, fullJSONPath)
			if err != nil {
				osutils.Logger.Fatal("fatal error",
					osutils.Logger.Args("error", err))
			}

			err = mpc.BuildAll()
			if err != nil {
				osutils.Logger.Fatal("fatal error",
					osutils.Logger.Args("error", err))
			}
		},
	}
)

func init() {
	rootCmd.AddCommand(buildCmd)
	buildCmd.Flags().BoolVarP(&project.CleanBuild,
		"cleanbuild", "c", false, "Remove $srcdir/ dir before building the package")
	buildCmd.Flags().BoolVarP(&project.NoMakeDeps,
		"nomakedeps", "d", false, "Skip all make dependency (makedeps) checks")
	buildCmd.Flags().BoolVarP(&project.NoBuild,
		"nobuild", "o", false, "Download and extract files only")
	buildCmd.PersistentFlags().StringVarP(&parser.OverridePkgVer,
		"pkgver", "w", "", "Use a custom package version (pkgver)")
	buildCmd.PersistentFlags().StringVarP(&parser.OverridePkgRel,
		"pkgrel", "r", "", "Use a custom package release (pkgrel)")
	buildCmd.Flags().BoolVarP(&project.SkipSyncDeps,
		"skip-sync", "s", false, "Skip sync with remotes for package managers")
	buildCmd.Flags().StringVarP(&source.SSHPassword,
		"ssh-password", "p", "", "Optional SSH password to use for private repositories")
	buildCmd.Flags().StringVarP(&project.FromPkgName,
		"from", "", "", "Build starting from a defined package name")
	buildCmd.Flags().StringVarP(&project.ToPkgName,
		"to", "", "", "Build until a defined package name")
	buildCmd.Flags().BoolVarP(&project.Zap,
		"zap", "z", false, "Remove entire staging dir before building the package")
}
