// Package deb822 provides a streaming parser for RFC-822-style multi-stanza
// key/value format used by Debian package metadata (apt Packages files, dpkg status, etc.).
package deb822

import (
	"bufio"
	"io"
	"strings"

	"github.com/M0Rf30/yap/v2/pkg/errors"
)

// Stanza is a parsed DEB822 paragraph: ordered map of field -> raw value
// (continuation lines preserved with leading-space marker stripped).
type Stanza map[string]string

// Parse calls fn for each stanza in r. Returning an error from fn aborts.
// Empty lines separate stanzas. Lines starting with '#' are comments (skipped at top level only).
// Continuation lines (start with space/tab) append to the previous field.
// A " ." (space + dot) continuation line is a blank-line marker; translate to
// a literal blank line in the value.
//
//nolint:gocyclo,cyclop // deb822 stanza parser inherently branchy
func Parse(r io.Reader, fn func(Stanza) error) error {
	scanner := bufio.NewScanner(r)
	// Some Packages files have very long Description lines.
	scanner.Buffer(make([]byte, 256*1024), 256*1024)

	stanza := make(Stanza)

	var (
		currentField string
		currentValue strings.Builder
	)

	for scanner.Scan() {
		line := scanner.Text()

		// Blank line → end of stanza.
		if line == "" {
			// Flush the in-flight field.
			if currentField != "" {
				stanza[currentField] = currentValue.String()
			}

			// Call the callback if we have a non-empty stanza.
			if len(stanza) > 0 {
				if err := fn(stanza); err != nil {
					return err
				}
			}

			// Reset for next stanza.
			stanza = make(Stanza)
			currentField = ""

			currentValue.Reset()

			continue
		}

		// Comment line (starts with '#') — skip at top level only (not inside multi-line values).
		if currentField == "" && strings.HasPrefix(line, "#") {
			continue
		}

		// Continuation line (starts with space or tab).
		if line != "" && (line[0] == ' ' || line[0] == '\t') {
			appendContinuation(&currentValue, line)
			continue
		}

		// Field line: "FieldName: value"
		field, value, ok := strings.Cut(line, ":")
		if !ok {
			// Malformed line — skip it.
			continue
		}

		// Flush the previous field.
		if currentField != "" {
			stanza[currentField] = currentValue.String()
		}

		// Start the new field.
		currentField = field

		currentValue.Reset()
		currentValue.WriteString(strings.TrimSpace(value))
	}

	// Flush the last stanza (file may not end with a blank line).
	if currentField != "" {
		stanza[currentField] = currentValue.String()
	}

	if len(stanza) > 0 {
		if err := fn(stanza); err != nil {
			return err
		}
	}

	if err := scanner.Err(); err != nil {
		return errors.Wrap(err, errors.ErrTypeParser, "failed to parse deb822 format").
			WithOperation("Parse")
	}

	return nil
}

// appendContinuation appends a deb822 continuation line to the in-flight value.
// Handles the " ." blank-line marker and strips the leading space/tab marker.
func appendContinuation(buf *strings.Builder, line string) {
	if line == " ." {
		buf.WriteString("\n")
		return
	}

	buf.WriteString("\n")

	if line[0] == ' ' {
		buf.WriteString(strings.TrimPrefix(line, " "))
	} else {
		buf.WriteString(strings.TrimPrefix(line, "\t"))
	}
}
