package command

import (
	"strings"

	"github.com/spf13/cobra"

	"github.com/M0Rf30/yap/v2/pkg/logger"
	"github.com/M0Rf30/yap/v2/pkg/platform"
	"github.com/M0Rf30/yap/v2/pkg/project"
)

// zapCmd represents the command to deeply clean build environments.
var zapCmd = &cobra.Command{
	Use:     "zap [distro] <path>",
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

DISTRIBUTION FORMAT:
  Use 'distro' or 'distro-release' format (e.g., 'ubuntu-jammy', 'fedora-38')
  If no distro is specified, uses the current system's distribution.

The zap command automatically:
  • Skips dependency installation (nomakedeps=true)
  • Skips package manager sync (skip-sync=true)
  • Enables deep cleaning mode (zap=true)`,
	Example: `  # Clean current system distribution
  yap zap .
  yap zap /path/to/project

  # Clean specific project for distribution
  yap zap ubuntu-jammy /path/to/project
  yap zap fedora-38 .
  yap zap alpine /home/user/myproject

  # Clean current directory project
  yap zap rocky-9 .`,
	Args:   cobra.RangeArgs(1, 2), // Allow 1 or 2 arguments like the build command
	PreRun: PreRunValidation,
	RunE: func(_ *cobra.Command, args []string) error {
		// Parse flexible arguments using shared function
		distro, release, fullJSONPath, err := ParseFlexibleArgs(args)
		if err != nil {
			return err
		}

		// Use the default distro if none is provided.
		if distro == "" {
			osRelease, _ := platform.ParseOSRelease()
			distro = osRelease.ID
			logger.Warn("No distribution specified, using detected",
				"distro", distro)
		} else {
			logger.Info("Cleaning for distribution",
				"distro", distro, "release", release)
		}

		// Show project path
		logger.Info("Project path", "path", fullJSONPath)

		mpc := project.MultipleProject{}

		project.NoMakeDeps = true
		project.SkipSyncDeps = true
		project.Zap = true

		err = mpc.MultiProject(distro, release, fullJSONPath)
		if err != nil {
			logger.Fatal("fatal error",
				"error", err)
		}

		err = mpc.Clean()
		if err != nil {
			logger.Fatal("fatal error",
				"error", err)
		}

		logger.Info("zap done.", "distro", distro, "release", release)
		return nil
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
			// First arg: distribution or path
			if strings.Contains(toComplete, "/") || toComplete == "." {
				return nil, cobra.ShellCompDirectiveFilterDirs
			}

			return ValidDistrosCompletion(cmd, args, toComplete)
		case 1:
			// Second arg: path (if first was distro)
			return nil, cobra.ShellCompDirectiveFilterDirs
		default:
			return nil, cobra.ShellCompDirectiveNoFileComp
		}
	}

	// BUILD RANGE CONTROL FLAGS - same as build command for target support
	zapCmd.Flags().StringVarP(&project.FromPkgName,
		"from", "", "", "▶️  start cleaning from specified package name (dependency-aware)")
	zapCmd.Flags().StringVarP(&project.ToPkgName,
		"to", "", "", "⏹️  stop cleaning at specified package name (dependency-aware)")
}
