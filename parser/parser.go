package parser

import (
	"fmt"
	"path/filepath"

	"github.com/M0Rf30/yap/pkgbuild"
	"github.com/M0Rf30/yap/utils"
	"mvdan.cc/sh/v3/shell"
	"mvdan.cc/sh/v3/syntax"
)

func File(distro, release, compiledOutput, home string) (*pkgbuild.PKGBUILD, error) {
	home, err := filepath.Abs(home)

	path := filepath.Join(compiledOutput, "PKGBUILD")

	pac := &pkgbuild.PKGBUILD{
		Distro:         distro,
		FullDistroName: release,
		Root:           compiledOutput,
		Home:           home,
		SourceDir:      filepath.Join(compiledOutput, "src"),
		PackageDir:     filepath.Join(compiledOutput, "pkg"),
	}

	if err != nil {
		fmt.Printf("parse: Failed to get root directory from '%s'\n",
			home)

		return pac, err
	}

	err = utils.ExistsMakeDir(compiledOutput)
	if err != nil {
		return pac, err
	}

	err = utils.CopyFiles(home, compiledOutput, false)
	if err != nil {
		return pac, err
	}

	pac.Init()

	file, err := utils.Open(path)
	if err != nil {
		return pac, err
	}
	defer file.Close()

	pkgbuildParser := syntax.NewParser(syntax.Variant(syntax.LangBash))
	pkgbuildSyntax, err := pkgbuildParser.Parse(file, home+"/PKGBUILD")

	if err != nil {
		return nil, err
	}

	env := func(name string) string {
		switch name {
		case "pkgname":
			return pac.PkgName
		case "pkgver":
			return pac.PkgVer
		case "pkgrel":
			return pac.PkgVer
		case "pkgdir":
			return pac.PackageDir
		case "srcdir":
			return pac.SourceDir
		case "url":
			return pac.URL
		default:
			return pac.Variables[name]
		}
	}

	var arrayDecl []string

	var funcDecl string

	var varDecl string

	syntax.Walk(pkgbuildSyntax, func(node syntax.Node) bool {
		switch nodeType := node.(type) {
		case *syntax.Assign:
			if nodeType.Array != nil {
				for _, line := range utils.StringifyArray(nodeType) {
					arrayDecl, _ = shell.Fields(line, env)
				}
				err = pac.AddItem(nodeType.Name.Value, arrayDecl)
			} else {
				varDecl, _ = shell.Expand(utils.StringifyAssign(nodeType), env)
				err = pac.AddItem(nodeType.Name.Value, varDecl)
			}

			if err != nil {
				return true
			}

		case *syntax.FuncDecl:
			for _, line := range utils.StringifyFuncDecl(nodeType) {
				funcDecl, _ = shell.Expand(line, env)
			}
			err = pac.AddItem(nodeType.Name.Value, funcDecl)

			if err != nil {
				return true
			}
		}

		return true
	})

	if err != nil {
		fmt.Print(err)
	}

	return pac, err
}
