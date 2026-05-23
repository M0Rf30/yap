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

// DownloadAndVerifyForTesting exposes downloadAndVerify for safety tests.
func DownloadAndVerifyForTesting(
	ctx context.Context, pkgURL, destFile, expectedSHA256 string, expectedSize int64,
) error {
	return downloadAndVerify(ctx, pkgURL, destFile, expectedSHA256, expectedSize)
}

// DebReposStateSnapshot is a fully-exported snapshot of debReposState for
// white-box testing. Tests build/inspect state through this struct and
// convert to/from the internal type via helpers below.
type DebReposStateSnapshot struct {
	CurTypes      string
	CurURIs       string
	CurSuites     string
	CurComponents string
	CurArchs      string
	CurSignedBy   string
}

func snapshotToInternal(s *DebReposStateSnapshot) debReposState {
	return debReposState{
		curTypes:      s.CurTypes,
		curURIs:       s.CurURIs,
		curSuites:     s.CurSuites,
		curComponents: s.CurComponents,
		curArchs:      s.CurArchs,
		curSignedBy:   s.CurSignedBy,
	}
}

func internalToSnapshot(st *debReposState) DebReposStateSnapshot {
	return DebReposStateSnapshot{
		CurTypes:      st.curTypes,
		CurURIs:       st.curURIs,
		CurSuites:     st.curSuites,
		CurComponents: st.curComponents,
		CurArchs:      st.curArchs,
		CurSignedBy:   st.curSignedBy,
	}
}

// HandleDebReposLineContinuationForTesting exposes handleDebReposLineContinuation for unit tests.
// Returns the updated state snapshot.
func HandleDebReposLineContinuationForTesting(line string, snap *DebReposStateSnapshot) DebReposStateSnapshot {
	st := snapshotToInternal(snap)
	handleDebReposLineContinuation(line, &st)

	return internalToSnapshot(&st)
}

// HandleDebReposLineFieldForTesting exposes handleDebReposLineField for unit tests.
// Returns the updated state snapshot.
func HandleDebReposLineFieldForTesting(field, value string, snap *DebReposStateSnapshot) DebReposStateSnapshot {
	st := snapshotToInternal(snap)
	handleDebReposLineField(field, value, &st)

	return internalToSnapshot(&st)
}

// HandleDebReposLineForTesting exposes handleDebReposLine for unit tests.
// Returns the updated state snapshot and any entries flushed.
func HandleDebReposLineForTesting(line string, snap *DebReposStateSnapshot, entries *[]SourceEntry) DebReposStateSnapshot {
	st := snapshotToInternal(snap)
	handleDebReposLine(line, &st, entries)

	return internalToSnapshot(&st)
}

// FlushDeb822RepoStanzaForTesting exposes flushDeb822RepoStanza for unit tests.
func FlushDeb822RepoStanzaForTesting(
	entries *[]SourceEntry,
	curTypes, curURIs, curSuites, curComponents, curArchs, curSignedBy string,
) {
	flushDeb822RepoStanza(entries, curTypes, curURIs, curSuites, curComponents, curArchs, curSignedBy)
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
