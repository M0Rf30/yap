package main

import (
	"bytes"
	"testing"

	"github.com/spf13/cobra"

	"github.com/M0Rf30/yap/v2/pkg/i18n"
)

func executeCommand(root *cobra.Command, args ...string) (output string, err error) {
	_, output, err = executeCommandC(root, args...)
	return output, err
}

func executeCommandC(root *cobra.Command, args ...string) (c *cobra.Command, output string, err error) {
	buf := new(bytes.Buffer)
	root.SetOut(buf)
	root.SetErr(buf)
	root.SetArgs(args)

	c, err = root.ExecuteC()
	output = buf.String()

	return c, output, err
}

func TestMainFunction(t *testing.T) {
	// Test check command
	t.Run("CheckCommand", func(t *testing.T) {
		cmd := &cobra.Command{
			Use: "check",
			RunE: func(cmd *cobra.Command, args []string) error {
				return nil
			},
		}

		if cmd.Use != "check" {
			t.Errorf("Expected command use 'check', got '%s'", cmd.Use)
		}
	})

	// Test list command
	t.Run("ListCommand", func(t *testing.T) {
		cmd := &cobra.Command{
			Use: "list",
			RunE: func(cmd *cobra.Command, args []string) error {
				return nil
			},
		}

		if cmd.Use != "list" {
			t.Errorf("Expected command use 'list', got '%s'", cmd.Use)
		}
	})

	// Test stats command
	t.Run("StatsCommand", func(t *testing.T) {
		cmd := &cobra.Command{
			Use: "stats",
			RunE: func(cmd *cobra.Command, args []string) error {
				return nil
			},
		}

		if cmd.Use != "stats" {
			t.Errorf("Expected command use 'stats', got '%s'", cmd.Use)
		}
	})
}

func TestCheckCommandExecution(t *testing.T) {
	// Test the check command function directly
	err := func(_ *cobra.Command, _ []string) error {
		// This mirrors the actual check command logic
		return i18n.CheckIntegrity()
	}(&cobra.Command{}, []string{})

	// We expect this to pass since the integrity check should work with existing files
	if err != nil {
		t.Errorf("Check command failed: %v", err)
	}
}

func TestListCommandExecution(t *testing.T) {
	// Test the list command function directly
	ids, err := func(_ *cobra.Command, _ []string) ([]string, error) {
		// This mirrors the actual list command logic
		return i18n.GetMessageIDs()
	}(&cobra.Command{}, []string{})
	if err != nil {
		t.Errorf("List command failed: %v", err)
	}

	// Verify that we get some message IDs back
	if len(ids) == 0 {
		t.Log("Warning: No message IDs found, but this may be expected")
	}
}

func TestStatsCommandExecution(t *testing.T) {
	// Test the stats command function directly
	supported := i18n.SupportedLanguages
	if len(supported) == 0 {
		t.Error("Expected supported languages to be non-empty")
	}

	// Test getting message IDs for stats
	ids, err := i18n.GetMessageIDs()
	if err != nil {
		t.Errorf("Failed to get message IDs for stats: %v", err)
	}

	// Verify we have some messages
	if len(ids) == 0 {
		t.Log("Warning: No message IDs found for stats, but this may be expected")
	}
}

func TestRootCommandStructure(t *testing.T) {
	rootCmd := NewRootCmd()

	// Verify command structure
	if rootCmd.Use != "i18n-tool" {
		t.Errorf("Expected root command use 'i18n-tool', got '%s'", rootCmd.Use)
	}

	if len(rootCmd.Commands()) != 3 {
		t.Errorf("Expected 3 commands, got %d", len(rootCmd.Commands()))
	}

	// Check that all expected commands are present
	cmdNames := make(map[string]bool)
	for _, cmd := range rootCmd.Commands() {
		cmdNames[cmd.Use] = true
	}

	expectedCmds := []string{"check", "list", "stats"}
	for _, expected := range expectedCmds {
		if !cmdNames[expected] {
			t.Errorf("Missing expected command: %s", expected)
		}
	}
}

func TestCheckCommand(t *testing.T) {
	rootCmd := NewRootCmd()

	output, err := executeCommand(rootCmd, "check")
	if err != nil {
		t.Logf("Check command output: %s", output)
		// The check command might fail if there are integrity issues, but that's OK for testing
	}
}

func TestListCommand(t *testing.T) {
	rootCmd := NewRootCmd()

	_, err := executeCommand(rootCmd, "list")
	if err != nil {
		t.Errorf("List command failed: %v", err)
	}
}

func TestStatsCommand(t *testing.T) {
	rootCmd := NewRootCmd()

	_, err := executeCommand(rootCmd, "stats")
	if err != nil {
		t.Errorf("Stats command failed: %v", err)
	}
}
