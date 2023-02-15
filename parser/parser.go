package parser

import (
	"fmt"
	"path/filepath"

	"github.com/M0Rf30/yap/pkgbuild"
	"github.com/M0Rf30/yap/utils"
	"mvdan.cc/sh/v3/shell"
	"mvdan.cc/sh/v3/syntax"
)

func getPKGBUILDSyntax(compiledOutput, home string) (*syntax.File, error) {
	path := filepath.Join(compiledOutput, "PKGBUILD")

	file, err := utils.Open(path)
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

func manageAssignment(env func(string) string, nodeType *syntax.Assign, pkgbuild *pkgbuild.PKGBUILD) {
	var arrayDecl []string

	var varDecl string

	if nodeType.Array != nil {
		for _, line := range utils.StringifyArray(nodeType) {
			arrayDecl, _ = shell.Fields(line, env)
		}

		pkgbuild.AddAssignments(nodeType.Name.Value, arrayDecl)
		pkgbuild.AddDependencies(nodeType.Name.Value, arrayDecl)
		pkgbuild.AddSources(nodeType.Name.Value, arrayDecl)
		pkgbuild.AddCustomAssignments(nodeType.Name.Value, arrayDecl)
		pkgbuild.AddNonMandatoryItems(nodeType.Name.Value, varDecl)
	} else {
		varDecl, _ = shell.Expand(utils.StringifyAssign(nodeType), env)
		pkgbuild.AddAssignments(nodeType.Name.Value, varDecl)
		pkgbuild.AddCustomAssignments(nodeType.Name.Value, varDecl)
		pkgbuild.AddNonMandatoryItems(nodeType.Name.Value, varDecl)
	}
}

func manageFunctionDeclaration(env func(string) string,
	nodeType *syntax.FuncDecl,
	pkgbuild *pkgbuild.PKGBUILD) {
	var funcDecl string

	for _, line := range utils.StringifyFuncDecl(nodeType) {
		funcDecl, _ = shell.Expand(line, env)
	}

	pkgbuild.AddFunctions(nodeType.Name.Value, funcDecl)
}

func parseSyntax(pkgbuild *pkgbuild.PKGBUILD, pkgbuildSyntax *syntax.File) {
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

	syntax.Walk(pkgbuildSyntax, func(node syntax.Node) bool {
		switch nodeType := node.(type) {
		case *syntax.Assign:
			manageAssignment(env, nodeType, pkgbuild)
		case *syntax.FuncDecl:
			manageFunctionDeclaration(env, nodeType, pkgbuild)
		}

		return true
	})
}

func ParseFile(distro, release, compiledOutput, home string) (*pkgbuild.PKGBUILD, error) {
	pkgbuild := &pkgbuild.PKGBUILD{
		Distro:     distro,
		CodeName:   release,
		Root:       compiledOutput,
		Home:       home,
		SourceDir:  filepath.Join(compiledOutput, "src"),
		PackageDir: filepath.Join(compiledOutput, "pkg"),
	}

	pkgbuild.Init()

	home, err := filepath.Abs(home)
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

	pkgbuildSyntax, err := getPKGBUILDSyntax(compiledOutput, home)
	if err != nil {
		return pkgbuild, err
	}

	parseSyntax(pkgbuild, pkgbuildSyntax)

	return pkgbuild, err
}
