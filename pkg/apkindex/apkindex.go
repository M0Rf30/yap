// Package apkindex provides a pure-Go reader and installer for Alpine Linux
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
				pkg = vp
			} else {
				// Package not found — skip it; apk will reject it later if needed.
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

		out = append(out, pkg)

		return nil
	}

	for _, n := range names {
		if err := visit(n); err != nil {
			return nil, err
		}
	}

	return out, nil
}

// Install is a convenience function that calls Update to fetch the index,
// then installs the requested packages. This is the main entry point for
// replacing "apk update && apk add <pkgs>" subprocess calls.
func Install(ctx context.Context, names []string) error {
	idx, err := Update(ctx)
	if err != nil {
		return err
	}

	return idx.InstallPackages(ctx, names)
}
