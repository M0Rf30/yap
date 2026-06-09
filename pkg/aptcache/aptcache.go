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
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
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
	// Version is the raw deb822 Version field. Used to resolve same name:arch
	// collisions across repositories (highest version wins).
	Version string
}

// archAll is the Debian "Architecture: all" constant.
const archAll = "all"

// ArchitectureAll reports whether the package is architecture-independent.
func (p PackageInfo) ArchitectureAll() bool { //nolint:gocritic
	return strings.EqualFold(p.Architecture, archAll)
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
	// byBareName maps a bare package name (without arch qualifier) to a slice of
	// entry keys (e.g., "gcc-11:amd64", "gcc-11:arm64"). Used to optimize
	// scanEntryByName from O(n) to O(1) lookup + O(k) iteration where k is the
	// number of architectures for that package.
	byBareName map[string][]string
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
// The cache is always non-nil; on non-Debian hosts it is empty.
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

// NewEmptyCache creates an empty Cache suitable for injection in tests.
// Use StoreGlobal to make it the active singleton returned by Load.
func NewEmptyCache() *Cache {
	return &Cache{
		entries:    make(map[string]*PackageInfo),
		providers:  make(map[string][]string),
		byBareName: make(map[string][]string),
	}
}

// GoarchToDebArch returns the Debian architecture name for the current
// runtime.GOARCH. Used by Lookup and ResolveDeps to default to the host
// architecture during multi-arch resolution.
func GoarchToDebArch() string {
	switch runtime.GOARCH {
	case "amd64":
		return "amd64"
	case "arm64":
		return "arm64"
	case "arm":
		return "armhf"
	case "386":
		return "i386"
	case "ppc64le":
		return "ppc64el"
	case "s390x":
		return "s390x"
	default:
		return runtime.GOARCH
	}
}

// entryKey builds the cache-internal key for a package, combining name
// and architecture. Architecture: all and empty-arch entries are stored
// under the bare name for backward compatibility.
func entryKey(name, arch string) string {
	if arch == "" || arch == archAll {
		return name
	}

	return name + ":" + arch
}

// addToBareName adds an entry key to the byBareName secondary index.
// Must be called with c.mu already held.
func (c *Cache) addToBareName(name, key string) {
	// Only add to secondary index if the key is different from the bare name
	// (i.e., it has an arch qualifier). Bare names and arch:all entries are
	// stored directly under the bare name, so they don't need indexing.
	if key != name {
		c.byBareName[name] = append(c.byBareName[name], key)
	}
}

// AddEntry inserts or replaces a PackageInfo entry in the cache.
// Intended for test setup; not safe for concurrent use during population.
func (c *Cache) AddEntry(info *PackageInfo) {
	c.mu.Lock()
	defer c.mu.Unlock()

	cp := *info
	key := entryKey(info.Name, info.Architecture)
	c.entries[key] = &cp
	c.addToBareName(info.Name, key)
}

// StoreGlobal replaces the process-global cache returned by Load.
// Intended for cross-package test injection; call with a cache built via
// NewEmptyCache + AddEntry before the code under test calls Load.
func StoreGlobal(c *Cache) {
	globalCache.Store(c)
}

// Lookup returns the PackageInfo for the named package and whether it was found.
// The name may include an arch qualifier (e.g., "gcc-11:arm64").
// Without a qualifier, it tries the host architecture, "all", then any arch.
func (c *Cache) Lookup(name string) (PackageInfo, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	// If the name has an explicit arch qualifier, look up the exact key.
	if idx := strings.IndexByte(name, ':'); idx >= 0 {
		p, ok := c.entries[name]
		if ok {
			return *p, true
		}
		// Fall through to try unqualified name.
		name = name[:idx]
	}

	// Try bare name (for arch:all, empty-arch, or old-style entries).
	p, ok := c.entries[name]
	if ok {
		return *p, true
	}

	// Try host arch.
	hostKey := name + ":" + GoarchToDebArch()

	p, ok = c.entries[hostKey]
	if ok {
		return *p, true
	}

	// Try "all" arch.
	allKey := name + ":all"

	p, ok = c.entries[allKey]
	if ok {
		return *p, true
	}

	// Last resort: any entry matching name:* via the byBareName secondary
	// index — O(1) instead of a full-map scan. This handles edge cases
	// where a package only exists for a foreign architecture (e.g. a test
	// or cross-build situation with arm64-only packages on an amd64 host).
	// Misses are the common case during dependency resolution (virtual
	// packages), so this path must stay cheap.
	if candidate, ok := c.scanEntryByName(name); ok {
		return *candidate, true
	}

	return PackageInfo{}, false
}

// PackageCount returns the number of packages indexed in the cache. Useful for
// diagnostic logging after a Reload to surface how many entries were parsed.
func (c *Cache) PackageCount() int {
	c.mu.RLock()
	defer c.mu.RUnlock()

	return len(c.entries)
}

// CapabilityCount returns the number of virtual package names (Provides) in
// the providers index.
func (c *Cache) CapabilityCount() int {
	c.mu.RLock()
	defer c.mu.RUnlock()

	return len(c.providers)
}

// ResolveVirtual returns the first concrete provider of a virtual package, or
// the original name if the package is real (has a candidate) or unknown.
// This replaces the `apt-cache policy` + `apt-cache showpkg` two-step.
func (c *Cache) ResolveVirtual(name string) string {
	c.mu.RLock()
	defer c.mu.RUnlock()

	// Try name as-is, then with host arch, then with "all" arch.
	candidates := []string{name, name + ":" + GoarchToDebArch(), name + ":all"}
	for _, key := range candidates {
		if p, ok := c.entries[key]; ok && p.HasCandidate {
			return name
		}
		// If the key has an explicit arch qualifier, also try bare name.
		if strings.Contains(key, ":") {
			bare, _, _ := strings.Cut(key, ":")
			if p, ok := c.entries[bare]; ok && p.HasCandidate {
				return name
			}
		}
	}

	// Look up the reverse-provides index.
	if providers, ok := c.providers[name]; ok && len(providers) > 0 {
		return providers[0]
	}

	return name
}

// InstalledNames returns the set of package names (bare, without arch
// qualifier) currently installed on the host according to the merged
// dpkg status overlay. A name is considered installed if at least one
// arch variant is marked Installed.
//
// Used by cross-build extractors to avoid overlaying foreign-arch
// payloads onto host binaries whose path is shared across architectures
// (e.g. /usr/bin/sudo): even though dpkg multi-arch annotates packages
// with separate entries per arch, the extractor writes to a single root
// tree where /usr/bin/sudo exists once and is held open by the running
// sudo process.
func (c *Cache) InstalledNames() map[string]bool {
	c.mu.RLock()
	defer c.mu.RUnlock()

	out := make(map[string]bool)

	for _, info := range c.entries {
		if !info.Installed {
			continue
		}

		name := info.Name
		if i := strings.Index(name, ":"); i >= 0 {
			name = name[:i]
		}

		out[name] = true
	}

	return out
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
// Seeds may include an arch qualifier ("libc6-dev:arm64"). The arch is
// preserved and inherited by all transitive dependencies, ensuring correct
// multi-arch resolution when both amd64 and arm64 indexes are loaded.
//
// Implementation: post-order DFS keyed by package name:arch. Cycles are
// short-circuited via the `visited` map (set before recursion). Go's growable
// goroutine stack handles real-world Debian dep depths (typically <20)
// comfortably; an iterative variant would only be worth the complexity for
// a pathologically deep graph.
func (c *Cache) ResolveDeps(seeds []string) ([]*PackageInfo, []string, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	visited := make(map[string]bool)

	var (
		order []*PackageInfo
		unres []string
	)

	// Create a visitor function that captures visited, order, and unres.
	visitor := c.makeDepVisitor(visited, &order, &unres)

	for _, seed := range seeds {
		seed = strings.TrimSpace(seed)

		// Split off arch qualifier but preserve it.
		arch := ""
		if i := strings.Index(seed, ":"); i >= 0 {
			arch = seed[i+1:]
			seed = seed[:i]
		}

		// Strip version constraint "(>= 1.0)".
		if i := strings.Index(seed, "("); i >= 0 {
			seed = strings.TrimSpace(seed[:i])
		}

		if err := visitor(seed, arch); err != nil {
			return nil, nil, err
		}
	}

	return order, unres, nil
}

// makeDepVisitor creates a closure that performs DFS traversal for dependency
// resolution. The closure captures visited, order, and unres to track
// visited packages and results.
//
// The returned visitor accepts (name, arch) where arch is the architecture
// for the target package. Empty arch means "any" (use host arch or --all).
func (c *Cache) makeDepVisitor(visited map[string]bool, order *[]*PackageInfo, unres *[]string,
) func(string, string) error {
	var visit func(name, arch string) error

	visit = func(name, arch string) error {
		name = strings.TrimSpace(name)
		if name == "" {
			return nil
		}

		// Use host arch if none specified.
		if arch == "" {
			arch = GoarchToDebArch()
		}

		// Build the cache lookup key.
		key := entryKey(name, arch)

		if visited[key] {
			return nil
		}

		visited[key] = true

		info, ok := c.entries[key]
		if !ok {
			// Fall back to bare name (arch:all or backward compat).
			info, ok = c.entries[name]
		}

		if !ok {
			// Fall back to host arch. This handles the case where
			// qualifyDepsForTargetArch added :arm64 to a host-only package
			// like gcc-aarch64-linux-gnu (which has no arm64 variant).
			hostKey := name + ":" + GoarchToDebArch()
			info, ok = c.entries[hostKey]
		}

		if !ok {
			// Last resort: scan for any entry matching name:*.
			// Handles edge cases where a package only exists for a foreign
			// arch (test scenarios, cross-build with arm64-only packages).
			info, ok = c.scanEntryByName(name)
		}

		if !ok {
			// Try virtual resolution.
			if resolved := c.tryResolveVirtual(name); resolved != "" {
				return visit(resolved, arch)
			}

			*unres = append(*unres, name)

			return nil
		}

		// Redirect Multi-Arch: foreign packages to host-arch (see helper).
		info, arch = c.redirectForeignToHost(info, name, arch)

		// If the found package has a different architecture than the requested
		// one (e.g., found arm64 entry when requesting amd64), use the actual
		// package's architecture for child dependencies so transitive deps
		// resolve to the correct arch.
		if info.Architecture != "" && info.Architecture != arch && info.Architecture != archAll {
			arch = info.Architecture
		}

		// Recurse on dependencies, inheriting the architecture.
		if err := c.visitDeps(info, arch, visit); err != nil {
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

// redirectForeignToHost redirects Multi-Arch: foreign packages to host-arch.
//
// Multi-Arch: foreign means "a single copy of this package satisfies
// dependencies from any architecture" — by definition a host tool (gawk,
// perl, m4, autoconf, debianutils, …). When a foreign-arch build pulls one
// in transitively, resolve it to host-arch so the cross-extract step
// doesn't overlay an arm64 gawk on top of the host x86_64 gawk and break
// later configure scripts with "Exec format error".
//
// Returns the (possibly redirected) info and arch.
func (c *Cache) redirectForeignToHost(
	info *PackageInfo, name, arch string,
) (resolved *PackageInfo, resolvedArch string) {
	hostArch := GoarchToDebArch()
	if !info.MultiArchForeign() || arch == hostArch {
		return info, arch
	}

	hostInfo, ok := c.entries[entryKey(name, hostArch)]
	if !ok {
		return info, arch
	}

	return hostInfo, hostArch
}

// tryResolveVirtual attempts to resolve a virtual package name to a concrete provider.
// Returns the provider name or empty string if not found.
func (c *Cache) tryResolveVirtual(name string) string {
	if providers, ok := c.providers[name]; ok && len(providers) > 0 {
		return providers[0]
	}

	return ""
}

// scanEntryByName performs a lookup in the byBareName secondary index to find
// any package matching the given bare name. Returns the first match and true,
// or nil and false if none found.
//
// This is now O(1) lookup + O(k) iteration where k is the number of
// architectures for that package (typically 1-3), replacing the previous O(n)
// full-map scan.
func (c *Cache) scanEntryByName(name string) (*PackageInfo, bool) {
	// Look up the secondary index for this bare name.
	keys, ok := c.byBareName[name]
	if !ok || len(keys) == 0 {
		return nil, false
	}

	// Return the first entry found for this bare name.
	if candidate, ok := c.entries[keys[0]]; ok {
		return candidate, true
	}

	return nil, false
}

// visitDeps recursively visits all dependencies of a package.
// The arch parameter is inherited by all child dependencies so that
// multi-arch resolution is consistent through the dependency graph.
// Handles both installed and uninstalled packages appropriately.
func (c *Cache) visitDeps(info *PackageInfo, arch string, visit func(string, string) error) error {
	// Always visit Pre-Depends.
	for _, d := range info.PreDepends {
		if err := visit(d, arch); err != nil {
			return err
		}
	}

	// Always visit Depends.
	for _, d := range info.Depends {
		if err := visit(d, arch); err != nil {
			return err
		}
	}

	return nil
}
