package parser

import (
	"fmt"
	"path/filepath"

	"github.com/M0Rf30/yap/constants"
	"github.com/M0Rf30/yap/pkgbuild"
	"github.com/M0Rf30/yap/utils"
	"github.com/otiai10/copy"
	"mvdan.cc/sh/v3/shell"
	"mvdan.cc/sh/v3/syntax"
)

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

	env := func(name string) string {
		switch name {
		case "pkgname":
			return pkgbuild.PkgName
		case "pkgver":
			return pkgbuild.PkgVer
		case "pkgrel":
			return pkgbuild.PkgVer
		case "pkgdir":
			if pkgbuild.Distro == "arch" {
				return filepath.Join(pkgbuild.PackageDir, pkgbuild.PkgName)
			}

			return pkgbuild.PackageDir
		case "srcdir":
			return pkgbuild.SourceDir
		case "url":
			return pkgbuild.URL
		default:
			return ""
		}
	}

	syntax.Walk(pkgbuildSyntax, func(node syntax.Node) bool {
		switch nodeType := node.(type) {
		case *syntax.Assign:
			if nodeType.Array != nil {
				for _, line := range utils.StringifyArray(nodeType) {
					arrayDecl, _ = shell.Fields(line, env)
				}
				err = pkgbuild.AddItem(nodeType.Name.Value, arrayDecl)
			} else {
				varDecl, _ = shell.Expand(utils.StringifyAssign(nodeType), env)
				err = pkgbuild.AddItem(nodeType.Name.Value, varDecl)
			}
		case *syntax.FuncDecl:
			for _, line := range utils.StringifyFuncDecl(nodeType) {
				funcDecl, _ = shell.Expand(line, env)
			}
			err = pkgbuild.AddItem(nodeType.Name.Value, funcDecl)
		}

		return true
	})

	if err != nil {
		return err
	}

	return err
}

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
		CodeName:   release,
		StartDir:   startDir,
		Home:       home,
		SourceDir:  filepath.Join(startDir, "src"),
		PackageDir: filepath.Join(startDir, "pkg"),
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

	return pkgbuild, err
}
