package command

import (
	"sort"
	"strings"

	"github.com/pterm/pterm"
	"github.com/spf13/cobra"

	"github.com/M0Rf30/yap/v2/pkg/constants"
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
		pterm.Printfln("Use '%s --help' for more information", cmd.CommandPath())
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
		if !subCmd.Hidden && subCmd.Name() != "help" {
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
		pterm.DefaultBox.WithTitle("💡 Did you mean?").
			WithTitleTopLeft().
			WithBoxStyle(pterm.NewStyle(pterm.FgYellow)).
			Println(strings.Join(suggestions, ", "))
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

		pterm.DefaultBox.WithTitle("🎯 Similar distributions").
			WithTitleTopLeft().
			WithBoxStyle(pterm.NewStyle(pterm.FgBlue)).
			Println(strings.Join(suggestions, "\n"))
	}

	pterm.Info.Println("💡 Use 'yap list-distros' to see all supported distributions")
}

// provideProjectSetupHelp provides guidance for setting up a YAP project.
func provideProjectSetupHelp() {
	helpText := `📁 YAP Project Setup Guide:

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

	pterm.DefaultBox.WithTitle("🛠️  Project Setup Help").
		WithTitleTopLeft().
		WithBoxStyle(pterm.NewStyle(pterm.FgGreen)).
		Println(helpText)
}

// provideArgumentHelp provides context-specific argument guidance.
func provideArgumentHelp(cmd *cobra.Command) {
	switch cmd.Name() {
	case buildCommand:
		pterm.Info.Println("💡 Build command formats:")
		pterm.Info.Println("  • yap build .                    (current directory, auto-detect distro)")
		pterm.Info.Println("  • yap build ubuntu-jammy .       (specific distro and path)")
		pterm.Info.Println("  • yap build fedora-38 /path/to/project")
	case "prepare":
		pterm.Info.Println("💡 Prepare command format:")
		pterm.Info.Println("  • yap prepare <distribution>")
		pterm.Info.Println("  • Example: yap prepare ubuntu-jammy")
	case "pull":
		pterm.Info.Println("💡 Pull command format:")
		pterm.Info.Println("  • yap pull <distribution>")
		pterm.Info.Println("  • Example: yap pull alpine")
	case "zap":
		pterm.Info.Println("💡 Zap command format:")
		pterm.Info.Println("  • yap zap <distribution> <path>")
		pterm.Info.Println("  • Example: yap zap ubuntu-jammy /path/to/project")
	}
}

// showCommandGroups displays available commands organized by groups.
func showCommandGroups(rootCmd *cobra.Command) {
	groups := make(map[string][]string)
	ungrouped := []string{}

	for _, cmd := range rootCmd.Commands() {
		if cmd.Hidden || cmd.Name() == "help" {
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

	pterm.Println()
	pterm.DefaultHeader.WithBackgroundStyle(pterm.NewStyle(pterm.BgBlue)).
		WithTextStyle(pterm.NewStyle(pterm.FgWhite, pterm.Bold)).
		Println("Available Commands")

	for groupTitle, commands := range groups {
		if len(commands) > 0 {
			pterm.Printf("\n%s:\n", pterm.NewStyle(pterm.FgCyan, pterm.Bold).Sprint(groupTitle))

			for _, cmdName := range commands {
				pterm.Printf("  %s\n", pterm.FgLightBlue.Sprint(cmdName))
			}
		}
	}

	if len(ungrouped) > 0 {
		pterm.Printf("\n%s:\n", pterm.NewStyle(pterm.FgMagenta, pterm.Bold).Sprint("Other Commands"))

		for _, cmdName := range ungrouped {
			pterm.Printf("  %s\n", pterm.FgLightBlue.Sprint(cmdName))
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
	pterm.DefaultBox.WithTitle("🎉 Welcome to YAP!").
		WithTitleTopCenter().
		WithBoxStyle(pterm.NewStyle(pterm.FgGreen)).
		Println(`Thank you for using Yet Another Packager!

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
			logger.Tips("💡 Use --verbose (-v) for detailed build output")
		}
	case "prepare":
		logger.Tips("💡 Use --golang (-g) to also install Go development tools")
	case "list-distros":
		logger.Tips("💡 Use these identifiers with other YAP commands")
	}
}
