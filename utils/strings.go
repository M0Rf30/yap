package utils

import (
	"strings"

	"mvdan.cc/sh/v3/syntax"
)

func StringifyArray(node *syntax.Assign) []string {
	fields := make([]string, 0)

	out := &strings.Builder{}

	for index := range node.Array.Elems {
		syntax.NewPrinter().Print(out, node.Array.Elems[index].Value)
		out.WriteString(" ")
		fields = append(fields, out.String())
	}

	return fields
}

func StringifyAssign(node *syntax.Assign) string {
	out := &strings.Builder{}
	syntax.NewPrinter().Print(out, node.Value)

	return strings.Trim(out.String(), "\"")
}

func StringifyFuncDecl(node *syntax.FuncDecl) []string {
	var fields []string

	out := &strings.Builder{}
	syntax.NewPrinter().Print(out, node.Body)

	fields = append(fields, out.String())

	return fields
}
