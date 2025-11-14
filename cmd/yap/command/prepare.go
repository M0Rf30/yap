package command

import (
	"strings"

	"github.com/spf13/cobra"

	"github.com/M0Rf30/yap/v2/pkg/i18n"
	"github.com/M0Rf30/yap/v2/pkg/logger"
	"github.com/M0Rf30/yap/v2/pkg/packer"
	"github.com/M0Rf30/yap/v2/pkg/pkgbuild"
	"github.com/M0Rf30/yap/v2/pkg/project"
)

var (
	// GoLang indicates whether to prepare Go language environment.
	GoLang bool

	// TargetArch specifies the target architecture for cross-compilation.
	TargetArch string

	// prepareCmd represents the prepare command.
	prepareCmd = &cobra.Command{
		Use:     "prepare <distro>",
		GroupID: "environment",
		Aliases: []string{"prep", "setup"},
		Short:   "🛠️  Prepare build environment with development packages", // Will be set in init()
		Long:    "",                                                        // Will be set in init()
		Example: "",                                                        // Will be set in init()
		Args:    createValidateDistroArgs(1),
		PreRun:  PreRunValidation,
		Run: func(_ *cobra.Command, args []string) {
			split := strings.Split(args[0], "-")
			distro := split[0]

			packageManager := packer.GetPackageManager(&pkgbuild.PKGBUILD{}, distro)
			if !project.SkipSyncDeps {
				err := packageManager.Update()
				if err != nil {
					logger.Error(err.Error(),
						"error", err)
				}
			}

			err := packageManager.PrepareEnvironment(GoLang, TargetArch)
			if err != nil {
				logger.Error(err.Error())
			}

			logger.Info(i18n.T("logger.unknown.info.basic_build_environment_successfully_1"))

			if GoLang {
				logger.Info(i18n.T("logger.unknown.info.go_successfully_installed_1"))
			}
		},
	}
)

// InitializePrepareDescriptions sets the localized descriptions for the prepare command.
// This must be called after i18n is initialized.
func InitializePrepareDescriptions() {
	prepareCmd.Short = i18n.T("commands.prepare.short")
	prepareCmd.Long = i18n.T("commands.prepare.long")
	prepareCmd.Example = i18n.T("commands.prepare.examples")

	// Update flag descriptions with localized text
	prepareCmd.Flag("skip-sync").Usage = i18n.T("flags.prepare.skip_sync")
	prepareCmd.Flag("golang").Usage = i18n.T("flags.prepare.golang")
	prepareCmd.Flag("target-arch").Usage = i18n.T("flags.prepare.target_arch")
}

//nolint:gochecknoinits // Required for cobra command registration
func init() {
	rootCmd.AddCommand(prepareCmd)

	// Add completion for distribution argument
	prepareCmd.ValidArgsFunction = ValidDistrosCompletion

	prepareCmd.Flags().BoolVarP(&project.SkipSyncDeps,
		"skip-sync", "s", false, "")
	prepareCmd.Flags().BoolVarP(&GoLang,
		"golang", "g", false, "")
	prepareCmd.Flags().StringVarP(&TargetArch,
		"target-arch", "t", "", "Target architecture for cross-compilation (e.g., arm64, armv7, x86_64)")
}
