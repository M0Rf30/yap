// Package aptcache is an in-memory index of apt + dpkg metadata.
//
// It parses /var/lib/apt/lists/*_Packages and /var/lib/dpkg/status (deb822
// format) once, giving callers O(1) Lookup, transitive ResolveDeps with
// virtual-package handling, and a concurrent Download backed by grab.
//
// Typical use during cross-compile dep partitioning:
//
//	cache := aptcache.Load()
//	info, ok := cache.Lookup("libssl-dev")
//	if ok && info.ArchitectureAll() { ... }
package aptcache

import (
	"bufio"
	"compress/bzip2"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/cavaliergopher/grab/v3"
	"github.com/klauspost/compress/zstd"
	lz4 "github.com/pierrec/lz4/v4"
	"github.com/ulikunitz/xz"

	"github.com/M0Rf30/yap/v2/pkg/httpclient"
)

const (
	aptListsDir    = "/var/lib/apt/lists"
	dpkgStatusFile = "/var/lib/dpkg/status"
)

// PackageInfo holds the subset of deb822 fields needed for cross-compilation
// partition decisions and virtual package resolution.
type PackageInfo struct {
	// Name is the package name (e.g. "gcc", "libssl-dev").
	Name string
	// Architecture is the value of the Architecture field (e.g. "amd64", "all", "arm64").
	Architecture string
	// Essential is true when the package carries "Essential: yes".
	Essential bool
	// MultiArch is the value of the Multi-Arch field ("same", "foreign", "allowed", or "").
	MultiArch string
	// Installed is true when the dpkg status database reports the package as
	// "install ok installed".
	Installed bool
	// HasCandidate is true when the package has at least one version in the apt index
	// (i.e. it is a real, installable package — not a pure virtual).
	HasCandidate bool
	// Filename is the relative path of the .deb in the apt repository
	// (e.g. "pool/main/g/gcc/gcc_12.2.0-14_amd64.deb").
	Filename string
	// SHA256 is the expected SHA-256 checksum of the .deb file.
	SHA256 string
	// Size is the expected size of the .deb file in bytes.
	Size int64
	// BaseURL is the repo base URL (scheme + host + path) where this package's
	// Filename is relative to. E.g. "https://ports.ubuntu.com/ubuntu-ports/".
	// Empty for packages from the dpkg status DB (not downloadable).
	BaseURL string
	// Depends is the parsed Depends field (list of package names without version constraints).
	Depends []string
	// PreDepends is the parsed Pre-Depends field (list of package names without version constraints).
	PreDepends []string
}

// ArchitectureAll reports whether the package is architecture-independent.
func (p PackageInfo) ArchitectureAll() bool { //nolint:gocritic
	return strings.EqualFold(p.Architecture, "all")
}

// MultiArchForeign reports whether a single host-arch copy of this package
// satisfies dependencies from any architecture (Multi-Arch: foreign or allowed).
// These are tools and daemons that run on the build host — they must NOT be
// qualified with a target arch during cross-compilation.
//
// Multi-Arch: same (dev libraries) is intentionally excluded: those packages
// must be installed separately for each architecture.
func (p PackageInfo) MultiArchForeign() bool { //nolint:gocritic
	ma := strings.ToLower(p.MultiArch)

	return ma == "foreign" || ma == "allowed"
}

// MultiArchSame reports whether this package must be installed separately for
// each architecture (Multi-Arch: same). These are typically -dev libraries
// that need to be qualified with the target arch during cross-compilation.
func (p PackageInfo) MultiArchSame() bool { //nolint:gocritic
	return strings.EqualFold(p.MultiArch, "same")
}

// Cache is an in-memory index of package metadata keyed by package name.
// It merges data from the apt package index and the dpkg status database.
// The zero value is not usable; call Load.
type Cache struct {
	mu      sync.RWMutex
	entries map[string]*PackageInfo
	// providers maps a virtual package name to the list of concrete packages
	// that provide it (populated from the Provides field in apt index files).
	providers map[string][]string
}

// global singleton so the expensive file scan happens at most once per
// process. Stored as atomic.Pointer so Load/Reload don't need a mutex on
// the read path — Lookup is called extremely frequently during dep
// resolution and any contention here would dominate the resolver hot
// path.
var (
	globalCache atomic.Pointer[Cache]
	loadOnce    sync.Once
)

// Load returns the process-global Cache, loading it on the first call.
// Subsequent calls return the cached result immediately.
// The cache is always non-nil; on non-Debian hosts it is simply empty.
func Load() *Cache {
	if c := globalCache.Load(); c != nil {
		return c
	}

	loadOnce.Do(func() {
		globalCache.Store(loadFromDisk())
	})

	return globalCache.Load()
}

// Reload discards the cached result and re-reads the apt/dpkg metadata from
// disk. Call this after running apt-get update so that packages from newly
// added repositories are visible to subsequent Lookup calls.
//
// The new cache is built before the old one is replaced, so concurrent
// readers always see a consistent snapshot (the old one until the swap,
// the new one afterwards).
func Reload() *Cache {
	fresh := loadFromDisk()
	globalCache.Store(fresh)
	// loadOnce stays Done so future Load() calls fast-path the
	// atomic.Load.

	return fresh
}

// Lookup returns the PackageInfo for the named package and whether it was found.
// The name must be the bare package name without version constraints or arch qualifiers.
func (c *Cache) Lookup(name string) (PackageInfo, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	p, ok := c.entries[name]
	if !ok {
		return PackageInfo{}, false
	}

	return *p, true
}

// ResolveVirtual returns the first concrete provider of a virtual package, or
// the original name if the package is real (has a candidate) or unknown.
// This replaces the `apt-cache policy` + `apt-cache showpkg` two-step.
func (c *Cache) ResolveVirtual(name string) string {
	c.mu.RLock()
	defer c.mu.RUnlock()

	// If the package has a real candidate in the index, it is not virtual.
	if p, ok := c.entries[name]; ok && p.HasCandidate {
		return name
	}

	// Look up the reverse-provides index.
	if providers, ok := c.providers[name]; ok && len(providers) > 0 {
		return providers[0]
	}

	return name
}

// ResolveDeps performs transitive dependency resolution starting from the
// given seed packages. It returns the topologically-ordered list of
// packages that must be downloaded (deps before dependents), and a list of
// unresolvable deps (not in the index, possibly virtual).
//
// Packages already marked as Installed are skipped (their Pre-Depends are
// still traversed for completeness, but the package itself is not added to
// the install list).
//
// Implementation: post-order DFS keyed by package name. Cycles are
// short-circuited via the `seen` map (set before recursion). Go's growable
// goroutine stack handles real-world Debian dep depths (typically <20)
// comfortably; an iterative variant would only be worth the complexity for
// a pathologically deep graph.
func (c *Cache) ResolveDeps(seeds []string) ([]*PackageInfo, []string, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	seen := make(map[string]bool)

	var (
		order []*PackageInfo
		unres []string
	)

	// Create a visitor function that captures seen, order, and unres.
	visitor := c.makeDepVisitor(seen, &order, &unres)

	for _, seed := range seeds {
		seed = strings.TrimSpace(seed)

		// Strip arch qualifier and version constraint.
		if i := strings.Index(seed, ":"); i >= 0 {
			seed = seed[:i]
		}

		if i := strings.Index(seed, "("); i >= 0 {
			seed = strings.TrimSpace(seed[:i])
		}

		if err := visitor(seed); err != nil {
			return nil, nil, err
		}
	}

	return order, unres, nil
}

// makeDepVisitor creates a closure that performs DFS traversal for dependency resolution.
// The closure captures seen, order, and unres to track visited packages and results.
func (c *Cache) makeDepVisitor(seen map[string]bool, order *[]*PackageInfo, unres *[]string) func(string) error {
	var visit func(name string) error

	visit = func(name string) error {
		name = strings.TrimSpace(name)
		if name == "" || seen[name] {
			return nil
		}

		seen[name] = true

		info, ok := c.entries[name]
		if !ok {
			// Try virtual resolution.
			if resolved := c.tryResolveVirtual(name); resolved != "" {
				return visit(resolved)
			}

			*unres = append(*unres, name)

			return nil
		}

		// Recurse on dependencies.
		if err := c.visitDeps(info, visit); err != nil {
			return err
		}

		// Only add to install list if not already installed.
		if !info.Installed {
			*order = append(*order, info)
		}

		return nil
	}

	return visit
}

// tryResolveVirtual attempts to resolve a virtual package name to a concrete provider.
// Returns the provider name or empty string if not found.
func (c *Cache) tryResolveVirtual(name string) string {
	if providers, ok := c.providers[name]; ok && len(providers) > 0 {
		return providers[0]
	}

	return ""
}

// visitDeps recursively visits all dependencies of a package.
// Handles both installed and uninstalled packages appropriately.
func (c *Cache) visitDeps(info *PackageInfo, visit func(string) error) error {
	// Always visit Pre-Depends.
	for _, d := range info.PreDepends {
		if err := visit(d); err != nil {
			return err
		}
	}

	// Always visit Depends.
	for _, d := range info.Depends {
		if err := visit(d); err != nil {
			return err
		}
	}

	return nil
}

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

		path := filepath.Join("/etc/apt/sources.list.d", name) //nolint:gocritic // #nosec G304
		if data, err := os.ReadFile(path); err == nil {        // #nosec G304
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

		path := filepath.Join("/etc/apt/sources.list.d", name) //nolint:gocritic // #nosec G304
		if data, err := os.ReadFile(path); err == nil {        // #nosec G304
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
	scanner := bufio.NewScanner(strings.NewReader(content))

	var curTypes, curURIs string

	for scanner.Scan() {
		line := scanner.Text()

		// Blank line → end of stanza
		if line == "" {
			flushDeb822Stanza(curTypes, curURIs, schemes)
			curTypes = ""
			curURIs = ""

			continue
		}

		// Continuation line (starts with space) — append to current field
		if line != "" && (line[0] == ' ' || line[0] == '\t') {
			if curURIs != "" {
				curURIs += " " + strings.TrimSpace(line)
			}

			continue
		}

		// Field line: "FieldName: value"
		field, value, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}

		field = strings.TrimSpace(field)
		value = strings.TrimSpace(value)

		switch field {
		case "Types":
			curTypes = value
		case "URIs":
			curURIs = value
		}
	}

	// Flush last stanza
	flushDeb822Stanza(curTypes, curURIs, schemes)
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

	scanner := bufio.NewScanner(strings.NewReader(content))
	st := debReposState{}

	for scanner.Scan() {
		line := scanner.Text()
		handleDebReposLine(line, &st, &entries)
	}

	// Flush last stanza
	flushDeb822RepoStanza(&entries, st.curTypes, st.curURIs, st.curSuites, st.curComponents, st.curArchs, st.curSignedBy)

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

// loadFromDisk reads all apt list files and the dpkg status file.
func loadFromDisk() *Cache {
	c := &Cache{
		entries:   make(map[string]*PackageInfo),
		providers: make(map[string][]string),
	}

	// Load source schemes first (for BaseURL resolution)
	sources := loadSourceSchemes()

	// 1. Parse apt package index files from /var/lib/apt/lists/
	// Non-fatal: apt lists may not exist (e.g. non-Debian host).
	_ = c.loadAptLists(aptListsDir, sources)

	// 2. Overlay dpkg status (installed packages) — sets Installed flag and
	//    fills in any fields missing from the apt index.
	// Non-fatal: may not exist on non-Debian hosts.
	_ = c.loadDpkgStatus(dpkgStatusFile)

	return c
}

// loadAptLists scans dir for *_Packages files in all compression variants
// that apt may write: uncompressed, .gz, .bz2, .xz, .lz4, .zst.
// sources is a map from encoded hostpath to sourceInfo, used to resolve BaseURL.
//
// Performance: each Packages.* file is parsed concurrently into a private
// per-file Cache. The dominant cost is xz/zstd decompression + line
// scanning (~3-5s per file on a typical Ubuntu noble install); doing them
// in parallel collapses 16 sequential files into a single core-bound
// round, dropping load time from ~55s to ~10s on a 4-core host.
//
// Concurrency cap is min(GOMAXPROCS, 8) — diminishing returns past that
// because the work is CPU-bound and bigger pools just thrash the
// scheduler.
func (c *Cache) loadAptLists(dir string, sources map[string]sourceInfo) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}

	type job struct {
		path    string
		baseURL string
	}

	jobs := make([]job, 0, len(entries))

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		name := entry.Name()
		if !isPackagesIndexName(name) {
			continue
		}

		jobs = append(jobs, job{
			path:    filepath.Join(dir, name),
			baseURL: deriveBaseURL(name, sources),
		})
	}

	if len(jobs) == 0 {
		return nil
	}

	concurrency := min(runtime.GOMAXPROCS(0), 8, len(jobs))

	// Each worker parses into a thread-local Cache (no shared lock during
	// parse), then we merge sequentially at the end. mergeFrom is a plain
	// map-walk — much cheaper than per-stanza mu.Lock.
	partials := make([]*Cache, len(jobs))

	jobCh := make(chan int, len(jobs))
	for i := range jobs {
		jobCh <- i
	}

	close(jobCh)

	var wg sync.WaitGroup

	wg.Add(concurrency)

	for range concurrency {
		go func() {
			defer wg.Done()

			for idx := range jobCh {
				local := &Cache{
					entries:   make(map[string]*PackageInfo),
					providers: make(map[string][]string),
				}
				// Skip unreadable/corrupt index files — apt itself is tolerant.
				_ = local.parseFile(jobs[idx].path, false, jobs[idx].baseURL)

				partials[idx] = local
			}
		}()
	}

	wg.Wait()

	for _, p := range partials {
		if p == nil {
			continue
		}

		c.mergeFrom(p)
	}

	return nil
}

// isPackagesIndexName reports whether name is one of the apt-emitted
// Packages index variants (uncompressed or one of the compressions apt
// supports).
func isPackagesIndexName(name string) bool {
	for _, suffix := range []string{
		"_Packages", "_Packages.gz", "_Packages.bz2",
		"_Packages.xz", "_Packages.lz4", "_Packages.zst",
	} {
		if strings.HasSuffix(name, suffix) {
			return true
		}
	}

	return false
}

// mergeFrom folds a worker-local Cache produced by loadAptLists into c.
// Last-writer-wins on per-field merge into existing entries; providers
// are append-merged. Called under c.mu held by the orchestrator goroutine.
func (c *Cache) mergeFrom(other *Cache) {
	c.mu.Lock()
	defer c.mu.Unlock()

	for name, info := range other.entries {
		existing, ok := c.entries[name]
		if !ok {
			// Copy the pointer; partial cache won't be touched after
			// this point.
			c.entries[name] = info

			continue
		}

		// Field-level merge: prefer non-empty values from the new cache.
		if info.Architecture != "" {
			existing.Architecture = info.Architecture
		}

		if info.MultiArch != "" {
			existing.MultiArch = info.MultiArch
		}

		if info.Essential {
			existing.Essential = true
		}

		if info.HasCandidate {
			existing.HasCandidate = true
		}

		if info.Filename != "" {
			existing.Filename = info.Filename
		}

		if info.SHA256 != "" {
			existing.SHA256 = info.SHA256
		}

		if info.Size > 0 {
			existing.Size = info.Size
		}

		if info.BaseURL != "" && existing.BaseURL == "" {
			existing.BaseURL = info.BaseURL
		}

		if len(info.Depends) > 0 {
			existing.Depends = info.Depends
		}

		if len(info.PreDepends) > 0 {
			existing.PreDepends = info.PreDepends
		}
	}

	for virt, providers := range other.providers {
		c.providers[virt] = append(c.providers[virt], providers...)
	}
}

// deriveBaseURL extracts the base URL from an apt list filename.
// Filename format: <encoded-hostpath>_dists_<suite>_<component>_binary-<arch>_Packages[.ext]
// Returns the full URL from sources map, or empty string if not found.
func deriveBaseURL(filename string, sources map[string]sourceInfo) string {
	// Strip compression suffix
	name := filename
	for _, ext := range []string{".gz", ".bz2", ".xz", ".lz4", ".zst"} {
		name = strings.TrimSuffix(name, ext)
	}

	// Find _dists_ separator
	prefix, _, found := strings.Cut(name, "_dists_")
	if !found {
		return ""
	}

	// Look up in sources map
	if info, ok := sources[prefix]; ok {
		return info.fullURL
	}

	return ""
}

// loadDpkgStatus parses the dpkg status database and marks packages as installed.
func (c *Cache) loadDpkgStatus(path string) error {
	return c.parseFile(path, true, "")
}

// parseFile opens a deb822 file (plain or gzip-compressed) and merges its
// stanzas into the cache. When dpkgStatus is true the "Status" field is
// checked and the Installed flag is set accordingly. baseURL is the repo
// base URL for apt index files (empty for dpkg status).
func (c *Cache) parseFile(path string, dpkgStatus bool, baseURL string) error {
	f, err := os.Open(path) // #nosec G304 — path is constructed from trusted constants
	if err != nil {
		return err
	}

	defer func() { _ = f.Close() }()

	var r io.Reader = f

	switch {
	case strings.HasSuffix(path, ".gz"):
		gz, err := gzip.NewReader(f)
		if err != nil {
			return err
		}

		defer func() { _ = gz.Close() }()

		r = gz
	case strings.HasSuffix(path, ".bz2"):
		r = bzip2.NewReader(f)
	case strings.HasSuffix(path, ".xz"):
		xzr, err := xz.NewReader(f)
		if err != nil {
			return err
		}

		r = xzr
	case strings.HasSuffix(path, ".lz4"):
		r = lz4.NewReader(f)
	case strings.HasSuffix(path, ".zst"):
		zr, err := zstd.NewReader(f)
		if err != nil {
			return err
		}

		defer zr.Close()

		r = zr
	}

	return c.parseDeb822(r, dpkgStatus, baseURL)
}

// stanza holds the fields extracted from a single deb822 stanza.
type stanza struct {
	pkgName    string
	arch       string
	multiArch  string
	provides   string
	depends    string
	preDepends string
	essential  bool
	installed  bool
	hasVersion bool
	filename   string
	sha256     string
	size       int64
	baseURL    string
}

// parseDeb822 reads deb822 stanzas from r and merges them into the cache.
//
// The deb822 format is a sequence of stanzas separated by blank lines.
// Each stanza is a set of "Field: value" lines; continuation lines start
// with a space or tab.  We only extract the fields we care about.
// baseURL is the repo base URL for apt index files (empty for dpkg status).
func (c *Cache) parseDeb822(r io.Reader, dpkgStatus bool, baseURL string) error {
	scanner := bufio.NewScanner(r)
	// Some Packages files have very long Description lines.
	scanner.Buffer(make([]byte, 256*1024), 256*1024)

	var cur stanza

	cur.baseURL = baseURL

	for scanner.Scan() {
		line := scanner.Text()

		// Blank line → end of stanza.
		if line == "" {
			c.flushStanza(&cur, dpkgStatus)
			cur = stanza{baseURL: baseURL}

			continue
		}

		// Continuation line (starts with space or tab) — skip, we don't need
		// multi-line field values for the fields we care about.
		if line[0] == ' ' || line[0] == '\t' {
			continue
		}

		// Field line: "FieldName: value"
		field, value, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}

		value = strings.TrimSpace(value)

		applyField(&cur, field, value, dpkgStatus)
	}

	// Flush the last stanza (file may not end with a blank line).
	c.flushStanza(&cur, dpkgStatus)

	return scanner.Err()
}

// applyField sets the appropriate stanza field from a parsed deb822 field line.
// applyField sets the appropriate stanza field from a parsed deb822 field line.
// Dispatches to applyCommonField or applyStatusField based on field type.
func applyField(s *stanza, field, value string, dpkgStatus bool) {
	switch field {
	case "Package":
		s.pkgName = value
	case "Version":
		s.hasVersion = value != ""
	case "Architecture":
		s.arch = value
	case "Essential":
		s.essential = strings.EqualFold(value, "yes")
	case "Multi-Arch":
		s.multiArch = value
	case "Provides":
		s.provides = value
	case "Depends":
		s.depends = value
	case "Pre-Depends":
		s.preDepends = value
	case "Filename":
		s.filename = value
	case "Status":
		applyStatusField(s, value, dpkgStatus)
	case "SHA256", "Size":
		applyIndexField(s, field, value, dpkgStatus)
	}
}

// applyStatusField handles the Status field from dpkg status database.
func applyStatusField(s *stanza, value string, dpkgStatus bool) {
	if dpkgStatus {
		s.installed = value == "install ok installed"
	}
}

// applyIndexField handles apt index-only fields (SHA256, Size).
func applyIndexField(s *stanza, field, value string, dpkgStatus bool) {
	if dpkgStatus {
		return // only in apt index, not dpkg/status
	}

	switch field {
	case "SHA256":
		s.sha256 = value
	case "Size":
		n, _ := strconv.ParseInt(value, 10, 64)
		s.size = n
	}
}

// flushStanza merges a completed stanza into the cache.
// Delegates field merging to mergePackageInfo helper.
func (c *Cache) flushStanza(s *stanza, dpkgStatus bool) {
	if s.pkgName == "" {
		return
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	existing, ok := c.entries[s.pkgName]
	if !ok {
		existing = &PackageInfo{}
		c.entries[s.pkgName] = existing
	}

	mergePackageInfo(existing, s, dpkgStatus)
	c.flushProvides(s.pkgName, s.provides)
}

// mergePackageInfo merges fields from a parsed stanza into an existing PackageInfo.
// Handles conditional merging based on field presence and dpkgStatus flag.
func mergePackageInfo(existing *PackageInfo, s *stanza, dpkgStatus bool) {
	existing.Name = s.pkgName

	if s.arch != "" {
		existing.Architecture = s.arch
	}

	if s.essential {
		existing.Essential = true
	}

	if s.multiArch != "" {
		existing.MultiArch = s.multiArch
	}

	if dpkgStatus && s.installed {
		existing.Installed = true
	}

	// A stanza with a Version field means the package is a real,
	// installable package (has a candidate), not a pure virtual.
	if s.hasVersion {
		existing.HasCandidate = true
	}

	if s.filename != "" {
		existing.Filename = s.filename
	}

	if s.sha256 != "" {
		existing.SHA256 = s.sha256
	}

	if s.size > 0 {
		existing.Size = s.size
	}

	// Set BaseURL if not already set (first-writer-wins: apt index takes
	// precedence over dpkg status, which has no BaseURL).
	if s.baseURL != "" && existing.BaseURL == "" {
		existing.BaseURL = s.baseURL
	}

	if s.depends != "" {
		existing.Depends = parseDependsField(s.depends)
	}

	if s.preDepends != "" {
		existing.PreDepends = parseDependsField(s.preDepends)
	}
}

// parseDependsField parses a Depends or Pre-Depends field value and returns
// a list of package names. It handles:
//   - Comma-separated dependencies
//   - Alternative dependencies (foo | bar) — takes the first option
//   - Version constraints (>= 1.0) — stripped
//   - Architecture qualifiers (:any) — stripped
func parseDependsField(value string) []string {
	if value == "" {
		return nil
	}

	parts := strings.Split(value, ",")
	out := make([]string, 0, len(parts))

	for _, p := range parts {
		p = strings.TrimSpace(p)

		// Alternative deps "foo | bar" — take the first.
		if i := strings.Index(p, "|"); i >= 0 {
			p = strings.TrimSpace(p[:i])
		}

		// Strip version constraint "(>= 1.0)".
		if i := strings.Index(p, "("); i >= 0 {
			p = strings.TrimSpace(p[:i])
		}

		// Strip arch qualifier ":any".
		if i := strings.Index(p, ":"); i >= 0 {
			p = p[:i]
		}

		if p != "" {
			out = append(out, p)
		}
	}

	return out
}

// flushProvides populates the reverse-provides index from a Provides field value.
// Must be called with c.mu already held.
func (c *Cache) flushProvides(pkgName, provides string) {
	if provides == "" {
		return
	}

	for prov := range strings.SplitSeq(provides, ",") {
		// Strip version constraint: "foo (= 1.0)" → "foo"
		vname, _, _ := strings.Cut(strings.TrimSpace(prov), " (")
		vname = strings.TrimSpace(vname)

		if vname == "" {
			continue
		}

		c.providers[vname] = append(c.providers[vname], pkgName)
	}
}

// DownloadClosure resolves the transitive closure of the supplied seed
// package names, downloads every resulting .deb into destDir, and returns
// the resolved PackageInfo slice in dependency order (deps before
// dependents) plus the list of names that could not be resolved.
//
// Behaviour:
//   - Packages already marked Installed are skipped (their dependencies are
//     still walked so transitive runtime deps reachable only through an
//     installed library still get pulled in).
//   - Virtual packages are resolved to their first concrete provider via
//     the reverse-Provides index.
//   - "foo | bar" alternatives resolve to the first option.
//   - Architecture / version qualifiers on seed names are stripped before
//     lookup; see Lookup for the bare-name contract.
//   - Cycles are short-circuited by ResolveDeps' internal `seen` map.
//
// This is the helper that callers should reach for when they need
// "everything required to actually use these packages on the target
// filesystem" — most importantly, cross-build runtime dep extraction in
// pkg/builders/common.DownloadAndExtractCrossDeps, where missing
// transitive deps cause cross-link failures like:
//
//	ld: warning: libvpx.so.12, needed by libavcodec.so, not found
//
// because PKGBUILDs only declare the direct dep (carbonio-ffmpeg) while
// the transitive arch-specific libs (carbonio-libvpx, carbonio-x264) are
// not surfaced unless we walk the dep graph ourselves.
func (c *Cache) DownloadClosure(
	ctx context.Context, destDir string, seeds []string,
) (resolved []*PackageInfo, unresolved []string, err error) {
	resolved, unresolved, err = c.ResolveDeps(seeds)
	if err != nil {
		return nil, nil, err
	}

	if len(resolved) == 0 {
		return resolved, unresolved, nil
	}

	names := make([]string, 0, len(resolved))
	for _, p := range resolved {
		names = append(names, p.Name)
	}

	if err := c.Download(ctx, destDir, names); err != nil {
		return resolved, unresolved, err
	}

	return resolved, unresolved, nil
}

// downloadConcurrency caps the number of parallel .deb downloads handed
// to grab.Client.DoBatch. Each mirror tolerates a handful of concurrent
// connections; 6 is enough to saturate a typical 100-1000 Mbit/s link
// without being rude to the mirror.
const downloadConcurrency = 6

// Download fetches the named packages into destDir using the apt package
// index metadata (Filename, SHA256, Size, BaseURL fields).
//
// Implementation: uses cavaliergopher/grab (the same library yap's
// pkg/download uses for source downloads). grab gives us for free:
//
//   - Concurrent batched downloads (DoBatch with a fixed worker pool).
//   - HTTP Range / resume on partially-downloaded files (so an interrupted
//     `yap build` doesn't re-fetch hundreds of MB).
//   - In-stream SHA-256 verification via Request.SetChecksum, with
//     delete-on-error so a corrupt .deb never lingers at destDir.
//
// Performance: a 100-package closure that took ~30s with sequential
// net/http drops to ~5-8s against archive.ubuntu.com.
//
// Returns an error if any package is not found in the index or any
// download fails. All downloads continue until completion (or context
// cancel); errors are aggregated and the first one returned. Partial
// files left by failed downloads are removed by grab itself.
//
// Most callers should prefer DownloadClosure, which performs transitive
// resolution before downloading. Use Download directly only when you
// already have an explicit, pre-resolved list of package names.
func (c *Cache) Download(ctx context.Context, destDir string, pkgs []string) error {
	if len(pkgs) == 0 {
		return nil
	}

	requests, err := c.buildDownloadRequests(ctx, destDir, pkgs)
	if err != nil {
		return err
	}

	workers := min(downloadConcurrency, len(requests))

	client := grab.NewClient()
	client.UserAgent = "YAP/2 (aptcache)"

	respCh := client.DoBatch(workers, requests...)

	var firstEr error

	for resp := range respCh {
		if err := resp.Err(); err != nil && firstEr == nil {
			firstEr = fmt.Errorf("aptcache: download %q: %w",
				filepath.Base(resp.Filename), err)
		}
	}

	return firstEr
}

// buildDownloadRequests turns each package name into a configured grab
// Request with the apt-index-supplied size + SHA-256 wired in. Resolving
// up-front means a missing-package error surfaces before any HTTP is
// done.
func (c *Cache) buildDownloadRequests(
	ctx context.Context, destDir string, pkgs []string,
) ([]*grab.Request, error) {
	requests := make([]*grab.Request, 0, len(pkgs))

	for _, pkg := range pkgs {
		name, _, _ := strings.Cut(pkg, ":")
		name, _, _ = strings.Cut(name, " (")
		name = strings.TrimSpace(name)

		info, ok := c.Lookup(name)
		if !ok || info.Filename == "" {
			return nil, fmt.Errorf(
				"aptcache: package %q not found in apt index (run apt-get update first)",
				name)
		}

		if info.BaseURL == "" {
			return nil, fmt.Errorf(
				"aptcache: package %q has no BaseURL (apt sources not parsed?)", name)
		}

		pkgURL := strings.TrimSuffix(info.BaseURL, "/") + "/" + info.Filename
		destFile := filepath.Join(destDir, filepath.Base(info.Filename))

		req, err := grab.NewRequest(destFile, pkgURL)
		if err != nil {
			return nil, fmt.Errorf("aptcache: build request for %q: %w", name, err)
		}

		req = req.WithContext(ctx)
		if info.Size > 0 {
			req.Size = info.Size
		}

		if info.SHA256 != "" {
			sum, decErr := hex.DecodeString(info.SHA256)
			if decErr == nil {
				// SetChecksum(hash, sum, deleteOnError=true):
				//   - streaming SHA-256 against `sum`;
				//   - delete the on-disk file if the hash mismatches,
				//     so a failed download never leaves a corrupt
				//     artifact at destFile.
				req.SetChecksum(sha256.New(), sum, true)
			}
		}

		requests = append(requests, req)
	}

	return requests, nil
}

// maxDebBytes caps an individual .deb download. Real Debian packages top
// out around 500 MB (e.g. texlive-full); 2 GiB is generous head-room while
// still defending against an unbounded mirror stream.
const maxDebBytes = 2 << 30

// downloadAndVerify downloads a file from pkgURL to destFile and verifies its
// SHA-256 checksum and size.
//
// The download is streamed through a size-capped io.LimitReader, written
// first to "<destFile>.tmp", hashed inline, and only renamed onto destFile
// after every verification step succeeds. A failed verification leaves no
// partial file at destFile — preventing callers from mistaking a corrupt
// stub for a verified package.
func downloadAndVerify(ctx context.Context, pkgURL, destFile, expectedSHA256 string, expectedSize int64) error {
	resp, err := startDownload(ctx, pkgURL)
	if err != nil {
		return err
	}

	defer func() { _ = resp.Body.Close() }()

	if err := preflightContentLength(resp, pkgURL, expectedSize); err != nil {
		return err
	}

	tmpFile := destFile + ".tmp"

	got, n, err := streamToTmp(resp, tmpFile)
	if err != nil {
		_ = os.Remove(tmpFile)
		return err
	}

	if err := verifySizeAndHash(n, got, expectedSize, expectedSHA256, pkgURL); err != nil {
		_ = os.Remove(tmpFile)
		return err
	}

	if err := os.Rename(tmpFile, destFile); err != nil {
		_ = os.Remove(tmpFile)
		return err
	}

	return nil
}

// startDownload issues the GET and validates the response status.
func startDownload(ctx context.Context, pkgURL string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, pkgURL, http.NoBody)
	if err != nil {
		return nil, err
	}

	resp, err := httpclient.Client().Do(req)
	if err != nil {
		return nil, err
	}

	if err := httpclient.CheckStatus(resp, pkgURL); err != nil {
		_ = resp.Body.Close()
		return nil, err
	}

	return resp, nil
}

// preflightContentLength fails fast if the server advertised a length that
// either exceeds the cap or contradicts the apt-index's expected size.
func preflightContentLength(resp *http.Response, pkgURL string, expectedSize int64) error {
	if resp.ContentLength <= 0 {
		return nil
	}

	if resp.ContentLength > maxDebBytes {
		return fmt.Errorf("aptcache: %s body too large: %d bytes (cap %d)",
			pkgURL, resp.ContentLength, maxDebBytes)
	}

	if expectedSize > 0 && resp.ContentLength != expectedSize {
		return fmt.Errorf("aptcache: Content-Length mismatch for %s: got %d, expected %d",
			pkgURL, resp.ContentLength, expectedSize)
	}

	return nil
}

// streamToTmp copies the response body into tmpFile, computing the SHA-256
// inline. Returns the hex-encoded hash and the byte count actually
// written. The LimitReader+1 trick detects servers that lie about
// Content-Length by yielding one byte beyond the cap.
func streamToTmp(resp *http.Response, tmpFile string) (hashHex string, written int64, err error) {
	f, err := os.Create(tmpFile) // #nosec G304 — tmpFile is destFile+".tmp", trusted index metadata
	if err != nil {
		return "", 0, err
	}

	defer func() { _ = f.Close() }()

	h := sha256.New()
	w := io.MultiWriter(f, h)
	body := io.LimitReader(resp.Body, maxDebBytes+1)

	n, err := io.Copy(w, body)
	if err != nil {
		return "", n, err
	}

	if err := f.Sync(); err != nil {
		return "", n, err
	}

	return hex.EncodeToString(h.Sum(nil)), n, nil
}

// verifySizeAndHash checks the streamed size against the cap and the
// expected size, and the hash against the expected SHA-256.
func verifySizeAndHash(n int64, gotHash string, expectedSize int64, expectedSHA256, pkgURL string) error {
	if n > maxDebBytes {
		return fmt.Errorf("aptcache: %s exceeded %d-byte cap", pkgURL, maxDebBytes)
	}

	if expectedSize > 0 && n != expectedSize {
		return fmt.Errorf("size mismatch: got %d, expected %d", n, expectedSize)
	}

	if expectedSHA256 != "" && gotHash != expectedSHA256 {
		return fmt.Errorf("SHA256 mismatch: got %s, expected %s", gotHash, expectedSHA256)
	}

	return nil
}
