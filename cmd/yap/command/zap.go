package command

import (
	"strings"

	"github.com/spf13/cobra"

	"github.com/M0Rf30/yap/v2/pkg/i18n"
	"github.com/M0Rf30/yap/v2/pkg/logger"
	"github.com/M0Rf30/yap/v2/pkg/platform"
	"github.com/M0Rf30/yap/v2/pkg/project"
)

// zapCmd represents the command to deeply clean build environments.
var zapCmd = &cobra.Command{
	Use:     "zap [distro] <path>",
	GroupID: "build",
	Aliases: []string{"clean"},
	Short:   "🧹 Deeply clean build environment and artifacts", // Will be set in init()
	Long:    "",                                               // Will be set in init()
	Example: "",                                               // Will be set in init()
	Args:    cobra.RangeArgs(1, 2),                            // Allow 1-2 arguments
	PreRun:  PreRunValidation,
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
			logger.Warn(i18n.T("logger.zap.no_distribution_specified"),
				"distro", distro)
		} else {
			logger.Info(i18n.T("logger.zap.cleaning_for_distribution"),
				"distro", distro, "release", release)
		}

		// Show project path
		logger.Info(i18n.T("logger.zap.project_path"), "path", fullJSONPath)

		mpc := project.MultipleProject{}

		project.NoMakeDeps = true
		project.SkipSyncDeps = true
		project.Zap = true

		err = mpc.MultiProject(distro, release, fullJSONPath)
		if err != nil {
			logger.Fatal(i18n.T("logger.zap.fatal_error"),
				"error", err)
		}

		err = mpc.Clean()
		if err != nil {
			logger.Fatal(i18n.T("logger.zap.fatal_error"),
				"error", err)
		}

		logger.Info(i18n.T("logger.zap.done"), "distro", distro, "release", release)
		return nil
	},
}

// InitializeZapDescriptions sets the localized descriptions for the zap command.
// This must be called after i18n is initialized.
func InitializeZapDescriptions() {
	zapCmd.Short = i18n.T("commands.zap.short")
	zapCmd.Long = i18n.T("commands.zap.long")
	zapCmd.Example = i18n.T("commands.zap.examples")

	// Update flag descriptions with localized text
	zapCmd.Flag("from").Usage = i18n.T("flags.zap.from")
	zapCmd.Flag("to").Usage = i18n.T("flags.zap.to")
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
		"from", "", "", "")
	zapCmd.Flags().StringVarP(&project.ToPkgName,
		"to", "", "", "")
}
