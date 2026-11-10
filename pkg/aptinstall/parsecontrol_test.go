// SPDX-FileCopyrightText: 2021-2026 Gianluca Boiano
// SPDX-License-Identifier: GPL-3.0-only

package aptinstall_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/M0Rf30/yap/v2/pkg/aptinstall"
)

// TestParseControlSimpleSingleLineFields tests parsing of simple single-line fields.
func TestParseControlSimpleSingleLineFields(t *testing.T) {
	t.Parallel()

	control := `Package: gcc
Version: 12.3.0-1
Architecture: amd64
`

	fields := aptinstall.ParseControlForTesting(control)

	assert.Equal(t, "gcc", fields["Package"])
	assert.Equal(t, "12.3.0-1", fields["Version"])
	assert.Equal(t, "amd64", fields["Architecture"])
	assert.Len(t, fields, 3)
}

// TestParseControlEmptyControl tests parsing an empty control string.
func TestParseControlEmptyControl(t *testing.T) {
	t.Parallel()

	control := ""
	fields := aptinstall.ParseControlForTesting(control)

	assert.Empty(t, fields)
}

// TestParseControlFieldWithEmptyValue tests parsing a field with an empty value.
func TestParseControlFieldWithEmptyValue(t *testing.T) {
	t.Parallel()

	control := `Package: test
Conffiles:
Version: 1.0
`

	fields := aptinstall.ParseControlForTesting(control)

	assert.Equal(t, "test", fields["Package"])
	assert.Equal(t, "", fields["Conffiles"])
	assert.Equal(t, "1.0", fields["Version"])
}

// TestParseControlMultilineWithSpaceContinuation tests multi-line fields
// with space-prefixed continuation lines.
func TestParseControlMultilineWithSpaceContinuation(t *testing.T) {
	t.Parallel()

	control := `Package: gcc
Description: GNU C compiler
 This is the GNU C compiler, a fairly portable optimizing compiler for C.
 It can compile C as well as C++.
Version: 12.3.0-1
`

	fields := aptinstall.ParseControlForTesting(control)

	assert.Equal(t, "gcc", fields["Package"])
	assert.Equal(t, "12.3.0-1", fields["Version"])
	// Multi-line value should have newlines preserved (leading space stripped).
	assert.Contains(t, fields["Description"], "GNU C compiler")
	assert.Contains(t, fields["Description"], "This is the GNU C compiler")
	assert.Contains(t, fields["Description"], "It can compile C as well as C++")
}

// TestParseControlMultilineWithTabContinuation tests multi-line fields
// with tab-prefixed continuation lines.
func TestParseControlMultilineWithTabContinuation(t *testing.T) {
	t.Parallel()

	control := `Package: test
Depends: libfoo,
	libbar,
	libbaz
Version: 1.0
`

	fields := aptinstall.ParseControlForTesting(control)

	assert.Equal(t, "test", fields["Package"])
	assert.Equal(t, "1.0", fields["Version"])
	// Tab continuation should be treated like space continuation.
	assert.Contains(t, fields["Depends"], "libfoo")
	assert.Contains(t, fields["Depends"], "libbar")
	assert.Contains(t, fields["Depends"], "libbaz")
}

// TestParseControlLastFieldFlushed tests that the last field in the control
// string is properly flushed (no trailing newline required).
func TestParseControlLastFieldFlushed(t *testing.T) {
	t.Parallel()

	control := `Package: test
Version: 1.0
Description: Last field without trailing newline`

	fields := aptinstall.ParseControlForTesting(control)

	assert.Equal(t, "test", fields["Package"])
	assert.Equal(t, "1.0", fields["Version"])
	assert.Equal(t, "Last field without trailing newline", fields["Description"])
}

// TestParseControlMultipleFields tests parsing multiple fields in sequence.
func TestParseControlMultipleFields(t *testing.T) {
	t.Parallel()

	control := `Package: curl
Version: 7.85.0-1
Architecture: amd64
Maintainer: Debian Curl Maintainers <pkg-curl-maintainers@lists.alioth.debian.org>
Installed-Size: 1234
Depends: libc6 (>= 2.34), libcurl4 (= 7.85.0-1)
Conflicts: curl-ssl
Provides: curl-replacement
Description: command line tool for transferring data with URLs
 curl is a tool to transfer data from or to a server, using URLs as
 the locator. It supports a range of protocols.
`

	fields := aptinstall.ParseControlForTesting(control)

	assert.Equal(t, "curl", fields["Package"])
	assert.Equal(t, "7.85.0-1", fields["Version"])
	assert.Equal(t, "amd64", fields["Architecture"])
	assert.Equal(t, "Debian Curl Maintainers <pkg-curl-maintainers@lists.alioth.debian.org>", fields["Maintainer"])
	assert.Equal(t, "1234", fields["Installed-Size"])
	assert.Contains(t, fields["Depends"], "libc6")
	assert.Equal(t, "curl-ssl", fields["Conflicts"])
	assert.Equal(t, "curl-replacement", fields["Provides"])
	assert.Contains(t, fields["Description"], "command line tool")
	assert.Len(t, fields, 9)
}

// TestParseControlWhitespaceHandling tests that leading/trailing whitespace
// in field values is properly trimmed.
func TestParseControlWhitespaceHandling(t *testing.T) {
	t.Parallel()

	control := `Package:   test   
Version:  1.0  
Architecture:amd64
`

	fields := aptinstall.ParseControlForTesting(control)

	// Leading/trailing spaces in values should be trimmed.
	assert.Equal(t, "test", fields["Package"])
	assert.Equal(t, "1.0", fields["Version"])
	assert.Equal(t, "amd64", fields["Architecture"])
}

// TestParseControlComplexMultilineDescription tests a complex multi-line
// Description field with dots and special formatting (as seen in real dpkg status).
func TestParseControlComplexMultilineDescription(t *testing.T) {
	t.Parallel()

	control := `Package: python3-minimal
Version: 3.11.2-1+0~20230227.5+debian~11.1+1
Description: minimal Python 3 runtime
 This package contains the minimal set of modules needed to run Python 3.
 .
 These include:
  * apt-get for retrieval of packages
  * apt-cache for searching packages
`

	fields := aptinstall.ParseControlForTesting(control)

	assert.Equal(t, "python3-minimal", fields["Package"])
	desc := fields["Description"]
	assert.Contains(t, desc, "minimal Python 3 runtime")
	assert.Contains(t, desc, "This package contains the minimal set")
	assert.Contains(t, desc, "These include:")
	assert.Contains(t, desc, "apt-get for retrieval")
}

// TestParseControlFieldNameWithHyphens tests field names containing hyphens.
func TestParseControlFieldNameWithHyphens(t *testing.T) {
	t.Parallel()

	control := `Package: test
Installed-Size: 1024
Pre-Depends: libc6
Build-Depends: gcc, make
`

	fields := aptinstall.ParseControlForTesting(control)

	assert.Equal(t, "test", fields["Package"])
	assert.Equal(t, "1024", fields["Installed-Size"])
	assert.Equal(t, "libc6", fields["Pre-Depends"])
	assert.Equal(t, "gcc, make", fields["Build-Depends"])
}

// TestParseControlNoColonLine tests that lines without colons are skipped
// (not treated as field headers).
func TestParseControlNoColonLine(t *testing.T) {
	t.Parallel()

	control := `Package: test
This line has no colon
Version: 1.0
`

	fields := aptinstall.ParseControlForTesting(control)

	// The line without a colon should be ignored.
	assert.Equal(t, "test", fields["Package"])
	assert.Equal(t, "1.0", fields["Version"])
	assert.Len(t, fields, 2)
}

// TestParseControlConsecutiveNewlines tests handling of consecutive newlines.
func TestParseControlConsecutiveNewlines(t *testing.T) {
	t.Parallel()

	control := `Package: test

Version: 1.0

Architecture: amd64
`

	fields := aptinstall.ParseControlForTesting(control)

	assert.Equal(t, "test", fields["Package"])
	assert.Equal(t, "1.0", fields["Version"])
	assert.Equal(t, "amd64", fields["Architecture"])
}
