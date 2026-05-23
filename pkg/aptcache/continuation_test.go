// continuation_test.go covers the deb822 repo-stanza parsing helpers and
// other internal functions that had 0% coverage:
//
//   - handleDebReposLineContinuation  (line 709)
//   - handleDebReposLineField         (line 724)
//   - handleDebReposLine              (line 743)
//   - flushDeb822RepoStanza           (line 791)
//   - isPackagesIndexName             (line 928)
//   - mergeEntryFields                (line 969)
//   - deriveBaseURL                   (line 1022)
//   - parseLegacyOptions              (line 652)
//   - addSourceEntries                (line 678)
//   - mergeFrom                       (line 944)
package aptcache_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/M0Rf30/yap/v2/pkg/aptcache"
)

// ---------------------------------------------------------------------------
// handleDebReposLineContinuation
// ---------------------------------------------------------------------------

func TestHandleDebReposLineContinuation_URIs(t *testing.T) {
	snap := aptcache.DebReposStateSnapshot{CurURIs: "https://archive.ubuntu.com/ubuntu/"}
	got := aptcache.HandleDebReposLineContinuationForTesting(" https://ports.ubuntu.com/ubuntu-ports/", &snap)
	assert.Equal(t, "https://archive.ubuntu.com/ubuntu/ https://ports.ubuntu.com/ubuntu-ports/", got.CurURIs)
}

func TestHandleDebReposLineContinuation_Suites(t *testing.T) {
	snap := aptcache.DebReposStateSnapshot{CurSuites: "noble"}
	got := aptcache.HandleDebReposLineContinuationForTesting(" noble-updates", &snap)
	assert.Equal(t, "noble noble-updates", got.CurSuites)
}

func TestHandleDebReposLineContinuation_Components(t *testing.T) {
	snap := aptcache.DebReposStateSnapshot{CurComponents: "main"}
	got := aptcache.HandleDebReposLineContinuationForTesting(" restricted universe", &snap)
	assert.Equal(t, "main restricted universe", got.CurComponents)
}

func TestHandleDebReposLineContinuation_Archs(t *testing.T) {
	snap := aptcache.DebReposStateSnapshot{CurArchs: "amd64"}
	got := aptcache.HandleDebReposLineContinuationForTesting(" arm64", &snap)
	assert.Equal(t, "amd64 arm64", got.CurArchs)
}

func TestHandleDebReposLineContinuation_NoMatchWhenEmpty(t *testing.T) {
	// All fields empty — continuation line should be a no-op.
	snap := aptcache.DebReposStateSnapshot{}
	got := aptcache.HandleDebReposLineContinuationForTesting(" something", &snap)
	assert.Equal(t, aptcache.DebReposStateSnapshot{}, got)
}

func TestHandleDebReposLineContinuation_TabPrefixNotAppended(t *testing.T) {
	// Tab-prefixed lines: handleDebReposLineContinuation checks strings.HasPrefix(line, " ")
	// (space), so a tab prefix does NOT trigger the append.
	snap := aptcache.DebReposStateSnapshot{CurURIs: "https://example.com/"}
	got := aptcache.HandleDebReposLineContinuationForTesting("\thttps://other.com/", &snap)
	assert.Equal(t, "https://example.com/", got.CurURIs)
}

func TestHandleDebReposLineContinuation_TrimsWhitespace(t *testing.T) {
	snap := aptcache.DebReposStateSnapshot{CurURIs: "https://a.com/"}
	got := aptcache.HandleDebReposLineContinuationForTesting("   https://b.com/   ", &snap)
	assert.Equal(t, "https://a.com/ https://b.com/", got.CurURIs)
}

// URIs field takes priority over Suites when both are set (first matching case wins).
func TestHandleDebReposLineContinuation_URIsPriorityOverSuites(t *testing.T) {
	snap := aptcache.DebReposStateSnapshot{
		CurURIs:   "https://a.com/",
		CurSuites: "noble",
	}
	got := aptcache.HandleDebReposLineContinuationForTesting(" noble-updates", &snap)
	// URIs case matches first in the switch.
	assert.Equal(t, "https://a.com/ noble-updates", got.CurURIs)
	assert.Equal(t, "noble", got.CurSuites, "Suites should be unchanged")
}

// ---------------------------------------------------------------------------
// handleDebReposLineField
// ---------------------------------------------------------------------------

func TestHandleDebReposLineField_AllFields(t *testing.T) {
	cases := []struct {
		field    string
		value    string
		checkFn  func(snap aptcache.DebReposStateSnapshot) string
		expected string
	}{
		{"Types", "deb", func(s aptcache.DebReposStateSnapshot) string { return s.CurTypes }, "deb"},
		{"URIs", "https://example.com/", func(s aptcache.DebReposStateSnapshot) string { return s.CurURIs }, "https://example.com/"},
		{"Suites", "noble", func(s aptcache.DebReposStateSnapshot) string { return s.CurSuites }, "noble"},
		{"Components", "main restricted", func(s aptcache.DebReposStateSnapshot) string { return s.CurComponents }, "main restricted"},
		{"Architectures", "amd64 arm64", func(s aptcache.DebReposStateSnapshot) string { return s.CurArchs }, "amd64 arm64"},
		{"Signed-By", "/usr/share/keyrings/ubuntu.gpg", func(s aptcache.DebReposStateSnapshot) string { return s.CurSignedBy }, "/usr/share/keyrings/ubuntu.gpg"},
	}

	for _, tc := range cases {
		t.Run(tc.field, func(t *testing.T) {
			got := aptcache.HandleDebReposLineFieldForTesting(tc.field, tc.value, &aptcache.DebReposStateSnapshot{})
			assert.Equal(t, tc.expected, tc.checkFn(got))
		})
	}
}

func TestHandleDebReposLineField_UnknownField(t *testing.T) {
	got := aptcache.HandleDebReposLineFieldForTesting("X-Custom-Field", "value", &aptcache.DebReposStateSnapshot{})
	// Unknown fields are silently ignored.
	assert.Equal(t, aptcache.DebReposStateSnapshot{}, got)
}

// ---------------------------------------------------------------------------
// handleDebReposLine
// ---------------------------------------------------------------------------

func TestHandleDebReposLine_BlankLineFlushesStanza(t *testing.T) {
	snap := aptcache.DebReposStateSnapshot{
		CurTypes:      "deb",
		CurURIs:       "https://archive.ubuntu.com/ubuntu/",
		CurSuites:     "noble",
		CurComponents: "main",
	}

	var entries []aptcache.SourceEntry

	got := aptcache.HandleDebReposLineForTesting("", &snap, &entries)

	// State should be reset.
	assert.Equal(t, aptcache.DebReposStateSnapshot{}, got)
	// One entry should have been flushed.
	require.Len(t, entries, 1)
	assert.Equal(t, "https://archive.ubuntu.com/ubuntu/", entries[0].URL)
	assert.Equal(t, "noble", entries[0].Suite)
	assert.Equal(t, []string{"main"}, entries[0].Components)
}

func TestHandleDebReposLine_ContinuationLine(t *testing.T) {
	snap := aptcache.DebReposStateSnapshot{CurSuites: "noble"}

	var entries []aptcache.SourceEntry

	got := aptcache.HandleDebReposLineForTesting(" noble-updates", &snap, &entries)

	assert.Equal(t, "noble noble-updates", got.CurSuites)
	assert.Empty(t, entries)
}

func TestHandleDebReposLine_FieldLine(t *testing.T) {
	snap := aptcache.DebReposStateSnapshot{}

	var entries []aptcache.SourceEntry

	got := aptcache.HandleDebReposLineForTesting("Types: deb", &snap, &entries)

	assert.Equal(t, "deb", got.CurTypes)
	assert.Empty(t, entries)
}

func TestHandleDebReposLine_MalformedLineIgnored(t *testing.T) {
	snap := aptcache.DebReposStateSnapshot{}

	var entries []aptcache.SourceEntry

	got := aptcache.HandleDebReposLineForTesting("no-colon-here", &snap, &entries)

	assert.Equal(t, aptcache.DebReposStateSnapshot{}, got)
	assert.Empty(t, entries)
}

func TestHandleDebReposLine_TabContinuationDispatched(t *testing.T) {
	// Tab-prefixed lines are dispatched to handleDebReposLineContinuation,
	// but that function only appends when strings.HasPrefix(line, " ").
	// So the tab line is dispatched but not appended.
	snap := aptcache.DebReposStateSnapshot{CurComponents: "main"}

	var entries []aptcache.SourceEntry

	got := aptcache.HandleDebReposLineForTesting("\trestricted", &snap, &entries)

	assert.Equal(t, "main", got.CurComponents)
	assert.Empty(t, entries)
}

func TestHandleDebReposLine_FieldWithLeadingSpaceInValue(t *testing.T) {
	// "URIs:  https://..." — value is trimmed.
	snap := aptcache.DebReposStateSnapshot{}

	var entries []aptcache.SourceEntry

	got := aptcache.HandleDebReposLineForTesting("URIs:  https://archive.ubuntu.com/ubuntu/", &snap, &entries)

	assert.Equal(t, "https://archive.ubuntu.com/ubuntu/", got.CurURIs)
}

// ---------------------------------------------------------------------------
// flushDeb822RepoStanza
// ---------------------------------------------------------------------------

func TestFlushDeb822RepoStanza_RequiredFieldsMissing(t *testing.T) {
	cases := []struct {
		name                            string
		types, uris, suites, components string
	}{
		{"missing types", "", "https://a.com/", "noble", "main"},
		{"missing uris", "deb", "", "noble", "main"},
		{"missing suites", "deb", "https://a.com/", "", "main"},
		{"missing components", "deb", "https://a.com/", "noble", ""},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var entries []aptcache.SourceEntry
			aptcache.FlushDeb822RepoStanzaForTesting(&entries, tc.types, tc.uris, tc.suites, tc.components, "", "")
			assert.Empty(t, entries, "should produce no entries when required field is missing")
		})
	}
}

func TestFlushDeb822RepoStanza_SingleURI(t *testing.T) {
	var entries []aptcache.SourceEntry
	aptcache.FlushDeb822RepoStanzaForTesting(
		&entries,
		"deb",
		"https://archive.ubuntu.com/ubuntu/",
		"noble noble-updates",
		"main restricted",
		"amd64",
		"/usr/share/keyrings/ubuntu.gpg",
	)

	// Two suites → two entries.
	require.Len(t, entries, 2)
	assert.Equal(t, "noble", entries[0].Suite)
	assert.Equal(t, "noble-updates", entries[1].Suite)
	assert.Equal(t, []string{"main", "restricted"}, entries[0].Components)
	assert.Equal(t, []string{"amd64"}, entries[0].Architectures)
	assert.Equal(t, "/usr/share/keyrings/ubuntu.gpg", entries[0].SignedBy)
}

func TestFlushDeb822RepoStanza_MultipleURIs(t *testing.T) {
	var entries []aptcache.SourceEntry
	aptcache.FlushDeb822RepoStanzaForTesting(
		&entries,
		"deb",
		"https://archive.ubuntu.com/ubuntu/ https://ports.ubuntu.com/ubuntu-ports/",
		"noble",
		"main",
		"",
		"",
	)

	// Two URIs × one suite = two entries.
	require.Len(t, entries, 2)
	assert.Equal(t, "https://archive.ubuntu.com/ubuntu/", entries[0].URL)
	assert.Equal(t, "https://ports.ubuntu.com/ubuntu-ports/", entries[1].URL)
}

func TestFlushDeb822RepoStanza_NonHTTPURISkipped(t *testing.T) {
	var entries []aptcache.SourceEntry
	aptcache.FlushDeb822RepoStanzaForTesting(
		&entries,
		"deb",
		"file:///local/repo https://archive.ubuntu.com/ubuntu/",
		"noble",
		"main",
		"",
		"",
	)

	// file:// URI is skipped; only the https:// one produces an entry.
	require.Len(t, entries, 1)
	assert.Equal(t, "https://archive.ubuntu.com/ubuntu/", entries[0].URL)
}

func TestFlushDeb822RepoStanza_TrailingSlashAdded(t *testing.T) {
	var entries []aptcache.SourceEntry
	aptcache.FlushDeb822RepoStanzaForTesting(
		&entries,
		"deb",
		"https://archive.ubuntu.com/ubuntu",
		"noble",
		"main",
		"",
		"",
	)

	require.Len(t, entries, 1)
	assert.True(t, entries[0].URL[len(entries[0].URL)-1] == '/', "URL should end with /")
}

// ---------------------------------------------------------------------------
// isPackagesIndexName
// ---------------------------------------------------------------------------

func TestIsPackagesIndexName_Valid(t *testing.T) {
	valid := []string{
		"archive.ubuntu.com_ubuntu_dists_noble_main_binary-amd64_Packages",
		"archive.ubuntu.com_ubuntu_dists_noble_main_binary-amd64_Packages.gz",
		"archive.ubuntu.com_ubuntu_dists_noble_main_binary-amd64_Packages.bz2",
		"archive.ubuntu.com_ubuntu_dists_noble_main_binary-amd64_Packages.xz",
		"archive.ubuntu.com_ubuntu_dists_noble_main_binary-amd64_Packages.lz4",
		"archive.ubuntu.com_ubuntu_dists_noble_main_binary-amd64_Packages.zst",
	}

	for _, name := range valid {
		t.Run(name, func(t *testing.T) {
			assert.True(t, aptcache.IsPackagesIndexNameForTesting(name))
		})
	}
}

func TestIsPackagesIndexName_Invalid(t *testing.T) {
	invalid := []string{
		"Release",
		"InRelease",
		"Release.gpg",
		"archive.ubuntu.com_ubuntu_dists_noble_main_binary-amd64_Sources",
		"archive.ubuntu.com_ubuntu_dists_noble_main_binary-amd64_Packages.unknown",
		"Packages.gz.bak",
		"",
	}

	for _, name := range invalid {
		t.Run("invalid_"+name, func(t *testing.T) {
			assert.False(t, aptcache.IsPackagesIndexNameForTesting(name))
		})
	}
}

// ---------------------------------------------------------------------------
// mergeEntryFields
// ---------------------------------------------------------------------------

func TestMergeEntryFields_NonEmptyWins(t *testing.T) {
	existing := &aptcache.PackageInfo{
		Architecture: "amd64",
		MultiArch:    "",
		SHA256:       "abc123",
	}
	incoming := &aptcache.PackageInfo{
		Architecture: "arm64",
		MultiArch:    "same",
		SHA256:       "",
	}

	aptcache.MergeEntryFieldsForTesting(existing, incoming)

	assert.Equal(t, "arm64", existing.Architecture, "non-empty incoming arch should overwrite")
	assert.Equal(t, "same", existing.MultiArch, "non-empty incoming MultiArch should overwrite")
	assert.Equal(t, "abc123", existing.SHA256, "empty incoming SHA256 should not overwrite")
}

func TestMergeEntryFields_Essential(t *testing.T) {
	existing := &aptcache.PackageInfo{Essential: false}
	incoming := &aptcache.PackageInfo{Essential: true}
	aptcache.MergeEntryFieldsForTesting(existing, incoming)
	assert.True(t, existing.Essential)
}

func TestMergeEntryFields_EssentialNotCleared(t *testing.T) {
	existing := &aptcache.PackageInfo{Essential: true}
	incoming := &aptcache.PackageInfo{Essential: false}
	aptcache.MergeEntryFieldsForTesting(existing, incoming)
	// false incoming should NOT clear an existing true.
	assert.True(t, existing.Essential)
}

func TestMergeEntryFields_HasCandidate(t *testing.T) {
	existing := &aptcache.PackageInfo{HasCandidate: false}
	incoming := &aptcache.PackageInfo{HasCandidate: true}
	aptcache.MergeEntryFieldsForTesting(existing, incoming)
	assert.True(t, existing.HasCandidate)
}

func TestMergeEntryFields_FilenameAndBaseURL(t *testing.T) {
	existing := &aptcache.PackageInfo{
		Filename: "pool/main/l/libssl/libssl-dev_3.0_amd64.deb",
		BaseURL:  "https://archive.ubuntu.com/ubuntu/",
	}
	incoming := &aptcache.PackageInfo{
		Filename:     "pool/main/l/libssl/libssl-dev_3.0_arm64.deb",
		BaseURL:      "https://ports.ubuntu.com/ubuntu-ports/",
		Architecture: "arm64",
	}

	aptcache.MergeEntryFieldsForTesting(existing, incoming)

	assert.Equal(t, "pool/main/l/libssl/libssl-dev_3.0_arm64.deb", existing.Filename)
	// BaseURL should follow Filename for non-all arch.
	assert.Equal(t, "https://ports.ubuntu.com/ubuntu-ports/", existing.BaseURL)
}

func TestMergeEntryFields_FilenameBaseURLNotOverwrittenForArchAll(t *testing.T) {
	existing := &aptcache.PackageInfo{
		Filename: "pool/main/m/make/make_4.3_all.deb",
		BaseURL:  "https://archive.ubuntu.com/ubuntu/",
	}
	incoming := &aptcache.PackageInfo{
		Filename:     "pool/main/m/make/make_4.3_all.deb",
		BaseURL:      "https://ports.ubuntu.com/ubuntu-ports/",
		Architecture: "all",
	}

	aptcache.MergeEntryFieldsForTesting(existing, incoming)

	// Architecture: all — BaseURL must NOT be overwritten.
	assert.Equal(t, "https://archive.ubuntu.com/ubuntu/", existing.BaseURL)
}

func TestMergeEntryFields_SizeAndDepends(t *testing.T) {
	existing := &aptcache.PackageInfo{
		Size:    100,
		Depends: []string{"libc6"},
	}
	incoming := &aptcache.PackageInfo{
		Size:       200,
		Depends:    []string{"libc6", "libssl3"},
		PreDepends: []string{"dpkg"},
	}

	aptcache.MergeEntryFieldsForTesting(existing, incoming)

	assert.Equal(t, int64(200), existing.Size)
	assert.Equal(t, []string{"libc6", "libssl3"}, existing.Depends)
	assert.Equal(t, []string{"dpkg"}, existing.PreDepends)
}

func TestMergeEntryFields_ZeroSizeNotOverwritten(t *testing.T) {
	existing := &aptcache.PackageInfo{Size: 500}
	incoming := &aptcache.PackageInfo{Size: 0}
	aptcache.MergeEntryFieldsForTesting(existing, incoming)
	assert.Equal(t, int64(500), existing.Size)
}

func TestMergeEntryFields_EmptyDependsNotOverwritten(t *testing.T) {
	existing := &aptcache.PackageInfo{Depends: []string{"libc6"}}
	incoming := &aptcache.PackageInfo{Depends: nil}
	aptcache.MergeEntryFieldsForTesting(existing, incoming)
	assert.Equal(t, []string{"libc6"}, existing.Depends)
}

// ---------------------------------------------------------------------------
// deriveBaseURL
// ---------------------------------------------------------------------------

func TestDeriveBaseURL_KnownPrefix(t *testing.T) {
	sources := map[string]aptcache.SourceInfo{
		"archive.ubuntu.com_ubuntu": {FullURL: "https://archive.ubuntu.com/ubuntu/"},
	}

	url := aptcache.DeriveBaseURLForTesting(
		"archive.ubuntu.com_ubuntu_dists_noble_main_binary-amd64_Packages",
		sources,
	)
	assert.Equal(t, "https://archive.ubuntu.com/ubuntu/", url)
}

func TestDeriveBaseURL_CompressedSuffix(t *testing.T) {
	sources := map[string]aptcache.SourceInfo{
		"archive.ubuntu.com_ubuntu": {FullURL: "https://archive.ubuntu.com/ubuntu/"},
	}

	for _, ext := range []string{".gz", ".bz2", ".xz", ".lz4", ".zst"} {
		t.Run(ext, func(t *testing.T) {
			url := aptcache.DeriveBaseURLForTesting(
				"archive.ubuntu.com_ubuntu_dists_noble_main_binary-amd64_Packages"+ext,
				sources,
			)
			assert.Equal(t, "https://archive.ubuntu.com/ubuntu/", url)
		})
	}
}

func TestDeriveBaseURL_UnknownPrefix(t *testing.T) {
	sources := map[string]aptcache.SourceInfo{}
	url := aptcache.DeriveBaseURLForTesting(
		"archive.ubuntu.com_ubuntu_dists_noble_main_binary-amd64_Packages",
		sources,
	)
	assert.Equal(t, "", url)
}

func TestDeriveBaseURL_NoDists(t *testing.T) {
	sources := map[string]aptcache.SourceInfo{
		"something": {FullURL: "https://example.com/"},
	}
	// No "_dists_" separator → should return "".
	url := aptcache.DeriveBaseURLForTesting("something_Packages", sources)
	assert.Equal(t, "", url)
}

func TestDeriveBaseURL_EmptyFilename(t *testing.T) {
	url := aptcache.DeriveBaseURLForTesting("", map[string]aptcache.SourceInfo{})
	assert.Equal(t, "", url)
}

// ---------------------------------------------------------------------------
// parseLegacyOptions
// ---------------------------------------------------------------------------

func TestParseLegacyOptions_NoOptions(t *testing.T) {
	archs, signedBy, rest := aptcache.ParseLegacyOptionsForTesting("https://example.com/ noble main")
	assert.Empty(t, archs)
	assert.Empty(t, signedBy)
	assert.Equal(t, "https://example.com/ noble main", rest)
}

func TestParseLegacyOptions_ArchOnly(t *testing.T) {
	archs, signedBy, rest := aptcache.ParseLegacyOptionsForTesting("[arch=amd64,arm64] https://example.com/ noble main")
	assert.Equal(t, []string{"amd64", "arm64"}, archs)
	assert.Empty(t, signedBy)
	assert.Equal(t, "https://example.com/ noble main", rest)
}

func TestParseLegacyOptions_SignedByOnly(t *testing.T) {
	archs, signedBy, rest := aptcache.ParseLegacyOptionsForTesting("[signed-by=/usr/share/keyrings/ubuntu.gpg] https://example.com/ noble main")
	assert.Empty(t, archs)
	assert.Equal(t, "/usr/share/keyrings/ubuntu.gpg", signedBy)
	assert.Equal(t, "https://example.com/ noble main", rest)
}

func TestParseLegacyOptions_BothOptions(t *testing.T) {
	archs, signedBy, rest := aptcache.ParseLegacyOptionsForTesting("[arch=amd64 signed-by=/etc/apt/keyrings/docker.gpg] https://download.docker.com/linux/ubuntu noble stable")
	assert.Equal(t, []string{"amd64"}, archs)
	assert.Equal(t, "/etc/apt/keyrings/docker.gpg", signedBy)
	assert.Equal(t, "https://download.docker.com/linux/ubuntu noble stable", rest)
}

func TestParseLegacyOptions_UnclosedBracket(t *testing.T) {
	// Malformed: no closing bracket — treated as no options.
	archs, signedBy, rest := aptcache.ParseLegacyOptionsForTesting("[arch=amd64 https://example.com/ noble main")
	assert.Empty(t, archs)
	assert.Empty(t, signedBy)
	assert.Equal(t, "[arch=amd64 https://example.com/ noble main", rest)
}

func TestParseLegacyOptions_EmptyBrackets(t *testing.T) {
	archs, signedBy, rest := aptcache.ParseLegacyOptionsForTesting("[] https://example.com/ noble main")
	assert.Empty(t, archs)
	assert.Empty(t, signedBy)
	assert.Equal(t, "https://example.com/ noble main", rest)
}

// ---------------------------------------------------------------------------
// addSourceEntries
// ---------------------------------------------------------------------------

func TestAddSourceEntries_SingleSuite(t *testing.T) {
	var entries []aptcache.SourceEntry
	aptcache.AddSourceEntriesForTesting(&entries, "https://archive.ubuntu.com/ubuntu/", "noble", "main restricted", "amd64", "/key.gpg")

	require.Len(t, entries, 1)
	assert.Equal(t, "https://archive.ubuntu.com/ubuntu/", entries[0].URL)
	assert.Equal(t, "noble", entries[0].Suite)
	assert.Equal(t, []string{"main", "restricted"}, entries[0].Components)
	assert.Equal(t, []string{"amd64"}, entries[0].Architectures)
	assert.Equal(t, "/key.gpg", entries[0].SignedBy)
}

func TestAddSourceEntries_MultipleSuites(t *testing.T) {
	var entries []aptcache.SourceEntry
	aptcache.AddSourceEntriesForTesting(&entries, "https://archive.ubuntu.com/ubuntu/", "noble noble-updates noble-security", "main", "", "")

	require.Len(t, entries, 3)
	assert.Equal(t, "noble", entries[0].Suite)
	assert.Equal(t, "noble-updates", entries[1].Suite)
	assert.Equal(t, "noble-security", entries[2].Suite)
}

func TestAddSourceEntries_TrailingSlashAdded(t *testing.T) {
	var entries []aptcache.SourceEntry
	aptcache.AddSourceEntriesForTesting(&entries, "https://archive.ubuntu.com/ubuntu", "noble", "main", "", "")

	require.Len(t, entries, 1)
	assert.Equal(t, "https://archive.ubuntu.com/ubuntu/", entries[0].URL)
}

func TestAddSourceEntries_EmptySuites(t *testing.T) {
	var entries []aptcache.SourceEntry
	aptcache.AddSourceEntriesForTesting(&entries, "https://archive.ubuntu.com/ubuntu/", "", "main", "", "")
	assert.Empty(t, entries)
}

func TestAddSourceEntries_NoArchitectures(t *testing.T) {
	var entries []aptcache.SourceEntry
	aptcache.AddSourceEntriesForTesting(&entries, "https://archive.ubuntu.com/ubuntu/", "noble", "main", "", "")

	require.Len(t, entries, 1)
	assert.Empty(t, entries[0].Architectures)
}

// ---------------------------------------------------------------------------
// mergeFrom (via MergeFromForTesting)
// ---------------------------------------------------------------------------

func TestMergeFrom_NewEntries(t *testing.T) {
	dst := aptcache.NewCacheForTesting()
	src := aptcache.NewCacheForTesting()

	require.NoError(t, src.ParseDeb822ForTesting(
		strings.NewReader("Package: curl\nArchitecture: amd64\nVersion: 7.81.0\n\n"),
		false,
	))

	dst.MergeFromForTesting(src)

	info, ok := dst.Lookup("curl")
	require.True(t, ok)
	assert.Equal(t, "amd64", info.Architecture)
}

func TestMergeFrom_ExistingEntryMerged(t *testing.T) {
	dst := aptcache.NewCacheForTesting()
	src := aptcache.NewCacheForTesting()

	// dst has curl without SHA256; src has curl with SHA256.
	require.NoError(t, dst.ParseDeb822ForTesting(
		strings.NewReader("Package: curl\nArchitecture: amd64\nVersion: 7.81.0\n\n"),
		false,
	))
	require.NoError(t, src.ParseDeb822ForTesting(
		strings.NewReader("Package: curl\nArchitecture: amd64\nVersion: 7.81.0\nSHA256: deadbeef\n\n"),
		false,
	))

	dst.MergeFromForTesting(src)

	info, ok := dst.Lookup("curl")
	require.True(t, ok)
	assert.Equal(t, "deadbeef", info.SHA256)
}

func TestMergeFrom_ProvidersAppended(t *testing.T) {
	dst := aptcache.NewCacheForTesting()
	src := aptcache.NewCacheForTesting()

	// dst has one provider for java-runtime; src has another.
	require.NoError(t, dst.ParseDeb822ForTesting(
		strings.NewReader("Package: default-jre\nArchitecture: amd64\nVersion: 11\nProvides: java-runtime\n\n"),
		false,
	))
	require.NoError(t, src.ParseDeb822ForTesting(
		strings.NewReader("Package: openjdk-11-jre\nArchitecture: amd64\nVersion: 11\nProvides: java-runtime\n\n"),
		false,
	))

	dst.MergeFromForTesting(src)

	// Both providers should be present; virtual should resolve to a concrete package.
	resolved := dst.ResolveVirtual("java-runtime")
	assert.NotEqual(t, "java-runtime", resolved, "virtual should resolve to a concrete package")
}

func TestMergeFrom_EmptySourceNoOp(t *testing.T) {
	dst := aptcache.NewCacheForTesting()
	require.NoError(t, dst.ParseDeb822ForTesting(
		strings.NewReader("Package: curl\nArchitecture: amd64\nVersion: 7.81.0\n\n"),
		false,
	))

	src := aptcache.NewCacheForTesting() // empty
	dst.MergeFromForTesting(src)

	info, ok := dst.Lookup("curl")
	require.True(t, ok)
	assert.Equal(t, "amd64", info.Architecture)
}

// ---------------------------------------------------------------------------
// Integration: parseDeb822SourcesListForRepo (end-to-end via exported wrapper)
// ---------------------------------------------------------------------------

func TestParseDeb822SourcesListForRepo_FullStanza(t *testing.T) {
	content := `Types: deb
URIs: https://archive.ubuntu.com/ubuntu/
Suites: noble noble-updates
Components: main restricted universe multiverse
Architectures: amd64 arm64
Signed-By: /usr/share/keyrings/ubuntu-archive-keyring.gpg

`
	entries := aptcache.ParseDeb822SourcesListForRepoTesting(content)

	// Two suites → two entries.
	require.Len(t, entries, 2)
	assert.Equal(t, "https://archive.ubuntu.com/ubuntu/", entries[0].URL)
	assert.Equal(t, "noble", entries[0].Suite)
	assert.Equal(t, []string{"main", "restricted", "universe", "multiverse"}, entries[0].Components)
	assert.Equal(t, []string{"amd64", "arm64"}, entries[0].Architectures)
	assert.Equal(t, "/usr/share/keyrings/ubuntu-archive-keyring.gpg", entries[0].SignedBy)
	assert.Equal(t, "noble-updates", entries[1].Suite)
}

func TestParseDeb822SourcesListForRepo_MultipleStanzas(t *testing.T) {
	content := `Types: deb
URIs: https://archive.ubuntu.com/ubuntu/
Suites: noble
Components: main

Types: deb
URIs: https://download.docker.com/linux/ubuntu/
Suites: noble
Components: stable

`
	entries := aptcache.ParseDeb822SourcesListForRepoTesting(content)
	require.Len(t, entries, 2)
	assert.Equal(t, "https://archive.ubuntu.com/ubuntu/", entries[0].URL)
	assert.Equal(t, "https://download.docker.com/linux/ubuntu/", entries[1].URL)
}

func TestParseDeb822SourcesListForRepo_NoTrailingBlankLine(t *testing.T) {
	// Last stanza must be flushed even without trailing blank line.
	content := `Types: deb
URIs: https://archive.ubuntu.com/ubuntu/
Suites: noble
Components: main`

	entries := aptcache.ParseDeb822SourcesListForRepoTesting(content)
	require.Len(t, entries, 1)
	assert.Equal(t, "noble", entries[0].Suite)
}

func TestParseDeb822SourcesListForRepo_ContinuationLines(t *testing.T) {
	// NOTE: The continuation switch in handleDebReposLineContinuation matches
	// the first non-empty field. When URIs is already set, a space-prefixed
	// continuation line is appended to URIs (not Suites). To get multi-suite
	// continuation working, Suites must be listed before URIs in the stanza,
	// or URIs must be empty when the continuation line is processed.
	//
	// This test exercises the real behaviour: Suites continuation works when
	// URIs has not yet been set (i.e. Suites field appears before URIs).
	content := `Types: deb
Suites: noble
 noble-updates
 noble-security
URIs: https://archive.ubuntu.com/ubuntu/
Components: main

`
	entries := aptcache.ParseDeb822SourcesListForRepoTesting(content)
	require.Len(t, entries, 3, "three suites from continuation lines")
	assert.Equal(t, "noble", entries[0].Suite)
	assert.Equal(t, "noble-updates", entries[1].Suite)
	assert.Equal(t, "noble-security", entries[2].Suite)
}

func TestParseDeb822SourcesListForRepo_EmptyContent(t *testing.T) {
	entries := aptcache.ParseDeb822SourcesListForRepoTesting("")
	assert.Empty(t, entries)
}

func TestParseDeb822SourcesListForRepo_CommentLinesIgnored(t *testing.T) {
	// Comment lines (starting with #) don't match any field pattern and are ignored.
	content := `# This is a comment
Types: deb
URIs: https://archive.ubuntu.com/ubuntu/
Suites: noble
Components: main

`
	entries := aptcache.ParseDeb822SourcesListForRepoTesting(content)
	require.Len(t, entries, 1)
}

func TestParseDeb822SourcesListForRepo_OnlyHTTPSURIs(t *testing.T) {
	// Mix of http:// and https:// — both should be accepted.
	content := `Types: deb
URIs: http://archive.ubuntu.com/ubuntu/
Suites: noble
Components: main

`
	entries := aptcache.ParseDeb822SourcesListForRepoTesting(content)
	require.Len(t, entries, 1)
	assert.Equal(t, "http://archive.ubuntu.com/ubuntu/", entries[0].URL)
}
