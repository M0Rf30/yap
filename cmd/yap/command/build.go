// Package command implements the YAP CLI commands including build, install, graph, and utility operations.
package command

import (
	"errors"
	"strings"

	"github.com/spf13/cobra"

	"github.com/M0Rf30/yap/v2/pkg/builders/common"
	yapErrors "github.com/M0Rf30/yap/v2/pkg/errors"
	"github.com/M0Rf30/yap/v2/pkg/i18n"
	"github.com/M0Rf30/yap/v2/pkg/logger"
	"github.com/M0Rf30/yap/v2/pkg/parser"
	"github.com/M0Rf30/yap/v2/pkg/platform"
	"github.com/M0Rf30/yap/v2/pkg/project"
	"github.com/M0Rf30/yap/v2/pkg/shell"
	"github.com/M0Rf30/yap/v2/pkg/source"
)

// sshPassword is the local holder for the --ssh-password flag value.
var sshPassword string

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
		// Propagate ssh-password flag value to the source package.
		source.SetSSHPassword(sshPassword)

		// Set the skip toolchain validation flag
		common.SkipToolchainValidation = project.SkipToolchainValidation

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
			// Also auto-detect codename when not already specified so that
			// distro+codename PKGBUILD directives (e.g. depends__ubuntu_jammy) are
			// correctly resolved for dependency ordering.
			if release == "" {
				release = osRelease.Codename
			}

			logger.Warn(i18n.T("logger.build.no_distribution_specified"), "distro", distro)
		} else {
			// If the user specified a distro but no codename, auto-detect the
			// codename from /etc/os-release so that distro+codename qualifiers
			// in PKGBUILD files (e.g. depends__ubuntu_jammy) are applied for
			// correct parallel dependency ordering.
			if release == "" {
				osRelease, err := platform.ParseOSRelease()
				if err == nil && osRelease.ID == distro && osRelease.Codename != "" {
					release = osRelease.Codename
					logger.Debug(i18n.T("logger.build.auto_detected_codename"),
						"distro", distro, "codename", release)
				}
			}

			logArgs := []any{"distro", distro}
			if release != "" {
				logArgs = append(logArgs, "release", release)
			}

			logArgs = append(logArgs, "path", fullJSONPath)
			logger.Info(i18n.T("logger.build.building_for_distribution"), logArgs...)
		}

		// Initialize MultipleProject
		mpc := project.MultipleProject{}

		err = mpc.MultiProject(distro, release, fullJSONPath)
		if err != nil {
			var yapErr *yapErrors.YapError
			if errors.As(err, &yapErr) {
				logStructuredError(yapErr)
			} else {
				logger.Fatal(i18n.T("logger.build.project_init_failed"), "error", err)
			}

			return err
		}

		logger.Info(i18n.T("logger.build.project_init_success"))

		// Build packages with timestamp logging
		logger.Info(i18n.T("logger.build.building_packages"))

		err = mpc.BuildAll()
		if err != nil {
			var yapErr *yapErrors.YapError
			if errors.As(err, &yapErr) {
				logStructuredError(yapErr)
			} else {
				logger.Fatal(i18n.T("logger.build.build_failed"), "error", err)
			}

			return err
		}

		logger.Info(i18n.T("logger.build.build_completed"))

		return nil
	},
}

// logStructuredError logs a concise fatal build error.
func logStructuredError(yapErr *yapErrors.YapError) {
	pkg, _ := yapErr.Context["package"].(string)
	ver, _ := yapErr.Context["version"].(string)
	rel, _ := yapErr.Context["release"].(string)
	stage, _ := yapErr.Context["stage"].(string)

	parts := []string{i18n.T("logger.build.build_failed")}

	if pkg != "" {
		coord := pkg
		if ver != "" {
			coord += " " + ver
			if rel != "" {
				coord += "-" + rel
			}
		}

		parts = append(parts, coord)
	}

	if stage != "" {
		parts = append(parts, "(stage: "+stage+")")
	} else if yapErr.Operation != "" {
		parts = append(parts, "("+yapErr.Operation+")")
	}

	msg := strings.Join(parts, ": ")

	if yapErr.Cause != nil {
		msg += " — " + yapErr.Cause.Error()
	}

	logger.Fatal(msg)
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
	buildCmd.Flag("skip-toolchain-validation").Usage = i18n.T("flags.build.skip_toolchain_validation")
	buildCmd.Flag("parallel").Usage = i18n.T("flags.build.parallel")
	buildCmd.Flag("pkgver").Usage = i18n.T("flags.build.pkgver")
	buildCmd.Flag("pkgrel").Usage = i18n.T("flags.build.pkgrel")
	buildCmd.Flag("ssh-password").Usage = i18n.T("flags.build.ssh_password")
	buildCmd.Flag("from").Usage = i18n.T("flags.build.from")
	buildCmd.Flag("to").Usage = i18n.T("flags.build.to")
	buildCmd.Flag("target-arch").Usage = i18n.T("flags.build.target_arch")
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
	buildCmd.Flags().BoolVarP(&project.SkipToolchainValidation,
		"skip-toolchain-validation", "", false, "")
	buildCmd.Flags().BoolVarP(&project.Parallel,
		"parallel", "P", false, "")

	// VERSION CONTROL FLAGS
	buildCmd.PersistentFlags().StringVarP(&parser.OverridePkgVer,
		"pkgver", "w", "", "")
	buildCmd.PersistentFlags().StringVarP(&parser.OverridePkgRel,
		"pkgrel", "r", "", "")

	// SOURCE ACCESS FLAGS
	buildCmd.Flags().StringVarP(&sshPassword,
		"ssh-password", "p", "", "")

	// BUILD RANGE CONTROL FLAGS
	buildCmd.Flags().StringVarP(&project.FromPkgName,
		"from", "", "", "")
	buildCmd.Flags().StringVarP(&project.ToPkgName,
		"to", "", "", "")

	// CROSS-COMPILATION FLAGS
	buildCmd.Flags().StringVarP(&project.TargetArch,
		"target-arch", "t", "", "Target architecture for cross-compilation (e.g., arm64, armv7, x86_64)")
}
