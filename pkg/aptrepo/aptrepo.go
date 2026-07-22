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
	"os"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/M0Rf30/yap/v2/pkg/aptcache"
	"github.com/M0Rf30/yap/v2/pkg/errors"
	"github.com/M0Rf30/yap/v2/pkg/i18n"
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
		return 0, errors.New(errors.ErrTypeConfiguration, "no apt sources configured").
			WithOperation("UpdateWithOptions")
	}

	if err := os.MkdirAll(aptListsDir, 0o755); err != nil {
		return 0, err
	}

	logger.Info(i18n.T("logger.aptrepo.info.updating_indexes"), "sources", len(sources),
		"allow_unverified", opts.AllowUnverifiedRepos)

	// Flatten (source, arch) into a single job list so all combinations run
	// concurrently. With several sources × 1-2 archs each, sequential
	// execution made apt-get update wall-clock-dominated by mirror latency
	// even though every fetch is independent. The shared http.Transport
	// (see pkg/httpclient) caps per-host parallelism, so even when two
	// sources share a mirror (e.g. archive.ubuntu.com main + security)
	// we won't hammer it past MaxConnsPerHost.
	type job struct {
		src  *aptcache.SourceEntry
		arch string
	}

	var jobs []job

	for i := range sources {
		src := &sources[i]

		archs := src.Architectures
		if len(archs) == 0 {
			archs = []string{detectHostDebArch()}
		}

		for _, arch := range archs {
			jobs = append(jobs, job{src: src, arch: arch})
		}
	}

	// 8 workers comfortably covers the typical source set (main, security,
	// updates, backports, plus a few vendor repos × 1-2 archs) without
	// thrashing any single mirror.
	concurrency := min(min(runtime.GOMAXPROCS(0)*2, 8), len(jobs))
	if concurrency == 0 {
		return 0, nil
	}

	jobCh := make(chan job, len(jobs))
	for _, j := range jobs {
		jobCh <- j
	}

	close(jobCh)

	type result struct {
		src  *aptcache.SourceEntry
		arch string
		n    int
		err  error
	}

	resCh := make(chan result, len(jobs))

	var wg sync.WaitGroup

	wg.Add(concurrency)

	relCache := newReleaseCache()

	for range concurrency {
		go func() {
			defer wg.Done()

			for j := range jobCh {
				logger.Debug(i18n.T("logger.aptrepo.debug.fetching_source"), "url", j.src.URL,
					"suite", j.src.Suite,
					"components", j.src.Components,
					"arch", j.arch)

				n, err := updateSource(ctx, j.src, j.arch, opts, relCache)
				resCh <- result{src: j.src, arch: j.arch, n: n, err: err}
			}
		}()
	}

	wg.Wait()
	close(resCh)

	var (
		firstErr    error
		succeeded64 int64
	)

	for res := range resCh {
		succeeded64 += int64(res.n)

		if res.err != nil {
			logger.Warn(i18n.T("logger.aptrepo.warn.source_fetch_failed"),
				"url", res.src.URL, "suite", res.src.Suite, "arch", res.arch, "error", res.err)

			if firstErr == nil {
				firstErr = res.err
			}
		} else {
			logger.Info(i18n.T("logger.aptrepo.info.source_fetched"),
				"url", res.src.URL, "suite", res.src.Suite, "arch", res.arch, "components", res.n)
		}
	}

	succeeded = int(succeeded64)

	// Refresh the in-process aptcache singleton so subsequent Lookup /
	// ResolveDeps calls see the fresh indexes. Without this the singleton
	// would keep returning the stale snapshot for the lifetime of the
	// process.
	if succeeded > 0 {
		aptcache.Reload()

		c := aptcache.Load()
		logger.Info(i18n.T("logger.aptrepo.info.cache_reloaded"), "packages", c.PackageCount(),
			"capabilities", c.CapabilityCount())
	}

	return succeeded, firstErr
}

// IsVerificationError reports whether err is solely a signature verification
// failure (unknown signer or no trust anchor) rather than a network or I/O
// error. Callers that have already fetched enough indexes for their purposes
// can use this to downgrade such errors to warnings instead of aborting.
func IsVerificationError(err error) bool {
	if err == nil {
		return false
	}

	msg := err.Error()

	return strings.Contains(msg, ErrUnknownSigner.Error()) ||
		strings.Contains(msg, ErrNoTrustAnchor.Error())
}

// releaseCache deduplicates fetchRelease calls within a single Update run.
// Stock sources.list files commonly split one (url, suite) across several
// deb lines (e.g. Ubuntu ships main+restricted, universe, and multiverse
// as separate lines), and multi-arch sources fan out one job per
// architecture — without dedup every such job would re-download and
// re-verify the identical InRelease document, multiplying round trips on
// slow mirrors.
type releaseCache struct {
	// fn is fetchRelease in production; tests inject a counter.
	fn func(ctx context.Context, baseURL, suite, signedBy string, allowUnverified bool) (*Release, error)

	mu sync.Mutex
	m  map[string]*releaseCacheEntry
}

type releaseCacheEntry struct {
	once sync.Once
	rel  *Release
	err  error
}

func newReleaseCache() *releaseCache {
	return &releaseCache{fn: fetchRelease, m: make(map[string]*releaseCacheEntry)}
}

// fetch returns the Release for (baseURL, suite, signedBy, allowUnverified),
// fetching and verifying it exactly once even under concurrent callers.
// Errors are cached too: a failed mirror fails every duplicate source
// instead of retrying once per deb line.
func (c *releaseCache) fetch(
	ctx context.Context, baseURL, suite, signedBy string, allowUnverified bool,
) (*Release, error) {
	key := baseURL + "\x00" + suite + "\x00" + signedBy
	if allowUnverified {
		key += "\x00u"
	}

	c.mu.Lock()

	e, ok := c.m[key]
	if !ok {
		e = &releaseCacheEntry{}
		c.m[key] = e
	}

	c.mu.Unlock()

	e.once.Do(func() {
		e.rel, e.err = c.fn(ctx, baseURL, suite, signedBy, allowUnverified)
	})

	return e.rel, e.err
}

// updateSource fetches Release and component indexes for a single source+arch.
//
// SECURITY: Signature verification is performed by fetchRelease against
// the trust anchor declared by `Signed-By:` (or the standard apt trust
// paths when unset). A signature that exists and fails to verify is
// fatal regardless of AllowUnverifiedRepos.
func updateSource(
	ctx context.Context, src *aptcache.SourceEntry, arch string, opts Options, rc *releaseCache,
) (int, error) {
	// Fetch + verify InRelease (or fall back to Release+Release.gpg).
	// verifyInRelease / verifyDetachedRelease are called inside
	// fetchRelease against the keyring referenced by src.SignedBy (or the
	// default apt trust paths when SignedBy is unset).
	rel, err := rc.fetch(ctx, src.URL, src.Suite, src.SignedBy, opts.AllowUnverifiedRepos)
	if err != nil {
		return 0, err
	}

	// 3. For each component, find the Packages.* entry in rel.SHA256.
	// Prefer .xz > .gz > .bz2 > uncompressed (smallest first).
	//
	// Components fan out concurrently — within one source they all hit the
	// same mirror, so the shared http.Transport's MaxConnsPerHost limit is
	// what actually caps parallelism. Going from serial to fan-out roughly
	// halves the per-source wall time on ubuntu (main + universe +
	// multiverse + restricted).
	type compResult struct {
		comp string
		err  error
	}

	resCh := make(chan compResult, len(src.Components))

	var wg sync.WaitGroup

	for _, comp := range src.Components {
		wg.Add(1)

		go func(comp string) {
			defer wg.Done()

			resCh <- compResult{comp: comp, err: fetchComponentIndex(ctx, src, comp, arch, rel)}
		}(comp)
	}

	wg.Wait()
	close(resCh)

	n := 0

	for res := range resCh {
		if res.err != nil {
			logger.Warn(i18n.T("logger.aptrepo.warn.apt_component_fetch_failed"),
				"url", src.URL, "component", res.comp, "arch", arch, "error", res.err)

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
