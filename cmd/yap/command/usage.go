package command

import (
	"fmt"
	"slices"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"github.com/M0Rf30/yap/v2/pkg/color"
)

// CustomUsageFunc provides enhanced usage information with organized flag groups.
func CustomUsageFunc(cmd *cobra.Command) error {
	// Print usage line
	fmt.Printf("%s\n", color.Cyan(fmt.Sprintf("Usage: %s", cmd.UseLine())))

	if len(cmd.Aliases) > 0 {
		fmt.Printf("\n%s\n  %s\n",
			color.BoldMagenta("Aliases:"),
			strings.Join(cmd.Aliases, ", "))
	}
	// Print description
	if cmd.Long != "" {
		fmt.Printf("\n%s\n%s\n",
			color.BoldBlue("Description:"),
			cmd.Long)
	}

	// Print examples
	if cmd.Example != "" {
		fmt.Printf("\n%s\n%s\n",
			color.BoldGreen("Examples:"),
			cmd.Example)
	}

	// Organized flags by category
	printOrganizedFlags(cmd)

	// Print inherited flags
	if cmd.HasInheritedFlags() {
		fmt.Printf("\n%s\n", color.BoldYellow("Global Flags:"))
		cmd.InheritedFlags().VisitAll(func(flag *pflag.Flag) {
			printFlag(flag)
		})
	}

	// Print subcommands if any
	if cmd.HasAvailableSubCommands() {
		fmt.Printf("\n%s\n", color.BoldBlue("Available Commands:"))

		for _, subCmd := range cmd.Commands() {
			if subCmd.IsAvailableCommand() {
				fmt.Printf("  %s  %s\n",
					color.HiBlue(fmt.Sprintf("%-12s", subCmd.Name())),
					subCmd.Short)
			}
		}
	}

	// Footer
	fmt.Printf("\n%s\n",
		color.Gray(fmt.Sprintf("Use \"%s [command] --help\" for more information about a command.",
			cmd.CommandPath())))

	return nil
}

// printOrganizedFlags prints flags organized by functional groups.
func printOrganizedFlags(cmd *cobra.Command) {
	if !cmd.HasLocalFlags() {
		return
	}

	// Define flag categories for the build command
	flagCategories := map[string][]string{
		"Build Behavior": {
			"cleanbuild", "no-build", "nocheck", commandZap,
		},
		"Dependency Management": {
			"no-makedeps", flagSkipSync,
		},
		"Version Control": {
			"pkgver", "pkgrel",
		},
		"Source Access": {
			"ssh-password",
		},
		"Build Range Control": {
			flagFrom, "to",
		},
	}

	// Collect flags by category
	categorizedFlags := make(map[string][]*pflag.Flag)
	uncategorizedFlags := []*pflag.Flag{}

	cmd.LocalFlags().VisitAll(func(flag *pflag.Flag) {
		categorized := false

		for category, flagNames := range flagCategories {
			if slices.Contains(flagNames, flag.Name) {
				categorizedFlags[category] = append(categorizedFlags[category], flag)
				categorized = true
			}

			if categorized {
				break
			}
		}

		if !categorized {
			uncategorizedFlags = append(uncategorizedFlags, flag)
		}
	})

	// Print categorized flags
	categoryOrder := []string{
		"Build Behavior",
		"Dependency Management",
		"Version Control",
		"Source Access",
		"Build Range Control",
	}

	for _, category := range categoryOrder {
		if flags, exists := categorizedFlags[category]; exists && len(flags) > 0 {
			fmt.Printf("\n%s:\n", color.BoldCyan(category))

			for _, flag := range flags {
				printFlag(flag)
			}
		}
	}

	// Print uncategorized flags
	if len(uncategorizedFlags) > 0 {
		fmt.Printf("\n%s:\n", color.BoldMagenta("Other Options"))

		for _, flag := range uncategorizedFlags {
			printFlag(flag)
		}
	}
}

// printFlag prints a single flag with enhanced formatting.
func printFlag(flag *pflag.Flag) {
	// Build flag display string
	flagStr := "  --" + flag.Name
	if flag.Shorthand != "" {
		flagStr = "  -" + flag.Shorthand + ", --" + flag.Name
	}

	// Add value type if needed
	if flag.Value.Type() != "bool" {
		flagStr += " " + strings.ToUpper(flag.Value.Type())
	}

	// Color the flag name
	coloredFlagStr := color.HiBlue(flagStr)

	// Format description with default value
	description := flag.Usage
	if flag.DefValue != "" && flag.DefValue != "false" {
		description += fmt.Sprintf(" (default: %s)", flag.DefValue)
	}

	fmt.Printf("%s\n      %s\n", coloredFlagStr, description)
}

// EnhancedHelpFunc provides a completely custom help function.
func EnhancedHelpFunc(cmd *cobra.Command, _ []string) {
	_ = CustomUsageFunc(cmd)
}
