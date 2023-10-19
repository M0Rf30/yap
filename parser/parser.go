package parser

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/M0Rf30/yap/constants"
	"github.com/M0Rf30/yap/pkgbuild"
	"github.com/M0Rf30/yap/utils"
	"github.com/otiai10/copy"
	"mvdan.cc/sh/v3/shell"
	"mvdan.cc/sh/v3/syntax"
)

var OverridePkgver string

func ParseFile(distro, release, startDir, home string) (*pkgbuild.PKGBUILD, error) {
	home, err := filepath.Abs(home)

	if err != nil {
		fmt.Printf("%s❌ :: %sfailed to get root directory from %s\n",
			string(constants.ColorBlue),
			string(constants.ColorYellow), home)

		return nil, err
	}

	pkgbuild := &pkgbuild.PKGBUILD{
		Distro:     distro,
		Codename:   release,
		StartDir:   startDir,
		Home:       home,
		SourceDir:  filepath.Join(startDir, "src"),
		PackageDir: filepath.Join(startDir, "staging"),
	}

	err = utils.ExistsMakeDir(startDir)
	if err != nil {
		return pkgbuild, err
	}

	err = copy.Copy(home, startDir)
	if err != nil {
		return pkgbuild, err
	}

	pkgbuild.Init()

	pkgbuildSyntax, err := getSyntaxFile(startDir, home)
	if err != nil {
		return nil, err
	}

	err = parseSyntaxFile(pkgbuildSyntax, pkgbuild)
	if err != nil {
		return nil, err
	}

	if OverridePkgver != "" {
		pkgbuild.PkgVer = OverridePkgver
	}

	return pkgbuild, err
}

func getSyntaxFile(startDir, home string) (*syntax.File, error) {
	file, err := utils.Open(filepath.Join(startDir, "PKGBUILD"))
	if err != nil {
		return nil, err
	}
	defer file.Close()

	pkgbuildParser := syntax.NewParser(syntax.Variant(syntax.LangBash))
	pkgbuildSyntax, err := pkgbuildParser.Parse(file, home+"/PKGBUILD")

	if err != nil {
		return nil, err
	}

	return pkgbuildSyntax, err
}

func parseSyntaxFile(pkgbuildSyntax *syntax.File, pkgbuild *pkgbuild.PKGBUILD) error {
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
				err = pkgbuild.AddItem(nodeType.Name.Value, arrayDecl)
			} else {
				varDecl, _ = shell.Expand(utils.StringifyAssign(nodeType), os.Getenv)
				err = pkgbuild.AddItem(nodeType.Name.Value, varDecl)
			}
		case *syntax.FuncDecl:
			funcDecl = utils.StringifyFuncDecl(nodeType)
			err = pkgbuild.AddItem(nodeType.Name.Value, funcDecl)
		}

		return true
	})

	if err != nil {
		return err
	}

	return err
}
