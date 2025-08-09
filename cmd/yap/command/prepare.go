package command

import (
	"strings"

	"github.com/spf13/cobra"

	"github.com/M0Rf30/yap/v2/pkg/osutils"
	"github.com/M0Rf30/yap/v2/pkg/packer"
	"github.com/M0Rf30/yap/v2/pkg/pkgbuild"
	"github.com/M0Rf30/yap/v2/pkg/project"
)

var (
	// GoLang indicates whether to prepare Go language environment.
	GoLang bool

	// prepareCmd represents the prepare command.
	prepareCmd = &cobra.Command{
		Use:     "prepare <distro>",
		GroupID: "environment",
		Aliases: []string{"prep", "setup"},
		Short:   "🛠️  Prepare build environment with development packages",
		Long: `Install essential development packages required for building on the target distribution.

This command sets up a complete build environment including:
  • Base development tools (gcc, make, etc.)
  • Package manager development headers
  • Build system utilities
  • Optionally Go language runtime and toolchain

DISTRIBUTION SUPPORT:
  Automatically detects and installs the appropriate packages for:
  Alpine, Arch, CentOS, Debian, Fedora, OpenSUSE, Rocky, Ubuntu

The prepare command ensures you have all necessary tools before attempting
to build packages, reducing build failures due to missing dependencies.`,
		Example: `  # Prepare basic build environment
  yap prepare ubuntu-jammy
  yap prepare fedora-38
  yap prepare alpine

  # Prepare with Go language support
  yap prepare --golang arch
  yap prepare -g debian-bookworm

  # Skip package manager sync (faster, but may miss updates)
  yap prepare --skip-sync rocky-9`,
		Args:   createValidateDistroArgs(1),
		PreRun: PreRunValidation,
		Run: func(_ *cobra.Command, args []string) {
			split := strings.Split(args[0], "-")
			distro := split[0]

			packageManager := packer.GetPackageManager(&pkgbuild.PKGBUILD{}, distro)
			if !project.SkipSyncDeps {
				err := packageManager.Update()
				if err != nil {
					osutils.Logger.Error(err.Error(),
						osutils.Logger.Args("error", err))
				}
			}

			err := packageManager.PrepareEnvironment(GoLang)
			if err != nil {
				osutils.Logger.Error(err.Error())
			}

			osutils.Logger.Info("basic build environment successfully prepared")

			if GoLang {
				osutils.Logger.Info("go successfully installed")
			}
		},
	}
)

//nolint:gochecknoinits // Required for cobra command registration
func init() {
	rootCmd.AddCommand(prepareCmd)

	// Add completion for distribution argument
	prepareCmd.ValidArgsFunction = ValidDistrosCompletion

	prepareCmd.Flags().BoolVarP(&project.SkipSyncDeps,
		"skip-sync", "s", false, "⚡ skip package manager synchronization (faster but may miss updates)")
	prepareCmd.Flags().BoolVarP(&GoLang,
		"golang", "g", false, "🐹 additionally install Go language runtime and development tools")
}
