// Package color provides minimal ANSI terminal color helpers.
// It has zero external dependencies and respects the NO_COLOR env var
// and a runtime disable flag (set by --no-color).
package color

import (
	"fmt"
	"os"
	"strings"
)

// ANSI escape sequences.
const (
	reset     = "\033[0m"
	bold      = "\033[1m"
	fgGray    = "\033[90m"
	fgRed     = "\033[31m"
	fgGreen   = "\033[32m"
	fgYellow  = "\033[33m"
	fgBlue    = "\033[34m"
	fgMagenta = "\033[35m"
	fgCyan    = "\033[36m"
	fgWhite   = "\033[37m"
	fgHiBlue  = "\033[94m"
	fgHiCyan  = "\033[96m"
)

var disabled bool

// Disable turns off all color output (called by --no-color flag).
func Disable() { disabled = true }

// Enable turns color output back on.
func Enable() { disabled = false }

// IsDisabled reports whether color is currently disabled.
func IsDisabled() bool {
	return disabled || os.Getenv("NO_COLOR") != "" || os.Getenv("TERM") == "dumb"
}

func wrap(code, s string) string {
	if IsDisabled() {
		return s
	}

	return code + s + reset
}

// Gray returns s in dark gray.
func Gray(s string) string { return wrap(fgGray, s) }

// Red returns s in red.
func Red(s string) string { return wrap(fgRed, s) }

// Green returns s in green.
func Green(s string) string { return wrap(fgGreen, s) }

// Yellow returns s in yellow.
func Yellow(s string) string { return wrap(fgYellow, s) }

// Blue returns s in blue.
func Blue(s string) string { return wrap(fgBlue, s) }

// Magenta returns s in magenta.
func Magenta(s string) string { return wrap(fgMagenta, s) }

// Cyan returns s in cyan.
func Cyan(s string) string { return wrap(fgCyan, s) }

// White returns s in white.
func White(s string) string { return wrap(fgWhite, s) }

// HiBlue returns s in bright blue.
func HiBlue(s string) string { return wrap(fgHiBlue, s) }

// HiCyan returns s in bright cyan.
func HiCyan(s string) string { return wrap(fgHiCyan, s) }

// Bold returns s in bold.
func Bold(s string) string { return wrap(bold, s) }

// BoldGreen returns s bold green.
func BoldGreen(s string) string { return wrap(bold+fgGreen, s) }

// BoldBlue returns s bold blue.
func BoldBlue(s string) string { return wrap(bold+fgBlue, s) }

// BoldCyan returns s bold cyan.
func BoldCyan(s string) string { return wrap(bold+fgCyan, s) }

// BoldYellow returns s bold yellow.
func BoldYellow(s string) string { return wrap(bold+fgYellow, s) }

// BoldMagenta returns s bold magenta.
func BoldMagenta(s string) string { return wrap(bold+fgMagenta, s) }

// Bracket wraps s in white brackets with yellow content: [s].
func Bracket(s string) string {
	return wrap(fgWhite, "[") + wrap(fgYellow, s) + wrap(fgWhite, "]")
}

// Section prints a bold section header (## Title).
func Section(title string) string {
	return fmt.Sprintf("\n%s\n", Bold(title))
}

// Table renders rows as a left-aligned plain-text table with a " | " separator.
// The first row is treated as a header and printed in bold cyan.
func Table(rows [][]string, hasHeader bool) string {
	if len(rows) == 0 {
		return ""
	}

	// Calculate column widths.
	cols := 0
	for _, row := range rows {
		if len(row) > cols {
			cols = len(row)
		}
	}

	widths := make([]int, cols)

	for _, row := range rows {
		for c, cell := range row {
			if len(cell) > widths[c] {
				widths[c] = len(cell)
			}
		}
	}

	var sb strings.Builder

	for r, row := range rows {
		for c, cell := range row {
			pad := widths[c] - len(cell)

			if hasHeader && r == 0 {
				// Color only the text; pad after so ANSI reset doesn't eat spaces.
				cell = BoldCyan(cell)
			}

			if c > 0 {
				sb.WriteString(Gray(" | "))
			}

			sb.WriteString(cell)
			sb.WriteString(spaces(pad))
		}

		sb.WriteByte('\n')
	}

	return sb.String()
}

func spaces(n int) string {
	if n <= 0 {
		return ""
	}

	b := make([]byte, n)
	for i := range b {
		b[i] = ' '
	}

	return string(b)
}
