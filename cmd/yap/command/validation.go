package command

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/M0Rf30/yap/v2/pkg/constants"
	yapErrors "github.com/M0Rf30/yap/v2/pkg/errors"
	"github.com/M0Rf30/yap/v2/pkg/i18n"
	"github.com/M0Rf30/yap/v2/pkg/logger"
	"github.com/M0Rf30/yap/v2/pkg/platform"
	"github.com/M0Rf30/yap/v2/pkg/shell"
)

// Static error definitions to satisfy err113 linter.
var (
	ErrDistributionEmpty   = errors.New(i18n.T("errors.validation.distribution_empty"))
	ErrProjectPathEmpty    = errors.New(i18n.T("errors.validation.project_path_empty"))
	ErrPathNotExist        = errors.New(i18n.T("errors.validation.path_not_exist"))
	ErrProjectFileNotFound = errors.New(i18n.T("errors.validation.project_file_not_found"))
	ErrInsufficientArgs    = errors.New(i18n.T("errors.validation.insufficient_args"))
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

	return yapErrors.Wrap(ErrDistributionEmpty,
		yapErrors.ErrTypeValidation,
		i18n.T("errors.validation.unsupported_distribution")).
		WithOperation("validateDistroArg").
		WithContext("distro", distro)
}

// validateProjectPath validates that the project path exists and contains yap.json or PKGBUILD.
func validateProjectPath(path string) error {
	if path == "" {
		return ErrProjectPathEmpty
	}

	absPath, err := filepath.Abs(path)
	if err != nil {
		return yapErrors.Wrap(err, yapErrors.ErrTypeValidation,
			i18n.T("errors.validation.invalid_path")).
			WithOperation("validateProjectPath").
			WithContext("path", path)
	}

	// Check if path exists
	_, err = os.Stat(absPath)
	if os.IsNotExist(err) {
		return yapErrors.Wrap(ErrPathNotExist,
			yapErrors.ErrTypeFileSystem,
			i18n.T("errors.validation.path_not_exist")).
			WithOperation("validateProjectPath").
			WithContext("path", absPath)
	}

	// Check for yap.json file (multiproject)
	yapJSONPath := filepath.Join(absPath, "yap.json")
	// Check for PKGBUILD file (single project)
	pkgbuildPath := filepath.Join(absPath, "PKGBUILD")

	_, yapJSONErr := os.Stat(yapJSONPath)
	_, pkgbuildErr := os.Stat(pkgbuildPath)

	if os.IsNotExist(yapJSONErr) && os.IsNotExist(pkgbuildErr) {
		return yapErrors.Wrap(ErrProjectFileNotFound,
			yapErrors.ErrTypeFileSystem,
			i18n.T("errors.validation.no_project_files")).
			WithOperation("validateProjectPath").
			WithContext("path", absPath)
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
			return yapErrors.Wrap(ErrInsufficientArgs,
				yapErrors.ErrTypeValidation,
				i18n.T("errors.validation.insufficient_args_detailed")).
				WithOperation("createValidateDistroArgs").
				WithContext("minArgs", minArgs).
				WithContext("providedArgs", len(args)).
				WithContext("command", cmd.CommandPath())
		}

		if err := validateDistroForCommand(cmd, args); err != nil {
			return err
		}

		return validatePathForCommand(cmd, args)
	}
}

// validateDistroForCommand validates distro argument based on command type.
func validateDistroForCommand(cmd *cobra.Command, args []string) error {
	if cmd.Name() == buildCommand && len(args) >= 1 {
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
	if len(args) >= 2 || (len(args) == 1 && cmd.Name() == buildCommand) {
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
		logger.Info(i18n.T("messages.verbose_mode_enabled"), "command", cmd.Name())
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
	return "", "", "", yapErrors.New(yapErrors.ErrTypeValidation,
		fmt.Sprintf(
			"argument '%s' is neither a valid distribution nor a valid project path",
			firstArg)).
		WithOperation("parseSingleArg").
		WithContext("argument", firstArg)
}

// isPathArgument checks if the argument looks like a path.
func isPathArgument(arg string) bool {
	return strings.Contains(arg, "/") || arg == "." || arg == ".."
}

// ResolveDistroRelease auto-detects the distribution and codename from
// /etc/os-release when not explicitly provided.
//
// Behavior:
//   - If distro is empty: the host distro and codename are used; a warning
//     is logged using noDistroKey (an i18n key for "no distribution specified").
//   - If distro is set but release is empty: the value is treated as an
//     explicitly-requested bare distro family (generic). The codename is NOT
//     back-filled from the host, so format-specific suffixes fall back to the
//     distro name (e.g. a deb release stays `1ubuntu` instead of `1jammy`).
//
// This keeps `build`, `zap`, and `prepare` consistent: host-codename
// auto-detection only kicks in when no distro argument was provided.
func ResolveDistroRelease(distro, release, noDistroKey string) (
	resolvedDistro, resolvedRelease string,
) {
	if distro == "" {
		osRelease, _ := platform.ParseOSRelease()
		distro = osRelease.ID

		if release == "" {
			release = osRelease.Codename
		}

		logger.Warn(i18n.T(noDistroKey), "distro", distro)

		return distro, release
	}

	// An explicitly-passed bare distro family (e.g. "ubuntu") stays generic:
	// the codename is intentionally left empty so the package suffix falls
	// back to the distro name. Do not back-fill from the host /etc/os-release.
	return distro, release
}

// ResolveFlexibleDistro parses "distro[-release] [path]" style arguments
// (via ParseFlexibleArgs) and auto-detects the distro/release from
// /etc/os-release when missing (via ResolveDistroRelease). userProvided
// reports whether the caller explicitly named a distro — used by `build`
// and `zap` to decide on container dispatch and logging.
func ResolveFlexibleDistro(args []string, noDistroKey string) (
	distro, release, fullJSONPath string, userProvided bool, err error,
) {
	distro, release, fullJSONPath, err = ParseFlexibleArgs(args)
	if err != nil {
		return "", "", "", false, err
	}

	userProvided = distro != ""
	distro, release = ResolveDistroRelease(distro, release, noDistroKey)

	return distro, release, fullJSONPath, userProvided, nil
}
