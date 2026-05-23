package aptcache_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/M0Rf30/yap/v2/pkg/aptcache"
)

// TestParseLegacySourcesListForRepo tests parsing legacy sources.list format.
func TestParseLegacySourcesListForRepo(t *testing.T) {
	t.Run("simple deb line", func(t *testing.T) {
		content := `deb https://archive.ubuntu.com/ubuntu/ jammy main universe`
		entries := aptcache.ParseLegacySourcesListForRepoTesting(content)
		assert.Len(t, entries, 1)
		assert.Equal(t, "https://archive.ubuntu.com/ubuntu/", entries[0].URL)
		assert.Equal(t, "jammy", entries[0].Suite)
		assert.Equal(t, []string{"main", "universe"}, entries[0].Components)
	})

	t.Run("with arch option", func(t *testing.T) {
		content := `deb [arch=amd64] https://archive.ubuntu.com/ubuntu/ jammy main`
		entries := aptcache.ParseLegacySourcesListForRepoTesting(content)
		assert.Len(t, entries, 1)
		assert.Equal(t, []string{"amd64"}, entries[0].Architectures)
	})

	t.Run("with signed-by option", func(t *testing.T) {
		content := `deb [signed-by=/usr/share/keyrings/ubuntu-archive-keyring.gpg] https://archive.ubuntu.com/ubuntu/ jammy main`
		entries := aptcache.ParseLegacySourcesListForRepoTesting(content)
		assert.Len(t, entries, 1)
		assert.Equal(t, "/usr/share/keyrings/ubuntu-archive-keyring.gpg", entries[0].SignedBy)
	})

	t.Run("skip deb-src lines", func(t *testing.T) {
		content := `deb https://archive.ubuntu.com/ubuntu/ jammy main
deb-src https://archive.ubuntu.com/ubuntu/ jammy main`
		entries := aptcache.ParseLegacySourcesListForRepoTesting(content)
		assert.Len(t, entries, 1)
	})

	t.Run("skip comments", func(t *testing.T) {
		content := `# This is a comment
deb https://archive.ubuntu.com/ubuntu/ jammy main`
		entries := aptcache.ParseLegacySourcesListForRepoTesting(content)
		assert.Len(t, entries, 1)
	})
}

// TestParseDeb822SourcesListForRepo tests parsing deb822 format.
func TestParseDeb822SourcesListForRepo(t *testing.T) {
	t.Run("simple deb822 stanza", func(t *testing.T) {
		content := `Types: deb
URIs: https://archive.ubuntu.com/ubuntu/
Suites: jammy
Components: main universe
`
		entries := aptcache.ParseDeb822SourcesListForRepoTesting(content)
		assert.Len(t, entries, 1)
		assert.Equal(t, "https://archive.ubuntu.com/ubuntu/", entries[0].URL)
		assert.Equal(t, "jammy", entries[0].Suite)
		assert.Equal(t, []string{"main", "universe"}, entries[0].Components)
	})

	t.Run("with architectures", func(t *testing.T) {
		content := `Types: deb
URIs: https://archive.ubuntu.com/ubuntu/
Suites: jammy
Components: main
Architectures: amd64 arm64
`
		entries := aptcache.ParseDeb822SourcesListForRepoTesting(content)
		assert.Len(t, entries, 1)
		assert.Equal(t, []string{"amd64", "arm64"}, entries[0].Architectures)
	})

	t.Run("with signed-by", func(t *testing.T) {
		content := `Types: deb
URIs: https://archive.ubuntu.com/ubuntu/
Suites: jammy
Components: main
Signed-By: /usr/share/keyrings/ubuntu-archive-keyring.gpg
`
		entries := aptcache.ParseDeb822SourcesListForRepoTesting(content)
		assert.Len(t, entries, 1)
		assert.Equal(t, "/usr/share/keyrings/ubuntu-archive-keyring.gpg", entries[0].SignedBy)
	})

	t.Run("multiple suites", func(t *testing.T) {
		content := `Types: deb
URIs: https://archive.ubuntu.com/ubuntu/
Suites: jammy jammy-updates
Components: main
`
		entries := aptcache.ParseDeb822SourcesListForRepoTesting(content)
		assert.Len(t, entries, 2)
		assert.Equal(t, "jammy", entries[0].Suite)
		assert.Equal(t, "jammy-updates", entries[1].Suite)
	})
}
