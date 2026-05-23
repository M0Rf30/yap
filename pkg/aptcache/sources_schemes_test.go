package aptcache_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/M0Rf30/yap/v2/pkg/aptcache"
)

// ---------------------------------------------------------------------------
// encodeHostPath
// ---------------------------------------------------------------------------

func TestEncodeHostPath(t *testing.T) {
	t.Run("ubuntu archive URL", func(t *testing.T) {
		got := aptcache.EncodeHostPathForTesting("https://archive.ubuntu.com/ubuntu/")
		assert.Equal(t, "archive.ubuntu.com_ubuntu", got)
	})

	t.Run("ubuntu ports URL", func(t *testing.T) {
		got := aptcache.EncodeHostPathForTesting("https://ports.ubuntu.com/ubuntu-ports/")
		assert.Equal(t, "ports.ubuntu.com_ubuntu-ports", got)
	})

	t.Run("URL without trailing slash gets same result", func(t *testing.T) {
		// addURLToSchemes normalises the slash before calling encodeHostPath,
		// but encodeHostPath itself strips a trailing slash via TrimSuffix.
		withSlash := aptcache.EncodeHostPathForTesting("https://archive.ubuntu.com/ubuntu/")
		withoutSlash := aptcache.EncodeHostPathForTesting("https://archive.ubuntu.com/ubuntu")
		assert.Equal(t, withSlash, withoutSlash)
	})

	t.Run("invalid URL returns empty string", func(t *testing.T) {
		got := aptcache.EncodeHostPathForTesting("://not a valid url")
		assert.Equal(t, "", got)
	})

	t.Run("URL with multiple path segments", func(t *testing.T) {
		got := aptcache.EncodeHostPathForTesting("https://example.com/a/b/c/")
		assert.Equal(t, "example.com_a_b_c", got)
	})

	t.Run("root path URL", func(t *testing.T) {
		got := aptcache.EncodeHostPathForTesting("https://example.com/")
		assert.Equal(t, "example.com", got)
	})

	t.Run("http scheme", func(t *testing.T) {
		got := aptcache.EncodeHostPathForTesting("http://deb.debian.org/debian/")
		assert.Equal(t, "deb.debian.org_debian", got)
	})
}

// ---------------------------------------------------------------------------
// parseLegacySourcesList (via ParseLegacySourcesListForTesting)
// ---------------------------------------------------------------------------

func TestParseLegacySources(t *testing.T) {
	t.Run("simple deb line adds entry", func(t *testing.T) {
		content := "deb https://archive.ubuntu.com/ubuntu/ jammy main universe"
		result := aptcache.ParseLegacySourcesListForTesting(content)
		assert.Contains(t, result, "archive.ubuntu.com_ubuntu")
		assert.Equal(t, "https://archive.ubuntu.com/ubuntu/", result["archive.ubuntu.com_ubuntu"])
	})

	t.Run("deb-src line is skipped", func(t *testing.T) {
		content := "deb-src https://archive.ubuntu.com/ubuntu/ jammy main"
		result := aptcache.ParseLegacySourcesListForTesting(content)
		assert.Empty(t, result)
	})

	t.Run("comment line is skipped", func(t *testing.T) {
		content := "# deb https://archive.ubuntu.com/ubuntu/ jammy main"
		result := aptcache.ParseLegacySourcesListForTesting(content)
		assert.Empty(t, result)
	})

	t.Run("line with arch option still extracts URL", func(t *testing.T) {
		content := "deb [arch=amd64] https://archive.ubuntu.com/ubuntu/ jammy main"
		result := aptcache.ParseLegacySourcesListForTesting(content)
		assert.Contains(t, result, "archive.ubuntu.com_ubuntu")
	})

	t.Run("line with signed-by option still extracts URL", func(t *testing.T) {
		content := "deb [signed-by=/usr/share/keyrings/ubuntu-archive-keyring.gpg] https://archive.ubuntu.com/ubuntu/ jammy main"
		result := aptcache.ParseLegacySourcesListForTesting(content)
		assert.Contains(t, result, "archive.ubuntu.com_ubuntu")
	})

	t.Run("non-http URL is skipped", func(t *testing.T) {
		content := "deb file:///var/cache/apt/archives/ ./"
		result := aptcache.ParseLegacySourcesListForTesting(content)
		assert.Empty(t, result)
	})

	t.Run("multiple deb lines produce multiple entries", func(t *testing.T) {
		content := "deb https://archive.ubuntu.com/ubuntu/ jammy main" + "\n" + "deb https://ports.ubuntu.com/ubuntu-ports/ jammy main"
		result := aptcache.ParseLegacySourcesListForTesting(content)
		assert.Len(t, result, 2)
		assert.Contains(t, result, "archive.ubuntu.com_ubuntu")
		assert.Contains(t, result, "ports.ubuntu.com_ubuntu-ports")
	})

	t.Run("mixed deb and deb-src lines only keeps deb", func(t *testing.T) {
		content := "deb https://archive.ubuntu.com/ubuntu/ jammy main" + "\n" + "deb-src https://archive.ubuntu.com/ubuntu/ jammy main"
		result := aptcache.ParseLegacySourcesListForTesting(content)
		assert.Len(t, result, 1)
	})

	t.Run("empty content produces empty map", func(t *testing.T) {
		result := aptcache.ParseLegacySourcesListForTesting("")
		assert.Empty(t, result)
	})

	t.Run("URL without trailing slash is normalised", func(t *testing.T) {
		content := "deb https://archive.ubuntu.com/ubuntu jammy main"
		result := aptcache.ParseLegacySourcesListForTesting(content)
		// addURLToSchemes appends "/" so the stored fullURL ends with "/"
		assert.Equal(t, "https://archive.ubuntu.com/ubuntu/", result["archive.ubuntu.com_ubuntu"])
	})
}

// ---------------------------------------------------------------------------
// parseDeb822SourcesList (via ParseDeb822SourcesListForTesting)
// ---------------------------------------------------------------------------

func TestParseDeb822Sources(t *testing.T) {
	t.Run("simple stanza with Types deb and URIs adds entry", func(t *testing.T) {
		content := "Types: deb\nURIs: https://archive.ubuntu.com/ubuntu/\nSuites: jammy\nComponents: main\n"
		result := aptcache.ParseDeb822SourcesListForTesting(content)
		assert.Contains(t, result, "archive.ubuntu.com_ubuntu")
		assert.Equal(t, "https://archive.ubuntu.com/ubuntu/", result["archive.ubuntu.com_ubuntu"])
	})

	t.Run("multiple URIs in one stanza produce multiple entries", func(t *testing.T) {
		content := strings.Join([]string{
			"Types: deb",
			"URIs: https://archive.ubuntu.com/ubuntu/ https://ports.ubuntu.com/ubuntu-ports/",
			"Suites: jammy",
			"Components: main",
			"",
		}, "\n")
		result := aptcache.ParseDeb822SourcesListForTesting(content)
		assert.Len(t, result, 2)
		assert.Contains(t, result, "archive.ubuntu.com_ubuntu")
		assert.Contains(t, result, "ports.ubuntu.com_ubuntu-ports")
	})

	t.Run("stanza without Types field is skipped", func(t *testing.T) {
		content := "URIs: https://archive.ubuntu.com/ubuntu/\nSuites: jammy\nComponents: main\n"
		result := aptcache.ParseDeb822SourcesListForTesting(content)
		assert.Empty(t, result)
	})

	t.Run("stanza without URIs field is skipped", func(t *testing.T) {
		content := "Types: deb\nSuites: jammy\nComponents: main\n"
		result := aptcache.ParseDeb822SourcesListForTesting(content)
		assert.Empty(t, result)
	})

	t.Run("non-http URI is skipped", func(t *testing.T) {
		content := "Types: deb\nURIs: file:///var/cache/apt/archives/\nSuites: ./\n"
		result := aptcache.ParseDeb822SourcesListForTesting(content)
		assert.Empty(t, result)
	})

	t.Run("two stanzas separated by blank line both parsed", func(t *testing.T) {
		content := strings.Join([]string{
			"Types: deb",
			"URIs: https://archive.ubuntu.com/ubuntu/",
			"Suites: jammy",
			"Components: main",
			"",
			"Types: deb",
			"URIs: https://ports.ubuntu.com/ubuntu-ports/",
			"Suites: jammy",
			"Components: main",
			"",
		}, "\n")
		result := aptcache.ParseDeb822SourcesListForTesting(content)
		assert.Len(t, result, 2)
	})

	t.Run("empty content produces empty map", func(t *testing.T) {
		result := aptcache.ParseDeb822SourcesListForTesting("")
		assert.Empty(t, result)
	})

	t.Run("stanza with deb-src type only is still added (types not filtered here)", func(t *testing.T) {
		// parseDeb822SourcesList (schemes variant) does not filter by type —
		// it only checks that Types and URIs are non-empty.
		content := "Types: deb-src\nURIs: https://archive.ubuntu.com/ubuntu/\nSuites: jammy\nComponents: main\n"
		result := aptcache.ParseDeb822SourcesListForTesting(content)
		// The function adds the URL regardless of the type value.
		assert.Contains(t, result, "archive.ubuntu.com_ubuntu")
	})
}

// ---------------------------------------------------------------------------
// PackageCount and CapabilityCount
// ---------------------------------------------------------------------------

func TestPackageCount(t *testing.T) {
	t.Run("empty cache returns 0", func(t *testing.T) {
		c := aptcache.NewCacheForTesting()
		assert.Equal(t, 0, c.PackageCount())
	})

	t.Run("after parsing packages count matches", func(t *testing.T) {
		c := aptcache.NewCacheForTesting()
		index := strings.Join([]string{
			"Package: libfoo",
			"Version: 1.0",
			"Architecture: amd64",
			"Filename: pool/main/f/foo/libfoo_1.0_amd64.deb",
			"SHA256: aaaa",
			"Size: 1234",
			"",
			"Package: libbar",
			"Version: 2.0",
			"Architecture: amd64",
			"Filename: pool/main/b/bar/libbar_2.0_amd64.deb",
			"SHA256: bbbb",
			"Size: 5678",
			"",
		}, "\n")
		err := c.ParseDeb822ForTesting(strings.NewReader(index), false)
		require.NoError(t, err)
		assert.Equal(t, 2, c.PackageCount())
	})
}

func TestCapabilityCount(t *testing.T) {
	t.Run("empty cache returns 0", func(t *testing.T) {
		c := aptcache.NewCacheForTesting()
		assert.Equal(t, 0, c.CapabilityCount())
	})

	t.Run("after parsing package with Provides count matches", func(t *testing.T) {
		c := aptcache.NewCacheForTesting()
		index := strings.Join([]string{
			"Package: libfoo-impl",
			"Version: 1.0",
			"Architecture: amd64",
			"Provides: libfoo, libfoo-abi1",
			"Filename: pool/main/f/foo/libfoo-impl_1.0_amd64.deb",
			"SHA256: cccc",
			"Size: 999",
			"",
		}, "\n")
		err := c.ParseDeb822ForTesting(strings.NewReader(index), false)
		require.NoError(t, err)
		// Two virtual names were registered.
		assert.Equal(t, 2, c.CapabilityCount())
	})

	t.Run("package without Provides does not increment capability count", func(t *testing.T) {
		c := aptcache.NewCacheForTesting()
		index := strings.Join([]string{
			"Package: libfoo",
			"Version: 1.0",
			"Architecture: amd64",
			"Filename: pool/main/f/foo/libfoo_1.0_amd64.deb",
			"SHA256: dddd",
			"Size: 100",
			"",
		}, "\n")
		err := c.ParseDeb822ForTesting(strings.NewReader(index), false)
		require.NoError(t, err)
		assert.Equal(t, 0, c.CapabilityCount())
	})
}
