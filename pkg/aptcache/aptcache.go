// Package aptcache provides a pure-Go reader for apt and dpkg metadata files.
//
// It parses /var/lib/apt/lists/*_Packages (apt package index) and
// /var/lib/dpkg/status (installed package database) using the deb822
// (RFC 822-like) plain-text format, building an in-memory index so that
// callers can perform O(1) lookups instead of spawning one apt-cache/dpkg
// subprocess per package.
//
// Typical usage in a cross-compilation context:
//
//	cache := aptcache.Load()
//	info, ok := cache.Lookup("libssl-dev")
//	if ok && info.ArchitectureAll() { ... }
package aptcache

import (
	"bufio"
	"compress/gzip"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

const (
	aptListsDir    = "/var/lib/apt/lists"
	dpkgStatusFile = "/var/lib/dpkg/status"
)

// PackageInfo holds the subset of deb822 fields needed for cross-compilation
// partition decisions and virtual package resolution.
type PackageInfo struct {
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
}

// ArchitectureAll reports whether the package is architecture-independent.
func (p PackageInfo) ArchitectureAll() bool {
	return strings.EqualFold(p.Architecture, "all")
}

// MultiArchForeign reports whether the package supports multi-arch installation
// alongside a host-arch copy (foreign, allowed, or same).
func (p PackageInfo) MultiArchForeign() bool {
	ma := strings.ToLower(p.MultiArch)

	return ma == "foreign" || ma == "allowed" || ma == "same"
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

// global singleton so the expensive file scan happens at most once per process.
var (
	globalOnce  sync.Once
	globalCache *Cache
)

// Load returns the process-global Cache, loading it on the first call.
// Subsequent calls return the cached result immediately.
// The cache is always non-nil; on non-Debian hosts it is simply empty.
func Load() *Cache {
	globalOnce.Do(func() {
		globalCache = loadFromDisk()
	})

	return globalCache
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

// loadFromDisk reads all apt list files and the dpkg status file.
func loadFromDisk() *Cache {
	c := &Cache{
		entries:   make(map[string]*PackageInfo),
		providers: make(map[string][]string),
	}

	// 1. Parse apt package index files from /var/lib/apt/lists/
	// Non-fatal: apt lists may not exist (e.g. non-Debian host).
	_ = c.loadAptLists(aptListsDir)

	// 2. Overlay dpkg status (installed packages) — sets Installed flag and
	//    fills in any fields missing from the apt index.
	// Non-fatal: may not exist on non-Debian hosts.
	_ = c.loadDpkgStatus(dpkgStatusFile)

	return c
}

// loadAptLists scans dir for *_Packages and *_Packages.gz files and parses them.
func (c *Cache) loadAptLists(dir string) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		name := entry.Name()
		// Only binary package index files are relevant.
		if !strings.HasSuffix(name, "_Packages") && !strings.HasSuffix(name, "_Packages.gz") {
			continue
		}

		path := filepath.Join(dir, name)
		// Skip unreadable/corrupt index files — apt itself is tolerant.
		_ = c.parseFile(path, false)
	}

	return nil
}

// loadDpkgStatus parses the dpkg status database and marks packages as installed.
func (c *Cache) loadDpkgStatus(path string) error {
	return c.parseFile(path, true)
}

// parseFile opens a deb822 file (plain or gzip-compressed) and merges its
// stanzas into the cache. When dpkgStatus is true the "Status" field is
// checked and the Installed flag is set accordingly.
func (c *Cache) parseFile(path string, dpkgStatus bool) error {
	f, err := os.Open(path) // #nosec G304 — path is constructed from trusted constants
	if err != nil {
		return err
	}

	defer func() { _ = f.Close() }()

	var r io.Reader = f

	if strings.HasSuffix(path, ".gz") {
		gz, err := gzip.NewReader(f)
		if err != nil {
			return err
		}

		defer func() { _ = gz.Close() }()

		r = gz
	}

	return c.parseDeb822(r, dpkgStatus)
}

// stanza holds the fields extracted from a single deb822 stanza.
type stanza struct {
	pkgName    string
	arch       string
	multiArch  string
	provides   string
	essential  bool
	installed  bool
	hasVersion bool
}

// parseDeb822 reads deb822 stanzas from r and merges them into the cache.
//
// The deb822 format is a sequence of stanzas separated by blank lines.
// Each stanza is a set of "Field: value" lines; continuation lines start
// with a space or tab.  We only extract the fields we care about.
func (c *Cache) parseDeb822(r io.Reader, dpkgStatus bool) error {
	scanner := bufio.NewScanner(r)
	// Some Packages files have very long Description lines.
	scanner.Buffer(make([]byte, 256*1024), 256*1024)

	var cur stanza

	for scanner.Scan() {
		line := scanner.Text()

		// Blank line → end of stanza.
		if line == "" {
			c.flushStanza(cur, dpkgStatus)
			cur = stanza{}

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
	c.flushStanza(cur, dpkgStatus)

	return scanner.Err()
}

// applyField sets the appropriate stanza field from a parsed deb822 field line.
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
	case "Status":
		if dpkgStatus {
			s.installed = value == "install ok installed"
		}
	}
}

// flushStanza merges a completed stanza into the cache.
func (c *Cache) flushStanza(s stanza, dpkgStatus bool) {
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

	c.flushProvides(s.pkgName, s.provides)
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
