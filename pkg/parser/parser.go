package parser

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/M0Rf30/yap/pkg/pkgbuild"
	"github.com/M0Rf30/yap/pkg/utils"
	"github.com/otiai10/copy"
	"mvdan.cc/sh/v3/shell"
	"mvdan.cc/sh/v3/syntax"
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
	copyOpt := copy.Options{
		OnSymlink: func(_ string) copy.SymlinkAction {
			return copy.Skip
		},
		Skip: func(_ os.FileInfo, src, _ string) (bool, error) {
			if strings.HasSuffix(src, ".apk") ||
				strings.HasSuffix(src, ".deb") ||
				strings.HasSuffix(src, ".pkg.tar.zst") ||
				strings.HasSuffix(src, ".rpm") {
				return true, nil
			}

			return false, nil
		},
	}

	if err != nil {
		utils.Logger.Error("failed to get root directory",
			utils.Logger.Args("path", home))

		return nil, err
	}

	pkgBuild := &pkgbuild.PKGBUILD{
		Distro:    distro,
		Codename:  release,
		StartDir:  startDir,
		Home:      home,
		SourceDir: filepath.Join(startDir, "src"),
	}

	err = utils.ExistsMakeDir(startDir)
	if err != nil {
		return pkgBuild, err
	}

	if home != startDir {
		err = copy.Copy(home, startDir, copyOpt)
	}

	if err != nil {
		return pkgBuild, err
	}

	pkgBuild.Init()

	pkgbuildSyntax, err := getSyntaxFile(startDir)
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
// It takes a startDir string as a parameter and returns a *syntax.File and an error.
func getSyntaxFile(startDir string) (*syntax.File, error) {
	filePath := filepath.Join(startDir, "PKGBUILD")
	file, err := utils.Open(filePath)

	if err != nil {
		return nil, err
	}

	defer file.Close()

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
	var err error

	var arrayDecl []string

	var funcDecl string

	var varDecl string

	syntax.Walk(pkgbuildSyntax, func(node syntax.Node) bool {
		switch nodeType := node.(type) {
		case *syntax.Assign:
			if nodeType.Array != nil {
				for _, line := range utils.StringifyArray(nodeType) {
					arrayDecl, _ = shell.Fields(line, os.Getenv)
				}

				err = pkgBuild.AddItem(nodeType.Name.Value, arrayDecl)
			} else {
				varDecl, _ = shell.Expand(utils.StringifyAssign(nodeType), os.Getenv)
				err = pkgBuild.AddItem(nodeType.Name.Value, varDecl)
			}
		case *syntax.FuncDecl:
			funcDecl = utils.StringifyFuncDecl(nodeType)
			err = pkgBuild.AddItem(nodeType.Name.Value, funcDecl)
		}

		return true
	})

	return err
}
