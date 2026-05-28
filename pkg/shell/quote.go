package shell

import "strings"

// SingleQuote wraps s in single quotes, escaping any inner single quote via
// the standard POSIX `'\”` trick. Safe for use in arbitrary `/bin/sh -c`
// command bodies. The empty string returns `”`.
func SingleQuote(s string) string {
	var b strings.Builder

	b.WriteByte('\'')

	for _, c := range s {
		if c == '\'' {
			b.WriteString(`'\''`)
		} else {
			b.WriteRune(c)
		}
	}

	b.WriteByte('\'')

	return b.String()
}

// Join single-quotes each arg and joins them with a space — suitable for
// concatenating an argv into a single `/bin/sh -c` script.
func Join(args []string) string {
	var out strings.Builder

	for i, a := range args {
		if i > 0 {
			out.WriteByte(' ')
		}

		out.WriteString(SingleQuote(a))
	}

	return out.String()
}
