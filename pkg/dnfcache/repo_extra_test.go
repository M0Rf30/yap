//nolint:testpackage
package dnfcache

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/M0Rf30/yap/v2/pkg/httpclient"
)

// ---- isNonFatalRepoError ----

// TestIsNonFatalRepoErrorNil tests that nil is not a non-fatal error.
func TestIsNonFatalRepoErrorNil(t *testing.T) {
	assert.False(t, isNonFatalRepoError(nil))
}

// TestIsNonFatalRepoErrorGeneric tests that a generic error is not non-fatal.
func TestIsNonFatalRepoErrorGeneric(t *testing.T) {
	assert.False(t, isNonFatalRepoError(errors.New("some network error")))
}

// TestIsNonFatalRepoError404 tests that an HTTP 404 is non-fatal.
func TestIsNonFatalRepoError404(t *testing.T) {
	err := &httpclient.HTTPStatusError{Code: 404, URL: "https://example.com/repo"}
	assert.True(t, isNonFatalRepoError(err))
}

// TestIsNonFatalRepoError403 tests that an HTTP 403 is non-fatal.
func TestIsNonFatalRepoError403(t *testing.T) {
	err := &httpclient.HTTPStatusError{Code: 403, URL: "https://example.com/repo"}
	assert.True(t, isNonFatalRepoError(err))
}

// TestIsNonFatalRepoError401 tests that an HTTP 401 is non-fatal.
func TestIsNonFatalRepoError401(t *testing.T) {
	err := &httpclient.HTTPStatusError{Code: 401, URL: "https://example.com/repo"}
	assert.True(t, isNonFatalRepoError(err))
}

// TestIsNonFatalRepoError500 tests that an HTTP 500 is NOT non-fatal (it is fatal).
func TestIsNonFatalRepoError500(t *testing.T) {
	err := &httpclient.HTTPStatusError{Code: 500, URL: "https://example.com/repo"}
	assert.False(t, isNonFatalRepoError(err))
}

// TestIsNonFatalRepoError503 tests that an HTTP 503 is NOT non-fatal (it is fatal).
func TestIsNonFatalRepoError503(t *testing.T) {
	err := &httpclient.HTTPStatusError{Code: 503, URL: "https://example.com/repo"}
	assert.False(t, isNonFatalRepoError(err))
}

// TestIsNonFatalRepoErrorWrapped tests that a wrapped HTTPStatusError is detected.
func TestIsNonFatalRepoErrorWrapped(t *testing.T) {
	inner := &httpclient.HTTPStatusError{Code: 404, URL: "https://example.com/repo"}
	wrapped := errors.Join(errors.New("outer"), inner)
	assert.True(t, isNonFatalRepoError(wrapped))
}

// ---- parseMetalinkURLs missing branch ----

// TestParseMetalinkURLsNoTrailingSlash tests a plain https:// URL without trailing
// slash and without /repodata/repomd.xml suffix — the branch that appends "/".
func TestParseMetalinkURLsNoTrailingSlash(t *testing.T) {
	// URL has no trailing slash and no /repodata/repomd.xml suffix.
	body := `<url>https://mirror.example.com/rocky/8/BaseOS/x86_64/os</url>`

	got, err := parseMetalinkURLs(body, "https://metalink.example.com/")
	require.NoError(t, err)
	require.Len(t, got, 1)
	// The function should append "/" to make it a proper base URL.
	assert.True(t, strings.HasSuffix(got[0], "/"),
		"expected trailing slash, got %q", got[0])
	assert.Contains(t, got[0], "https://mirror.example.com/rocky/8/BaseOS/x86_64/os")
}

// ---- parsePrimaryXML malformed XML ----

// TestParsePrimaryXMLMalformed tests that malformed XML returns an error.
func TestParsePrimaryXMLMalformed(t *testing.T) {
	malformed := `<?xml version="1.0"?><metadata><package><name>foo</BROKEN`

	c := newCache()
	r := strings.NewReader(malformed)

	err := c.parsePrimaryXML(r, "http://mirror.example.com/")
	assert.Error(t, err, "parsePrimaryXML should return error for malformed XML")
}

// TestParsePrimaryXMLEmpty tests that an empty reader returns no error and no packages.
func TestParsePrimaryXMLEmpty(t *testing.T) {
	c := newCache()
	r := strings.NewReader("")

	err := c.parsePrimaryXML(r, "http://mirror.example.com/")
	assert.NoError(t, err)

	_, ok := c.Lookup("anything")
	assert.False(t, ok)
}

// TestParsePrimaryXMLEOFReader tests that an io.EOF reader returns no error.
func TestParsePrimaryXMLEOFReader(t *testing.T) {
	c := newCache()

	err := c.parsePrimaryXML(io.LimitReader(strings.NewReader(""), 0), "http://mirror.example.com/")
	assert.NoError(t, err)
}

// ---- goArchToRPM ----

// TestGoArchToRPMReturnsNonEmpty tests that goArchToRPM always returns a non-empty string.
func TestGoArchToRPMReturnsNonEmpty(t *testing.T) {
	got := goArchToRPM()
	assert.NotEmpty(t, got, "goArchToRPM() should return a non-empty string")
}

// ---- readReleasever ----

// TestReadReleaseverReturnsString tests that readReleasever returns a string (may be empty).
func TestReadReleaseverReturnsString(t *testing.T) {
	// Just call it — verifies it doesn't panic and returns a string.
	ver := readReleasever()
	// ver may be empty on non-RPM systems; that's fine.
	// Verify it contains no surrounding quotes (the function strips them).
	assert.NotContains(t, ver, `"`)
	assert.NotContains(t, ver, `'`)
}

// ---- expandDNFVars missing branch ----

// TestExpandDNFVarsMultipleVars tests that multiple $var tokens in a URL are
// each processed independently (some may expand, some may not).
func TestExpandDNFVarsMultipleVars(t *testing.T) {
	// Two unknown vars — both should remain unexpanded on systems without
	// /etc/dnf/vars/foo or /etc/dnf/vars/bar.
	url := "http://mirror.example.com/$foo/$bar/os/"
	got := expandDNFVars(url)

	// Result should not be empty.
	assert.NotEmpty(t, got)
}

// TestExpandDNFVarsVarAtEnd tests a $var token at the very end of the URL.
func TestExpandDNFVarsVarAtEnd(t *testing.T) {
	url := "http://mirror.example.com/os/$arch"
	got := expandDNFVars(url)
	assert.NotEmpty(t, got)
}

// TestExpandDNFVarsNumericSuffix tests that $var123 (alphanumeric) is matched.
func TestExpandDNFVarsNumericSuffix(t *testing.T) {
	url := "http://mirror.example.com/$var123/os/"
	got := expandDNFVars(url)
	// Unknown var stays as-is on systems without /etc/dnf/vars/var123.
	assert.NotEmpty(t, got)
}

// ---- loadInstalledProvidesSubprocess ----

// TestLoadInstalledProvidesSubprocessReturnsMap tests that the function always
// returns a non-nil map, even when rpm is not installed or fails.
func TestLoadInstalledProvidesSubprocessReturnsMap(t *testing.T) {
	ctx := context.Background()
	result := loadInstalledProvidesSubprocess(ctx)
	assert.NotNil(t, result, "loadInstalledProvidesSubprocess should return non-nil map")
}

// ---- file-based Requires (e.g. "/usr/bin/perl") ----

// TestParsePrimaryXMLFilePathRequires mirrors the Rocky 9 autoconf scenario:
// autoconf declares `Requires: /usr/bin/perl` (a file-path requirement) and the
// perl-interpreter package owns `/usr/bin/perl` via a `<file>` entry in
// primary.xml. The resolver must walk autoconf → /usr/bin/perl →
// perl-interpreter so perl-interpreter gets pulled into the install closure.
func TestParsePrimaryXMLFilePathRequires(t *testing.T) {
	const primary = `<?xml version="1.0" encoding="UTF-8"?>
<metadata xmlns="http://linux.duke.edu/metadata/common"
          xmlns:rpm="http://linux.duke.edu/metadata/rpm">
  <package type="rpm">
    <name>autoconf</name>
    <arch>noarch</arch>
    <version epoch="0" ver="2.71" rel="9.el9"/>
    <checksum type="sha256" pkgid="YES">aaaa</checksum>
    <size package="700000"/>
    <location href="Packages/a/autoconf-2.71-9.el9.noarch.rpm"/>
    <format>
      <rpm:provides><rpm:entry name="autoconf"/></rpm:provides>
      <rpm:requires>
        <rpm:entry name="/usr/bin/perl"/>
        <rpm:entry name="m4" flags="GE" ver="1.4.16"/>
        <rpm:entry name="rpmlib(CompressedFileNames)" flags="LE" ver="3.0.4-1"/>
      </rpm:requires>
    </format>
  </package>
  <package type="rpm">
    <name>perl-interpreter</name>
    <arch>x86_64</arch>
    <version epoch="4" ver="5.32.1" rel="481.el9"/>
    <checksum type="sha256" pkgid="YES">bbbb</checksum>
    <size package="80000"/>
    <location href="Packages/p/perl-interpreter-5.32.1-481.el9.x86_64.rpm"/>
    <format>
      <rpm:provides><rpm:entry name="perl-interpreter"/></rpm:provides>
      <file>/usr/bin/perl</file>
    </format>
  </package>
</metadata>`

	c := newCache()
	require.NoError(t, c.parsePrimaryXML(strings.NewReader(primary), "http://mirror/"))

	// autoconf must keep /usr/bin/perl as a require (not stripped).
	autoconf, ok := c.Lookup("autoconf")
	require.True(t, ok, "autoconf not indexed")
	assert.Contains(t, autoconf.Requires, "/usr/bin/perl",
		"path-style requires must survive parsing")

	// /usr/bin/perl must be indexed as a virtual provider of perl-interpreter.
	c.mu.RLock()
	providers := c.providers["/usr/bin/perl"]
	c.mu.RUnlock()
	require.Len(t, providers, 1, "expected one provider for /usr/bin/perl")
	assert.Equal(t, "perl-interpreter", providers[0].Name)

	// rpmlib() requires must still be filtered out.
	for _, r := range autoconf.Requires {
		assert.NotContains(t, r, "rpmlib(",
			"rpmlib() requires should be stripped, got %q", r)
	}
}
