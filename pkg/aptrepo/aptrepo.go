// Package aptrepo provides a pure-Go replacement for "apt-get update".
//
// It fetches InRelease/Release files and Packages indexes from apt repositories
// configured in /etc/apt/sources.list and /etc/apt/sources.list.d/, verifies
// SHA-256 checksums, and writes them to /var/lib/apt/lists/ in the same
// filename format apt-get itself uses — so pkg/aptcache can read them.
package aptrepo

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/M0Rf30/yap/v2/pkg/aptcache"
	"github.com/M0Rf30/yap/v2/pkg/logger"
)

const aptListsDir = "/var/lib/apt/lists"

// Update fetches every (suite, component, arch) combination from every
// configured apt source, verifies hashes, and writes the indexes to disk.
//
// Returns the number of (source, component, arch) tuples that succeeded
// and any error encountered. On partial failure (e.g. one mirror down) it
// returns the count + a non-fatal error.
func Update(ctx context.Context) (succeeded int, err error) {
	sources := aptcache.LoadSources()
	if len(sources) == 0 {
		return 0, fmt.Errorf("aptrepo: no apt sources configured")
	}

	if err := os.MkdirAll(aptListsDir, 0o755); err != nil {
		return 0, err
	}

	var firstErr error

	for i := range sources {
		src := &sources[i]

		archs := src.Architectures
		if len(archs) == 0 {
			archs = []string{detectHostDebArch()}
		}

		for _, arch := range archs {
			n, err := updateSource(ctx, src, arch)
			succeeded += n

			if err != nil && firstErr == nil {
				firstErr = err
			}
		}
	}

	return succeeded, firstErr
}

// updateSource fetches Release and component indexes for a single source+arch.
func updateSource(ctx context.Context, src *aptcache.SourceEntry, arch string) (int, error) {
	// 1. Fetch InRelease (or Release if InRelease 404s).
	rel, err := fetchRelease(ctx, src.URL, src.Suite)
	if err != nil {
		return 0, err
	}

	// 2. Optional GPG verification — skip for now if Signed-By unset.
	// TODO: Implement GPG verification using ProtonMail/go-crypto.
	if src.SignedBy != "" {
		logger.Debug("aptrepo: GPG verification deferred", "signed-by", src.SignedBy)
	}

	// 3. For each component, find the Packages.* entry in rel.SHA256.
	// Prefer .xz > .gz > .bz2 > uncompressed (smallest first).
	n := 0

	for _, comp := range src.Components {
		if err := fetchComponentIndex(ctx, src, comp, arch, rel); err != nil {
			// Log warning, continue with other components.
			logger.Warn("aptrepo: component fetch failed",
				"url", src.URL, "component", comp, "arch", arch, "error", err)

			continue
		}

		n++
	}

	return n, nil
}

// detectHostDebArch returns the Debian architecture for the current host.
// Falls back to "amd64" if detection fails.
func detectHostDebArch() string {
	// Simple detection: check /etc/dpkg/architecture if it exists.
	if data, err := os.ReadFile("/etc/dpkg/architecture"); err == nil {
		arch := strings.TrimSpace(string(data))
		if arch != "" {
			return arch
		}
	}

	// Fallback to amd64 (most common in containers).
	return "amd64"
}
