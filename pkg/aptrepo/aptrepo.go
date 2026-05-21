// Package aptrepo refreshes apt repository indexes.
//
// It parses /etc/apt/sources.list and /etc/apt/sources.list.d/, fetches
// each source's InRelease (or Release + Release.gpg), verifies the PGP
// signature against the repo's keyring, then downloads and SHA-256-checks
// the binary-arch Packages indexes. Output goes to /var/lib/apt/lists/ in
// apt's own filename layout, so pkg/aptcache reads it directly.
//
// AllowUnverifiedRepos relaxes the missing-trust-anchor case (no key in
// the source's Signed-By target, no key matched in the default trust
// paths). A signature that is present but fails to verify is always
// fatal.
package aptrepo

import (
	"context"
	"fmt"
	"os"
	"runtime"
	"strings"
	"sync/atomic"

	"github.com/M0Rf30/yap/v2/pkg/aptcache"
	"github.com/M0Rf30/yap/v2/pkg/logger"
)

const aptListsDir = "/var/lib/apt/lists"

// envAllowUnverifiedRepos lets users (or container Dockerfiles) opt into
// the unverified-repo path without a CLI flag. Set to "1", "true", or
// "yes" (case-insensitive) to allow updates against sources that declare
// Signed-By but for which we don't yet verify the PGP signature.
const envAllowUnverifiedRepos = "YAP_ALLOW_UNVERIFIED_REPOS"

// allowUnverifiedFlag is the process-wide override toggled by CLI code via
// SetAllowUnverifiedRepos. Atomic so callers can flip it from any
// goroutine; default false (strict).
var allowUnverifiedFlag atomic.Bool

// SetAllowUnverifiedRepos sets the process-wide opt-in to bypass the
// Signed-By guard. Wire this from the CLI (e.g. --allow-unverified-repos)
// before calling Update / GetUpdates. The flag persists for the lifetime
// of the process.
func SetAllowUnverifiedRepos(v bool) { allowUnverifiedFlag.Store(v) }

// AllowUnverifiedRepos returns the effective opt-in state — the union of
// the CLI flag and the env var fallback.
func AllowUnverifiedRepos() bool {
	if allowUnverifiedFlag.Load() {
		return true
	}

	switch strings.ToLower(strings.TrimSpace(os.Getenv(envAllowUnverifiedRepos))) {
	case "1", "true", "yes", "on":
		return true
	}

	return false
}

// Options controls Update's behaviour. The zero value is the strict default.
type Options struct {
	// AllowUnverifiedRepos disables the refusal-on-Signed-By guard. Callers
	// running inside CI containers against well-known mirrors over HTTPS
	// may opt into this until GPG verification lands. When false, the
	// effective value falls back to the process-wide flag set via
	// SetAllowUnverifiedRepos or the YAP_ALLOW_UNVERIFIED_REPOS env var.
	AllowUnverifiedRepos bool
}

// Update fetches every (suite, component, arch) combination from every
// configured apt source, verifies hashes, and writes the indexes to disk.
//
// Returns the number of (source, component, arch) tuples that succeeded
// and any error encountered. On partial failure (e.g. one mirror down) it
// returns the count + a non-fatal error.
//
// The Signed-By guard is honoured according to the global opt-in
// (SetAllowUnverifiedRepos / YAP_ALLOW_UNVERIFIED_REPOS). Callers wanting
// per-invocation control should use UpdateWithOptions directly.
func Update(ctx context.Context) (succeeded int, err error) {
	return UpdateWithOptions(ctx, Options{AllowUnverifiedRepos: AllowUnverifiedRepos()})
}

// UpdateWithOptions is the explicit-options variant of Update.
func UpdateWithOptions(ctx context.Context, opts Options) (succeeded int, err error) {
	sources := aptcache.LoadSources()
	if len(sources) == 0 {
		return 0, fmt.Errorf("aptrepo: no apt sources configured")
	}

	if err := os.MkdirAll(aptListsDir, 0o755); err != nil {
		return 0, err
	}

	logger.Info("aptrepo: updating indexes",
		"sources", len(sources),
		"allow_unverified", opts.AllowUnverifiedRepos)

	var firstErr error

	for i := range sources {
		src := &sources[i]

		archs := src.Architectures
		if len(archs) == 0 {
			archs = []string{detectHostDebArch()}
		}

		for _, arch := range archs {
			logger.Debug("aptrepo: fetching source",
				"url", src.URL,
				"suite", src.Suite,
				"components", src.Components,
				"arch", arch)

			n, err := updateSource(ctx, src, arch, opts)
			succeeded += n

			if err != nil {
				logger.Warn("aptrepo: source fetch failed",
					"url", src.URL, "suite", src.Suite, "arch", arch, "error", err)

				if firstErr == nil {
					firstErr = err
				}
			} else {
				logger.Info("aptrepo: source fetched",
					"url", src.URL, "suite", src.Suite, "arch", arch, "components", n)
			}
		}
	}

	// Refresh the in-process aptcache singleton so subsequent Lookup /
	// ResolveDeps calls see the fresh indexes. Without this the singleton
	// would keep returning the stale snapshot for the lifetime of the
	// process.
	if succeeded > 0 {
		aptcache.Reload()

		c := aptcache.Load()
		logger.Info("aptrepo: cache reloaded",
			"packages", c.PackageCount(),
			"capabilities", c.CapabilityCount())
	}

	return succeeded, firstErr
}

// updateSource fetches Release and component indexes for a single source+arch.
//
// SECURITY: Signature verification is performed by fetchRelease against
// the trust anchor declared by `Signed-By:` (or the standard apt trust
// paths when unset). A signature that exists and fails to verify is
// fatal regardless of AllowUnverifiedRepos.
func updateSource(ctx context.Context, src *aptcache.SourceEntry, arch string, opts Options) (int, error) {
	// Fetch + verify InRelease (or fall back to Release+Release.gpg).
	// verifyInRelease / verifyDetachedRelease are called inside
	// fetchRelease against the keyring referenced by src.SignedBy (or the
	// default apt trust paths when SignedBy is unset).
	rel, err := fetchRelease(ctx, src.URL, src.Suite, src.SignedBy, opts.AllowUnverifiedRepos)
	if err != nil {
		return 0, err
	}

	// 3. For each component, find the Packages.* entry in rel.SHA256.
	// Prefer .xz > .gz > .bz2 > uncompressed (smallest first).
	n := 0

	for _, comp := range src.Components {
		if err := fetchComponentIndex(ctx, src, comp, arch, rel); err != nil {
			// Log warning, continue with other components.
			logger.Warn("apt component fetch failed",
				"url", src.URL, "component", comp, "arch", arch, "error", err)

			continue
		}

		n++
	}

	return n, nil
}

// detectHostDebArch returns the Debian architecture for the current host.
// Falls back to runtime.GOARCH-derived value if /var/lib/dpkg/arch is
// unavailable.
//
// `/etc/dpkg/architecture` is NOT a real dpkg file. The canonical sources
// of truth (in priority order) are:
//
//  1. `/var/lib/dpkg/arch` — written by dpkg --add-architecture.
//  2. `dpkg --print-architecture` — subprocess fallback (we don't use it).
//  3. `runtime.GOARCH` mapped to Debian arch names.
func detectHostDebArch() string {
	// 1. dpkg's per-arch list file.
	if data, err := os.ReadFile("/var/lib/dpkg/arch"); err == nil {
		// First non-empty line is the host (primary) architecture.
		for line := range strings.SplitSeq(string(data), "\n") {
			arch := strings.TrimSpace(line)
			if arch != "" {
				return arch
			}
		}
	}

	// 2. Map GOARCH → Debian arch names. Covers the architectures we ship
	//    container images for.
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
	case "riscv64":
		return "riscv64"
	default:
		return runtime.GOARCH
	}
}
