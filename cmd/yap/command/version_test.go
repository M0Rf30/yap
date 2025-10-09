package command

import (
	"testing"
	"time"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
)

func TestVersionCommand(t *testing.T) {
	tests := []struct {
		name        string
		args        []string
		expectError bool
	}{
		{
			name:        "Version command execution",
			args:        []string{"version"},
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

			cmd.AddCommand(versionCmd)
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

func TestGetCurrentTime(t *testing.T) {
	result := getCurrentTime()

	// Check that the result is not empty
	assert.NotEmpty(t, result)

	// Check that the result can be parsed as time
	_, err := time.Parse("2006-01-02 15:04:05 MST", result)
	assert.NoError(t, err)
}

func TestVersionCommandDefinition(t *testing.T) {
	// Test command properties
	assert.Equal(t, "version", versionCmd.Use)
	assert.Equal(t, "utility", versionCmd.GroupID)
	assert.NotEmpty(t, versionCmd.Short)
	assert.NotEmpty(t, versionCmd.Long)
}
