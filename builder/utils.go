package builder

import "github.com/M0Rf30/yap/utils"

func RunScript(cmds string) error {
	err := utils.Exec("", "sh", "-c", cmds)
	if err != nil {
		return err
	}

	return err
}
