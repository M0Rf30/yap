package utils

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/M0Rf30/yap/constants"
)

func MkdirAll(path string) error {
	err := os.MkdirAll(path, 0o755)
	if err != nil {
		fmt.Printf("%s❌ :: %sfailed to mkdir '%s'%s\n",
			string(constants.ColorBlue),
			string(constants.ColorYellow),
			path,
			string(constants.ColorWhite))

		return err
	}

	return err
}

func Chmod(path string, perm os.FileMode) error {
	err := os.Chmod(path, perm)
	if err != nil {
		fmt.Printf("%s❌ :: %sfailed to chmod '%s'%s\n",
			string(constants.ColorBlue),
			string(constants.ColorYellow),
			path,
			string(constants.ColorWhite))

		return err
	}

	return err
}

func Remove(path string) error {
	err := os.Remove(path)
	if err != nil {
		fmt.Printf("%s❌ :: %sfailed to remove '%s'%s\n",
			string(constants.ColorBlue),
			string(constants.ColorYellow),
			path,
			string(constants.ColorWhite))

		return err
	}

	return err
}

func RemoveAll(path string) error {
	err := os.RemoveAll(path)
	if err != nil {
		fmt.Printf("%s❌ :: %sfailed to remove '%s'%s\n",
			string(constants.ColorBlue),
			string(constants.ColorYellow),
			path,
			string(constants.ColorWhite))

		return err
	}

	return err
}

func ExistsMakeDir(path string) error {
	_, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			err = MkdirAll(path)
			if err != nil {
				return err
			}
		} else {
			fmt.Printf("%s❌ :: %sfailed to stat '%s'%s\n",
				string(constants.ColorBlue),
				string(constants.ColorYellow),
				path,
				string(constants.ColorWhite))

			return err
		}

		return err
	}

	return err
}

func Create(path string) (*os.File, error) {
	file, err := os.Create(path)
	if err != nil {
		fmt.Printf("%s❌ :: %sfailed to create '%s'%s\n",
			string(constants.ColorBlue),
			string(constants.ColorYellow),
			path,
			string(constants.ColorWhite))
	}

	return file, err
}

func CreateWrite(path string, data string) error {
	file, err := Create(path)
	if err != nil {
		return err
	}
	defer file.Close()

	_, err = file.WriteString(data)
	if err != nil {
		fmt.Printf("%s❌ :: %sfailed to write to file '%s'%s\n",
			string(constants.ColorBlue),
			string(constants.ColorYellow),
			path,
			string(constants.ColorWhite))

		return err
	}

	return err
}

func Open(path string) (*os.File, error) {
	file, err := os.Open(path)
	if err != nil {
		fmt.Printf("%s❌ :: %sfailed to open file '%s'%s\n",
			string(constants.ColorBlue),
			string(constants.ColorYellow),
			path,
			string(constants.ColorWhite))
	}

	return file, err
}

func Filename(path string) string {
	n := strings.LastIndex(path, "/")
	if n == -1 {
		return path
	}

	return path[n+1:]
}

func GetDirSize(path string) (int64, error) {
	var size int64

	err := filepath.Walk(path, func(_ string, info os.FileInfo, err error) error {
		if err != nil {
			fmt.Printf("%s❌ :: %sfailed to get dir size '%s'%s\n",
				string(constants.ColorBlue),
				string(constants.ColorYellow),
				path,
				string(constants.ColorWhite))

			os.Exit(1)
		}
		if !info.IsDir() {
			size += info.Size()
		}

		return err
	})

	size = size / 1024

	return size, err
}

func Exists(filename string) bool {
	_, err := os.Stat(filename)

	return !os.IsNotExist(err)
}
