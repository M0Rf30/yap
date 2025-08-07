package command

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/M0Rf30/yap/v2/pkg/constants"
	"github.com/M0Rf30/yap/v2/pkg/osutils"
)

// Static error definitions to satisfy err113 linter.
var (
	ErrDistributionEmpty = errors.New("distribution cannot be empty")
	ErrProjectPathEmpty  = errors.New("project path cannot be empty")
	ErrPathNotExist      = errors.New("path does not exist")
	ErrYapJSONNotFound   = errors.New("yap.json not found")
	ErrInsufficientArgs  = errors.New("requires at least one argument")
)

// ValidDistrosCompletion provides completion for valid distributions.
func ValidDistrosCompletion(_ *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	if len(args) > 0 {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}

	var completions []string

	// Extract base distro from partial input (e.g., "ubuntu" from "ubuntu-foc")
	baseDist := strings.Split(toComplete, "-")[0]

	for _, release := range &constants.Releases {
		// Match if the release name starts with the base distro
		if strings.HasPrefix(release, baseDist) {
			// If user typed just base name (e.g., "ubuntu"), suggest the base name
			if toComplete == baseDist || toComplete == "" {
				completions = append(completions, release)
			} else if strings.HasPrefix(toComplete, release) {
				// If user typed something like "ubuntu-", suggest with common suffixes
				// This is just for basic completion, they can type any suffix
				completions = append(completions, toComplete)
			}
		}
	}

	return completions, cobra.ShellCompDirectiveNoFileComp
}

// ProjectPathCompletion provides completion for project paths (directories with yap.json).
func ProjectPathCompletion(_ *cobra.Command, _ []string, _ string) ([]string, cobra.ShellCompDirective) {
	return nil, cobra.ShellCompDirectiveFilterDirs
}

// validateDistroArg validates distribution argument format and availability.
func validateDistroArg(distro string) error {
	if distro == "" {
		return ErrDistributionEmpty
	}

	// Extract base distribution name (everything before the first hyphen)
	baseDist := strings.Split(distro, "-")[0]

	// Check if base distro exists in supported releases
	for _, release := range &constants.Releases {
		if release == baseDist {
			return nil
		}
	}

	return fmt.Errorf("unsupported distribution '%s'\n\n"+
		"Supported distributions:\n%s\n\n"+
		"Use 'yap list-distros' to see all available options: %w",
		distro, formatDistroSuggestions(baseDist), ErrDistributionEmpty)
}

// validateProjectPath validates that the project path exists and contains yap.json.
func validateProjectPath(path string) error {
	if path == "" {
		return ErrProjectPathEmpty
	}

	absPath, err := filepath.Abs(path)
	if err != nil {
		return fmt.Errorf("invalid path '%s': %w", path, err)
	}

	// Check if path exists
	_, err = os.Stat(absPath)
	if os.IsNotExist(err) {
		return fmt.Errorf("%s: %w", absPath, ErrPathNotExist)
	}

	// Check for yap.json file
	yapJSONPath := filepath.Join(absPath, "yap.json")

	_, err = os.Stat(yapJSONPath)
	if os.IsNotExist(err) {
		return fmt.Errorf("yap.json not found in %s\n\n"+
			"Make sure you're in a YAP project directory or specify the correct path: %w",
			absPath, ErrYapJSONNotFound)
	}

	return nil
}

// formatDistroSuggestions returns formatted suggestions for similar distributions.
func formatDistroSuggestions(input string) string {
	var suggestions []string

	input = strings.ToLower(input)

	// Find similar distributions
	for _, release := range &constants.Releases {
		if strings.Contains(strings.ToLower(release), input) {
			suggestions = append(suggestions, "  • "+release)
		}
	}

	if len(suggestions) == 0 {
		// Show first few distributions as examples
		for i, release := range &constants.Releases {
			if i >= 5 {
				break
			}

			suggestions = append(suggestions, "  • "+release)
		}

		return strings.Join(suggestions, "\n") + "\n  • ..."
	}

	return strings.Join(suggestions, "\n")
}

// createValidateDistroArgs creates a validation function for distro arguments.
func createValidateDistroArgs(minArgs int) cobra.PositionalArgs {
	return func(cmd *cobra.Command, args []string) error {
		if len(args) < minArgs {
			return fmt.Errorf("requires at least %d argument(s), only received %d\n\nUse '%s --help' for usage information: %w",
				minArgs, len(args), cmd.CommandPath(), ErrInsufficientArgs)
		}

		// For commands with distro as first argument
		if minArgs > 0 {
			distro := args[0]

			err := validateDistroArg(distro)
			if err != nil {
				return err
			}
		}

		// For commands with path as last argument (build, zap)
		if len(args) >= 2 || (len(args) == 1 && cmd.Name() == "build") {
			pathArg := args[len(args)-1]

			err := validateProjectPath(pathArg)
			if err != nil {
				return err
			}
		}

		return nil
	}
}

// PreRunValidation provides pre-run validation with interactive prompts for missing arguments.
func PreRunValidation(cmd *cobra.Command, _ []string) {
	// Set verbose logging early
	osutils.SetVerbose(verbose)

	// Additional pre-run setup can go here
	if verbose {
		osutils.Logger.Info("verbose mode enabled", osutils.Logger.Args("command", cmd.Name()))
	}
}
