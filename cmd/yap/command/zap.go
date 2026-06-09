package command

import (
	"strings"

	"github.com/spf13/cobra"

	"github.com/M0Rf30/yap/v2/pkg/i18n"
	"github.com/M0Rf30/yap/v2/pkg/logger"
	"github.com/M0Rf30/yap/v2/pkg/project"
)

// zapFromPkgName is the local holder for the --from flag value in the zap command.
var zapFromPkgName string

// zapToPkgName is the local holder for the --to flag value in the zap command.
var zapToPkgName string

// zapCmd represents the command to deeply clean build environments.
var zapCmd = &cobra.Command{
	Use:     commandZap + " [distro] <path>",
	GroupID: buildGroup,
	Aliases: []string{"clean"},
	Short:   "🧹 Deeply clean build environment and artifacts", // Will be set in init()
	Long:    "",                                               // Will be set in init()
	Example: "",                                               // Will be set in init()
	Args:    cobra.RangeArgs(1, 2),                            // Allow 1-2 arguments
	PreRun:  PreRunValidation,
	RunE: func(_ *cobra.Command, args []string) error {
		// Parse flexible arguments and auto-detect distro/codename from
		// /etc/os-release when missing.
		distro, release, fullJSONPath, userProvidedDistro, err := ResolveFlexibleDistro(args,
			"logger.zap.no_distribution_specified")
		if err != nil {
			return err
		}

		if userProvidedDistro {
			logger.Info(i18n.T("logger.zap.cleaning_for_distribution"),
				"distro", distro, "release", release)
		}

		// Show project path
		logger.Info(i18n.T("logger.zap.project_path"), "path", fullJSONPath)

		mpc := project.MultipleProject{
			Opts: project.BuildOptions{
				NoMakeDeps:   true,
				SkipSyncDeps: true,
				Zap:          true,
				FromPkgName:  zapFromPkgName,
				ToPkgName:    zapToPkgName,
			},
		}

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
	initCommandDescriptions(zapCmd, "zap", map[string]string{
		flagFrom: "flags.zap.from",
		"to":     "flags.zap.to",
	})
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
	zapCmd.Flags().StringVarP(&zapFromPkgName,
		flagFrom, "", "", "")
	zapCmd.Flags().StringVarP(&zapToPkgName,
		"to", "", "", "")
}
