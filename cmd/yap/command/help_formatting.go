package command

import (
	"fmt"
	"strings"
	"unicode"

	"github.com/pterm/pterm"
	"github.com/spf13/cobra"

	"github.com/M0Rf30/yap/v2/pkg/i18n"
	"github.com/M0Rf30/yap/v2/pkg/logger"
)

// SetupEnhancedHelp configures enhanced help formatting for commands.
func SetupEnhancedHelp() {
	// Custom usage template with enhanced formatting
	usageTemplate := `{{if .HasAvailableSubCommands}}` +
		`{{.CommandPath | styleUsage}} [command]{{else if .Runnable}}` +
		`{{.UseLine | styleUsage}}{{end}}{{if gt (len .Aliases) 0}}

{{styleAliases}}
  {{.NameAndAliases}}{{end}}{{if .HasExample}}

{{styleExamples}}
{{.Example}}{{end}}{{if .HasAvailableSubCommands}}{{$cmds := .Commands}}{{if eq (len .Groups) 0}}

{{styleAvailableCommands}}{{range $cmds}}{{if (or .IsAvailableCommand (eq .Name "help"))}}
  {{rpad .Name .NamePadding | styleCommand}} ` +
		`{{.Short | styleShort}}{{end}}{{end}}{{else}}{{range $group := .Groups}}

{{$group.Title | styleGroupTitle}}{{range $cmds}}` +
		`{{if (and (eq .GroupID $group.ID) (or .IsAvailableCommand (eq .Name "help")))}}
  {{rpad .Name .NamePadding | styleCommand}} {{.Short | styleShort}}{{end}}{{end}}{{end}}` +
		`{{if not .AllChildCommandsHaveGroup}}

{{styleAdditionalCommands}}{{range $cmds}}` +
		`{{if (and (eq .GroupID "") (or .IsAvailableCommand (eq .Name "help")))}}
  {{rpad .Name .NamePadding | styleCommand}} {{.Short | styleShort}}{{end}}{{end}}{{end}}` +
		`{{end}}{{end}}{{if .HasAvailableLocalFlags}}

{{styleFlags}}
{{.LocalFlags.FlagUsages | trimTrailingWhitespaces}}{{end}}{{if .HasAvailableInheritedFlags}}

{{styleGlobalFlags}}
{{.InheritedFlags.FlagUsages | trimTrailingWhitespaces}}{{end}}{{if .HasHelpSubCommands}}

{{styleAdditionalHelp}}{{range .Commands}}{{if .IsAdditionalHelpTopicCommand}}
  {{rpad .Name .NamePadding | styleCommand}} {{.Short | styleShort}}{{end}}{{end}}{{end}}{{if
  .HasAvailableSubCommands}}

{{styleMoreInfo}}{{end}}

{{styleFooter}}`

	// Set up template functions
	cobra.AddTemplateFunc("styleUsage", func(s string) string {
		return pterm.FgLightCyan.Sprintf("Usage: %s", s)
	})

	cobra.AddTemplateFunc("styleAliases", func() string {
		return pterm.NewStyle(pterm.FgMagenta, pterm.Bold).Sprint("Aliases:")
	})

	cobra.AddTemplateFunc("styleExamples", func() string {
		return pterm.NewStyle(pterm.FgGreen, pterm.Bold).Sprint("Examples:")
	})

	cobra.AddTemplateFunc("styleAvailableCommands", func() string {
		return pterm.NewStyle(pterm.FgBlue, pterm.Bold).Sprint("Available Commands:")
	})

	cobra.AddTemplateFunc("styleGroupTitle", func(s string) string {
		return pterm.NewStyle(pterm.FgCyan, pterm.Bold).Sprintf("%s:", s)
	})

	cobra.AddTemplateFunc("styleAdditionalCommands", func() string {
		return pterm.NewStyle(pterm.FgBlue, pterm.Bold).Sprint("Additional Commands:")
	})

	cobra.AddTemplateFunc("styleCommand", func(s string) string {
		return pterm.FgLightBlue.Sprint(strings.TrimSpace(s))
	})

	cobra.AddTemplateFunc("styleShort", func(s string) string {
		return pterm.FgWhite.Sprint(s)
	})

	// Fix the styleFlags function to accept variable arguments to handle both cases
	cobra.AddTemplateFunc("styleFlags", func(_ ...any) string {
		return pterm.NewStyle(pterm.FgYellow, pterm.Bold).Sprint("Flags:")
	})

	cobra.AddTemplateFunc("styleGlobalFlags", func(_ ...any) string {
		return pterm.NewStyle(pterm.FgLightYellow, pterm.Bold).Sprint("Global Flags:")
	})

	cobra.AddTemplateFunc("styleAdditionalHelp", func() string {
		return pterm.NewStyle(pterm.FgMagenta, pterm.Bold).Sprint("Additional help topics:")
	})

	cobra.AddTemplateFunc("styleMoreInfo", func() string {
		return pterm.FgGray.Sprint(i18n.T("footer.more_info"))
	})

	cobra.AddTemplateFunc("styleFooter", func() string {
		return fmt.Sprintf(`%s %s
%s %s
`,
			pterm.FgBlue.Sprint(i18n.T("footer.documentation")),
			pterm.FgLightBlue.Sprint("https://github.com/M0Rf30/yap"),
			pterm.FgRed.Sprint(i18n.T("footer.report_issues")),
			pterm.FgLightRed.Sprint("https://github.com/M0Rf30/yap/issues"))
	})

	// Add required template helper functions
	cobra.AddTemplateFunc("trimTrailingWhitespaces", func(s string) string {
		return strings.TrimRightFunc(s, unicode.IsSpace)
	})
	cobra.AddTemplateFunc("rpad", func(s string, padding int) string {
		template := fmt.Sprintf("%%-%ds", padding)

		return fmt.Sprintf(template, s)
	})

	// Apply the template to root command
	rootCmd.SetUsageTemplate(usageTemplate)

	// Apply template to all subcommands
	for _, cmd := range rootCmd.Commands() {
		cmd.SetUsageTemplate(usageTemplate)
	}
}

// CustomErrorHandler provides enhanced error messages with styling.
func CustomErrorHandler(cmd *cobra.Command, err error) error {
	if err != nil {
		// Style the error message
		styledError := pterm.NewStyle(pterm.FgRed, pterm.Bold).Sprintf("Error: %s", err.Error())
		_, _ = fmt.Fprintln(cmd.ErrOrStderr(), styledError)

		// Add helpful suggestion
		if strings.Contains(err.Error(), "unknown command") ||
			strings.Contains(err.Error(), "unsupported distribution") {
			logger.Tips(fmt.Sprintf(i18n.T("logger.tips.use_help"), cmd.CommandPath()))
		}

		return err
	}

	return nil
}
