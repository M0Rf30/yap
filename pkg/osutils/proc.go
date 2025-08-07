package osutils

import (
	"os/exec"
)

// Exec executes a command in the specified directory.
//
// It takes the following parameters:
// - dir: the directory in which the command will be executed.
// - name: the name of the command to be executed.
// - arg: optional arguments to be passed to the command.
//
// It returns an error if the command execution fails.
func Exec(excludeStdout bool, dir, name string, args ...string) error {
	//nolint:noctx // Legacy function without context support
	cmd := exec.Command(name, args...)

	if !excludeStdout {
		// Start multiprinter for consistent output handling
		_, err := MultiPrinter.Start()
		if err != nil {
			return err
		}

		// Create decorated writer for command output
		decoratedWriter := NewPackageDecoratedWriter(MultiPrinter.Writer, "yap")
		cmd.Stdout = decoratedWriter
		cmd.Stderr = decoratedWriter
	}

	if dir != "" {
		cmd.Dir = dir
	}

	err := cmd.Run()
	if err != nil {
		return err
	}

	return nil
}
