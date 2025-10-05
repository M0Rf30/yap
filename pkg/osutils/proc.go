package osutils

import (
	"context"
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
	cmd := exec.CommandContext(context.Background(), name, args...)
	cmd.Stdout = MultiPrinter.Writer
	cmd.Stderr = MultiPrinter.Writer

	if excludeStdout {
		cmd.Stderr = nil
		cmd.Stdout = nil
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
