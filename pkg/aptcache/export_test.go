// export_test.go exposes internal helpers for white-box testing.
// This file is only compiled when running tests.
package aptcache

import "io"

// NewCacheForTesting creates an empty Cache suitable for unit tests.
func NewCacheForTesting() *Cache {
	return &Cache{
		entries:   make(map[string]*PackageInfo),
		providers: make(map[string][]string),
	}
}

// ParseDeb822ForTesting exposes parseDeb822 for unit tests.
func (c *Cache) ParseDeb822ForTesting(r io.Reader, dpkgStatus bool) error {
	return c.parseDeb822(r, dpkgStatus, "")
}

// ParseLegacySourcesListForRepoTesting exposes parseLegacySourcesListForRepo for unit tests.
func ParseLegacySourcesListForRepoTesting(content string) []SourceEntry {
	return parseLegacySourcesListForRepo(content)
}

// ParseDeb822SourcesListForRepoTesting exposes parseDeb822SourcesListForRepo for unit tests.
func ParseDeb822SourcesListForRepoTesting(content string) []SourceEntry {
	return parseDeb822SourcesListForRepo(content)
}

// ParseDependsFieldForTesting exposes parseDependsField for unit tests.
func ParseDependsFieldForTesting(value string) []string {
	return parseDependsField(value)
}

// ResolveDepsForTesting exposes ResolveDeps for unit tests.
func (c *Cache) ResolveDepsForTesting(seeds []string) ([]*PackageInfo, []string, error) {
	return c.ResolveDeps(seeds)
}
