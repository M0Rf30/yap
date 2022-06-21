package utils

import (
	"fmt"
	"os"
	"os/exec"

	"github.com/packagefoundation/yap/constants"
)

func Rsync(source, dest string) error {
	cmd := exec.Command("rsync", "-a", "-A", //nolint:gosec
		source+string(os.PathSeparator),
		dest+string(os.PathSeparator))
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	err := cmd.Run()
	if err != nil {
		fmt.Printf("%s❌ :: %sfailed to rsync '%s' to '%s'%s\n",
			string(constants.ColorBlue),
			string(constants.ColorYellow),
			source,
			dest,
			string(constants.ColorWhite))

		return err
	}

	return err
}

func RsyncExt(source, dest, ext string) error {
	cmd := exec.Command("rsync", "-a", "-A", //nolint:gosec
		"--include", "*"+ext, "--exclude", "*",
		source+string(os.PathSeparator),
		dest+string(os.PathSeparator))
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	err := cmd.Run()
	if err != nil {
		fmt.Printf("%s❌ :: %sfailed to rsync '%s' to '%s'%s\n",
			string(constants.ColorBlue),
			string(constants.ColorYellow),
			source,
			dest,
			string(constants.ColorWhite))

		return err
	}

	return err
}

func RsyncMatch(source, dest, match string) error {
	cmd := exec.Command("rsync", "-a", "-A", //nolint:gosec
		"--include", "*"+match+"*", "--exclude", "*",
		source+string(os.PathSeparator),
		dest+string(os.PathSeparator))
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	err := cmd.Run()
	if err != nil {
		fmt.Printf("%s❌ :: %sfailed to rsync '%s' to '%s'%s\n",
			string(constants.ColorBlue),
			string(constants.ColorYellow),
			source,
			dest,
			string(constants.ColorWhite))

		return err
	}

	return err
}
