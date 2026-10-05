//nolint:testpackage
package dnfcache

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestPreferHTTPS(t *testing.T) {
	got := preferHTTPS([]string{"https://a/"}, []string{"http://b/"}, "src")
	assert.Equal(t, []string{"https://a/"}, got)

	got = preferHTTPS(nil, []string{"http://b/"}, "src")
	assert.Equal(t, []string{"http://b/"}, got)
}

func TestResolveVirtualMatchesPickProvider(t *testing.T) {
	c := newCache()
	foreign := &PackageInfo{
		Name: "foreign", Arch: "s390x", LocationHref: "f.rpm",
		Provides: []string{"cap"},
	}
	native := &PackageInfo{
		Name: "native", Arch: goArchToRPM(), LocationHref: "n.rpm",
		Provides: []string{"cap"},
	}

	c.mu.Lock()
	c.addPackage(foreign)
	c.addPackage(native)
	c.mu.Unlock()

	assert.Equal(t, "native", c.ResolveVirtual("cap"))
}
