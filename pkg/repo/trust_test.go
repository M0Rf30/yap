//nolint:testpackage // exercises unexported render helpers
package repo

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRenderDebSourceTrust(t *testing.T) {
	r := &Repo{URL: "https://e.com", Suite: "jammy"}
	comps := []string{"main"}

	assert.Contains(t, renderDebSource(r, comps, ""), "Trusted: yes")
	assert.Contains(t, renderDebSource(r, comps, "/k.asc"), "Signed-By: /k.asc")
	assert.NotContains(t, renderDebSource(r, comps, "/k.asc"), "Trusted")

	r.GPGCheck = true
	assert.NotContains(t, renderDebSource(r, comps, ""), "Trusted",
		"explicit gpgCheck must never emit Trusted: yes")
}

func TestRenderRPMRepoTrust(t *testing.T) {
	r := &Repo{Name: "x", URL: "https://e.com"}
	assert.Contains(t, renderRPMRepo(r, ""), "gpgcheck=0")

	r.GPGCheck = true
	out := renderRPMRepo(r, "")
	assert.Contains(t, out, "gpgcheck=1", "gpgCheck without key must still enable checking")
	assert.NotContains(t, out, "gpgkey=")

	assert.Contains(t, renderRPMRepo(r, "/k"), "gpgkey=file:///k")
}
