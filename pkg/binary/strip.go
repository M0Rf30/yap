// Package binary provides binary file manipulation utilities.
package binary //nolint:revive // intentional name; conflicts with stdlib encoding/binary but scope is unambiguous

import (
	"context"
	"os"

	"github.com/M0Rf30/yap/v2/pkg/shell"
)

// StripFile removes debugging symbols from a binary file.
// It respects the STRIP environment variable for cross-compilation support.
func StripFile(path string, args ...string) error {
	return strip(path, args...)
}

// StripLTO removes LTO (Link Time Optimization) sections from a binary file.
// It respects the STRIP environment variable for cross-compilation support.
func StripLTO(path string, args ...string) error {
	return strip(
		path,
		append(args, "-R", ".gnu.lto_*", "-R", ".gnu.debuglto_*", "-N", "__gnu_lto_v1")...)
}

func strip(path string, args ...string) error {
	args = append(args, path)

	// Use cross-compilation strip if STRIP environment variable is set
	// This allows stripping binaries compiled for foreign architectures
	stripCmd := os.Getenv("STRIP")
	if stripCmd == "" {
		stripCmd = "strip"
	}

	return shell.Exec(context.Background(), false, "", stripCmd, args...)
}
