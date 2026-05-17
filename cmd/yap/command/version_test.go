package command

import (
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
)

func TestVersionCommand(t *testing.T) {
	cmd := &cobra.Command{Use: "yap"}
	cmd.AddGroup(&cobra.Group{ID: "utility", Title: "Utility Commands"})
	cmd.AddCommand(versionCmd)
	cmd.SetArgs([]string{"version"})

	assert.NoError(t, cmd.Execute())
}

func TestVersionCommandDefinition(t *testing.T) {
	assert.Equal(t, "version", versionCmd.Use)
	assert.Equal(t, "utility", versionCmd.GroupID)
	assert.NotEmpty(t, versionCmd.Short)
}
