// Package parser provides PKGBUILD parsing and processing functionality.
package parser //nolint:revive // intentional name; conflicts with stdlib go/parser but scope is unambiguous

import (
	"os"
	"path/filepath"

	"mvdan.cc/sh/v3/shell"
	"mvdan.cc/sh/v3/syntax"

	"github.com/M0Rf30/yap/v2/pkg/files"
	"github.com/M0Rf30/yap/v2/pkg/i18n"
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
		logger.Error(i18n.T("logger.parsefile.error.failed_to_get_root_1"),
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

	err = parseSyntaxFile(pkgbuildSyntax, pkgBuild)
	if err != nil {
		return nil, err
	}

	pkgBuild.SetMainFolders()

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
//
// It takes in a pkgbuildSyntax object and a pkgBuild object as parameters.
// It returns an error if any error occurs during parsing.
func parseSyntaxFile(pkgbuildSyntax *syntax.File, pkgBuild *pkgbuild.PKGBUILD) error {
	// First pass: collect top-level variables and arrays (does NOT recurse into functions)
	err := collectVariablesAndArrays(pkgbuildSyntax, pkgBuild)
	if err != nil {
		return err
	}

	// Second pass: process function declarations
	return processFunctions(pkgbuildSyntax, pkgBuild)
}

func collectVariablesAndArrays(pkgbuildSyntax *syntax.File, pkgBuild *pkgbuild.PKGBUILD) error {
	var (
		err       error
		arrayDecl []string
		varDecl   string
	)

	syntax.Walk(pkgbuildSyntax, func(node syntax.Node) bool {
		// Do NOT recurse into function bodies — assignments inside functions are
		// local and must not be treated as top-level PKGBUILD variables.
		if _, ok := node.(*syntax.FuncDecl); ok {
			return false
		}

		if nodeType, ok := node.(*syntax.Assign); ok {
			if nodeType.Array != nil {
				for _, line := range set.StringifyArray(nodeType) {
					arrayDecl, _ = shell.Fields(line, os.Getenv)
				}

				err = pkgBuild.AddItem(nodeType.Name.Value, arrayDecl)
			} else {
				varDecl, _ = shell.Expand(set.StringifyAssign(nodeType), os.Getenv)
				err = pkgBuild.AddItem(nodeType.Name.Value, varDecl)
			}
		}

		return true
	})

	return err
}

func processFunctions(pkgbuildSyntax *syntax.File, pkgBuild *pkgbuild.PKGBUILD) error {
	var err error

	syntax.Walk(pkgbuildSyntax, func(node syntax.Node) bool {
		if nodeType, ok := node.(*syntax.FuncDecl); ok {
			// Store the raw function body wrapped in pkgbuild.FuncBody so that
			// mapFunctions can distinguish it from plain string variables.
			// Variables will be resolved at runtime via the preamble emitted by
			// BuildScriptPreamble() and the environment variables set by
			// SetEnvironmentVariables().
			funcDecl := set.StringifyFuncDecl(nodeType)
			err = pkgBuild.AddItem(nodeType.Name.Value, pkgbuild.FuncBody(funcDecl))

			// Do not recurse into nested function declarations.
			return false
		}

		return true
	})

	return err
}
