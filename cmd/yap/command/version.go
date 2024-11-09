package command

import (
	"github.com/pterm/pterm"
	"github.com/pterm/pterm/putils"
	"github.com/spf13/cobra"
)

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print the version number of Yap",
	Long:  `All software has versions. This is Yap's`,
	Run: func(_ *cobra.Command, _ []string) {
		logo, _ := pterm.DefaultBigText.WithLetters(
			putils.LettersFromStringWithStyle("Y", pterm.NewStyle(pterm.FgBlue)),
			putils.LettersFromStringWithStyle("A", pterm.NewStyle(pterm.FgLightBlue)),
			putils.LettersFromStringWithStyle("P", pterm.NewStyle(pterm.FgLightCyan))).
			Srender()

		pterm.DefaultCenter.Print(logo)

		pterm.DefaultCenter.Print(
			pterm.DefaultHeader.WithFullWidth().WithMargin(10).Sprint("Yet Another Packager"))

		pterm.Println("Version v1.26")
		pterm.Println("Coded with \u2764 by M0Rf30")
	},
}

func init() {
	rootCmd.AddCommand(versionCmd)
}
