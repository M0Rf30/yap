// Package command provides the CLI commands for the YAP package builder.
package command

import (
	"errors"
	"strings"

	"github.com/spf13/cobra"

	yapErrors "github.com/M0Rf30/yap/v2/pkg/errors"
	"github.com/M0Rf30/yap/v2/pkg/i18n"
	"github.com/M0Rf30/yap/v2/pkg/logger"
	"github.com/M0Rf30/yap/v2/pkg/parser"
	"github.com/M0Rf30/yap/v2/pkg/platform"
	"github.com/M0Rf30/yap/v2/pkg/project"
	"github.com/M0Rf30/yap/v2/pkg/shell"
	"github.com/M0Rf30/yap/v2/pkg/source"
)

// buildCmd represents the command to build the entire project.
var buildCmd = &cobra.Command{
	Use:     "build [distro] <path>",
	GroupID: "build",
	Aliases: []string{"b"},
	Short:   "🔨 Build packages from yap.json project definition", // Will be set in init()
	Long:    "",                                                  // Will be set in init()
	Example: "",                                                  // Will be set in init()
	Args:    cobra.RangeArgs(1, 2),                               // Allow 1-2 arguments
	PreRun:  PreRunValidation,
	RunE: func(_ *cobra.Command, args []string) error {
		// Set verbose flag from global flag
		project.Verbose = verbose
		shell.SetVerbose(verbose)

		// Enhanced user feedback with progress
		if verbose {
			logger.Debug(i18n.T("logger.build.starting_verbose"))
		}

		// Parse flexible arguments using shared function
		distro, release, fullJSONPath, err := ParseFlexibleArgs(args)
		if err != nil {
			return err
		}

		// Use the default distro if none is provided.
		if distro == "" {
			osRelease, _ := platform.ParseOSRelease()
			distro = osRelease.ID
			logger.Warn(i18n.T("logger.build.no_distribution_specified"),
				"distro", distro)
		} else {
			logger.Info(i18n.T("logger.build.building_for_distribution"),
				"distro", distro, "release", release)
		}

		// Show project path
		logger.Info(i18n.T("logger.build.project_path"), "path", fullJSONPath)

		// Initialize project with timestamp logging
		logger.Info(i18n.T("logger.build.initializing_project"))

		// Initialize MultipleProject
		mpc := project.MultipleProject{}
		err = mpc.MultiProject(distro, release, fullJSONPath)
		if err != nil {
			// Enhanced error logging with context
			var yapErr *yapErrors.YapError
			if errors.As(err, &yapErr) {
				logStructuredError(yapErr)
			} else {
				logger.Error(i18n.T("logger.build.project_init_failed"), "error", err)
				return err
			}
			return err
		}

		logger.Info(i18n.T("logger.build.project_init_success"))

		// Build packages with timestamp logging
		logger.Info(i18n.T("logger.build.building_packages"))

		err = mpc.BuildAll()
		if err != nil {
			// Enhanced error logging with context
			var yapErr *yapErrors.YapError
			if errors.As(err, &yapErr) {
				logStructuredError(yapErr)
			} else {
				logger.Error(i18n.T("logger.build.build_failed"), "error", err)
				return err
			}
			return err
		}

		logger.Info(i18n.T("logger.build.build_completed"))
		return nil
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

	logger.Fatal(i18n.T("logger.build.build_failed"), args...)
}

// InitializeBuildDescriptions sets the localized descriptions for the build command.
// This must be called after i18n is initialized.
func InitializeBuildDescriptions() {
	buildCmd.Short = i18n.T("commands.build.short")
	buildCmd.Long = i18n.T("commands.build.long")
	buildCmd.Example = i18n.T("commands.build.examples")

	// Update flag descriptions with localized text
	buildCmd.Flag("cleanbuild").Usage = i18n.T("flags.build.cleanbuild")
	buildCmd.Flag("nobuild").Usage = i18n.T("flags.build.nobuild")
	buildCmd.Flag("zap").Usage = i18n.T("flags.build.zap")
	buildCmd.Flag("nomakedeps").Usage = i18n.T("flags.build.nomakedeps")
	buildCmd.Flag("skip-sync").Usage = i18n.T("flags.build.skip_sync")
	buildCmd.Flag("pkgver").Usage = i18n.T("flags.build.pkgver")
	buildCmd.Flag("pkgrel").Usage = i18n.T("flags.build.pkgrel")
	buildCmd.Flag("ssh-password").Usage = i18n.T("flags.build.ssh_password")
	buildCmd.Flag("from").Usage = i18n.T("flags.build.from")
	buildCmd.Flag("to").Usage = i18n.T("flags.build.to")
}

//nolint:gochecknoinits // Required for cobra command registration
func init() {
	// Command descriptions will be set later via InitializeLocalizedDescriptions()
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
		"cleanbuild", "c", false, "")
	buildCmd.Flags().BoolVarP(&project.NoBuild,
		"nobuild", "o", false, "")
	buildCmd.Flags().BoolVarP(&project.Zap,
		"zap", "z", false, "")

	// DEPENDENCY MANAGEMENT FLAGS
	buildCmd.Flags().BoolVarP(&project.NoMakeDeps,
		"nomakedeps", "d", false, "")
	buildCmd.Flags().BoolVarP(&project.SkipSyncDeps,
		"skip-sync", "s", false, "")

	// VERSION CONTROL FLAGS
	buildCmd.PersistentFlags().StringVarP(&parser.OverridePkgVer,
		"pkgver", "w", "", "")
	buildCmd.PersistentFlags().StringVarP(&parser.OverridePkgRel,
		"pkgrel", "r", "", "")

	// SOURCE ACCESS FLAGS
	buildCmd.Flags().StringVarP(&source.SSHPassword,
		"ssh-password", "p", "", "")

	// BUILD RANGE CONTROL FLAGS
	buildCmd.Flags().StringVarP(&project.FromPkgName,
		"from", "", "", "")
	buildCmd.Flags().StringVarP(&project.ToPkgName,
		"to", "", "", "")
}
