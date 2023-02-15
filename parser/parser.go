package parser

import (
	"fmt"
	"path/filepath"

	"github.com/M0Rf30/yap/pkgbuild"
	"github.com/M0Rf30/yap/utils"
	"mvdan.cc/sh/v3/shell"
	"mvdan.cc/sh/v3/syntax"
)

func ParseFile(distro, release, compiledOutput, home string) (*pkgbuild.PKGBUILD, error) {
	home, err := filepath.Abs(home)

	path := filepath.Join(compiledOutput, "PKGBUILD")

	pkgbuild := &pkgbuild.PKGBUILD{
		Distro:     distro,
		CodeName:   release,
		Root:       compiledOutput,
		Home:       home,
		SourceDir:  filepath.Join(compiledOutput, "src"),
		PackageDir: filepath.Join(compiledOutput, "pkg"),
	}

	if err != nil {
		fmt.Printf("parse: Failed to get root directory from '%s'\n",
			home)

		return pkgbuild, err
	}

	err = utils.ExistsMakeDir(compiledOutput)
	if err != nil {
		return pkgbuild, err
	}

	err = utils.CopyFiles(home, compiledOutput, false)
	if err != nil {
		return pkgbuild, err
	}

	pkgbuild.Init()

	file, err := utils.Open(path)
	if err != nil {
		return pkgbuild, err
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
			return pkgbuild.PkgName
		case "pkgver":
			return pkgbuild.PkgVer
		case "pkgrel":
			return pkgbuild.PkgVer
		case "pkgdir":
			return pkgbuild.PackageDir
		case "srcdir":
			return pkgbuild.SourceDir
		case "url":
			return pkgbuild.URL
		default:
			return pkgbuild.Variables[name]
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
				err = pkgbuild.AddItem(nodeType.Name.Value, arrayDecl)
			} else {
				varDecl, _ = shell.Expand(utils.StringifyAssign(nodeType), env)
				err = pkgbuild.AddItem(nodeType.Name.Value, varDecl)
			}

			if err != nil {
				return true
			}

		case *syntax.FuncDecl:
			for _, line := range utils.StringifyFuncDecl(nodeType) {
				funcDecl, _ = shell.Expand(line, env)
			}
			err = pkgbuild.AddItem(nodeType.Name.Value, funcDecl)

			if err != nil {
				return true
			}
		}

		return true
	})

	if err != nil {
		fmt.Print(err)
	}

	return pkgbuild, err
}
