// export_test.go exposes internal helpers for white-box testing.
// This file is only compiled when running tests.
package aptcache

import (
	"context"
	"io"
)

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

// ParseDeb822WithBaseURLForTesting exposes parseDeb822 with an explicit
// baseURL so closure / download integration tests can wire packages up to
// an httptest server.
func (c *Cache) ParseDeb822WithBaseURLForTesting(r io.Reader, dpkgStatus bool, baseURL string) error {
	return c.parseDeb822(r, dpkgStatus, baseURL)
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

// LoadAptListsForTesting exposes loadAptLists so benchmarks can measure
// the real parallel-load code path. Passes an empty sources map; the
// only effect on the parsed entries is that BaseURL stays empty.
func LoadAptListsForTesting(c *Cache, dir string) error {
	return c.loadAptLists(dir, map[string]sourceInfo{})
}

// DownloadAndVerifyForTesting exposes downloadAndVerify for safety tests.
func DownloadAndVerifyForTesting(
	ctx context.Context, pkgURL, destFile, expectedSHA256 string, expectedSize int64,
) error {
	return downloadAndVerify(ctx, pkgURL, destFile, expectedSHA256, expectedSize)
}
