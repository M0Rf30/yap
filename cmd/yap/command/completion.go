package command

import (
	"github.com/M0Rf30/yap/pkg/utils"
	"github.com/spf13/cobra"
)

// completionCmd represents the completion command.
var completionCmd = &cobra.Command{
	Use:   "completion [bash|zsh|fish]",
	Short: "Generate completion script",
	Long: `To load completions:

Bash:

$ source <(yap completion bash)

# To load completions for each session, execute once:
# Linux:
$ yap completion bash > /etc/bash_completion.d/yap
# macOS:
$ yap completion bash > /usr/local/etc/bash_completion.d/yap

fish:

$ yap completion fish | source

# To load completions for each session, execute once:
$ yap completion fish > ~/.config/fish/completions/yap.fish

Zsh:

# If shell completion is not already enabled in your environment,
# you will need to enable it.  You can execute the following once:

$ echo "autoload -U compinit; compinit" >> ~/.zshrc

# To load completions for each session, execute once:
$ yap completion zsh > "${fpath[1]}/_yap"

# You will need to start a new shell for this setup to take effect.
`,
	DisableFlagsInUseLine: true,
	ValidArgs:             []string{"bash", "fish", "zsh"},
	Args:                  cobra.MatchAll(cobra.MinimumNArgs(1), cobra.OnlyValidArgs),
	Run: func(cmd *cobra.Command, args []string) {
		switch args[0] {
		case "bash":
			err := cmd.Root().GenBashCompletion(utils.MultiPrinter.Writer)
			if err != nil {
				utils.Logger.Fatal("failed to generate bash completion",
					utils.Logger.Args("error", err))
			}
		case "fish":
			err := cmd.Root().GenFishCompletion(utils.MultiPrinter.Writer, true)
			if err != nil {
				utils.Logger.Fatal("failed to generate fish completion",
					utils.Logger.Args("error", err))
			}
		case "zsh":
			err := cmd.Root().GenZshCompletion(utils.MultiPrinter.Writer)
			if err != nil {
				utils.Logger.Fatal("failed to generate zsh completion",
					utils.Logger.Args("error", err))
			}
		}
	},
}

func init() {
	rootCmd.AddCommand(completionCmd)
}
