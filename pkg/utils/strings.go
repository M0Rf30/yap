package utils

import (
	"strings"

	"mvdan.cc/sh/v3/syntax"
)

// StringifyArray generates a string representation of an array in the given syntax.
//
// node: A pointer to the syntax.Assign node representing the array.
// []string: An array of strings representing the stringified elements of the array.
func StringifyArray(node *syntax.Assign) []string {
	fields := make([]string, 0)
	printer := syntax.NewPrinter(syntax.Indent(2))
	out := &strings.Builder{}

	for index := range node.Array.Elems {
		if err := printer.Print(out, node.Array.Elems[index].Value); err != nil {
			Logger.Error("unable to parse array element",
				Logger.Args("name", out.String()))
		}

		out.WriteString(" ")
		fields = append(fields, out.String())
	}

	return fields
}

// StringifyAssign returns a string representation of the given *syntax.Assign node.
//
// It takes a pointer to a *syntax.Assign node as its parameter.
// It returns a string.
func StringifyAssign(node *syntax.Assign) string {
	out := &strings.Builder{}
	printer := syntax.NewPrinter(syntax.Indent(2))

	if err := printer.Print(out, node.Value); err != nil {
		Logger.Error("unable to parse variable",
			Logger.Args("name", out.String()))
	}

	return strings.Trim(out.String(), "\"")
}

// StringifyFuncDecl converts a syntax.FuncDecl node to a string representation.
//
// It takes a pointer to a syntax.FuncDecl node as a parameter and returns a string.
func StringifyFuncDecl(node *syntax.FuncDecl) string {
	out := &strings.Builder{}
	printer := syntax.NewPrinter(syntax.Indent(2))

	err := printer.Print(out, node.Body)

	if err != nil {
		Logger.Error("unable to parse function",
			Logger.Args("name", out.String()))
	}

	funcDecl := strings.Trim(out.String(), "{\n}")

	return funcDecl
}

// Contains checks if a string is present in an array of strings.
func Contains(array []string, str string) bool {
	for _, item := range array {
		if item == str {
			return true
		}
	}

	return false
}
