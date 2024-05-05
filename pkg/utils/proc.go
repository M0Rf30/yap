package utils

import (
	"os"
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
	cmd := exec.Command(name, args...)
	cmd.Stdout = MultiPrinter.Writer
	cmd.Stderr = MultiPrinter.Writer

	if excludeStdout {
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

// ExecOutput executes a command with the given arguments and returns its output as a string.
//
// It takes the following parameters:
// - dir: the directory in which the command should be executed.
// - name: the name of the command to be executed.
// - arg: a variadic parameter representing the arguments to be passed to the command.
//
// It returns a string representing the output of the command and an error if any occurred.
func ExecOutput(dir, name string, arg ...string) (string, error) {
	cmd := exec.Command(name, arg...)
	cmd.Stderr = os.Stderr

	if dir != "" {
		cmd.Dir = dir
	}

	outputByte, err := cmd.Output()
	if err != nil {
		return "", err
	}

	output := string(outputByte)

	return output, err
}
