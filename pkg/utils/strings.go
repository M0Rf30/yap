package utils

import (
	"fmt"
	"log"
	"strings"

	"github.com/M0Rf30/yap/pkg/constants"
	"mvdan.cc/sh/v3/syntax"
)

// StringifyArray generates a string representation of an array in the given syntax.
//
// node: A pointer to the syntax.Assign node representing the array.
// []string: An array of strings representing the stringified elements of the array.
func StringifyArray(node *syntax.Assign) []string {
	fields := make([]string, len(node.Array.Elems))
	printer := syntax.NewPrinter(syntax.Indent(2))

	for index := range node.Array.Elems {
		out := &strings.Builder{}
		if err := printer.Print(out, node.Array.Elems[index].Value); err != nil {
			fmt.Printf("%s❌ :: %sunable to parse array element: %s\n",
				string(constants.ColorBlue),
				string(constants.ColorYellow), out.String())
		}

		fields[index] = out.String() + " "
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
		fmt.Printf("%s❌ :: %sunable to parse variable: %s\n",
			string(constants.ColorBlue),
			string(constants.ColorYellow), out.String())
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
		log.Fatalf("%s❌ :: %sunable to parse function: %s\n",
			string(constants.ColorBlue),
			string(constants.ColorYellow), out.String())
	}

	funcDecl := strings.Trim(out.String(), "{\n}")

	return funcDecl
}
