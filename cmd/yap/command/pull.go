package command

import (
	"strings"

	"github.com/spf13/cobra"

	"github.com/M0Rf30/yap/v2/pkg/osutils"
)

const (
	alpineDistro = "alpine"
	archDistro   = "arch"
)

// pullCmd represents the pull command.
var pullCmd = &cobra.Command{
	Use:     "pull <distro>",
	GroupID: "environment",
	Aliases: []string{"download"},
	Short:   "📦 Pull pre-built container images for building",
	Long: `Download container images used for isolated package building environments.

YAP uses containerization to provide clean, reproducible build environments
for each target distribution. This command pre-downloads the required
container images to speed up subsequent builds.

DISTRIBUTION REQUIREMENTS:
  • Alpine and Arch: Use base names (alpine, arch)
  • Others: Require release codename (ubuntu-jammy, rocky-9, fedora-38)

Images are pulled from official registries and stored locally.
Re-running this command will update to the latest image versions.`,
	Example: `  # Pull images for distributions with default releases
  yap pull alpine
  yap pull arch

  # Pull images for specific distribution releases
  yap pull ubuntu-jammy
  yap pull fedora-38
  yap pull debian-bookworm
  yap pull rocky-9`,
	Args:   createValidateDistroArgs(1),
	PreRun: PreRunValidation,
	Run: func(_ *cobra.Command, args []string) {
		split := strings.Split(args[0], "-")

		if len(split) == 1 && split[0] != alpineDistro && split[0] != archDistro {
			osutils.Logger.Fatal("except for alpine and arch, specify also the codename " +
				"(i. e. rocky-9, ubuntu-jammy)")
		}

		err := osutils.PullContainers(args[0])
		if err != nil {
			osutils.Logger.Fatal("failed to pull image",
				osutils.Logger.Args("error", err))
		}
	},
}

//nolint:gochecknoinits // Required for cobra command registration
func init() {
	rootCmd.AddCommand(pullCmd)

	// Add completion for distribution argument
	pullCmd.ValidArgsFunction = ValidDistrosCompletion
}
