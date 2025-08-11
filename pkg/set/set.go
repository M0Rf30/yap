// Package set provides generic set data structure implementation and string utilities.
package set

import (
	"slices"
	"strings"

	"mvdan.cc/sh/v3/syntax"

	"github.com/M0Rf30/yap/v2/pkg/logger"
)

var exists = struct{}{}

// Set represents a simple set data structure implemented using a map.
type Set struct {
	m map[string]struct{}
}

// NewSet creates a new Set.
//
// It initializes a new Set with an empty map and returns a pointer to it.
// The returned Set is ready to use.
// Returns a pointer to the newly created Set.
func NewSet() *Set {
	s := &Set{
		m: make(map[string]struct{}),
	}

	return s
}

// Add adds a value to the Set.
//
// value: the value to be added.
func (s *Set) Add(value string) {
	s.m[value] = exists
}

// Contains checks if the given value is present in the set.
//
// value: the value to check for.
// bool: true if the value is present, false otherwise.
func (s *Set) Contains(value string) bool {
	_, c := s.m[value]

	return c
}

// Iter returns a channel that iterates over the elements of the set.
//
// It returns a channel of type string.
func (s *Set) Iter() <-chan string {
	iter := make(chan string)

	go func() {
		for key := range s.m {
			iter <- key
		}

		close(iter)
	}()

	return iter
}

// Remove removes the specified value from the set.
//
// value: the value to be removed from the set.
func (s *Set) Remove(value string) {
	delete(s.m, value)
}

// Contains checks if a string is present in an array of strings.
func Contains(array []string, str string) bool {
	return slices.Contains(array, str)
}

// StringifyArray generates a string representation of an array in the given syntax.
//
// node: A pointer to the syntax.Assign node representing the array.
// []string: An array of strings representing the stringified elements of the array.
func StringifyArray(node *syntax.Assign) []string {
	fields := make([]string, 0)
	printer := syntax.NewPrinter(syntax.Indent(2))
	out := &strings.Builder{}

	if len(node.Array.Elems) == 0 {
		logger.Fatal("empty array, please give it a value",
			"path", node.Name.Value)
	}

	for index := range node.Array.Elems {
		err := printer.Print(out, node.Array.Elems[index].Value)
		if err != nil {
			logger.Error("unable to parse array element",
				"path", out.String())
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

	if node.Value == nil {
		logger.Fatal("empty variable, please give it a value",
			"path", node.Name.Value)
	}

	err := printer.Print(out, node.Value)
	if err != nil {
		return ""
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
		logger.Error("unable to parse function",
			"path", out.String())
	}

	funcDecl := strings.Trim(out.String(), "{\n}")

	if funcDecl == "" {
		logger.Fatal("empty function, please give it a value",
			"path", node.Name.Value)
	}

	return funcDecl
}
