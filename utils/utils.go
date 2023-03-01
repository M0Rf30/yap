package utils

import (
	"fmt"
	"os"
	"os/exec"

	"github.com/M0Rf30/yap/constants"
)

func HTTPGet(url, output string, protocol string) error {
	var cmd *exec.Cmd

	switch protocol {
	case "http":
		cmd = exec.Command("curl", "-gqb", "\"\"", "-fLC", "-", "-o", output, url)
	case "https":
		cmd = exec.Command("curl", "-gqb", "\"\"", "-fLC", "-", "-o", output, url)
	case "ftp":
		cmd = exec.Command("curl", "-gqfC", "-", "--ftp-pasv", "-o", output, url)
	}

	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	err := cmd.Run()
	if err != nil {
		fmt.Printf("%s❌ :: %sfailed to get %s%s\n",
			string(constants.ColorBlue),
			string(constants.ColorYellow),
			url,
			string(constants.ColorWhite))

		return err
	}

	return err
}

func PullContainers(target string) error {
	containerApp := "/usr/bin/docker"
	args := []string{
		"pull",
		constants.DockerOrg + target,
	}

	var err error

	if _, err = os.Stat(containerApp); err == nil {
		err = Exec("", containerApp, args...)
	} else {
		err = Exec("", "podman", args...)
	}

	if err != nil {
		return err
	}

	return err
}
