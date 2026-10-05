package command

import (
	"github.com/spf13/cobra"

	"github.com/M0Rf30/yap/v2/pkg/i18n"
	"github.com/M0Rf30/yap/v2/pkg/logger"
)

// completionCmd represents the completion command.
var completionCmd = &cobra.Command{
	Use:                   "completion <shell>",
	GroupID:               commandUtility,
	Short:                 "", // Set by InitializeLocalizedDescriptions
	Long:                  "", // Set by InitializeLocalizedDescriptions
	Example:               "", // Set by InitializeLocalizedDescriptions
	DisableFlagsInUseLine: true,
	ValidArgs:             []string{"bash", "fish", "zsh"},
	Args:                  cobra.MatchAll(cobra.ExactArgs(1), cobra.OnlyValidArgs),
	Run: func(cmd *cobra.Command, args []string) {
		out := cmd.OutOrStdout()

		switch args[0] {
		case "bash":
			err := cmd.Root().GenBashCompletion(out)
			if err != nil {
				logger.Fatal(i18n.T("errors.completion.failed_to_generate_bash_completion"),
					"error", err)
			}
		case "fish":
			err := cmd.Root().GenFishCompletion(out, true)
			if err != nil {
				logger.Fatal(i18n.T("errors.completion.failed_to_generate_fish_completion"),
					"error", err)
			}
		case "zsh":
			err := cmd.Root().GenZshCompletion(out)
			if err != nil {
				logger.Fatal(i18n.T("errors.completion.failed_to_generate_zsh_completion"),
					"error", err)
			}
		}
	},
}

//nolint:gochecknoinits // Required for cobra command registration
func init() {
	rootCmd.AddCommand(completionCmd)
}
