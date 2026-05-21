// Package apkindex parses Alpine APKINDEX files and installs .apk packages
// APK packages. It replaces "apk update" and "apk add" subprocess calls.
//
// Typical usage:
//
//	idx, err := apkindex.Update(ctx)
//	if err != nil {
//		return err
//	}
//	if err := idx.Install(ctx, []string{"gcc", "musl-dev"}); err != nil {
//		return err
//	}
package apkindex

import (
	"context"
	"sync"

	"github.com/M0Rf30/yap/v2/pkg/logger"
	"github.com/M0Rf30/yap/v2/pkg/platform"
)

// Package holds the subset of APKINDEX fields needed for install/dep resolution.
type Package struct {
	Name        string
	Version     string
	Arch        string
	Size        int64
	InstSize    int64
	Description string
	URL         string
	License     string
	Origin      string
	Maintainer  string
	Depends     []string
	Provides    []string
	Checksum    string // Q1+base64-SHA1 — for content addressing
	RepoBaseURL string // populated at parse time
}

// Index is an in-memory APKINDEX from one or more Alpine repositories.
type Index struct {
	mu        sync.RWMutex
	packages  map[string]*Package   // name → first-winning Package
	providers map[string][]*Package // virtual name → providers
}

// NewIndex creates a new empty Index.
func NewIndex() *Index {
	return &Index{
		packages:  make(map[string]*Package),
		providers: make(map[string][]*Package),
	}
}

// Lookup returns the Package for the named package and whether it was found.
// The name must be the bare package name without version constraints.
func (idx *Index) Lookup(name string) (*Package, bool) {
	idx.mu.RLock()
	defer idx.mu.RUnlock()

	pkg, ok := idx.packages[name]

	return pkg, ok
}

// Stats returns the number of packages and capabilities (Provides) indexed.
// Useful for diagnostic logging after Update.
func (idx *Index) Stats() (packages, capabilities int) {
	idx.mu.RLock()
	defer idx.mu.RUnlock()

	return len(idx.packages), len(idx.providers)
}

// ResolveVirtual returns the first concrete provider of a virtual package, or
// nil if no provider is found.
func (idx *Index) ResolveVirtual(name string) (*Package, bool) {
	idx.mu.RLock()
	defer idx.mu.RUnlock()

	providers, ok := idx.providers[name]
	if !ok || len(providers) == 0 {
		return nil, false
	}

	return providers[0], true
}

// ResolveDeps does a simple greedy BFS over the index to resolve transitive
// dependencies. Returns a list of packages to install (in dependency order).
// Alpine deps are not a true SAT problem; conflicts are rare in practice.
func (idx *Index) ResolveDeps(names []string) ([]*Package, error) {
	var out []*Package

	seen := make(map[string]bool)

	var (
		viaVirtual int
		unresolved int
	)

	var visit func(n string) error

	visit = func(n string) error {
		n = stripVersionConstraint(n)
		if seen[n] {
			return nil
		}

		seen[n] = true

		pkg, ok := idx.Lookup(n)
		if !ok {
			// Try virtual package resolution.
			if vp, ok := idx.ResolveVirtual(n); ok {
				logger.Debug("apkindex: resolved virtual",
					"capability", n,
					"provider", vp.Name)

				viaVirtual++

				pkg = vp
			} else {
				// Package not found — skip it; apk will reject it later if needed.
				logger.Debug("apkindex: unresolved", "package", n)

				unresolved++

				return nil
			}
		}

		// Recursively visit dependencies.
		for _, d := range pkg.Depends {
			// Skip negations like "!conflict".
			if d != "" && d[0] == '!' {
				continue
			}

			// Skip /-prefixed paths (file-based deps); apk handles these via Provides.
			if d != "" && d[0] == '/' {
				continue
			}

			if err := visit(d); err != nil {
				return err
			}
		}

		logger.Debug("apkindex: enqueue install",
			"package", pkg.Name,
			"version", pkg.Version,
			"size", pkg.Size)

		out = append(out, pkg)

		return nil
	}

	for _, n := range names {
		if err := visit(n); err != nil {
			return nil, err
		}
	}

	logger.Info("apkindex: resolved transitive deps",
		"seeds", len(names),
		"to_install", len(out),
		"via_virtual", viaVirtual,
		"unresolved", unresolved)

	return out, nil
}

// Install is a convenience function that calls Update to fetch the index,
// then installs the requested packages. This is the main entry point for
// replacing "apk update && apk add <pkgs>" subprocess calls.
//
// APK package signature verification (RSA against the trusted keyring in
// /etc/apk/keys) is not yet implemented; the convenience wrapper accepts
// that gap when running on a privileged host (uid 0 — the expected case
// inside a yap build container) and refuses on a developer workstation.
// Use InstallPackagesWithOptions for explicit control.
func Install(ctx context.Context, names []string) error {
	idx, err := Update(ctx)
	if err != nil {
		return err
	}

	return idx.InstallPackagesWithOptions(ctx, names, InstallOptions{
		AllowUnverifiedPackages: platform.IsPrivilegedHost(),
	})
}
