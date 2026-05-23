package deb822_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/M0Rf30/yap/v2/pkg/deb822"
)

func TestParse_SingleStanza(t *testing.T) {
	input := `Package: hello
Version: 1.0
Description: A simple package
`
	var stanzas []deb822.Stanza
	err := deb822.Parse(strings.NewReader(input), func(s deb822.Stanza) error {
		stanzas = append(stanzas, s)
		return nil
	})

	require.NoError(t, err)
	require.Len(t, stanzas, 1)
	assert.Equal(t, "hello", stanzas[0]["Package"])
	assert.Equal(t, "1.0", stanzas[0]["Version"])
	assert.Equal(t, "A simple package", stanzas[0]["Description"])
}

func TestParse_MultipleStanzas(t *testing.T) {
	input := `Package: hello
Version: 1.0

Package: world
Version: 2.0

`
	var stanzas []deb822.Stanza
	err := deb822.Parse(strings.NewReader(input), func(s deb822.Stanza) error {
		stanzas = append(stanzas, s)
		return nil
	})

	require.NoError(t, err)
	require.Len(t, stanzas, 2)
	assert.Equal(t, "hello", stanzas[0]["Package"])
	assert.Equal(t, "world", stanzas[1]["Package"])
}

func TestParse_MultilineContinuation(t *testing.T) {
	input := `Package: hello
Description: This is a long
 description that spans
 multiple lines
Version: 1.0

`
	var stanzas []deb822.Stanza
	err := deb822.Parse(strings.NewReader(input), func(s deb822.Stanza) error {
		stanzas = append(stanzas, s)
		return nil
	})

	require.NoError(t, err)
	require.Len(t, stanzas, 1)
	// Continuation lines should be preserved with newlines
	expected := "This is a long\ndescription that spans\nmultiple lines"
	assert.Equal(t, expected, stanzas[0]["Description"])
}

func TestParse_BlankLineMarker(t *testing.T) {
	input := `Package: hello
Description: First paragraph
 .
 Second paragraph
Version: 1.0

`
	var stanzas []deb822.Stanza
	err := deb822.Parse(strings.NewReader(input), func(s deb822.Stanza) error {
		stanzas = append(stanzas, s)
		return nil
	})

	require.NoError(t, err)
	require.Len(t, stanzas, 1)
	// " ." should translate to a blank line
	expected := "First paragraph\n\nSecond paragraph"
	assert.Equal(t, expected, stanzas[0]["Description"])
}

func TestParse_CommentsAtTopLevel(t *testing.T) {
	input := `# This is a comment
Package: hello
# Another comment
Version: 1.0

`
	var stanzas []deb822.Stanza
	err := deb822.Parse(strings.NewReader(input), func(s deb822.Stanza) error {
		stanzas = append(stanzas, s)
		return nil
	})

	require.NoError(t, err)
	require.Len(t, stanzas, 1)
	assert.Equal(t, "hello", stanzas[0]["Package"])
	assert.Equal(t, "1.0", stanzas[0]["Version"])
}

func TestParse_CommentsInsideContinuation(t *testing.T) {
	// Comments inside continuation lines should NOT be stripped — they're part of the value
	input := `Package: hello
Description: First line
 # This is not a comment, it's part of the value
 Second line
Version: 1.0

`
	var stanzas []deb822.Stanza
	err := deb822.Parse(strings.NewReader(input), func(s deb822.Stanza) error {
		stanzas = append(stanzas, s)
		return nil
	})

	require.NoError(t, err)
	require.Len(t, stanzas, 1)
	expected := "First line\n# This is not a comment, it's part of the value\nSecond line"
	assert.Equal(t, expected, stanzas[0]["Description"])
}

func TestParse_EmptyInput(t *testing.T) {
	input := ""
	var stanzas []deb822.Stanza
	err := deb822.Parse(strings.NewReader(input), func(s deb822.Stanza) error {
		stanzas = append(stanzas, s)
		return nil
	})

	require.NoError(t, err)
	require.Len(t, stanzas, 0)
}

func TestParse_EOFWithoutTrailingNewline(t *testing.T) {
	input := `Package: hello
Version: 1.0`
	var stanzas []deb822.Stanza
	err := deb822.Parse(strings.NewReader(input), func(s deb822.Stanza) error {
		stanzas = append(stanzas, s)
		return nil
	})

	require.NoError(t, err)
	require.Len(t, stanzas, 1)
	assert.Equal(t, "hello", stanzas[0]["Package"])
	assert.Equal(t, "1.0", stanzas[0]["Version"])
}

func TestParse_StanzaWithTrailingBlankLine(t *testing.T) {
	input := `Package: hello
Version: 1.0

`
	var stanzas []deb822.Stanza
	err := deb822.Parse(strings.NewReader(input), func(s deb822.Stanza) error {
		stanzas = append(stanzas, s)
		return nil
	})

	require.NoError(t, err)
	require.Len(t, stanzas, 1)
	assert.Equal(t, "hello", stanzas[0]["Package"])
}

func TestParse_TabContinuation(t *testing.T) {
	input := `Package: hello
Description: First line
	Second line with tab
Version: 1.0

`
	var stanzas []deb822.Stanza
	err := deb822.Parse(strings.NewReader(input), func(s deb822.Stanza) error {
		stanzas = append(stanzas, s)
		return nil
	})

	require.NoError(t, err)
	require.Len(t, stanzas, 1)
	expected := "First line\nSecond line with tab"
	assert.Equal(t, expected, stanzas[0]["Description"])
}

func TestParse_MalformedLines(t *testing.T) {
	// Lines without colons should be skipped
	input := `Package: hello
This line has no colon
Version: 1.0

`
	var stanzas []deb822.Stanza
	err := deb822.Parse(strings.NewReader(input), func(s deb822.Stanza) error {
		stanzas = append(stanzas, s)
		return nil
	})

	require.NoError(t, err)
	require.Len(t, stanzas, 1)
	assert.Equal(t, "hello", stanzas[0]["Package"])
	assert.Equal(t, "1.0", stanzas[0]["Version"])
	assert.NotContains(t, stanzas[0], "This line has no colon")
}

func TestParse_CallbackError(t *testing.T) {
	input := `Package: hello
Version: 1.0

Package: world
Version: 2.0

`
	var stanzas []deb822.Stanza
	callCount := 0
	testErr := &testError{msg: "test error"}
	err := deb822.Parse(strings.NewReader(input), func(s deb822.Stanza) error {
		stanzas = append(stanzas, s)
		callCount++
		if callCount > 1 {
			return testErr
		}
		return nil
	})

	require.Error(t, err)
	require.Equal(t, testErr, err)
	require.Len(t, stanzas, 2)
}

type testError struct {
	msg string
}

func (e *testError) Error() string {
	return e.msg
}

func TestParse_WhitespaceHandling(t *testing.T) {
	input := `Package:   hello   
Version:  1.0  
Description:  A package  

`
	var stanzas []deb822.Stanza
	err := deb822.Parse(strings.NewReader(input), func(s deb822.Stanza) error {
		stanzas = append(stanzas, s)
		return nil
	})

	require.NoError(t, err)
	require.Len(t, stanzas, 1)
	// Values should be trimmed
	assert.Equal(t, "hello", stanzas[0]["Package"])
	assert.Equal(t, "1.0", stanzas[0]["Version"])
	assert.Equal(t, "A package", stanzas[0]["Description"])
}

func TestParse_ComplexMultilineDescription(t *testing.T) {
	// Real-world example from apt Packages file
	input := `Package: curl
Version: 7.68.0-1ubuntu1
Description: command line tool for transferring data with URLs
 curl is a tool to transfer data from or to a server, using one of the
 supported protocols (HTTPS, FTP, FTPS, GOPHER, HTTP, IMAP, IMAPS, LDAP,
 LDAPS, POP3, POP3S, RTMP, RTMPS, RTSP, SCP, SFTP, SMTP, SMTPS, TELNET
 and TFTP). The command is designed to work without user interaction.
 .
 curl offers a busload of useful tricks like proxy support, user
 authentication, FTP upload, HTTP post, SSL connections, cookies, file
 transfer resume and more.

`
	var stanzas []deb822.Stanza
	err := deb822.Parse(strings.NewReader(input), func(s deb822.Stanza) error {
		stanzas = append(stanzas, s)
		return nil
	})

	require.NoError(t, err)
	require.Len(t, stanzas, 1)
	desc := stanzas[0]["Description"]
	assert.Contains(t, desc, "command line tool for transferring data with URLs")
	assert.Contains(t, desc, "curl offers a busload of useful tricks")
	// Check that the blank line marker was converted
	assert.Contains(t, desc, "\n\ncurl offers")
}

func TestParse_EmptyStanzas(t *testing.T) {
	// Multiple blank lines should not create empty stanzas
	input := `Package: hello
Version: 1.0


Package: world
Version: 2.0

`
	var stanzas []deb822.Stanza
	err := deb822.Parse(strings.NewReader(input), func(s deb822.Stanza) error {
		stanzas = append(stanzas, s)
		return nil
	})

	require.NoError(t, err)
	require.Len(t, stanzas, 2)
	assert.Equal(t, "hello", stanzas[0]["Package"])
	assert.Equal(t, "world", stanzas[1]["Package"])
}

func TestParse_FieldOrder(t *testing.T) {
	// Stanza is a map, so order is not guaranteed, but all fields should be present
	input := `Package: hello
Version: 1.0
Architecture: amd64
Depends: libc6
Description: A package

`
	var stanzas []deb822.Stanza
	err := deb822.Parse(strings.NewReader(input), func(s deb822.Stanza) error {
		stanzas = append(stanzas, s)
		return nil
	})

	require.NoError(t, err)
	require.Len(t, stanzas, 1)
	assert.Equal(t, "hello", stanzas[0]["Package"])
	assert.Equal(t, "1.0", stanzas[0]["Version"])
	assert.Equal(t, "amd64", stanzas[0]["Architecture"])
	assert.Equal(t, "libc6", stanzas[0]["Depends"])
	assert.Equal(t, "A package", stanzas[0]["Description"])
}
