package command

import (
	"github.com/spf13/cobra"

	"github.com/M0Rf30/yap/v2/pkg/gensum"
	"github.com/M0Rf30/yap/v2/pkg/i18n"
	"github.com/M0Rf30/yap/v2/pkg/logger"
)

const commandGensum = "gensum"

// gensumCmd recomputes PKGBUILD checksums in-place.
var gensumCmd = &cobra.Command{
	Use:     commandGensum + " <path>",
	GroupID: buildGroup,
	Short:   "", // Set by InitializeLocalizedDescriptions
	Long:    "", // Set by InitializeLocalizedDescriptions
	Example: "", // Set by InitializeLocalizedDescriptions
	Args:    cobra.ExactArgs(1),
	RunE: func(_ *cobra.Command, args []string) error {
		pkgbuildDir := args[0]

		err := gensum.UpdateChecksums(pkgbuildDir)
		if err != nil {
			logger.Error(i18n.T("errors.gensum.failed"), "error", err)

			return err
		}

		return nil
	},
}

// InitializeGensumDescriptions sets the localized descriptions for gensum.
func InitializeGensumDescriptions() {
	initCommandDescriptions(gensumCmd, commandGensum, map[string]string{})
}

//nolint:gochecknoinits // Required for cobra command registration
func init() {
	rootCmd.AddCommand(gensumCmd)
}
