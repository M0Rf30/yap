package utils

import (
	"fmt"
	"math/rand"
	"os"
	"os/exec"

	"github.com/packagefoundation/yap/constants"
)

var chars = []rune(
	"abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789")

func HTTPGet(url, output string, protocol string) (err error) {
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

	err = cmd.Run()
	if err != nil {
		fmt.Printf("%s❌ :: %sfailed to get %s%s\n",
			string(constants.ColorBlue),
			string(constants.ColorYellow),
			url,
			string(constants.ColorWhite))

		return
	}

	return
}

func RandStr(n int) (str string) {
	strList := make([]rune, n)
	for i := range strList {
		strList[i] = chars[rand.Intn(len(chars))] //nolint:gosec
	}

	str = string(strList)

	return
}

func PullContainers() (err error) {
	for _, release := range constants.Releases {
		err = Exec("", "podman", "pull", constants.DockerOrg+release)
		if err != nil {
			return
		}
	}

	return
}
