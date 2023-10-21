package command

import (
	"github.com/spf13/cobra"
)

// rootCmd represents the base command when called without any subcommands.
var rootCmd = &cobra.Command{
	Use:   "yap",
	Short: "Automated deb, rpm and pkgbuild build system",
	Long: `Yap allows building packages for multiple GNU/Linux distributions with a
consistent package spec format.

Complete documentation is available at
🌐 https://github.com/M0Rf30/yap`,
}

// Execute adds all child commands to the root command and sets flags
// appropriately. This is called by main.main(). It only needs to happen once to
// the rootCmd.
func Execute() {
	cobra.CheckErr(rootCmd.Execute())
}
