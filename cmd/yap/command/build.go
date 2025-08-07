// Package command provides CLI commands for the yap package management tool.
package command

import (
	"errors"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	yapErrors "github.com/M0Rf30/yap/v2/pkg/errors"
	"github.com/M0Rf30/yap/v2/pkg/osutils"
	"github.com/M0Rf30/yap/v2/pkg/parser"
	"github.com/M0Rf30/yap/v2/pkg/project"
	"github.com/M0Rf30/yap/v2/pkg/source"
)

// buildCmd represents the command to build the entire project.
var buildCmd = &cobra.Command{
	Use:     "build [distro] <path>",
	GroupID: "build",
	Aliases: []string{"b"},
	Short:   "🔨 Build packages from yap.json project definition",
	Long: `Build packages for one or more distributions using a yap.json project file.

The build command orchestrates the entire package building process:
  • Parses yap.json project configuration
  • Resolves build dependencies and order
  • Creates isolated container environments
  • Builds packages according to PKGBUILD specifications
  • Handles cross-distribution compatibility

DISTRIBUTION FORMAT:
  Use 'distro' or 'distro-release' format (e.g., 'ubuntu-jammy', 'fedora-38')
  If no distro is specified, uses the current system's distribution.

DEPENDENCY RESOLUTION:
  Build order is automatically determined from package dependencies.
  Use --from and --to flags to build specific package ranges.`,
	Example: `  # Build for current system distribution
  yap build .
  yap build /path/to/project

  # Build for specific distributions
  yap build ubuntu-jammy .
  yap build fedora-38 /path/to/project
  yap build alpine /home/user/myproject

  # Build with specific options
  yap build --cleanbuild --nomakedeps ubuntu-jammy .
  yap build --from package1 --to package5 fedora-38 .

  # Build with custom version override
  yap build --pkgver 1.2.3 --pkgrel 2 ubuntu-jammy .`,
	Args:   createValidateDistroArgs(1),
	PreRun: PreRunValidation,
	Run: func(_ *cobra.Command, args []string) {
		// Set verbose flag from global flag
		project.Verbose = verbose
		osutils.SetVerbose(verbose)

		// Enhanced user feedback with progress
		if verbose {
			osutils.Logger.Info("verbose mode enabled", osutils.Logger.Args("command", "build"))
			osutils.Logger.Info("Starting build process with verbose logging enabled")
		}

		fullJSONPath, _ := filepath.Abs(args[len(args)-1]) // Always take the last argument as path
		var distro, release string

		if len(args) == 2 {
			split := strings.Split(args[0], "-")
			distro = split[0]

			if len(split) > 1 {
				release = split[1]
			}
		}

		// Use the default distro if none is provided.
		if distro == "" {
			osRelease, _ := osutils.ParseOSRelease()
			distro = osRelease.ID
			osutils.Logger.Warn("No distribution specified, using detected",
				osutils.Logger.Args("distro", distro))
		} else {
			osutils.Logger.Info("Building for distribution",
				osutils.Logger.Args("distro", distro, "release", release))
		}

		// Show project path
		osutils.Logger.Info("Project path", osutils.Logger.Args("path", fullJSONPath))

		// Initialize project with timestamp logging
		osutils.Logger.Info("Initializing project...")

		// Initialize MultipleProject
		mpc := project.MultipleProject{}
		err := mpc.MultiProject(distro, release, fullJSONPath)
		if err != nil {
			// Enhanced error logging with context
			var yapErr *yapErrors.YapError
			if errors.As(err, &yapErr) {
				logStructuredError(yapErr)
			} else {
				osutils.Logger.Error("Project initialization failed", osutils.Logger.Args("error", err))
				osutils.Logger.Fatal("fatal error",
					osutils.Logger.Args("error", err))
			}
			os.Exit(1)
		}

		osutils.Logger.Info("Project initialized successfully")

		// Build packages with timestamp logging
		osutils.Logger.Info("Building packages...")

		err = mpc.BuildAll()
		if err != nil {
			// Enhanced error logging with context
			var yapErr *yapErrors.YapError
			if errors.As(err, &yapErr) {
				logStructuredError(yapErr)
			} else {
				osutils.Logger.Error("Build failed", osutils.Logger.Args("error", err))
				osutils.Logger.Fatal("fatal error",
					osutils.Logger.Args("error", err))
			}
			os.Exit(1)
		}

		osutils.Logger.Info("All packages built successfully")
	},
}

// logStructuredError logs structured YapError with detailed context.
func logStructuredError(yapErr *yapErrors.YapError) {
	args := []any{"error_type", yapErr.Type, "message", yapErr.Message}

	if yapErr.Operation != "" {
		args = append(args, "operation", yapErr.Operation)
	}

	if yapErr.Context != nil {
		for key, value := range yapErr.Context {
			args = append(args, key, value)
		}
	}

	if yapErr.Cause != nil {
		args = append(args, "underlying_error", yapErr.Cause.Error())
	}

	osutils.Logger.Fatal("build failed", osutils.Logger.Args(args...))
}

//nolint:gochecknoinits // Required for cobra command registration
func init() {
	rootCmd.AddCommand(buildCmd)

	// Add completion for command arguments
	buildCmd.ValidArgsFunction = func(
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

	// BUILD BEHAVIOR FLAGS
	buildCmd.Flags().BoolVarP(&project.CleanBuild,
		"cleanbuild", "c", false, "🧹 remove $srcdir/ directory before building (ensures clean build)")
	buildCmd.Flags().BoolVarP(&project.NoBuild,
		"nobuild", "o", false, "📥 download and extract source files only (no compilation)")
	buildCmd.Flags().BoolVarP(&project.Zap,
		"zap", "z", false, "💥 remove entire staging directory before building (deep clean)")

	// DEPENDENCY MANAGEMENT FLAGS
	buildCmd.Flags().BoolVarP(&project.NoMakeDeps,
		"nomakedeps", "d", false, "⏭️  skip all make dependency (makedeps) installation and checks")
	buildCmd.Flags().BoolVarP(&project.SkipSyncDeps,
		"skip-sync", "s", false, "🚫 skip package manager synchronization with remotes")

	// VERSION CONTROL FLAGS
	buildCmd.PersistentFlags().StringVarP(&parser.OverridePkgVer,
		"pkgver", "w", "", "🏷️  override package version (pkgver) for all packages in project")
	buildCmd.PersistentFlags().StringVarP(&parser.OverridePkgRel,
		"pkgrel", "r", "", "🔢 override package release number (pkgrel) for all packages")

	// SOURCE ACCESS FLAGS
	buildCmd.Flags().StringVarP(&source.SSHPassword,
		"ssh-password", "p", "", "🔐 SSH password for accessing private repositories")

	// BUILD RANGE CONTROL FLAGS
	buildCmd.Flags().StringVarP(&project.FromPkgName,
		"from", "", "", "▶️  start building from specified package name (dependency-aware)")
	buildCmd.Flags().StringVarP(&project.ToPkgName,
		"to", "", "", "⏹️  stop building at specified package name (dependency-aware)")
}
