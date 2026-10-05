// Package set provides generic set data structure implementation and string utilities.
package set

import (
	"iter"
	"maps"
	"slices"
	"strings"

	"mvdan.cc/sh/v3/syntax"

	"github.com/M0Rf30/yap/v2/pkg/errors"
	"github.com/M0Rf30/yap/v2/pkg/i18n"
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

// Iter returns an iterator over the elements of the set in unspecified order.
// Breaking out of a range loop over it releases all resources.
func (s *Set) Iter() iter.Seq[string] {
	return maps.Keys(s.m)
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

// StringifyArrayElems returns one printed string per element of the array
// assignment in node (no trailing separator). Elements that fail to print are
// logged and skipped.
func StringifyArrayElems(node *syntax.Assign) []string {
	if node == nil || node.Array == nil || len(node.Array.Elems) == 0 {
		return nil
	}

	printer := syntax.NewPrinter(syntax.Indent(2))
	fields := make([]string, 0, len(node.Array.Elems))

	for index, elem := range node.Array.Elems {
		var out strings.Builder

		err := printer.Print(&out, elem.Value)
		if err != nil {
			logger.Error(i18n.T("logger.set.error.unable_to_parse_array"),
				"index", index, "error", err)

			continue
		}

		fields = append(fields, out.String())
	}

	return fields
}

// StringifyArray generates a string representation of an array in the given
// syntax.
//
// It returns either nil (empty array) or a single-element slice holding every
// array element, each followed by a space, so callers can pass the one string
// to a shell field splitter. Use StringifyArrayElems to get one string per
// element.
func StringifyArray(node *syntax.Assign) []string {
	elems := StringifyArrayElems(node)
	if len(elems) == 0 {
		return nil
	}

	var joined strings.Builder

	for _, elem := range elems {
		joined.WriteString(elem)
		joined.WriteString(" ")
	}

	return []string{joined.String()}
}

// StringifyAssign returns a string representation of the given *syntax.Assign node.
//
// It takes a pointer to a *syntax.Assign node as its parameter.
// It returns a string and an error.
func StringifyAssign(node *syntax.Assign) (string, error) {
	out := &strings.Builder{}
	printer := syntax.NewPrinter(syntax.Indent(2))

	if node.Value == nil {
		return "", errors.New(errors.ErrTypeConfiguration, i18n.T("errors.set.empty_variable")).
			WithContext("variable", node.Name.Value)
	}

	err := printer.Print(out, node.Value)
	if err != nil {
		return "", err
	}

	return strings.Trim(out.String(), "\""), nil
}

// StringifyFuncDecl converts a syntax.FuncDecl node to a string representation.
//
// It takes a pointer to a syntax.FuncDecl node as a parameter and returns a string and an error.
func StringifyFuncDecl(node *syntax.FuncDecl) (string, error) {
	out := &strings.Builder{}
	printer := syntax.NewPrinter(syntax.Indent(2), syntax.SwitchCaseIndent(true))

	err := printer.Print(out, node.Body)
	if err != nil {
		return "", err
	}

	// Strip the outer braces emitted by the printer for the function body.
	// IMPORTANT: do NOT use strings.Trim(s, "{\n}") here — Trim treats its
	// second argument as a *cutset* of characters, which would also strip a
	// trailing '}' that legitimately closes a parameter expansion such as
	// ${_tag} on the last line of the body, producing an unbalanced ${ and
	// a downstream parse error ("reached EOF without matching `${` with `}`").
	funcDecl := out.String()
	funcDecl = strings.TrimSpace(funcDecl)
	funcDecl = strings.TrimPrefix(funcDecl, "{")
	funcDecl = strings.TrimSuffix(funcDecl, "}")
	funcDecl = strings.Trim(funcDecl, "\n")

	if strings.TrimSpace(funcDecl) == "" {
		return "", errors.New(errors.ErrTypeConfiguration, i18n.T("errors.set.empty_function")).
			WithContext("function", node.Name.Value)
	}

	return funcDecl, nil
}
