package command

import (
	"fmt"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	"github.com/M0Rf30/yap/v2/pkg/color"
	"github.com/M0Rf30/yap/v2/pkg/constants"
	"github.com/M0Rf30/yap/v2/pkg/i18n"
	"github.com/M0Rf30/yap/v2/pkg/logger"
)

const (
	buildCommand   = "build"
	prepareCommand = "prepare"
)

// SmartErrorHandler provides enhanced error messages with intelligent suggestions.
func SmartErrorHandler(cmd *cobra.Command, err error) error {
	if err == nil {
		return nil
	}

	errorStr := err.Error()

	// Provide context-aware suggestions
	switch {
	case strings.Contains(errorStr, "unknown command"):
		provideSimilarCommands(cmd, errorStr)
	case strings.Contains(errorStr, "unsupported distribution"):
		provideSimilarDistributions(errorStr)
	case strings.Contains(errorStr, "yap.json not found") ||
		strings.Contains(errorStr, "neither yap.json nor PKGBUILD found"):
		provideProjectSetupHelp()
	case strings.Contains(errorStr, "requires at least"):
		provideArgumentHelp(cmd)
	default:
		// General help suggestion
		fmt.Printf("Use '%s --help' for more information\n", cmd.CommandPath())
	}

	return err
}

// provideSimilarCommands suggests similar commands when an unknown command is used.
func provideSimilarCommands(cmd *cobra.Command, errorStr string) {
	// Extract the unknown command from error message
	parts := strings.Split(errorStr, "\"")
	if len(parts) < 2 {
		return
	}

	unknown := parts[1]

	var suggestions []string

	for _, subCmd := range cmd.Root().Commands() {
		if !subCmd.Hidden && subCmd.Name() != aliasHelp {
			// Calculate simple similarity (starts with same letter, contains similar chars)
			if calculateSimilarity(unknown, subCmd.Name()) > 0.3 {
				suggestions = append(suggestions, subCmd.Name())
			}
			// Also check aliases
			for _, alias := range subCmd.Aliases {
				if calculateSimilarity(unknown, alias) > 0.3 {
					suggestions = append(suggestions, alias)
				}
			}
		}
	}

	if len(suggestions) > 0 {
		fmt.Printf("\n%s\n%s\n\n", color.BoldYellow("Did you mean?:"), strings.Join(suggestions, ", "))
	}

	// Show available commands grouped
	showCommandGroups(cmd.Root())
}

// provideSimilarDistributions suggests similar distributions.
func provideSimilarDistributions(errorStr string) {
	// Extract the invalid distro from error message
	parts := strings.Split(errorStr, "'")
	if len(parts) < 2 {
		return
	}

	invalid := strings.ToLower(parts[1])

	var suggestions []string

	for _, release := range &constants.Releases {
		if calculateSimilarity(invalid, strings.ToLower(release)) > 0.4 {
			suggestions = append(suggestions, release)
		}
	}

	if len(suggestions) > 0 {
		sort.Strings(suggestions)

		if len(suggestions) > 5 {
			suggestions = suggestions[:5]
		}

		fmt.Printf("\n%s\n%s\n\n", color.BoldBlue("Similar distributions:"), strings.Join(suggestions, "\n"))
	}

	fmt.Printf("%s %s\n", color.BoldBlue("INFO"), "Use 'yap list-distros' to see all supported distributions")
}

// provideProjectSetupHelp provides guidance for setting up a YAP project.
func provideProjectSetupHelp() {
	helpText := `YAP Project Setup Guide:

For Multi-Project (recommended for multiple packages):
1. Create a yap.json file in your project directory
2. Define your package specifications
3. Add PKGBUILD files for each package in subdirectories

For Single Project (one package):
1. Create a PKGBUILD file in your project directory
2. Define your package specification in the PKGBUILD

Example yap.json structure:
{
  "name": "my-project",
  "description": "My multi-package project",
  "buildDir": "build",
  "output": "output",
  "projects": [
    {"name": "package1"},
    {"name": "package2"}
  ]
}`

	fmt.Printf("\n%s\n%s\n\n", color.BoldGreen("Project Setup Help:"), helpText)
}

// provideArgumentHelp provides context-specific argument guidance.
func provideArgumentHelp(cmd *cobra.Command) {
	switch cmd.Name() {
	case buildCommand:
		fmt.Printf("%s %s\n", color.BoldBlue("INFO"), "Build command formats:")
		fmt.Printf("%s %s\n", color.BoldBlue("INFO"),
			"  • yap build .                    (current directory, auto-detect distro)")
		fmt.Printf("%s %s\n", color.BoldBlue("INFO"), "  • yap build ubuntu-jammy .       (specific distro and path)")
		fmt.Printf("%s %s\n", color.BoldBlue("INFO"), "  • yap build fedora-38 /path/to/project")
	case prepareCommand:
		fmt.Printf("%s %s\n", color.BoldBlue("INFO"), "Prepare command format:")
		fmt.Printf("%s %s\n", color.BoldBlue("INFO"), "  • yap "+prepareCommand+" <distribution>")
		fmt.Printf("%s %s\n", color.BoldBlue("INFO"), "  • Example: yap "+prepareCommand+" ubuntu-jammy")
	case commandPull:
		fmt.Printf("%s %s\n", color.BoldBlue("INFO"), "Pull command format:")
		fmt.Printf("%s %s\n", color.BoldBlue("INFO"), "  • yap "+commandPull+" <distribution>")
		fmt.Printf("%s %s\n", color.BoldBlue("INFO"), "  • Example: yap "+commandPull+" alpine")
	case commandZap:
		fmt.Printf("%s %s\n", color.BoldBlue("INFO"), "Zap command format:")
		fmt.Printf("%s %s\n", color.BoldBlue("INFO"), "  • yap "+commandZap+" <distribution> <path>")
		fmt.Printf("%s %s\n", color.BoldBlue("INFO"), "  • Example: yap "+commandZap+" ubuntu-jammy /path/to/project")
	}
}

// showCommandGroups displays available commands organized by groups.
func showCommandGroups(rootCmd *cobra.Command) {
	groups := make(map[string][]string)
	ungrouped := []string{}

	for _, cmd := range rootCmd.Commands() {
		if cmd.Hidden || cmd.Name() == aliasHelp {
			continue
		}

		if cmd.GroupID != "" {
			// Find group title
			groupTitle := cmd.GroupID

			for _, group := range rootCmd.Groups() {
				if group.ID == cmd.GroupID {
					groupTitle = group.Title

					break
				}
			}

			groups[groupTitle] = append(groups[groupTitle], cmd.Name())
		} else {
			ungrouped = append(ungrouped, cmd.Name())
		}
	}

	fmt.Println()
	fmt.Printf("%s\n", color.BoldCyan("Available Commands"))

	for groupTitle, commands := range groups {
		if len(commands) > 0 {
			fmt.Printf("\n%s:\n", color.BoldCyan(groupTitle))

			for _, cmdName := range commands {
				fmt.Printf("  %s\n", color.HiBlue(cmdName))
			}
		}
	}

	if len(ungrouped) > 0 {
		fmt.Printf("\n%s:\n", color.BoldMagenta("Other Commands"))

		for _, cmdName := range ungrouped {
			fmt.Printf("  %s\n", color.HiBlue(cmdName))
		}
	}
}

// calculateSimilarity calculates a simple similarity score between two strings.
func calculateSimilarity(str1, str2 string) float64 {
	if str1 == str2 {
		return 1.0
	}

	str1 = strings.ToLower(str1)
	str2 = strings.ToLower(str2)

	if str1 == "" || str2 == "" {
		return 0.0
	}

	// Simple character similarity
	matches := 0

	for _, r1 := range str1 {
		for _, r2 := range str2 {
			if r1 == r2 {
				matches++

				break
			}
		}
	}

	maxLen := max(len(str2), len(str1))

	return float64(matches) / float64(maxLen)
}

// ShowWelcomeMessage displays a welcome message for first-time users.
func ShowWelcomeMessage() {
	fmt.Printf("\n%s\n%s\n\n", color.BoldGreen("Welcome to YAP:"), `Thank you for using Yet Another Packager!

Quick start:
1. Run 'yap list-distros' to see supported distributions
2. Run 'yap prepare <distro>' to set up your environment
3. Create a project with yap.json (multiproject) or PKGBUILD (single project)
4. Run 'yap build <distro> .' to build your packages

Need help? Visit: https://github.com/M0Rf30/yap`)
}

// ShowCommandTips shows helpful tips based on command usage.
func ShowCommandTips(cmd *cobra.Command) {
	switch cmd.Name() {
	case buildCommand:
		if !verbose {
			logger.Tips(i18n.T("logger.tips.use_verbose"))
		}
	case prepareCommand:
		logger.Tips(i18n.T("logger.tips.use_verbose"))
	}
}
