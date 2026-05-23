package command

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/spf13/cobra"

	"github.com/M0Rf30/yap/v2/pkg/builders/common"
	"github.com/M0Rf30/yap/v2/pkg/i18n"
	"github.com/M0Rf30/yap/v2/pkg/logger"
	"github.com/M0Rf30/yap/v2/pkg/packer"
	"github.com/M0Rf30/yap/v2/pkg/pkgbuild"
	"github.com/M0Rf30/yap/v2/pkg/repo"
)

var (
	// GoLang indicates whether to prepare Go language environment.
	GoLang bool

	// TargetArch specifies the target architecture for cross-compilation.
	TargetArch string

	// prepareSkipSyncDeps is the local holder for --skip-sync in the prepare command.
	prepareSkipSyncDeps bool

	// prepareSkipToolchainValidation is the local holder for --skip-toolchain-validation in prepare.
	prepareSkipToolchainValidation bool

	// prepareExtraRepos is the local holder for --repo flags in the prepare command.
	prepareExtraRepos []string

	// prepareCmd represents the prepare command.
	prepareCmd = &cobra.Command{
		Use:     prepareCommand + " [distro]",
		GroupID: commandEnvironment,
		Aliases: []string{aliasPrep, "setup"},
		Short:   "🛠️  Prepare build environment with development packages", // Will be set in init()
		Long:    "",                                                        // Will be set in init()
		Example: "",                                                        // Will be set in init()
		Args:    cobra.RangeArgs(0, 1),
		PreRun:  PreRunValidation,
		Run: func(_ *cobra.Command, args []string) {
			// Set the skip toolchain validation flag
			common.SkipToolchainValidation = prepareSkipToolchainValidation

			// Parse optional distro arg (supports "distro" or "distro-release"),
			// then auto-detect from /etc/os-release when missing — matching
			// the behavior of the `build` command.
			var distro, release string
			if len(args) > 0 {
				distro, release = parseDistroAndRelease(args[0])
			}

			distro, _ = ResolveDistroRelease(distro, release,
				"logger.prepare.no_distribution_specified")

			// Dispatch to container when a distro was explicitly requested and
			// we are not already inside a container. Use --no-container to skip.
			if len(args) > 0 && !noContainer {
				distroTag := args[0]
				// YAP_IN_CONTAINER=1 (injected by the runtime) prevents re-dispatch.
				subArgs := []string{prepareCommand, distroTag}
				if RunCommandInContainer(distroTag, ".", subArgs) {
					return
				}
			}

			packageManager, err := packer.GetPackageManager(&pkgbuild.PKGBUILD{}, distro, "", "")
			if err != nil {
				logger.Error(err.Error(), "error", err)

				return
			}

			cliRepos, err := repo.ParseFlags(prepareExtraRepos)
			if err != nil {
				logger.Error(err.Error(), "error", err)

				return
			}

			if err := repo.Setup(distro, cliRepos); err != nil {
				logger.Error(err.Error(), "error", err)

				return
			}

			ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
			defer cancel()

			if !prepareSkipSyncDeps {
				err := packageManager.Update(ctx)
				if err != nil {
					logger.Error(err.Error(),
						"error", err)
				}
			}

			err = packageManager.PrepareEnvironment(ctx, GoLang, TargetArch)
			if err != nil {
				logger.Error(err.Error())

				return
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
	initCommandDescriptions(prepareCmd, "prepare", map[string]string{
		flagSkipSync:                "flags.prepare.skip_sync",
		"skip-toolchain-validation": "flags.prepare.skip_toolchain_validation",
		"golang":                    "flags.prepare.golang",
		"target-arch":               "flags.prepare.target_arch",
	})
}

//nolint:gochecknoinits // Required for cobra command registration
func init() {
	rootCmd.AddCommand(prepareCmd)

	// Add completion for distribution argument
	prepareCmd.ValidArgsFunction = ValidDistrosCompletion

	prepareCmd.Flags().BoolVarP(&prepareSkipSyncDeps,
		flagSkipSync, "s", false, "")
	prepareCmd.Flags().BoolVarP(&prepareSkipToolchainValidation,
		"skip-toolchain-validation", "", false, "")
	prepareCmd.Flags().BoolVarP(&GoLang,
		"golang", "g", false, "")
	prepareCmd.Flags().StringVarP(&TargetArch,
		"target-arch", "t", "", "Target architecture for cross-compilation (e.g., arm64, armv7, x86_64)")
	// StringArrayVar (not StringSliceVar) so commas inside the spec are not
	// treated as multi-value separators — every --repo invocation maps to one
	// repository definition.
	prepareCmd.Flags().StringArrayVar(&prepareExtraRepos,
		"repo", nil,
		"Extra repository spec (repeatable): name=<n>,url=<u>,suite=<s>,components=<a+b>,"+
			"keyURL=<u>,distros=<d1+d2>,format=<deb|rpm>,gpgCheck=<true|false>")

	// CONTAINER FLAGS
	prepareCmd.Flags().BoolVar(&noContainer,
		"no-container", false,
		"skip container dispatch and prepare natively on the host")
}
