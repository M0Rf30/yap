package command

import (
	"github.com/spf13/cobra"

	"github.com/M0Rf30/yap/pkg/osutils"
)

// completionCmd represents the completion command.
var completionCmd = &cobra.Command{
	Use:     "completion <shell>",
	GroupID: "utility",
	Short:   "🔧 Generate shell completion scripts",
	Long: `Generate completion scripts for yap commands, flags, and arguments.

Shell completion enables tab-completion for yap commands, making the CLI
more user-friendly and reducing typing errors.

SUPPORTED SHELLS:
  • bash - Bash shell completion
  • zsh  - Zsh shell completion
  • fish - Fish shell completion

INSTALLATION:
The generated scripts should be sourced by your shell or saved to the
appropriate completion directory for automatic loading.`,
	Example: `  # Generate and use completions temporarily
  source <(yap completion bash)
  yap completion fish | source

  # Install completions permanently
  # Bash (Linux):
  yap completion bash | sudo tee /etc/bash_completion.d/yap
  # Bash (macOS):
  yap completion bash > /usr/local/etc/bash_completion.d/yap

  # Fish:
  yap completion fish > ~/.config/fish/completions/yap.fish

  # Zsh (add to .zshrc if needed):
  echo "autoload -U compinit; compinit" >> ~/.zshrc
  yap completion zsh > "${fpath[1]}/_yap"`,
	DisableFlagsInUseLine: true,
	ValidArgs:             []string{"bash", "fish", "zsh"},
	Args:                  cobra.MatchAll(cobra.ExactArgs(1), cobra.OnlyValidArgs),
	Run: func(cmd *cobra.Command, args []string) {
		switch args[0] {
		case "bash":
			err := cmd.Root().GenBashCompletion(osutils.MultiPrinter.Writer)
			if err != nil {
				osutils.Logger.Fatal("failed to generate bash completion",
					osutils.Logger.Args("error", err))
			}
		case "fish":
			err := cmd.Root().GenFishCompletion(osutils.MultiPrinter.Writer, true)
			if err != nil {
				osutils.Logger.Fatal("failed to generate fish completion",
					osutils.Logger.Args("error", err))
			}
		case "zsh":
			err := cmd.Root().GenZshCompletion(osutils.MultiPrinter.Writer)
			if err != nil {
				osutils.Logger.Fatal("failed to generate zsh completion",
					osutils.Logger.Args("error", err))
			}
		}
	},
}

//nolint:gochecknoinits // Required for cobra command registration
func init() {
	rootCmd.AddCommand(completionCmd)
}
