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
	// Handle rootlesskit child re-execution before anything else.
	// When yap is re-executed as the rootlesskit child process, this call
	// completes the namespace setup and exits — cobra never runs in that path.
	initRootless()

	// Pre-parse -l/--language before cobra runs so the correct locale is
	// active when InitializeLocalizedDescriptions sets all command strings
	// (including the --help path, which never fires PersistentPreRun).
	command.ParseLanguageFlag()

	// Initialize localized descriptions after all commands are registered
	command.InitializeLocalizedDescriptions()
	command.Execute()
}
