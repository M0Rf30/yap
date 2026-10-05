package command

import (
	"bytes"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

// TestCompletionWritesToCommandOutput guards against the script being sent to
// stderr (via the logger writer), which breaks `source <(yap completion bash)`.
func TestCompletionWritesToCommandOutput(t *testing.T) {
	for _, shellName := range []string{"bash", "fish", "zsh"} {
		t.Run(shellName, func(t *testing.T) {
			// Use a throwaway command tree: cobra's generators mutate the tree
			// they walk (e.g. sorting aliases), which must not leak into the
			// real rootCmd shared with other tests.
			root := &cobra.Command{Use: "yap"}
			comp := &cobra.Command{Use: "completion", Run: completionCmd.Run}
			root.AddCommand(comp)

			var buf bytes.Buffer

			comp.SetOut(&buf)
			comp.Run(comp, []string{shellName})

			if !strings.Contains(buf.String(), "yap") {
				t.Errorf("%s completion script not written to command output (len=%d)",
					shellName, buf.Len())
			}
		})
	}
}
