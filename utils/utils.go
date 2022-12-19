package utils

import (
	"fmt"
	"math/rand"
	"os"
	"os/exec"

	"github.com/M0Rf30/yap/constants"
)

var chars = []rune(
	"abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789")

func HTTPGet(url, output string, protocol string) error {
	var cmd *exec.Cmd

	switch protocol {
	case "http":
		cmd = exec.Command("curl", "-gqb", "\"\"", "-fLC", "-", "-o", output, url)
	case "ftp":
		cmd = exec.Command("curl", "-gqfC", "-", "--ftp-pasv", "-o", output, url)
	case "git":
		cmd = exec.Command("git", "clone", "--mirror", url, output)
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

func RandStr(n int) string {
	strList := make([]rune, n)
	for i := range strList {
		strList[i] = chars[rand.Intn(len(chars))] //nolint:gosec
	}

	str := string(strList)

	return str
}

func PullContainers(target string) error {
	containerApp := "/usr/bin/docker"

	var err error

	if _, err = os.Stat(containerApp); err == nil {
		err = Exec("", containerApp, "pull", constants.DockerOrg+target)
	} else {
		err = Exec("", "podman", "pull", constants.DockerOrg+target)
	}

	if err != nil {
		return err
	}

	return err
}
