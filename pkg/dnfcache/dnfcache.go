// Package dnfcache is an in-memory index of DNF/YUM repository metadata.
//
// It parses /etc/yum.repos.d/*.repo files, fetches repomd.xml from each
// enabled repository, downloads and parses primary.xml.gz to build a
// package index, and provides O(1) Lookup, transitive ResolveDeps with
// virtual-package (Provides) handling, and concurrent SHA256-verified
// downloads.
//
// Typical use:
//
//	if err := dnfcache.Update(ctx); err != nil {
//	    return err
//	}
//	if err := dnfcache.Install(ctx, []string{"gcc", "make"}); err != nil {
//	    return err
//	}
package dnfcache

import (
	"context"
	"sync"
	"sync/atomic"

	"github.com/M0Rf30/yap/v2/pkg/logger"
)

// PackageInfo holds the subset of primary.xml fields needed for dep
// resolution and download.
type PackageInfo struct {
	// Name is the package name (e.g. "gcc", "glibc-devel").
	Name string
	// Arch is the package architecture (e.g. "x86_64", "noarch").
	Arch string
	// Version is the package version string.
	Version string
	// Release is the package release string.
	Release string
	// Epoch is the package epoch (0 if absent).
	Epoch string
	// LocationHref is the relative path of the .rpm in the repository
	// (e.g. "Packages/g/gcc-12.2.0-1.el8.x86_64.rpm").
	LocationHref string
	// SHA256 is the expected SHA-256 checksum of the .rpm file.
	SHA256 string
	// Size is the expected size of the .rpm file in bytes.
	Size int64
	// BaseURL is the repo base URL where LocationHref is relative to.
	BaseURL string
	// Requires is the list of package names this package depends on.
	Requires []string
	// Provides is the list of capabilities this package provides.
	Provides []string
}

// Cache is an in-memory index of RPM package metadata keyed by package name.
// The zero value is not usable; call Load or Update.
type Cache struct {
	mu        sync.RWMutex
	packages  map[string]*PackageInfo   // name → best candidate
	providers map[string][]*PackageInfo // virtual/capability → providers
	modules   *moduleIndex              // module-stream filter (never nil after newCache)
}

var (
	globalCache atomic.Pointer[Cache]
	loadOnce    sync.Once
)

// Load returns the process-global Cache, loading it from disk on the first
// call. Subsequent calls return the cached result immediately.
// The cache is always non-nil; on non-RPM hosts it is empty.
func Load() *Cache {
	if c := globalCache.Load(); c != nil {
		return c
	}

	loadOnce.Do(func() {
		c := newCache()
		c.loadFromDisk()
		globalCache.Store(c)
	})

	return globalCache.Load()
}

// Reload discards the cached result and re-reads repo metadata from disk.
// Call this after Update so newly fetched indexes are visible.
func Reload() *Cache {
	c := newCache()
	c.loadFromDisk()
	globalCache.Store(c)

	return c
}

// Update fetches fresh repomd.xml + primary.xml.gz from all enabled repos,
// writes them to the DNF cache directory, and reloads the in-memory index.
// This replaces "dnf makecache".
func Update(ctx context.Context) error {
	if err := fetchAllRepos(ctx); err != nil {
		return err
	}

	Reload()

	return nil
}

// Install resolves the transitive closure of names, downloads the .rpm
// files, and installs them via rpm --install. This replaces "dnf install".
func Install(ctx context.Context, names []string) error {
	c := Load()

	resolved, unresolved, err := c.ResolveDeps(ctx, names)
	if err != nil {
		return err
	}

	if len(unresolved) > 0 {
		// Non-fatal: log and continue — rpm will surface hard errors.
		logUnresolved(unresolved)
	}

	if len(resolved) == 0 {
		return nil
	}

	return c.downloadAndInstall(ctx, resolved)
}

// Lookup returns the PackageInfo for the named package and whether it was found.
func (c *Cache) Lookup(name string) (*PackageInfo, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	p, ok := c.packages[name]

	return p, ok
}

// ResolveVirtual returns the first concrete provider of a virtual package
// name (capability), or the original name if it is a real package or unknown.
func (c *Cache) ResolveVirtual(name string) string {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if p, ok := c.packages[name]; ok && p.LocationHref != "" {
		return name
	}

	if providers, ok := c.providers[name]; ok && len(providers) > 0 {
		return providers[0].Name
	}

	return name
}

// ResolveDeps performs transitive dependency resolution starting from the
// given seed package names. Returns packages in dependency order (deps
// before dependents) and a list of unresolvable names.
//
// Already-installed packages (detected via rpmdb) are skipped but their
// dependency edges are still walked so transitive-only packages are pulled in.
func (c *Cache) ResolveDeps(ctx context.Context, seeds []string) ([]*PackageInfo, []string, error) {
	installed := loadInstalledSet(ctx)
	provides := loadInstalledProvides(ctx)

	c.mu.RLock()
	defer c.mu.RUnlock()

	logger.Debug("dnfcache: resolver state loaded",
		"installed_packages", len(installed),
		"installed_capabilities", len(provides),
		"index_packages", len(c.packages),
		"index_capabilities", len(c.providers))

	seen := make(map[string]bool)

	var (
		order              []*PackageInfo
		unres              []string
		skippedInstalled   int
		resolvedViaVirtual int
	)

	var visit func(name string)

	visit = func(name string) {
		name = StripRPMConstraint(name)
		if name == "" || seen[name] {
			return
		}

		seen[name] = true

		// Capability already provided by an installed package: do not pull
		// any alternative provider. Prevents conflicts like coreutils vs
		// coreutils-single where both own /usr/bin/ls.
		if installed[name] || provides[name] {
			logger.Debug("dnfcache: skip (already satisfied)", "package", name)

			skippedInstalled++

			return
		}

		info, ok := c.packages[name]
		if !ok {
			// Try virtual/capability resolution.
			if providers, ok2 := c.providers[name]; ok2 && len(providers) > 0 {
				logger.Debug("dnfcache: resolved virtual",
					"capability", name,
					"provider", providers[0].Name)

				resolvedViaVirtual++

				visit(providers[0].Name)

				return
			}

			logger.Debug("dnfcache: unresolved", "package", name)
			unres = append(unres, name)

			return
		}

		// Recurse on Requires.
		for _, req := range info.Requires {
			visit(req)
		}

		logger.Debug("dnfcache: enqueue install",
			"package", info.Name,
			"version", info.Version,
			"size", info.Size)
		order = append(order, info)
	}

	for _, seed := range seeds {
		visit(StripRPMConstraint(seed))
	}

	logger.Info("dnfcache: resolved transitive deps",
		"seeds", len(seeds),
		"to_install", len(order),
		"skipped_installed", skippedInstalled,
		"via_virtual", resolvedViaVirtual,
		"unresolved", len(unres))

	return order, unres, nil
}

const archNoarch = "noarch"

// newCache allocates an empty Cache.
func newCache() *Cache {
	return &Cache{
		packages:  make(map[string]*PackageInfo),
		providers: make(map[string][]*PackageInfo),
		modules:   newModuleIndex(),
	}
}

// isBlockedByModuleFilter reports whether p is a modular package belonging
// to a non-default stream and should therefore be excluded from the cache.
// Returns false for non-modular packages and when no module metadata is
// loaded (non-AppStream repos).
func (c *Cache) isBlockedByModuleFilter(p *PackageInfo) bool {
	if c.modules == nil || len(c.modules.defaultStream) == 0 {
		return false
	}

	if !isModularPackage(p.Release) {
		return false
	}

	if _, hasDefault := c.modules.defaultStream[p.Name]; !hasDefault {
		return false
	}

	nvra := packageNVRA(p.Name, p.Epoch, p.Version, p.Release, p.Arch)

	return !c.modules.allowedNVRA[nvra]
}

// addPackage inserts or updates a package in the cache and populates the
// providers index from its Provides list.
// Must be called with c.mu held (write).
func (c *Cache) addPackage(p *PackageInfo) {
	// Filter non-default modular streams (e.g. perl 5.24 on Rocky 8 where
	// 5.26 is the default) so they neither shadow non-modular variants nor
	// surface as virtual providers — matching `dnf install` without
	// `module enable`.
	if c.isBlockedByModuleFilter(p) {
		return
	}

	hostArch := goArchToRPM()

	existing, ok := c.packages[p.Name]
	switch {
	case !ok:
		c.packages[p.Name] = p
	case existing.LocationHref == "" && p.LocationHref != "":
		// Prefer entries that are actually downloadable.
		c.packages[p.Name] = p
	case existing.Arch != p.Arch && p.Arch == hostArch:
		// Prefer the host architecture entry over a foreign-arch one.
		c.packages[p.Name] = p
	case existing.Arch != p.Arch && p.Arch == archNoarch && existing.Arch != hostArch:
		// Prefer noarch over foreign-arch when no host-arch entry exists.
		c.packages[p.Name] = p
	}

	for _, prov := range p.Provides {
		capName := StripRPMConstraint(prov)
		if capName == "" || capName == p.Name {
			continue
		}

		c.providers[capName] = append(c.providers[capName], p)
	}
}
