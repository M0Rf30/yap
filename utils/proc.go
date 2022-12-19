package utils

import (
	"fmt"
	"io"
	"os"
	"os/exec"

	"github.com/M0Rf30/yap/constants"
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

func ExecInput(dir, input, name string, arg ...string) error {
	cmd := exec.Command(name, arg...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	stdin, err := cmd.StdinPipe()
	if err != nil {
		fmt.Printf("%s❌ :: %sfailed to get stdin in exec '%s'%s\n",
			string(constants.ColorBlue),
			string(constants.ColorYellow),
			name,
			string(constants.ColorWhite))

		return err
	}
	defer stdin.Close()

	if dir != "" {
		cmd.Dir = dir
	}

	err = cmd.Start()
	if err != nil {
		return err
	}

	_, err = io.WriteString(stdin, input)
	if err != nil {
		return err
	}

	err = cmd.Wait()
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
