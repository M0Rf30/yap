// Package main provides the yap command-line package management tool.
package main

import (
	"github.com/M0Rf30/yap/v2/cmd/yap/command"
)

// main is the entry point of the Go program.
//
// It does not take any parameters.
// It does not return any values.
func main() {
	// Initialize localized descriptions after all commands are registered
	command.InitializeLocalizedDescriptions()
	command.Execute()
}
