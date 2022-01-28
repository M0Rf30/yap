package utils

import (
	"fmt"
	"io"
	"os"
	"os/exec"

	"github.com/packagefoundation/yap/constants"
)

func Exec(dir, name string, arg ...string) (err error) {
	cmd := exec.Command(name, arg...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if dir != "" {
		cmd.Dir = dir
	}

	err = cmd.Run()
	if err != nil {
		fmt.Printf("%s❌ :: %sfailed to exec '%s'%s\n",
			string(constants.ColorBlue),
			string(constants.ColorYellow),
			name,
			string(constants.ColorWhite))
	}

	return err
}

func ExecInput(dir, input, name string, arg ...string) (err error) {
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

		return
	}
	defer stdin.Close()

	if dir != "" {
		cmd.Dir = dir
	}

	err = cmd.Start()
	if err != nil {
		fmt.Printf("%s❌ :: %sfailed to exec '%s'%s\n",
			string(constants.ColorBlue),
			string(constants.ColorYellow),
			name,
			string(constants.ColorWhite))

		return
	}

	_, err = io.WriteString(stdin, input)
	if err != nil {
		fmt.Printf("%s❌ :: %sfailed to write stdin in exec '%s'%s\n",
			string(constants.ColorBlue),
			string(constants.ColorYellow),
			name,
			string(constants.ColorWhite))

		return
	}

	err = cmd.Wait()
	if err != nil {
		fmt.Printf("%s❌ :: %sfailed to exec '%s'%s\n",
			string(constants.ColorBlue),
			string(constants.ColorYellow),
			name,
			string(constants.ColorWhite))

		return
	}

	return err
}

func ExecOutput(dir, name string, arg ...string) (output string, err error) {
	cmd := exec.Command(name, arg...)
	cmd.Stderr = os.Stderr

	if dir != "" {
		cmd.Dir = dir
	}

	outputByt, err := cmd.Output()
	if err != nil {
		fmt.Printf("%s❌ :: %sfailed to exec '%s'%s\n",
			string(constants.ColorBlue),
			string(constants.ColorYellow),
			name,
			string(constants.ColorWhite))

		return
	}

	output = string(outputByt)

	return output, err
}
