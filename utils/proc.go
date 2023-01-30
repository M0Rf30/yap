package utils

import (
	"os"
	"os/exec"
)

func Exec(dir, name string, arg ...string) error {
	cmd := exec.Command(name, arg...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if dir != "" {
		cmd.Dir = dir
	}

	err := cmd.Run()
	if err != nil {
		return err
	}

	return err
}

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
