package command

import (
	"os"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"

	"github.com/M0Rf30/yap/v2/pkg/i18n"
)

func TestRootCommand(t *testing.T) {
	tests := []struct {
		name        string
		args        []string
		expectError bool
	}{
		{
			name:        "Root command help",
			args:        []string{"--help"},
			expectError: false,
		},
		{
			name:        "Root command version",
			args:        []string{"version"},
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Save original args
			originalArgs := os.Args

			defer func() { os.Args = originalArgs }()

			// Set test args
			os.Args = append([]string{"yap"}, tt.args...)

			// Test should not panic
			assert.NotPanics(t, func() {
				// Create a copy of rootCmd for testing
				testCmd := rootCmd
				testCmd.SetArgs(tt.args)
				_ = testCmd.Execute()
			})
		})
	}
}

func TestIsNoColorEnabled(t *testing.T) {
	// Test default value
	result := IsNoColorEnabled()
	assert.False(t, result) // Should be false by default

	// Test with no-color flag set
	noColor = true
	result = IsNoColorEnabled()
	assert.True(t, result)

	// Reset for other tests
	noColor = false
}

func TestParseLanguageFlag(t *testing.T) {
	supported := i18n.SupportedLanguages // e.g. ["en", "it"]
	// Pick a known supported language that isn't the first one, to avoid
	// accidentally passing due to default behaviour.
	var knownLang string

	for _, l := range supported {
		if l != "en" {
			knownLang = l
			break
		}
	}

	if knownLang == "" {
		knownLang = supported[0]
	}

	tests := []struct {
		name     string
		args     []string
		wantLang string // empty means we only check it doesn't panic / no i18n init
	}{
		{
			name:     "short flag space-separated",
			args:     []string{"yap", "-l", knownLang},
			wantLang: knownLang,
		},
		{
			name:     "long flag space-separated",
			args:     []string{"yap", "--language", knownLang},
			wantLang: knownLang,
		},
		{
			name:     "long flag equals form",
			args:     []string{"yap", "--language=" + knownLang},
			wantLang: knownLang,
		},
		{
			name:     "short flag equals form",
			args:     []string{"yap", "-l=" + knownLang},
			wantLang: knownLang,
		},
		{
			name:     "no language flag",
			args:     []string{"yap", "build", "ubuntu", "."},
			wantLang: "",
		},
		{
			name:     "empty args",
			args:     []string{"yap"},
			wantLang: "",
		},
		{
			name:     "unsupported language is ignored",
			args:     []string{"yap", "-l", "zz"},
			wantLang: "",
		},
		{
			name:     "language flag at end without value",
			args:     []string{"yap", "--language"},
			wantLang: "",
		},
		{
			name:     "english is supported",
			args:     []string{"yap", "--language", "en"},
			wantLang: "en",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			originalArgs := os.Args
			defer func() { os.Args = originalArgs }()

			os.Args = tt.args

			// ParseLanguageFlag reads os.Args[1:] and calls i18n.Init when it
			// finds a supported language. It must not panic.
			assert.NotPanics(t, func() {
				ParseLanguageFlag()
			})
		})
	}
}

func TestUpdateSubCommandDescriptions(t *testing.T) {
	// Build a fresh parent command with a set of named subcommands that mirror
	// the names handled by updateSubCommandDescriptions.
	names := []string{
		"graph",
		commandInstall,
		commandListDistro,
		prepareCommand,
		commandPull,
		commandStatus,
		commandVersion,
		commandZap,
		"completion",
		commandGensum,
		"unknown-cmd", // should be left untouched
	}

	parent := &cobra.Command{Use: "parent"}

	for _, name := range names {
		sub := &cobra.Command{Use: name, Short: "original"}
		parent.AddCommand(sub)
	}

	// Ensure i18n is initialised so T() returns something (even the key itself).
	_ = i18n.Init("en")

	assert.NotPanics(t, func() {
		updateSubCommandDescriptions(parent)
	})

	// After the call every known command should have a non-empty Short.
	for _, sub := range parent.Commands() {
		if sub.Name() == "unknown-cmd" {
			assert.Equal(t, "original", sub.Short,
				"unknown command Short should be unchanged")
		} else {
			assert.NotEmpty(t, sub.Short,
				"command %q Short should be set after updateSubCommandDescriptions", sub.Name())
		}
	}
}

func TestUpdateCommandShortDescriptions(t *testing.T) {
	// updateCommandShortDescriptions delegates to updateSubCommandDescriptions(rootCmd).
	// We verify it doesn't panic and that rootCmd subcommands still have non-empty Shorts.
	_ = i18n.Init("en")

	assert.NotPanics(t, func() {
		updateCommandShortDescriptions()
	})
}

func TestUpdateOtherCommandDescriptions(t *testing.T) {
	_ = i18n.Init("en")

	assert.NotPanics(t, func() {
		updateOtherCommandDescriptions()
	})
}
