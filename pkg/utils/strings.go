package utils

import (
	"crypto/rand"
	"fmt"
	"math/big"
	"os"
	"strings"

	"github.com/M0Rf30/yap/pkg/constants"
	"mvdan.cc/sh/v3/syntax"
)

// GenerateRandomString returns a securely generated random string.
// It will return an error if the system's secure random
// number generator fails to function correctly, in which
// case the caller should not continue.
func GenerateRandomString(n int) string {
	const letters = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz-"

	ret := make([]byte, n)

	for index := 0; index < n; index++ {
		num, err := rand.Int(rand.Reader, big.NewInt(int64(len(letters))))
		if err != nil {
			return ""
		}

		ret[index] = letters[num.Int64()]
	}

	return string(ret)
}

// Generates a string from a *syntax.Assign of an array declaration.
func StringifyArray(node *syntax.Assign) []string {
	fields := make([]string, 0)
	printer := syntax.NewPrinter(syntax.Indent(2))
	out := &strings.Builder{}

	for index := range node.Array.Elems {
		err := printer.Print(out, node.Array.Elems[index].Value)
		if err != nil {
			fmt.Printf("%s❌ :: %sunable to parse variable: %s\n",
				string(constants.ColorBlue),
				string(constants.ColorYellow), out.String())

			os.Exit(1)
		}

		out.WriteString(" ")
		fields = append(fields, out.String())
	}

	return fields
}

// Generates a string from a *syntax.Assign of a variable declaration.
func StringifyAssign(node *syntax.Assign) string {
	out := &strings.Builder{}
	printer := syntax.NewPrinter(syntax.Indent(2))
	err := printer.Print(out, node.Value)

	if err != nil {
		fmt.Printf("%s❌ :: %sunable to parse variable: %s\n",
			string(constants.ColorBlue),
			string(constants.ColorYellow), out.String())

		os.Exit(1)
	}

	return strings.Trim(out.String(), "\"")
}

// Generates strings from a *syntax.Assign of a function declaration.
func StringifyFuncDecl(node *syntax.FuncDecl) string {
	out := &strings.Builder{}
	printer := syntax.NewPrinter(syntax.Indent(2))

	err := printer.Print(out, node.Body)

	if err != nil {
		fmt.Printf("%s❌ :: %sunable to parse function: %s\n",
			string(constants.ColorBlue),
			string(constants.ColorYellow), out.String())

		os.Exit(1)
	}

	funcDecl := strings.TrimPrefix(out.String(), "{")
	funcDecl = strings.TrimPrefix(funcDecl, "\n")
	funcDecl = strings.TrimSuffix(funcDecl, "}")
	funcDecl = strings.TrimSuffix(funcDecl, "\n")

	return funcDecl
}
