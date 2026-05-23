package color_test

import (
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/M0Rf30/yap/v2/pkg/color"
)

// Helper to save and restore disabled state and environment variables.
func saveState() (noColor bool, envVars map[string]string) {
	disabled := color.IsDisabled()
	env := map[string]string{
		"NO_COLOR": os.Getenv("NO_COLOR"),
		"TERM":     os.Getenv("TERM"),
	}

	return disabled, env
}

func restoreState(disabled bool, env map[string]string) {
	if disabled {
		color.Disable()
	} else {
		color.Enable()
	}

	for key, val := range env {
		if val == "" {
			_ = os.Unsetenv(key)
		} else {
			_ = os.Setenv(key, val)
		}
	}
}

// TestDisable verifies that Disable() sets the disabled flag.
func TestDisable(t *testing.T) {
	disabled, env := saveState()
	defer restoreState(disabled, env)

	// Clear environment variables that might affect IsDisabled.
	_ = os.Unsetenv("NO_COLOR")
	_ = os.Unsetenv("TERM")

	color.Enable()
	assert.False(t, color.IsDisabled(), "color should be enabled initially")

	color.Disable()
	assert.True(t, color.IsDisabled(), "color should be disabled after Disable()")
}

// TestEnable verifies that Enable() clears the disabled flag.
func TestEnable(t *testing.T) {
	disabled, env := saveState()
	defer restoreState(disabled, env)

	// Clear environment variables.
	_ = os.Unsetenv("NO_COLOR")
	_ = os.Unsetenv("TERM")

	color.Disable()
	assert.True(t, color.IsDisabled(), "color should be disabled initially")

	color.Enable()
	assert.False(t, color.IsDisabled(), "color should be enabled after Enable()")
}

// TestIsDisabledByFlag verifies IsDisabled() respects the disabled flag.
func TestIsDisabledByFlag(t *testing.T) {
	disabled, env := saveState()
	defer restoreState(disabled, env)

	_ = os.Unsetenv("NO_COLOR")
	_ = os.Unsetenv("TERM")

	color.Enable()
	assert.False(t, color.IsDisabled())

	color.Disable()
	assert.True(t, color.IsDisabled())
}

// TestIsDisabledByNOCOLOR verifies IsDisabled() respects NO_COLOR env var.
func TestIsDisabledByNOCOLOR(t *testing.T) {
	disabled, env := saveState()
	defer restoreState(disabled, env)

	color.Enable()

	_ = os.Unsetenv("NO_COLOR")
	_ = os.Unsetenv("TERM")

	assert.False(t, color.IsDisabled(), "should be enabled with no env vars")

	_ = os.Setenv("NO_COLOR", "1")

	assert.True(t, color.IsDisabled(), "should be disabled when NO_COLOR is set")

	_ = os.Unsetenv("NO_COLOR")

	assert.False(t, color.IsDisabled(), "should be enabled when NO_COLOR is unset")
}

// TestIsDisabledByTERMDumb verifies IsDisabled() respects TERM=dumb.
func TestIsDisabledByTERMDumb(t *testing.T) {
	disabled, env := saveState()
	defer restoreState(disabled, env)

	color.Enable()

	_ = os.Unsetenv("NO_COLOR")
	_ = os.Unsetenv("TERM")

	assert.False(t, color.IsDisabled(), "should be enabled with no TERM")

	_ = os.Setenv("TERM", "dumb")

	assert.True(t, color.IsDisabled(), "should be disabled when TERM=dumb")

	_ = os.Setenv("TERM", "xterm-256color")

	assert.False(t, color.IsDisabled(), "should be enabled when TERM is not dumb")
}

// TestGray verifies Gray() wraps text with gray ANSI code when enabled.
func TestGray(t *testing.T) {
	disabled, env := saveState()
	defer restoreState(disabled, env)

	_ = os.Unsetenv("NO_COLOR")
	_ = os.Unsetenv("TERM")

	color.Enable()

	result := color.Gray("test")
	assert.Contains(t, result, "test", "should contain original text")
	assert.Contains(t, result, "\033[90m", "should contain gray ANSI code")
	assert.Contains(t, result, "\033[0m", "should contain reset code")

	color.Disable()

	result = color.Gray("test")
	assert.Equal(t, "test", result, "should return plain text when disabled")
}

// TestRed verifies Red() wraps text with red ANSI code when enabled.
func TestRed(t *testing.T) {
	disabled, env := saveState()
	defer restoreState(disabled, env)

	_ = os.Unsetenv("NO_COLOR")
	_ = os.Unsetenv("TERM")

	color.Enable()

	result := color.Red("error")
	assert.Contains(t, result, "error")
	assert.Contains(t, result, "\033[31m", "should contain red ANSI code")
	assert.Contains(t, result, "\033[0m", "should contain reset code")

	color.Disable()

	result = color.Red("error")
	assert.Equal(t, "error", result)
}

// TestGreen verifies Green() wraps text with green ANSI code when enabled.
func TestGreen(t *testing.T) {
	disabled, env := saveState()
	defer restoreState(disabled, env)

	_ = os.Unsetenv("NO_COLOR")
	_ = os.Unsetenv("TERM")

	color.Enable()

	result := color.Green("success")
	assert.Contains(t, result, "success")
	assert.Contains(t, result, "\033[32m", "should contain green ANSI code")
	assert.Contains(t, result, "\033[0m", "should contain reset code")

	color.Disable()

	result = color.Green("success")
	assert.Equal(t, "success", result)
}

// TestYellow verifies Yellow() wraps text with yellow ANSI code when enabled.
func TestYellow(t *testing.T) {
	disabled, env := saveState()
	defer restoreState(disabled, env)

	_ = os.Unsetenv("NO_COLOR")
	_ = os.Unsetenv("TERM")

	color.Enable()

	result := color.Yellow("warning")
	assert.Contains(t, result, "warning")
	assert.Contains(t, result, "\033[33m", "should contain yellow ANSI code")
	assert.Contains(t, result, "\033[0m", "should contain reset code")

	color.Disable()

	result = color.Yellow("warning")
	assert.Equal(t, "warning", result)
}

// TestBlue verifies Blue() wraps text with blue ANSI code when enabled.
func TestBlue(t *testing.T) {
	disabled, env := saveState()
	defer restoreState(disabled, env)

	_ = os.Unsetenv("NO_COLOR")
	_ = os.Unsetenv("TERM")

	color.Enable()

	result := color.Blue("info")
	assert.Contains(t, result, "info")
	assert.Contains(t, result, "\033[34m", "should contain blue ANSI code")
	assert.Contains(t, result, "\033[0m", "should contain reset code")

	color.Disable()

	result = color.Blue("info")
	assert.Equal(t, "info", result)
}

// TestMagenta verifies Magenta() wraps text with magenta ANSI code when enabled.
func TestMagenta(t *testing.T) {
	disabled, env := saveState()
	defer restoreState(disabled, env)

	_ = os.Unsetenv("NO_COLOR")
	_ = os.Unsetenv("TERM")

	color.Enable()

	result := color.Magenta("debug")
	assert.Contains(t, result, "debug")
	assert.Contains(t, result, "\033[35m", "should contain magenta ANSI code")
	assert.Contains(t, result, "\033[0m", "should contain reset code")

	color.Disable()

	result = color.Magenta("debug")
	assert.Equal(t, "debug", result)
}

// TestCyan verifies Cyan() wraps text with cyan ANSI code when enabled.
func TestCyan(t *testing.T) {
	disabled, env := saveState()
	defer restoreState(disabled, env)

	_ = os.Unsetenv("NO_COLOR")
	_ = os.Unsetenv("TERM")

	color.Enable()

	result := color.Cyan("trace")
	assert.Contains(t, result, "trace")
	assert.Contains(t, result, "\033[36m", "should contain cyan ANSI code")
	assert.Contains(t, result, "\033[0m", "should contain reset code")

	color.Disable()

	result = color.Cyan("trace")
	assert.Equal(t, "trace", result)
}

// TestWhite verifies White() wraps text with white ANSI code when enabled.
func TestWhite(t *testing.T) {
	disabled, env := saveState()
	defer restoreState(disabled, env)

	_ = os.Unsetenv("NO_COLOR")
	_ = os.Unsetenv("TERM")

	color.Enable()

	result := color.White("text")
	assert.Contains(t, result, "text")
	assert.Contains(t, result, "\033[37m", "should contain white ANSI code")
	assert.Contains(t, result, "\033[0m", "should contain reset code")

	color.Disable()

	result = color.White("text")
	assert.Equal(t, "text", result)
}

// TestHiBlue verifies HiBlue() wraps text with bright blue ANSI code when enabled.
func TestHiBlue(t *testing.T) {
	disabled, env := saveState()
	defer restoreState(disabled, env)

	_ = os.Unsetenv("NO_COLOR")
	_ = os.Unsetenv("TERM")

	color.Enable()

	result := color.HiBlue("bright")
	assert.Contains(t, result, "bright")
	assert.Contains(t, result, "\033[94m", "should contain bright blue ANSI code")
	assert.Contains(t, result, "\033[0m", "should contain reset code")

	color.Disable()

	result = color.HiBlue("bright")
	assert.Equal(t, "bright", result)
}

// TestHiCyan verifies HiCyan() wraps text with bright cyan ANSI code when enabled.
func TestHiCyan(t *testing.T) {
	disabled, env := saveState()
	defer restoreState(disabled, env)

	_ = os.Unsetenv("NO_COLOR")
	_ = os.Unsetenv("TERM")

	color.Enable()

	result := color.HiCyan("bright")
	assert.Contains(t, result, "bright")
	assert.Contains(t, result, "\033[96m", "should contain bright cyan ANSI code")
	assert.Contains(t, result, "\033[0m", "should contain reset code")

	color.Disable()

	result = color.HiCyan("bright")
	assert.Equal(t, "bright", result)
}

// TestBold verifies Bold() wraps text with bold ANSI code when enabled.
func TestBold(t *testing.T) {
	disabled, env := saveState()
	defer restoreState(disabled, env)

	_ = os.Unsetenv("NO_COLOR")
	_ = os.Unsetenv("TERM")

	color.Enable()

	result := color.Bold("bold")
	assert.Contains(t, result, "bold")
	assert.Contains(t, result, "\033[1m", "should contain bold ANSI code")
	assert.Contains(t, result, "\033[0m", "should contain reset code")

	color.Disable()

	result = color.Bold("bold")
	assert.Equal(t, "bold", result)
}

// TestBoldGreen verifies BoldGreen() wraps text with bold+green ANSI codes when enabled.
func TestBoldGreen(t *testing.T) {
	disabled, env := saveState()
	defer restoreState(disabled, env)

	_ = os.Unsetenv("NO_COLOR")
	_ = os.Unsetenv("TERM")

	color.Enable()

	result := color.BoldGreen("success")
	assert.Contains(t, result, "success")
	assert.Contains(t, result, "\033[1m", "should contain bold ANSI code")
	assert.Contains(t, result, "\033[32m", "should contain green ANSI code")
	assert.Contains(t, result, "\033[0m", "should contain reset code")

	color.Disable()

	result = color.BoldGreen("success")
	assert.Equal(t, "success", result)
}

// TestBoldBlue verifies BoldBlue() wraps text with bold+blue ANSI codes when enabled.
func TestBoldBlue(t *testing.T) {
	disabled, env := saveState()
	defer restoreState(disabled, env)

	_ = os.Unsetenv("NO_COLOR")
	_ = os.Unsetenv("TERM")

	color.Enable()

	result := color.BoldBlue("info")
	assert.Contains(t, result, "info")
	assert.Contains(t, result, "\033[1m", "should contain bold ANSI code")
	assert.Contains(t, result, "\033[34m", "should contain blue ANSI code")
	assert.Contains(t, result, "\033[0m", "should contain reset code")

	color.Disable()

	result = color.BoldBlue("info")
	assert.Equal(t, "info", result)
}

// TestBoldCyan verifies BoldCyan() wraps text with bold+cyan ANSI codes when enabled.
func TestBoldCyan(t *testing.T) {
	disabled, env := saveState()
	defer restoreState(disabled, env)

	_ = os.Unsetenv("NO_COLOR")
	_ = os.Unsetenv("TERM")

	color.Enable()

	result := color.BoldCyan("header")
	assert.Contains(t, result, "header")
	assert.Contains(t, result, "\033[1m", "should contain bold ANSI code")
	assert.Contains(t, result, "\033[36m", "should contain cyan ANSI code")
	assert.Contains(t, result, "\033[0m", "should contain reset code")

	color.Disable()

	result = color.BoldCyan("header")
	assert.Equal(t, "header", result)
}

// TestBoldYellow verifies BoldYellow() wraps text with bold+yellow ANSI codes when enabled.
func TestBoldYellow(t *testing.T) {
	disabled, env := saveState()
	defer restoreState(disabled, env)

	_ = os.Unsetenv("NO_COLOR")
	_ = os.Unsetenv("TERM")

	color.Enable()

	result := color.BoldYellow("warning")
	assert.Contains(t, result, "warning")
	assert.Contains(t, result, "\033[1m", "should contain bold ANSI code")
	assert.Contains(t, result, "\033[33m", "should contain yellow ANSI code")
	assert.Contains(t, result, "\033[0m", "should contain reset code")

	color.Disable()

	result = color.BoldYellow("warning")
	assert.Equal(t, "warning", result)
}

// TestBoldMagenta verifies BoldMagenta() wraps text with bold+magenta ANSI codes when enabled.
func TestBoldMagenta(t *testing.T) {
	disabled, env := saveState()
	defer restoreState(disabled, env)

	_ = os.Unsetenv("NO_COLOR")
	_ = os.Unsetenv("TERM")

	color.Enable()

	result := color.BoldMagenta("debug")
	assert.Contains(t, result, "debug")
	assert.Contains(t, result, "\033[1m", "should contain bold ANSI code")
	assert.Contains(t, result, "\033[35m", "should contain magenta ANSI code")
	assert.Contains(t, result, "\033[0m", "should contain reset code")

	color.Disable()

	result = color.BoldMagenta("debug")
	assert.Equal(t, "debug", result)
}

// TestBracket verifies Bracket() wraps text in white brackets with yellow content.
func TestBracket(t *testing.T) {
	disabled, env := saveState()
	defer restoreState(disabled, env)

	_ = os.Unsetenv("NO_COLOR")
	_ = os.Unsetenv("TERM")

	color.Enable()

	result := color.Bracket("OK")
	assert.Contains(t, result, "OK", "should contain original text")
	assert.Contains(t, result, "[", "should contain opening bracket")
	assert.Contains(t, result, "]", "should contain closing bracket")
	assert.Contains(t, result, "\033[37m", "should contain white ANSI code for brackets")
	assert.Contains(t, result, "\033[33m", "should contain yellow ANSI code for content")

	color.Disable()

	result = color.Bracket("OK")
	assert.Equal(t, "[OK]", result, "should return plain bracketed text when disabled")
}

// TestBracketEmpty verifies Bracket() handles empty strings.
func TestBracketEmpty(t *testing.T) {
	disabled, env := saveState()
	defer restoreState(disabled, env)

	_ = os.Unsetenv("NO_COLOR")
	_ = os.Unsetenv("TERM")

	color.Enable()

	result := color.Bracket("")
	assert.Contains(t, result, "[")
	assert.Contains(t, result, "]")

	color.Disable()

	result = color.Bracket("")
	assert.Equal(t, "[]", result)
}

// TestSection verifies Section() returns a bold title with newlines.
func TestSection(t *testing.T) {
	disabled, env := saveState()
	defer restoreState(disabled, env)

	_ = os.Unsetenv("NO_COLOR")
	_ = os.Unsetenv("TERM")

	color.Enable()

	result := color.Section("Title")
	assert.Contains(t, result, "Title", "should contain title text")
	assert.Contains(t, result, "\033[1m", "should contain bold ANSI code")
	assert.Contains(t, result, "\033[0m", "should contain reset code")
	assert.True(t, strings.HasPrefix(result, "\n"), "should start with newline")
	assert.True(t, strings.HasSuffix(result, "\n"), "should end with newline")

	color.Disable()

	result = color.Section("Title")
	assert.Equal(t, "\nTitle\n", result, "should return plain title with newlines when disabled")
}

// TestTableEmpty verifies Table() returns empty string for empty rows.
func TestTableEmpty(t *testing.T) {
	disabled, env := saveState()
	defer restoreState(disabled, env)

	_ = os.Unsetenv("NO_COLOR")
	_ = os.Unsetenv("TERM")

	color.Enable()

	result := color.Table([][]string{}, false)
	assert.Equal(t, "", result, "should return empty string for empty rows")

	color.Disable()

	result = color.Table([][]string{}, false)
	assert.Equal(t, "", result, "should return empty string for empty rows when disabled")
}

// TestTableNoHeader verifies Table() renders rows without header formatting.
func TestTableNoHeader(t *testing.T) {
	disabled, env := saveState()
	defer restoreState(disabled, env)

	_ = os.Unsetenv("NO_COLOR")
	_ = os.Unsetenv("TERM")

	color.Enable()

	rows := [][]string{
		{"Name", "Value"},
		{"foo", "bar"},
		{"baz", "qux"},
	}
	result := color.Table(rows, false)

	// Should contain all values
	assert.Contains(t, result, "Name")
	assert.Contains(t, result, "Value")
	assert.Contains(t, result, "foo")
	assert.Contains(t, result, "bar")
	assert.Contains(t, result, "baz")
	assert.Contains(t, result, "qux")

	// Should have separator
	assert.Contains(t, result, " | ")

	// Should have newlines
	lines := strings.Split(strings.TrimSuffix(result, "\n"), "\n")
	assert.Equal(t, 3, len(lines), "should have 3 rows")

	color.Disable()

	result = color.Table(rows, false)
	// When disabled, should still have structure but no ANSI codes
	assert.NotContains(t, result, "\033[")
	assert.Contains(t, result, "Name")
	assert.Contains(t, result, " | ")
}

// TestTableWithHeader verifies Table() renders header row in bold cyan.
func TestTableWithHeader(t *testing.T) {
	disabled, env := saveState()
	defer restoreState(disabled, env)

	_ = os.Unsetenv("NO_COLOR")
	_ = os.Unsetenv("TERM")

	color.Enable()

	rows := [][]string{
		{"Name", "Value"},
		{"foo", "bar"},
		{"baz", "qux"},
	}
	result := color.Table(rows, true)

	// Should contain all values
	assert.Contains(t, result, "Name")
	assert.Contains(t, result, "Value")
	assert.Contains(t, result, "foo")
	assert.Contains(t, result, "bar")

	// Header should be bold cyan
	assert.Contains(t, result, "\033[1m", "should contain bold code for header")
	assert.Contains(t, result, "\033[36m", "should contain cyan code for header")

	// Should have separator
	assert.Contains(t, result, " | ")

	color.Disable()

	result = color.Table(rows, true)
	// When disabled, should still have structure but no ANSI codes
	assert.NotContains(t, result, "\033[")
	assert.Contains(t, result, "Name")
	assert.Contains(t, result, " | ")
}

// TestTableColumnAlignment verifies Table() pads columns correctly.
func TestTableColumnAlignment(t *testing.T) {
	disabled, env := saveState()
	defer restoreState(disabled, env)

	_ = os.Unsetenv("NO_COLOR")
	_ = os.Unsetenv("TERM")

	color.Enable()

	rows := [][]string{
		{"A", "BB"},
		{"CCC", "D"},
	}
	result := color.Table(rows, false)

	// Strip ANSI codes for easier testing
	stripped := stripANSI(result)

	// First row should have "A" padded to 3 chars (width of "CCC")
	// Second row should have "D" padded to 2 chars (width of "BB")
	lines := strings.Split(strings.TrimSuffix(stripped, "\n"), "\n")
	assert.Equal(t, 2, len(lines))

	// Check that columns are aligned
	assert.Contains(t, lines[0], "A  ")
	assert.Contains(t, lines[1], "CCC")

	color.Disable()

	result = color.Table(rows, false)
	stripped = stripANSI(result)
	lines = strings.Split(strings.TrimSuffix(stripped, "\n"), "\n")
	assert.Contains(t, lines[0], "A  ")
	assert.Contains(t, lines[1], "CCC")
}

// TestTableSingleRow verifies Table() handles single row correctly.
func TestTableSingleRow(t *testing.T) {
	disabled, env := saveState()
	defer restoreState(disabled, env)

	_ = os.Unsetenv("NO_COLOR")
	_ = os.Unsetenv("TERM")

	color.Enable()

	rows := [][]string{
		{"Single", "Row"},
	}
	result := color.Table(rows, false)

	assert.Contains(t, result, "Single")
	assert.Contains(t, result, "Row")
	assert.Contains(t, result, " | ")

	color.Disable()

	result = color.Table(rows, false)
	assert.Contains(t, result, "Single")
	assert.Contains(t, result, "Row")
	assert.Contains(t, result, " | ")
}

// TestTableMultipleColumns verifies Table() handles many columns.
func TestTableMultipleColumns(t *testing.T) {
	disabled, env := saveState()
	defer restoreState(disabled, env)

	_ = os.Unsetenv("NO_COLOR")
	_ = os.Unsetenv("TERM")

	color.Enable()

	rows := [][]string{
		{"Col1", "Col2", "Col3", "Col4"},
		{"a", "b", "c", "d"},
		{"e", "f", "g", "h"},
	}
	result := color.Table(rows, true)

	assert.Contains(t, result, "Col1")
	assert.Contains(t, result, "Col2")
	assert.Contains(t, result, "Col3")
	assert.Contains(t, result, "Col4")
	assert.Contains(t, result, " | ")

	// Count separators per line (should be 3 for 4 columns)
	lines := strings.Split(strings.TrimSuffix(result, "\n"), "\n")
	assert.Equal(t, 3, len(lines))

	color.Disable()

	result = color.Table(rows, true)
	assert.Contains(t, result, "Col1")
	assert.Contains(t, result, "Col4")
}

// TestTableUnevenColumns verifies Table() handles rows with different column counts.
func TestTableUnevenColumns(t *testing.T) {
	disabled, env := saveState()
	defer restoreState(disabled, env)

	_ = os.Unsetenv("NO_COLOR")
	_ = os.Unsetenv("TERM")

	color.Enable()

	rows := [][]string{
		{"A", "B", "C"},
		{"D", "E"},
		{"F"},
	}
	result := color.Table(rows, false)

	assert.Contains(t, result, "A")
	assert.Contains(t, result, "B")
	assert.Contains(t, result, "C")
	assert.Contains(t, result, "D")
	assert.Contains(t, result, "E")
	assert.Contains(t, result, "F")

	color.Disable()

	result = color.Table(rows, false)
	assert.Contains(t, result, "A")
	assert.Contains(t, result, "F")
}

// TestTableSeparatorColor verifies Table() uses gray for separators.
func TestTableSeparatorColor(t *testing.T) {
	disabled, env := saveState()
	defer restoreState(disabled, env)

	_ = os.Unsetenv("NO_COLOR")
	_ = os.Unsetenv("TERM")

	color.Enable()

	rows := [][]string{
		{"A", "B"},
		{"C", "D"},
	}
	result := color.Table(rows, false)

	// Separator should be gray
	assert.Contains(t, result, "\033[90m", "should contain gray code for separator")

	color.Disable()

	result = color.Table(rows, false)
	assert.NotContains(t, result, "\033[90m")
}

// TestColorDisabledByNOCOLOREnv verifies all color functions respect NO_COLOR.
func TestColorDisabledByNOCOLOREnv(t *testing.T) {
	disabled, env := saveState()
	defer restoreState(disabled, env)

	color.Enable()

	_ = os.Setenv("NO_COLOR", "1")
	_ = os.Unsetenv("TERM")

	// All color functions should return plain text
	assert.Equal(t, "text", color.Red("text"))
	assert.Equal(t, "text", color.Green("text"))
	assert.Equal(t, "text", color.Blue("text"))
	assert.Equal(t, "text", color.Bold("text"))
	assert.Equal(t, "[text]", color.Bracket("text"))
	assert.Equal(t, "\ntext\n", color.Section("text"))
}

// TestColorDisabledByTERMDumbEnv verifies all color functions respect TERM=dumb.
func TestColorDisabledByTERMDumbEnv(t *testing.T) {
	disabled, env := saveState()
	defer restoreState(disabled, env)

	color.Enable()

	_ = os.Unsetenv("NO_COLOR")
	_ = os.Setenv("TERM", "dumb")

	// All color functions should return plain text
	assert.Equal(t, "text", color.Red("text"))
	assert.Equal(t, "text", color.Green("text"))
	assert.Equal(t, "text", color.Blue("text"))
	assert.Equal(t, "text", color.Bold("text"))
	assert.Equal(t, "[text]", color.Bracket("text"))
	assert.Equal(t, "\ntext\n", color.Section("text"))
}

// TestColorEnabledWithValidTERM verifies colors work with valid TERM values.
func TestColorEnabledWithValidTERM(t *testing.T) {
	disabled, env := saveState()
	defer restoreState(disabled, env)

	color.Enable()

	_ = os.Unsetenv("NO_COLOR")
	_ = os.Setenv("TERM", "xterm-256color")

	// All color functions should return colored text
	assert.Contains(t, color.Red("text"), "\033[31m")
	assert.Contains(t, color.Green("text"), "\033[32m")
	assert.Contains(t, color.Blue("text"), "\033[34m")
	assert.Contains(t, color.Bold("text"), "\033[1m")
}

// TestEmptyString verifies color functions handle empty strings.
func TestEmptyString(t *testing.T) {
	disabled, env := saveState()
	defer restoreState(disabled, env)

	_ = os.Unsetenv("NO_COLOR")
	_ = os.Unsetenv("TERM")

	color.Enable()
	assert.Contains(t, color.Red(""), "\033[31m")
	assert.Contains(t, color.Green(""), "\033[32m")
	assert.Contains(t, color.Bold(""), "\033[1m")

	color.Disable()
	assert.Equal(t, "", color.Red(""))
	assert.Equal(t, "", color.Green(""))
	assert.Equal(t, "", color.Bold(""))
}

// TestStringWithSpecialChars verifies color functions handle special characters.
func TestStringWithSpecialChars(t *testing.T) {
	disabled, env := saveState()
	defer restoreState(disabled, env)

	_ = os.Unsetenv("NO_COLOR")
	_ = os.Unsetenv("TERM")

	color.Enable()

	special := "test\nwith\ttabs"
	result := color.Red(special)
	assert.Contains(t, result, special)

	color.Disable()

	result = color.Red(special)
	assert.Equal(t, special, result)
}

// stripANSI removes ANSI escape sequences from a string for easier testing.
func stripANSI(s string) string {
	// Simple regex-free approach: remove all \033[...m sequences
	var result strings.Builder

	i := 0
	for i < len(s) {
		if i+1 < len(s) && s[i:i+2] == "\033[" {
			// Find the 'm' that ends the sequence
			j := i + 2
			for j < len(s) && s[j] != 'm' {
				j++
			}

			if j < len(s) {
				i = j + 1
				continue
			}
		}

		result.WriteByte(s[i])
		i++
	}

	return result.String()
}
