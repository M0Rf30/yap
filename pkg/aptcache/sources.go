// sources.go: sources.list / deb822 sources parsing (legacy and modern formats).

package aptcache

import (
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/M0Rf30/yap/v2/pkg/deb822"
)

// sourceInfo holds the scheme and full URL for a repository source.
type sourceInfo struct {
	scheme  string // "http" or "https"
	fullURL string // e.g. "https://ports.ubuntu.com/ubuntu-ports/"
}

// SourceEntry represents a single apt source with its URL, suite, components, and architectures.
// Exported for use by pkg/aptrepo.
type SourceEntry struct {
	URL           string   // e.g. "https://archive.ubuntu.com/ubuntu/"
	Suite         string   // e.g. "jammy"
	Components    []string // e.g. ["main", "universe"]
	Architectures []string // e.g. ["amd64"] — empty means default
	SignedBy      string   // path to GPG keyring file, or ""
}

// LoadSources parses /etc/apt/sources.list and /etc/apt/sources.list.d/*.{list,sources}
// and returns a slice of SourceEntry for each configured source.
// This is exported for use by pkg/aptrepo to fetch repository metadata.
func LoadSources() []SourceEntry {
	var entries []SourceEntry

	// Parse legacy /etc/apt/sources.list
	if data, err := os.ReadFile("/etc/apt/sources.list"); err == nil {
		entries = append(entries, parseLegacySourcesListForRepo(string(data))...)
	}

	// Parse deb822 files in /etc/apt/sources.list.d/
	entries = append(entries, readSourcesListD()...)

	return entries
}

// readSourcesListD reads and parses all .list and .sources files from /etc/apt/sources.list.d/.
// Returns a slice of SourceEntry for each file found.
func readSourcesListD() []SourceEntry {
	var entries []SourceEntry

	dirEntries, err := os.ReadDir("/etc/apt/sources.list.d")
	if err != nil {
		return entries
	}

	for _, e := range dirEntries {
		if e.IsDir() {
			continue
		}

		name := e.Name()
		// Only .list (legacy) and .sources (deb822) files
		if !strings.HasSuffix(name, ".list") && !strings.HasSuffix(name, ".sources") {
			continue
		}

		path := filepath.Join("/etc/apt/sources.list.d", name) //nolint:gocritic
		if data, err := os.ReadFile(path); err == nil {        //nolint:gosec
			if strings.HasSuffix(name, ".sources") {
				entries = append(entries, parseDeb822SourcesListForRepo(string(data))...)
			} else {
				entries = append(entries, parseLegacySourcesListForRepo(string(data))...)
			}
		}
	}

	return entries
}

// encodeHostPath converts a full URL to the "encoded hostpath" key used in
// /var/lib/apt/lists/ filenames. E.g. "https://ports.ubuntu.com/ubuntu-ports/"
// becomes "ports.ubuntu.com_ubuntu-ports".
func encodeHostPath(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		return ""
	}

	p := strings.TrimSuffix(u.Path, "/")

	return u.Host + strings.ReplaceAll(p, "/", "_")
}

// loadSourceSchemes parses /etc/apt/sources.list and /etc/apt/sources.list.d/*.{list,sources}
// to build a map from encoded hostpath to sourceInfo (scheme + full URL).
// This allows us to correctly resolve the base URL for each package at parse time.
func loadSourceSchemes() map[string]sourceInfo {
	schemes := make(map[string]sourceInfo)

	// Parse legacy /etc/apt/sources.list
	if data, err := os.ReadFile("/etc/apt/sources.list"); err == nil {
		parseLegacySourcesList(string(data), schemes)
	}

	// Parse deb822 files in /etc/apt/sources.list.d/
	loadSourceSchemesFromD(schemes)

	return schemes
}

// loadSourceSchemesFromD reads and parses all .list and .sources files from /etc/apt/sources.list.d/,
// populating the schemes map with encoded hostpath → sourceInfo entries.
func loadSourceSchemesFromD(schemes map[string]sourceInfo) {
	entries, err := os.ReadDir("/etc/apt/sources.list.d")
	if err != nil {
		return
	}

	for _, e := range entries {
		if e.IsDir() {
			continue
		}

		name := e.Name()
		// Only .list (legacy) and .sources (deb822) files
		if !strings.HasSuffix(name, ".list") && !strings.HasSuffix(name, ".sources") {
			continue
		}

		path := filepath.Join("/etc/apt/sources.list.d", name) //nolint:gocritic
		if data, err := os.ReadFile(path); err == nil {        //nolint:gosec
			if strings.HasSuffix(name, ".sources") {
				parseDeb822SourcesList(string(data), schemes)
			} else {
				parseLegacySourcesList(string(data), schemes)
			}
		}
	}
}

// addURLToSchemes adds a URL to the schemes map with its scheme and full URL.
func addURLToSchemes(rawURL string, schemes map[string]sourceInfo) {
	if !strings.HasSuffix(rawURL, "/") {
		rawURL += "/"
	}

	key := encodeHostPath(rawURL)
	if key != "" {
		u, _ := url.Parse(rawURL)
		if u != nil {
			schemes[key] = sourceInfo{
				scheme:  u.Scheme,
				fullURL: rawURL,
			}
		}
	}
}

// parseLegacySourcesList parses /etc/apt/sources.list format (one entry per line).
// Lines like: deb [arch=amd64] https://archive.ubuntu.com/ubuntu/ jammy main
func parseLegacySourcesList(content string, schemes map[string]sourceInfo) {
	for line := range strings.SplitSeq(content, "\n") {
		line = strings.TrimSpace(line)

		// Skip empty lines and comments
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		// Skip deb-src lines (source packages, not binary)
		if strings.HasPrefix(line, "deb-src ") {
			continue
		}

		// Must start with "deb "
		if !strings.HasPrefix(line, "deb ") {
			continue
		}

		// Remove "deb " prefix
		line = strings.TrimPrefix(line, "deb ")

		// Strip [options] block if present
		if strings.HasPrefix(line, "[") {
			if idx := strings.Index(line, "]"); idx >= 0 {
				line = strings.TrimSpace(line[idx+1:])
			}
		}

		// Extract URL (first token)
		parts := strings.Fields(line)
		if len(parts) < 1 {
			continue
		}

		rawURL := parts[0]
		if !strings.HasPrefix(rawURL, "http://") && !strings.HasPrefix(rawURL, "https://") {
			continue
		}

		addURLToSchemes(rawURL, schemes)
	}
}

// parseDeb822SourcesList parses /etc/apt/sources.list.d/*.sources format (deb822).
// Stanzas with Types: deb and URIs: https://...
func parseDeb822SourcesList(content string, schemes map[string]sourceInfo) {
	_ = deb822.Parse(strings.NewReader(content), func(stanzaMap deb822.Stanza) error {
		curTypes := stanzaMap["Types"]
		curURIs := stanzaMap["URIs"]
		flushDeb822Stanza(curTypes, curURIs, schemes)

		return nil
	})
}

// flushDeb822Stanza processes a completed deb822 stanza by extracting URIs and adding them to schemes.
// Only processes stanzas with both Types and URIs fields.
func flushDeb822Stanza(curTypes, curURIs string, schemes map[string]sourceInfo) {
	if curTypes == "" || curURIs == "" {
		return
	}

	// Parse URIs (space-separated, may span multiple lines)
	for rawURL := range strings.FieldsSeq(curURIs) {
		if !strings.HasPrefix(rawURL, "http://") && !strings.HasPrefix(rawURL, "https://") {
			continue
		}

		addURLToSchemes(rawURL, schemes)
	}
}

// parseLegacySourcesListForRepo parses /etc/apt/sources.list format and returns SourceEntry slice.
// Lines like: deb [arch=amd64] https://archive.ubuntu.com/ubuntu/ jammy main universe
func parseLegacySourcesListForRepo(content string) []SourceEntry {
	var entries []SourceEntry

	for line := range strings.SplitSeq(content, "\n") {
		line = strings.TrimSpace(line)

		// Skip empty lines and comments
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		// Skip deb-src lines (source packages, not binary)
		if strings.HasPrefix(line, "deb-src ") {
			continue
		}

		// Must start with "deb "
		if !strings.HasPrefix(line, "deb ") {
			continue
		}

		// Remove "deb " prefix
		line = strings.TrimPrefix(line, "deb ")

		// Strip [options] block if present (may contain arch=, signed-by=, etc.)
		archs, signedBy, line := parseLegacyOptions(line)

		// Extract URL and suite/components
		parts := strings.Fields(line)
		if len(parts) < 2 {
			continue
		}

		rawURL := parts[0]
		if !strings.HasPrefix(rawURL, "http://") && !strings.HasPrefix(rawURL, "https://") {
			continue
		}

		// Ensure trailing slash
		if !strings.HasSuffix(rawURL, "/") {
			rawURL += "/"
		}

		suite := parts[1]
		components := parts[2:]

		if len(components) == 0 {
			continue
		}

		entries = append(entries, SourceEntry{
			URL:           rawURL,
			Suite:         suite,
			Components:    components,
			Architectures: archs,
			SignedBy:      signedBy,
		})
	}

	return entries
}

// parseLegacyOptions extracts [options] block from a deb line.
// Returns (archs, signedBy, remaining line after options).
func parseLegacyOptions(line string) (archs []string, signedBy, rest string) {
	if !strings.HasPrefix(line, "[") {
		return archs, signedBy, line
	}

	idx := strings.Index(line, "]")
	if idx < 0 {
		return archs, signedBy, line
	}

	opts := line[1:idx]
	rest = strings.TrimSpace(line[idx+1:])

	// Parse options: arch=amd64,arm64 signed-by=/path/to/key
	for opt := range strings.FieldsSeq(opts) {
		if after, ok := strings.CutPrefix(opt, "arch="); ok {
			archs = strings.Split(after, ",")
		} else if after, ok := strings.CutPrefix(opt, "signed-by="); ok {
			signedBy = after
		}
	}

	return archs, signedBy, rest
}

// addSourceEntries adds SourceEntry records for each suite/component combination.
func addSourceEntries(entries *[]SourceEntry, rawURL string, curSuites, curComponents, curArchs, curSignedBy string) {
	if !strings.HasSuffix(rawURL, "/") {
		rawURL += "/"
	}

	suites := strings.Fields(curSuites)
	components := strings.Fields(curComponents)
	archs := strings.Fields(curArchs)

	for _, suite := range suites {
		*entries = append(*entries, SourceEntry{
			URL:           rawURL,
			Suite:         suite,
			Components:    components,
			Architectures: archs,
			SignedBy:      curSignedBy,
		})
	}
}

// debReposState holds the state of a deb822 repo stanza being parsed.
type debReposState struct {
	curTypes      string
	curURIs       string
	curSuites     string
	curComponents string
	curArchs      string
	curSignedBy   string
}

// handleDebReposLineContinuation handles continuation lines in a deb822 repo stanza.
func handleDebReposLineContinuation(line string, st *debReposState) {
	trimmed := strings.TrimSpace(line)
	switch {
	case st.curURIs != "" && strings.HasPrefix(line, " "):
		st.curURIs += " " + trimmed
	case st.curSuites != "" && strings.HasPrefix(line, " "):
		st.curSuites += " " + trimmed
	case st.curComponents != "" && strings.HasPrefix(line, " "):
		st.curComponents += " " + trimmed
	case st.curArchs != "" && strings.HasPrefix(line, " "):
		st.curArchs += " " + trimmed
	}
}

// handleDebReposLineField handles field lines in a deb822 repo stanza.
func handleDebReposLineField(field, value string, st *debReposState) {
	switch field {
	case "Types":
		st.curTypes = value
	case "URIs":
		st.curURIs = value
	case "Suites":
		st.curSuites = value
	case "Components":
		st.curComponents = value
	case "Architectures":
		st.curArchs = value
	case "Signed-By":
		st.curSignedBy = value
	}
}

// handleDebReposLine processes a single line from a deb822 repo stanza.
// Returns nothing; mutates state. Handles blank line (flush), continuation, or field.
func handleDebReposLine(line string, st *debReposState, entries *[]SourceEntry) {
	// Blank line → end of stanza
	if line == "" {
		flushDeb822RepoStanza(entries, st.curTypes, st.curURIs, st.curSuites, st.curComponents, st.curArchs, st.curSignedBy)
		*st = debReposState{}

		return
	}

	// Continuation line (starts with space) — append to current field
	if line != "" && (line[0] == ' ' || line[0] == '\t') {
		handleDebReposLineContinuation(line, st)

		return
	}

	// Field line: "FieldName: value"
	field, value, ok := strings.Cut(line, ":")
	if !ok {
		return
	}

	field = strings.TrimSpace(field)
	value = strings.TrimSpace(value)

	handleDebReposLineField(field, value, st)
}

// parseDeb822SourcesListForRepo parses /etc/apt/sources.list.d/*.sources format and returns SourceEntry slice.
func parseDeb822SourcesListForRepo(content string) []SourceEntry {
	var entries []SourceEntry

	_ = deb822.Parse(strings.NewReader(content), func(stanzaMap deb822.Stanza) error {
		flushDeb822RepoStanza(&entries, stanzaMap["Types"], stanzaMap["URIs"], stanzaMap["Suites"], stanzaMap["Components"], stanzaMap["Architectures"], stanzaMap["Signed-By"]) //nolint:lll
		return nil
	})

	return entries
}

// flushDeb822RepoStanza processes a completed deb822 repo stanza by extracting URIs and adding SourceEntry records.
// Only processes stanzas with all required fields (Types, URIs, Suites, Components).
func flushDeb822RepoStanza(
	entries *[]SourceEntry,
	curTypes, curURIs, curSuites, curComponents, curArchs, curSignedBy string,
) {
	if curTypes == "" || curURIs == "" || curSuites == "" || curComponents == "" {
		return
	}

	// Parse URIs (space-separated, may span multiple lines)
	for rawURL := range strings.FieldsSeq(curURIs) {
		if !strings.HasPrefix(rawURL, "http://") && !strings.HasPrefix(rawURL, "https://") {
			continue
		}

		addSourceEntries(entries, rawURL, curSuites, curComponents, curArchs, curSignedBy)
	}
}
