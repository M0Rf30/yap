package builder

import (
	"github.com/packagefoundation/yap/utils"
)

func runScript(cmds string) error {
	err := utils.Exec("", "sh", "-c", cmds)
	if err != nil {
		return err
	}

	return err
}
