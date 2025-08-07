package command

import (
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/M0Rf30/yap/v2/pkg/osutils"
	"github.com/M0Rf30/yap/v2/pkg/project"
)

// zapCmd represents the command to deeply clean build environments.
var zapCmd = &cobra.Command{
	Use:     "zap <distro> <path>",
	GroupID: "build",
	Aliases: []string{"clean"},
	Short:   "🧹 Deeply clean build environment and artifacts",
	Long: `Perform deep cleaning of build environments, removing all build artifacts,
temporary files, and staging directories for the specified project.

This command is useful for:
  • Freeing disk space occupied by build artifacts
  • Resolving build issues caused by stale files
  • Ensuring completely clean builds
  • Maintenance of build environments

WARNING: This operation is destructive and removes all build outputs
and intermediate files. Use with caution in production environments.

The zap command automatically:
  • Skips dependency installation (nomakedeps=true)
  • Skips package manager sync (skip-sync=true)
  • Enables deep cleaning mode (zap=true)`,
	Example: `  # Clean specific project for distribution
  yap zap ubuntu-jammy /path/to/project
  yap zap fedora-38 .
  yap zap alpine /home/user/myproject

  # Clean current directory project
  yap zap rocky-9 .`,
	Args:   createValidateDistroArgs(2),
	PreRun: PreRunValidation,
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

		osutils.Logger.Info("zap done.", osutils.Logger.Args("distro", distro, "release", release))
	},
}

//nolint:gochecknoinits // Required for cobra command registration
func init() {
	rootCmd.AddCommand(zapCmd)

	// Add completion for command arguments
	zapCmd.ValidArgsFunction = func(
		cmd *cobra.Command,
		args []string,
		toComplete string,
	) ([]string, cobra.ShellCompDirective) {
		switch len(args) {
		case 0:
			// First arg: distribution
			return ValidDistrosCompletion(cmd, args, toComplete)
		case 1:
			// Second arg: path
			return nil, cobra.ShellCompDirectiveFilterDirs
		default:
			return nil, cobra.ShellCompDirectiveNoFileComp
		}
	}
}
