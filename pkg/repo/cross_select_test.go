//nolint:testpackage // exercises unexported cross-arch selection helpers
package repo

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCrossArchiveSelection(t *testing.T) {
	cases := []struct {
		distro, arch, uri, keyring, suites string
	}{
		{"ubuntu", "amd64", ubuntuPrimaryURI, "/usr/share/keyrings/ubuntu-archive-keyring.gpg",
			"jammy jammy-updates jammy-security"},
		{"ubuntu", "i386", ubuntuPrimaryURI, "/usr/share/keyrings/ubuntu-archive-keyring.gpg",
			"jammy jammy-updates jammy-security"},
		{"ubuntu", "arm64", ubuntuPortsURI, "/usr/share/keyrings/ubuntu-archive-keyring.gpg",
			"jammy jammy-updates jammy-security"},
		{"debian", "arm64", debianPrimaryURI, "/usr/share/keyrings/debian-archive-keyring.gpg",
			"jammy"},
		{"debian", "amd64", debianPrimaryURI, "/usr/share/keyrings/debian-archive-keyring.gpg",
			"jammy"},
		{"debian", "m68k", debianPortsURI, debianPortsKeyring, "unstable"},
	}

	for _, c := range cases {
		t.Run(c.distro+"/"+c.arch, func(t *testing.T) {
			assert.Equal(t, c.uri, crossURIFor(c.distro, c.arch))
			assert.Equal(t, c.keyring, crossKeyringFor(c.distro, c.arch))
			assert.Equal(t, c.suites, crossSuitesFor(c.distro, "jammy", c.arch))
		})
	}
}

func TestCrossComponentsFor(t *testing.T) {
	assert.Equal(t, []string{"main"}, crossComponentsFor("debian"))
	assert.Contains(t, crossComponentsFor("ubuntu"), "universe")
}

func TestCrossURIForUnknownDistro(t *testing.T) {
	assert.Empty(t, crossURIFor("fedora", "arm64"))
}
