// Package parser provides PKGBUILD parsing and processing functionality.
package parser

import (
	"maps"
	"os"
	"path/filepath"
	"strings"

	"mvdan.cc/sh/v3/shell"
	"mvdan.cc/sh/v3/syntax"

	"github.com/M0Rf30/yap/v2/pkg/files"
	"github.com/M0Rf30/yap/v2/pkg/logger"
	"github.com/M0Rf30/yap/v2/pkg/pkgbuild"
	"github.com/M0Rf30/yap/v2/pkg/set"
)

// OverridePkgRel is a variable that allows overriding the Pkgrel field in
// PKGBUILD. This can be useful for setting a custom package revision in
// CI, as a timestamp, for example.
var OverridePkgRel string

// OverridePkgVer is a variable that allows overriding the PkgVer field in
// PKGBUILD. This can be useful for setting a custom package version.
var OverridePkgVer string

// ParseFile parses a file and returns a PKGBUILD object and an error.
//
// Parameters:
// - distro: the distribution name.
// - release: the release version.
// - startDir: the starting directory.
// - home: the home directory.
//
// Returns:
// - *pkgbuild.PKGBUILD: the parsed PKGBUILD object.
// - error: an error if any occurred during parsing.
func ParseFile(distro, release, startDir, home string) (*pkgbuild.PKGBUILD, error) {
	home, err := filepath.Abs(home)
	if err != nil {
		logger.Error("failed to get root directory",
			"path", home)

		return nil, err
	}

	pkgBuild := &pkgbuild.PKGBUILD{
		Distro:    distro,
		Codename:  release,
		StartDir:  startDir,
		Home:      home,
		SourceDir: filepath.Join(startDir, "src"),
	}

	pkgBuild.Init()

	pkgbuildSyntax, err := getSyntaxFile(home)
	if err != nil {
		return nil, err
	}

	pkgBuild.SetMainFolders()

	err = parseSyntaxFile(pkgbuildSyntax, pkgBuild)
	if err != nil {
		return nil, err
	}

	if OverridePkgRel != "" {
		pkgBuild.PkgRel = OverridePkgRel
	}

	if OverridePkgVer != "" {
		pkgBuild.PkgVer = OverridePkgVer
	}

	return pkgBuild, err
}

// getSyntaxFile returns a syntax.File and an error.
//
// It takes a path string as a parameter and returns a *syntax.File and an error.
func getSyntaxFile(path string) (*syntax.File, error) {
	filePath := filepath.Join(path, "PKGBUILD")

	file, err := files.Open(filePath)
	if err != nil {
		return nil, err
	}

	defer func() {
		err := file.Close()
		if err != nil {
			logger.Warn(
				"failed to close PKGBUILD file",
				"path", filePath, "error", err)
		}
	}()

	pkgbuildParser := syntax.NewParser(syntax.Variant(syntax.LangBash))

	pkgbuildSyntax, err := pkgbuildParser.Parse(file, filePath)
	if err != nil {
		return nil, err
	}

	return pkgbuildSyntax, nil
}

// parseSyntaxFile parses the given pkgbuildSyntax and populates the pkgBuild object.
// Enhanced version with support for custom variables in build() and package() functions.
//
// It takes in a pkgbuildSyntax object and a pkgBuild object as parameters.
// It returns an error if any error occurs during parsing.
func parseSyntaxFile(pkgbuildSyntax *syntax.File, pkgBuild *pkgbuild.PKGBUILD) error {
	customVars := make(map[string]string)

	// First pass: collect variables and arrays
	err := collectVariablesAndArrays(pkgbuildSyntax, pkgBuild, customVars)
	if err != nil {
		return err
	}

	// Second pass: process functions with enhanced variable support
	return processFunctions(pkgbuildSyntax, pkgBuild, customVars)
}

func collectVariablesAndArrays(pkgbuildSyntax *syntax.File, pkgBuild *pkgbuild.PKGBUILD,
	customVars map[string]string,
) error {
	var (
		err       error
		arrayDecl []string
		varDecl   string
	)

	syntax.Walk(pkgbuildSyntax, func(node syntax.Node) bool {
		if nodeType, ok := node.(*syntax.Assign); ok {
			if nodeType.Array != nil {
				for _, line := range set.StringifyArray(nodeType) {
					arrayDecl, _ = shell.Fields(line, os.Getenv)
				}

				err = pkgBuild.AddItem(nodeType.Name.Value, arrayDecl)
				// Store array elements as unquoted space-separated string for proper bash expansion
				// This allows "${array_name[@]}" to expand to individual elements
				customVars[nodeType.Name.Value] = strings.Join(arrayDecl, " ")
			} else {
				varDecl, _ = shell.Expand(set.StringifyAssign(nodeType), os.Getenv)
				customVars[nodeType.Name.Value] = varDecl
				err = pkgBuild.AddItem(nodeType.Name.Value, varDecl)
			}
		}

		return true
	})

	return err
}

func processFunctions(
	pkgbuildSyntax *syntax.File, pkgBuild *pkgbuild.PKGBUILD, customVars map[string]string) error {
	var err error

	syntax.Walk(pkgbuildSyntax, func(node syntax.Node) bool {
		if nodeType, ok := node.(*syntax.FuncDecl); ok {
			funcDecl := set.StringifyFuncDecl(nodeType)

			// Pre-process array expansions before any other processing
			expandedFunc := preprocessArrayExpansions(funcDecl, customVars)

			// Only expand known variables, preserve runtime variables like ${bin}
			expandedFunc = expandKnownVariables(expandedFunc, customVars, pkgBuild)

			err = pkgBuild.AddItem(nodeType.Name.Value, expandedFunc)
		}

		return true
	})

	return err
}

// preprocessArrayExpansions handles bash array syntax like "${array[@]}" before shell.Expand.
func preprocessArrayExpansions(funcDecl string, customVars map[string]string) string {
	// Handle "${array[@]}" syntax
	for arrayName, arrayValue := range customVars {
		// Look for "${arrayName[@]}" patterns
		pattern := `"${` + arrayName + `[@]}"`
		if strings.Contains(funcDecl, pattern) {
			// Replace with unquoted space-separated values for proper bash expansion
			funcDecl = strings.ReplaceAll(funcDecl, pattern, arrayValue)
		}
	}

	return funcDecl
}

// expandKnownVariables expands only variables we know about, leaving runtime variables intact.
func expandKnownVariables(
	funcDecl string, customVars map[string]string, pkgBuild *pkgbuild.PKGBUILD) string {
	// Create a map of all known variables
	knownVars := make(map[string]string)

	// Add custom variables
	maps.Copy(knownVars, customVars)

	// Add PKGBUILD standard variables
	knownVars["pkgdir"] = pkgBuild.PackageDir
	knownVars["srcdir"] = pkgBuild.SourceDir
	knownVars["startdir"] = pkgBuild.StartDir
	knownVars["pkgname"] = pkgBuild.PkgName
	knownVars["pkgver"] = pkgBuild.PkgVer
	knownVars["pkgrel"] = pkgBuild.PkgRel
	knownVars["epoch"] = pkgBuild.Epoch
	knownVars["url"] = pkgBuild.URL
	knownVars["maintainer"] = pkgBuild.Maintainer

	// Only expand variables that we know about
	for varName, varValue := range knownVars {
		if varValue != "" {
			// Replace ${varName} with varValue
			pattern := "${" + varName + "}"
			if strings.Contains(funcDecl, pattern) {
				funcDecl = strings.ReplaceAll(funcDecl, pattern, varValue)
			}
		}
	}

	return funcDecl
}
