package command

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/M0Rf30/yap/v2/pkg/constants"
	"github.com/M0Rf30/yap/v2/pkg/logger"
	"github.com/M0Rf30/yap/v2/pkg/shell"
)

// Static error definitions to satisfy err113 linter.
var (
	ErrDistributionEmpty   = errors.New("distribution cannot be empty")
	ErrProjectPathEmpty    = errors.New("project path cannot be empty")
	ErrPathNotExist        = errors.New("path does not exist")
	ErrProjectFileNotFound = errors.New("project file not found")
	ErrInsufficientArgs    = errors.New("requires at least one argument")
)

// ValidDistrosCompletion provides completion for valid distributions.
func ValidDistrosCompletion(_ *cobra.Command, args []string, toComplete string) (
	[]string, cobra.ShellCompDirective) {
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

// ProjectPathCompletion provides completion for project paths
// (directories with yap.json or PKGBUILD).
func ProjectPathCompletion(_ *cobra.Command, _ []string, _ string) (
	[]string, cobra.ShellCompDirective) {
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

// validateProjectPath validates that the project path exists and contains yap.json or PKGBUILD.
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

	// Check for yap.json file (multiproject)
	yapJSONPath := filepath.Join(absPath, "yap.json")
	// Check for PKGBUILD file (single project)
	pkgbuildPath := filepath.Join(absPath, "PKGBUILD")

	_, yapJSONErr := os.Stat(yapJSONPath)
	_, pkgbuildErr := os.Stat(pkgbuildPath)

	if os.IsNotExist(yapJSONErr) && os.IsNotExist(pkgbuildErr) {
		return fmt.Errorf("neither yap.json nor PKGBUILD found in %s\n\n"+
			"Make sure you're in a YAP project directory (containing yap.json for "+
			"multiproject or PKGBUILD for single project) or specify the correct path: %w",
			absPath, ErrProjectFileNotFound)
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
			return fmt.Errorf("requires at least %d argument(s), only received %d\n\n"+
				"Use '%s --help' for usage information: %w",
				minArgs, len(args), cmd.CommandPath(), ErrInsufficientArgs)
		}

		if err := validateDistroForCommand(cmd, args); err != nil {
			return err
		}

		return validatePathForCommand(cmd, args)
	}
}

// validateDistroForCommand validates distro argument based on command type.
func validateDistroForCommand(cmd *cobra.Command, args []string) error {
	if cmd.Name() == "build" && len(args) >= 1 {
		return validateDistroForBuildCommand(args[0])
	}

	if len(args) >= 1 {
		return validateDistroArg(args[0])
	}

	return nil
}

// validateDistroForBuildCommand handles distro validation for build command.
func validateDistroForBuildCommand(firstArg string) error {
	// If first argument looks like a path, don't validate as distro
	if isPathLike(firstArg) {
		return nil
	}

	// Try to validate as distro
	if err := validateDistroArg(firstArg); err != nil {
		// If distro validation fails, check if it might be a path
		if validateProjectPath(firstArg) == nil {
			return nil // It's a valid path
		}

		return err // Not a valid path either, return original distro error
	}

	return nil
}

// isPathLike checks if a string looks like a file path.
func isPathLike(arg string) bool {
	return strings.Contains(arg, "/") || arg == "." || arg == ".."
}

// validatePathForCommand validates path arguments for commands.
func validatePathForCommand(cmd *cobra.Command, args []string) error {
	// For commands with path as last argument (build, zap)
	if len(args) >= 2 || (len(args) == 1 && cmd.Name() == "build") {
		pathArg := args[len(args)-1]
		return validateProjectPath(pathArg)
	}

	return nil
}

// PreRunValidation provides pre-run validation with interactive prompts for missing arguments.
func PreRunValidation(cmd *cobra.Command, _ []string) {
	// Set verbose logging early
	shell.SetVerbose(verbose)

	// Additional pre-run setup can go here
	if verbose {
		logger.Info("verbose mode enabled", "command", cmd.Name())
	}
}

// ParseFlexibleArgs parses flexible command arguments for build and zap commands
// that support both "command path" and "command distro path" formats.
// Returns (distro, release, fullJSONPath, error).
func ParseFlexibleArgs(args []string) (distro, release, fullJSONPath string, err error) {
	fullJSONPath, _ = filepath.Abs(args[len(args)-1]) // Always take the last argument as path

	if len(args) == 2 {
		distro, release = parseDistroAndRelease(args[0])
		return distro, release, fullJSONPath, nil
	}

	if len(args) == 1 {
		return parseSingleArg(args[0])
	}

	return "", "", fullJSONPath, nil
}

// parseDistroAndRelease parses distro-release format and returns distro and release.
func parseDistroAndRelease(arg string) (distro, release string) {
	split := strings.Split(arg, "-")
	distro = split[0]

	if len(split) > 1 {
		release = split[1]
	}

	return distro, release
}

// parseSingleArg parses a single argument that could be path or distro.
func parseSingleArg(firstArg string) (distro, release, fullJSONPath string, err error) {
	// Check if it's a path (contains /, is . or .., or is a valid project path)
	if isPathArgument(firstArg) {
		fullJSONPath, _ = filepath.Abs(firstArg)
		return "", "", fullJSONPath, nil
	}

	// Check if it's a valid project path
	if validateProjectPath(firstArg) == nil {
		fullJSONPath, _ = filepath.Abs(firstArg)
		return "", "", fullJSONPath, nil
	}

	// Try to validate as distro
	if validateDistroArg(firstArg) == nil {
		distro, release = parseDistroAndRelease(firstArg)
		fullJSONPath, _ = filepath.Abs(".")

		return distro, release, fullJSONPath, nil
	}

	// Neither valid distro nor valid path
	return "", "", "", fmt.Errorf(
		"argument '%s' is neither a valid distribution nor a valid project path",
		firstArg)
}

// isPathArgument checks if the argument looks like a path.
func isPathArgument(arg string) bool {
	return strings.Contains(arg, "/") || arg == "." || arg == ".."
}
