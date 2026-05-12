package command

import (
	"github.com/spf13/cobra"

	"github.com/M0Rf30/yap/v2/pkg/i18n"
)

// initCommandDescriptions sets i18n-translated descriptions on a cobra command and its flags.
// It takes the command, a command key for i18n lookups, and a map of flag names to i18n keys.
func initCommandDescriptions(cmd *cobra.Command, cmdKey string, flagKeys map[string]string) {
	cmd.Short = i18n.T("commands." + cmdKey + ".short")
	cmd.Long = i18n.T("commands." + cmdKey + ".long")
	cmd.Example = i18n.T("commands." + cmdKey + ".examples")

	for flagName, i18nKey := range flagKeys {
		if flag := cmd.Flag(flagName); flag != nil {
			flag.Usage = i18n.T(i18nKey)
		}
	}
}
