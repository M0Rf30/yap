package utils

import (
	"os"

	"github.com/M0Rf30/yap/constants"
)

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
