package command

import (
	"strings"

	"github.com/spf13/cobra"

	"github.com/M0Rf30/yap/v2/pkg/container"
	"github.com/M0Rf30/yap/v2/pkg/i18n"
	"github.com/M0Rf30/yap/v2/pkg/logger"
)

const (
	alpineDistro = "alpine"
	archDistro   = "arch"
)

// pullCmd represents the pull command.
var pullCmd = &cobra.Command{
	Use:     commandPull + " <distro>",
	GroupID: commandEnvironment,
	Aliases: []string{"download"},
	Short:   "", // Set by InitializeLocalizedDescriptions
	Long:    "", // Set by InitializeLocalizedDescriptions
	Example: "", // Set by InitializeLocalizedDescriptions
	Args:    createValidateDistroArgs(1),
	PreRun:  PreRunValidation,
	Run: func(_ *cobra.Command, args []string) {
		split := strings.Split(args[0], "-")

		if len(split) == 1 && split[0] != alpineDistro && split[0] != archDistro {
			logger.Fatal(i18n.T("logger.pull.specify_codename"))
		}

		rt, err := container.Detect(ContainerRuntimeOverride())
		if err != nil {
			logger.Fatal(i18n.T("logger.pull.failed_to_pull"), "error", err)
		}

		logger.Info("using container runtime", "type", string(rt.Type()))

		if err := rt.Pull(args[0]); err != nil {
			logger.Fatal(i18n.T("logger.pull.failed_to_pull"), "error", err)
		}
	},
}

//nolint:gochecknoinits // Required for cobra command registration
func init() {
	rootCmd.AddCommand(pullCmd)

	// Add completion for distribution argument
	pullCmd.ValidArgsFunction = ValidDistrosCompletion
}
