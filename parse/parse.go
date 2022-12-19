package parse

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/M0Rf30/yap/pack"
	"github.com/M0Rf30/yap/utils"
	"mvdan.cc/sh/v3/shell"
	"mvdan.cc/sh/v3/syntax"
)

func stringifyArray(node *syntax.Assign) []string {
	fields := make([]string, 0)

	out := &strings.Builder{}

	for index := range node.Array.Elems {
		syntax.NewPrinter().Print(out, node.Array.Elems[index].Value)
		out.WriteString(" ")
		fields = append(fields, out.String())
	}

	return fields
}

func stringifyAssign(node *syntax.Assign) string {
	out := &strings.Builder{}
	syntax.NewPrinter().Print(out, node.Value)

	return strings.Trim(out.String(), "\"")
}

func stringifyFuncDecl(node *syntax.FuncDecl) []string {
	var fields []string

	out := &strings.Builder{}
	syntax.NewPrinter().Print(out, node.Body)

	fields = append(fields, out.String())

	return fields
}

func File(distro, release, compiledOutput, home string) (*pack.Pack, error) {
	home, err := filepath.Abs(home)

	path := filepath.Join(compiledOutput, "PKGBUILD")

	pac := &pack.Pack{
		Distro:     distro,
		Release:    release,
		Root:       compiledOutput,
		Home:       home,
		SourceDir:  filepath.Join(compiledOutput, "src"),
		PackageDir: filepath.Join(compiledOutput, "pkg"),
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
				for _, line := range stringifyArray(nodeType) {
					arrayDecl, _ = shell.Fields(line, env)
				}
				err = pac.AddItem(nodeType.Name.Value, arrayDecl)
			} else {
				varDecl, _ = shell.Expand(stringifyAssign(nodeType), env)
				err = pac.AddItem(nodeType.Name.Value, varDecl)
			}

			if err != nil {
				return true
			}

		case *syntax.FuncDecl:
			for _, line := range stringifyFuncDecl(nodeType) {
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
