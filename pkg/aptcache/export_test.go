// SPDX-FileCopyrightText: 2021-2026 Gianluca Boiano
// SPDX-License-Identifier: GPL-3.0-only

// export_test.go exposes internal helpers for white-box testing.
// This file is only compiled when running tests.
package aptcache

import (
	"context"
	"io"

	"github.com/cavaliergopher/grab/v3"

	"github.com/M0Rf30/yap/v2/pkg/deb822"
)

// NewCacheForTesting creates an empty Cache suitable for unit tests.
func NewCacheForTesting() *Cache {
	return &Cache{
		entries:    make(map[string]*PackageInfo),
		providers:  make(map[string][]string),
		byBareName: make(map[string][]string),
	}
}

// ParseDeb822ForTesting exposes parseDeb822 for unit tests.
func (c *Cache) ParseDeb822ForTesting(r io.Reader, dpkgStatus bool) error {
	return c.parseDeb822(r, "test", dpkgStatus, "")
}

// ParseDeb822WithBaseURLForTesting exposes parseDeb822 with an explicit
// baseURL so closure / download integration tests can wire packages up to
// an httptest server.
func (c *Cache) ParseDeb822WithBaseURLForTesting(r io.Reader, dpkgStatus bool, baseURL string) error {
	return c.parseDeb822(r, "test", dpkgStatus, baseURL)
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

// LoadAptListsForTesting exposes loadAptLists so benchmarks can measure
// the real parallel-load code path. Passes an empty sources map; the
// only effect on the parsed entries is that BaseURL stays empty.
func LoadAptListsForTesting(c *Cache, dir string) error {
	return c.loadAptLists(dir, map[string]sourceInfo{})
}

// DownloadWithClientForTesting exposes downloadWithClient so tests can
// inject a fake grab.Client (custom HTTPClient/transport) to exercise the
// http->https fallback path deterministically, without real network I/O.
func (c *Cache) DownloadWithClientForTesting(
	ctx context.Context, client *grab.Client, destDir string, pkgs []string,
) error {
	return c.downloadWithClient(ctx, client, destDir, pkgs)
}

// FlushDeb822RepoStanzaForTesting exposes flushDeb822RepoStanza for unit tests.
func FlushDeb822RepoStanzaForTesting(
	entries *[]SourceEntry,
	curTypes, curURIs, curSuites, curComponents, curArchs, curSignedBy string,
) {
	flushDeb822RepoStanza(entries, deb822.Stanza{
		"Types":         curTypes,
		"URIs":          curURIs,
		"Suites":        curSuites,
		"Components":    curComponents,
		"Architectures": curArchs,
		"Signed-By":     curSignedBy,
	})
}

// IsPackagesIndexNameForTesting exposes isPackagesIndexName for unit tests.
func IsPackagesIndexNameForTesting(name string) bool {
	return isPackagesIndexName(name)
}

// MergeEntryFieldsForTesting exposes mergeEntryFields for unit tests.
func MergeEntryFieldsForTesting(existing, info *PackageInfo) {
	mergeEntryFields(existing, info)
}

// DeriveBaseURLForTesting exposes deriveBaseURL for unit tests.
func DeriveBaseURLForTesting(filename string, sources map[string]SourceInfo) string {
	// Convert exported SourceInfo to internal sourceInfo
	internal := make(map[string]sourceInfo, len(sources))
	for k, v := range sources {
		internal[k] = sourceInfo{fullURL: v.FullURL}
	}

	return deriveBaseURL(filename, internal)
}

// SourceInfo is the exported alias for sourceInfo for white-box testing.
type SourceInfo struct {
	FullURL string
}

// ParseLegacyOptionsForTesting exposes parseLegacyOptions for unit tests.
func ParseLegacyOptionsForTesting(line string) (archs []string, signedBy, rest string) {
	return parseLegacyOptions(line)
}

// AddSourceEntriesForTesting exposes addSourceEntries for unit tests.
func AddSourceEntriesForTesting(entries *[]SourceEntry, rawURL, curSuites, curComponents, curArchs, curSignedBy string) {
	addSourceEntries(entries, rawURL, curSuites, curComponents, curArchs, curSignedBy)
}

// MergeFromForTesting exposes mergeFrom for unit tests.
func (c *Cache) MergeFromForTesting(other *Cache) {
	c.mergeFrom(other)
}

// EncodeHostPathForTesting exposes encodeHostPath for unit tests.
func EncodeHostPathForTesting(rawURL string) string {
	return encodeHostPath(rawURL)
}

// ParseLegacySourcesListForTesting exposes parseLegacySourcesList for unit tests.
// Returns a map from encoded hostpath key to full URL.
func ParseLegacySourcesListForTesting(content string) map[string]string {
	schemes := make(map[string]sourceInfo)
	parseLegacySourcesList(content, schemes)

	result := make(map[string]string, len(schemes))
	for k, v := range schemes {
		result[k] = v.fullURL
	}

	return result
}

// ParseDeb822SourcesListForTesting exposes parseDeb822SourcesList for unit tests.
// Returns a map from encoded hostpath key to full URL.
func ParseDeb822SourcesListForTesting(content string) map[string]string {
	schemes := make(map[string]sourceInfo)
	parseDeb822SourcesList(content, schemes)

	result := make(map[string]string, len(schemes))
	for k, v := range schemes {
		result[k] = v.fullURL
	}

	return result
}

// BareNameKeysForTesting returns a copy of the byBareName secondary-index
// keys recorded for name.
func (c *Cache) BareNameKeysForTesting(name string) []string {
	c.mu.RLock()
	defer c.mu.RUnlock()

	return append([]string(nil), c.byBareName[name]...)
}
