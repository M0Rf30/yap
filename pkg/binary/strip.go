// Package binary provides binary file manipulation utilities.
package binary

import (
	"github.com/M0Rf30/yap/v2/pkg/shell"
)

// StripFile removes debugging symbols from a binary file.
func StripFile(path string, args ...string) error {
	return strip(path, args...)
}

// StripLTO removes LTO (Link Time Optimization) sections from a binary file.
func StripLTO(path string, args ...string) error {
	return strip(
		path,
		append(args, "-R", ".gnu.lto_*", "-R", ".gnu.debuglto_*", "-N", "__gnu_lto_v1")...)
}

func strip(path string, args ...string) error {
	args = append(args, path)
	return shell.Exec(false, "", "strip", args...)
}
