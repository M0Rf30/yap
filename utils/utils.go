package utils

import (
	"crypto/rand"
	"fmt"
	"math/big"
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

// GenerateRandomString returns a securely generated random string.
// It will return an error if the system's secure random
// number generator fails to function correctly, in which
// case the caller should not continue.
func GenerateRandomString(n int) string {
	const letters = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz-"

	ret := make([]byte, n)

	for index := 0; index < n; index++ {
		num, err := rand.Int(rand.Reader, big.NewInt(int64(len(letters))))
		if err != nil {
			return ""
		}

		ret[index] = letters[num.Int64()]
	}

	return string(ret)
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
