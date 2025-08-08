package command

import (
	"errors"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
)

func TestCalculateSimilarity(t *testing.T) {
	tests := []struct {
		name     string
		str1     string
		str2     string
		expected float64
	}{
		{
			name:     "identical strings",
			str1:     "build",
			str2:     "build",
			expected: 1.0,
		},
		{
			name:     "empty strings",
			str1:     "",
			str2:     "",
			expected: 1.0, // Empty strings are identical
		},
		{
			name:     "one empty string",
			str1:     "test",
			str2:     "",
			expected: 0.0,
		},
		{
			name:     "similar strings",
			str1:     "build",
			str2:     "biuld",
			expected: 1.0, // All 5 characters match (b,i,u,l,d), so 5/5 = 1.0
		},
		{
			name:     "case insensitive",
			str1:     "BUILD",
			str2:     "build",
			expected: 1.0,
		},
		{
			name:     "different strings",
			str1:     "abc",
			str2:     "xyz",
			expected: 0.0,
		},
		{
			name:     "partial match",
			str1:     "prepare",
			str2:     "prep",
			expected: 6.0 / 7.0, // p,r,e,p,r,e match (a doesn't), 6 out of max length 7
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := calculateSimilarity(tt.str1, tt.str2)
			assert.InDelta(t, tt.expected, result, 0.01, "Similarity calculation mismatch")
		})
	}
}

func TestSmartErrorHandler(t *testing.T) {
	// Create a test cobra command
	rootCmd := &cobra.Command{
		Use:   "yap",
		Short: "Yet Another Packager",
	}

	buildCmd := &cobra.Command{
		Use:   "build",
		Short: "Build packages",
	}

	prepareCmd := &cobra.Command{
		Use:   "prepare",
		Short: "Prepare environment",
	}

	rootCmd.AddCommand(buildCmd, prepareCmd)

	tests := []struct {
		name        string
		err         error
		expectError bool
	}{
		{
			name:        "nil error",
			err:         nil,
			expectError: false,
		},
		{
			name:        "unknown command error",
			err:         errors.New("unknown command \"buidl\" for \"yap\""),
			expectError: true,
		},
		{
			name:        "unsupported distribution error",
			err:         errors.New("unsupported distribution 'invalid-distro'"),
			expectError: true,
		},
		{
			name:        "yap.json not found error",
			err:         errors.New("yap.json not found in current directory"),
			expectError: true,
		},
		{
			name:        "requires at least error",
			err:         errors.New("requires at least 1 arg(s), only received 0"),
			expectError: true,
		},
		{
			name:        "generic error",
			err:         errors.New("some other error"),
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := SmartErrorHandler(rootCmd, tt.err)

			if tt.expectError {
				assert.Error(t, err)
				assert.Equal(t, tt.err, err, "Error should be passed through")
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestProvideArgumentHelp(t *testing.T) {
	tests := []struct {
		name    string
		cmdName string
	}{
		{
			name:    "build command",
			cmdName: "build",
		},
		{
			name:    "prepare command",
			cmdName: "prepare",
		},
		{
			name:    "pull command",
			cmdName: "pull",
		},
		{
			name:    "zap command",
			cmdName: "zap",
		},
		{
			name:    "unknown command",
			cmdName: "unknown",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := &cobra.Command{
				Use: tt.cmdName,
			}

			// Should not panic
			assert.NotPanics(t, func() {
				provideArgumentHelp(cmd)
			})
		})
	}
}

func TestShowWelcomeMessage(t *testing.T) {
	// Test that ShowWelcomeMessage doesn't panic
	assert.NotPanics(t, func() {
		ShowWelcomeMessage()
	})
}

func TestShowCommandTips(t *testing.T) {
	tests := []struct {
		name    string
		cmdName string
	}{
		{
			name:    "build command",
			cmdName: "build",
		},
		{
			name:    "prepare command",
			cmdName: "prepare",
		},
		{
			name:    "list-distros command",
			cmdName: "list-distros",
		},
		{
			name:    "unknown command",
			cmdName: "unknown",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := &cobra.Command{
				Use: tt.cmdName,
			}

			// Should not panic
			assert.NotPanics(t, func() {
				ShowCommandTips(cmd)
			})
		})
	}
}

func TestProvideProjectSetupHelp(t *testing.T) {
	// Test that provideProjectSetupHelp doesn't panic
	assert.NotPanics(t, func() {
		provideProjectSetupHelp()
	})
}

func TestProvideSimilarCommands(t *testing.T) {
	// Create a test cobra command with subcommands
	rootCmd := &cobra.Command{
		Use:   "yap",
		Short: "Yet Another Packager",
	}

	buildCmd := &cobra.Command{
		Use:     "build",
		Aliases: []string{"b"},
		Short:   "Build packages",
	}

	prepareCmd := &cobra.Command{
		Use:     "prepare",
		Aliases: []string{"prep"},
		Short:   "Prepare environment",
	}

	hiddenCmd := &cobra.Command{
		Use:    "hidden",
		Short:  "Hidden command",
		Hidden: true,
	}

	rootCmd.AddCommand(buildCmd, prepareCmd, hiddenCmd)

	tests := []struct {
		name     string
		errorStr string
	}{
		{
			name:     "unknown command with quotes",
			errorStr: "unknown command \"buidl\" for \"yap\"",
		},
		{
			name:     "unknown command without quotes",
			errorStr: "unknown command for yap",
		},
		{
			name:     "error without command",
			errorStr: "some other error",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Should not panic
			assert.NotPanics(t, func() {
				provideSimilarCommands(rootCmd, tt.errorStr)
			})
		})
	}
}

func TestProvideSimilarDistributions(t *testing.T) {
	tests := []struct {
		name     string
		errorStr string
	}{
		{
			name:     "distro with single quotes",
			errorStr: "unsupported distribution 'ubunto'",
		},
		{
			name:     "distro without quotes",
			errorStr: "unsupported distribution",
		},
		{
			name:     "different error format",
			errorStr: "some other error",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Should not panic
			assert.NotPanics(t, func() {
				provideSimilarDistributions(tt.errorStr)
			})
		})
	}
}

func TestShowCommandGroups(t *testing.T) {
	// Create a test cobra command with groups
	rootCmd := &cobra.Command{
		Use:   "yap",
		Short: "Yet Another Packager",
	}

	// Add groups
	rootCmd.AddGroup(&cobra.Group{
		ID:    "build",
		Title: "Build Commands",
	})

	// Add commands with groups
	buildCmd := &cobra.Command{
		Use:     "build",
		Short:   "Build packages",
		GroupID: "build",
	}

	zapCmd := &cobra.Command{
		Use:     "zap",
		Short:   "Clean artifacts",
		GroupID: "build",
	}

	// Add command without group
	prepareCmd := &cobra.Command{
		Use:   "prepare",
		Short: "Prepare environment",
	}

	// Add hidden command
	hiddenCmd := &cobra.Command{
		Use:    "hidden",
		Short:  "Hidden command",
		Hidden: true,
	}

	// Add help command
	helpCmd := &cobra.Command{
		Use:   "help",
		Short: "Help about any command",
	}

	rootCmd.AddCommand(buildCmd, zapCmd, prepareCmd, hiddenCmd, helpCmd)

	// Should not panic
	assert.NotPanics(t, func() {
		showCommandGroups(rootCmd)
	})
}
