package builder

import (
	"github.com/packagefoundation/yap/utils"
)

func createScript(path string, cmds []string) error {
	data := "set -e\n"
	for _, cmd := range cmds {
		data += cmd + "\n"
	}

	err := utils.CreateWrite(path, data)
	if err != nil {
		return err
	}

	return err
}

func runScript(path, dir string) error {
	err := utils.Exec(dir, "sh", path)
	if err != nil {
		return err
	}

	return err
}
