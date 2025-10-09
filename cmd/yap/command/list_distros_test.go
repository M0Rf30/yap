package command

import (
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"

	"github.com/M0Rf30/yap/v2/pkg/i18n"
)

func TestListDistrosCommand(t *testing.T) {
	tests := []struct {
		name        string
		args        []string
		expectError bool
	}{
		{
			name:        "List distros command",
			args:        []string{"list-distros"},
			expectError: false,
		},
		{
			name:        "List distros alias - list",
			args:        []string{"list"},
			expectError: false,
		},
		{
			name:        "List distros alias - distros",
			args:        []string{"distros"},
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a new command for testing
			cmd := &cobra.Command{
				Use: "yap",
			}

			// Add command groups that the subcommands expect
			cmd.AddGroup(&cobra.Group{ID: "utility", Title: "Utility Commands"})

			cmd.AddCommand(listDistrosCmd)
			cmd.SetArgs(tt.args)

			// Execute the command and verify it doesn't error
			err := cmd.Execute()
			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestListDistros(t *testing.T) {
	// Test that it doesn't panic
	assert.NotPanics(t, func() {
		ListDistros()
	})
}

func TestListDistrosCommandDefinition(t *testing.T) {
	// Initialize i18n and descriptions for testing
	_ = i18n.Init("en")

	InitializeListDistrosDescriptions()

	// Test command properties
	assert.Equal(t, "list-distros", listDistrosCmd.Use)
	assert.Equal(t, "utility", listDistrosCmd.GroupID)
	assert.Contains(t, listDistrosCmd.Aliases, "list")
	assert.Contains(t, listDistrosCmd.Aliases, "distros")
	assert.NotEmpty(t, listDistrosCmd.Short)
	assert.NotEmpty(t, listDistrosCmd.Long)
}
