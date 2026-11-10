// SPDX-FileCopyrightText: 2021-2026 Gianluca Boiano
// SPDX-License-Identifier: GPL-3.0-only

// Package apthost binds the apt client of github.com/M0Rf30/pkgformat/deb/apt
// to the host: the apt and dpkg paths of a Debian system, the trust policy
// that yap's --allow-unverified-repos flag and YAP_ALLOW_UNVERIFIED_REPOS
// select, and the process-wide package cache that dependency resolution and
// the installers share.
//
// The client has no host defaults of its own, so every path below is passed
// to it from here.
package apthost

import (
	"context"
	"os"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/M0Rf30/pkgformat/deb/apt"
	"github.com/M0Rf30/pkgformat/httpclient"

	"github.com/M0Rf30/yap/v2/pkg/errors"
	"github.com/M0Rf30/yap/v2/pkg/i18n"
	"github.com/M0Rf30/yap/v2/pkg/logger"
)

// Host locations of apt's and dpkg's state.
const (
	listsDir       = "/var/lib/apt/lists"
	statusFile     = "/var/lib/dpkg/status"
	dpkgArchFile   = "/var/lib/dpkg/arch"
	sourcesList    = "/etc/apt/sources.list"
	sourcesListDir = "/etc/apt/sources.list.d"
)

// envAllowUnverifiedRepos lets users (or container Dockerfiles) opt into
// accepting repositories without a verifiable signature without a CLI flag.
// "1", "true", "yes" or "on" (case-insensitive) enables it.
const envAllowUnverifiedRepos = "YAP_ALLOW_UNVERIFIED_REPOS"

// userAgent identifies yap to the mirrors.
const userAgent = "YAP/2 (apt)"

// defaultKeyrings are the places apt trusts keys from, for sources without a
// Signed-By option.
var defaultKeyrings = []string{
	"/etc/apt/trusted.gpg.d",
	"/usr/share/keyrings",
	"/etc/apt/keyrings",
	"/etc/apt/trusted.gpg",
}

// allowUnverifiedFlag is the process-wide override toggled by CLI code via
// SetAllowUnverified. Atomic so callers can flip it from any goroutine.
var allowUnverifiedFlag atomic.Bool

// SetAllowUnverified sets the process-wide opt-in to accept repositories that
// no key vouches for. Wire it from the CLI before calling Update.
func SetAllowUnverified(v bool) { allowUnverifiedFlag.Store(v) }

// AllowUnverified returns the effective opt-in state: the union of the CLI
// flag and the environment variable.
func AllowUnverified() bool {
	if allowUnverifiedFlag.Load() {
		return true
	}

	switch strings.ToLower(strings.TrimSpace(os.Getenv(envAllowUnverifiedRepos))) {
	case "1", "true", "yes", "on":
		return true
	}

	return false
}

// HostArch is the Debian architecture of the running program, the one
// unqualified package names resolve to.
func HostArch() string { return apt.ArchFromGoarch(runtime.GOARCH) }

// dpkgArch is the primary architecture dpkg is configured for: the first line
// of /var/lib/dpkg/arch (written by dpkg --add-architecture), else HostArch.
func dpkgArch() string {
	if data, err := os.ReadFile(dpkgArchFile); err == nil {
		for line := range strings.SplitSeq(string(data), "\n") {
			if arch := strings.TrimSpace(line); arch != "" {
				return arch
			}
		}
	}

	return HostArch()
}

// DownloadOptions are the transfer settings for Cache.Download and
// Cache.DownloadClosure: no overall request timeout (a large package on a slow
// link must not be aborted) and yap's user agent.
func DownloadOptions() apt.DownloadOptions {
	return apt.DownloadOptions{
		Client: httpclient.New(httpclient.Options{Timeout: -1, UserAgent: userAgent, Logger: logger.Slog()}),
	}
}

// Sources returns the sources apt is configured with.
func Sources() []apt.Source {
	sources, err := apt.LoadSources(sourcesList, sourcesListDir)
	if err != nil {
		logger.Warn(i18n.T("logger.apthost.warn.sources_partially_loaded"), "error", err)
	}

	return sources
}

// load reads the cache from the host's apt lists and dpkg status. A file that
// cannot be read is skipped, as apt does; on a non-Debian host the cache is
// empty.
func load() *apt.Cache {
	cache, err := apt.Load(apt.LoadOptions{
		ListsDir:   listsDir,
		StatusFile: statusFile,
		Sources:    Sources(),
		HostArch:   HostArch(),
		Logger:     logger.Slog(),
	})
	if err != nil {
		logger.Debug(i18n.T("logger.apthost.debug.index_files_skipped"), "error", err)
	}

	return cache
}

// globalCache is the process-wide cache, loaded on first use so the expensive
// file scan happens at most once. Atomic so the read path (Lookup is hit
// constantly during resolution) takes no lock.
var (
	globalCache atomic.Pointer[apt.Cache]
	loadOnce    sync.Once
)

// Cache returns the process-wide package cache, loading it on the first call.
// It is never nil; on non-Debian hosts it is empty.
func Cache() *apt.Cache {
	if c := globalCache.Load(); c != nil {
		return c
	}

	loadOnce.Do(func() { globalCache.CompareAndSwap(nil, load()) })

	return globalCache.Load()
}

// Reload re-reads the apt and dpkg metadata from disk. Call it after the
// indexes or the dpkg database changed so later lookups see them. The new
// cache is built before the old one is replaced, so concurrent readers always
// see a consistent snapshot.
func Reload() *apt.Cache {
	fresh := load()
	globalCache.Store(fresh)

	return fresh
}

// SetCache replaces the process-wide cache. It exists for tests that inject a
// cache built with apt.NewCache and AddEntry.
func SetCache(c *apt.Cache) { globalCache.Store(c) }

// Update refreshes the host's apt indexes from every configured source into
// /var/lib/apt/lists, verifying signatures under the AllowUnverified policy,
// and reloads the process-wide cache when anything was fetched. It returns the
// number of indexes now in place and an *apt.UpdateError for the sources that
// failed (see apt.IsVerificationError).
func Update(ctx context.Context) (int, error) {
	sources := Sources()
	if len(sources) == 0 {
		return 0, errors.New(errors.ErrTypeConfiguration, "no apt sources configured").
			WithOperation("Update")
	}

	allowUnverified := AllowUnverified()

	logger.Info(i18n.T("logger.apthost.info.updating_indexes"), "sources", len(sources),
		"allow_unverified", allowUnverified)

	n, err := apt.Update(ctx, sources, apt.UpdateOptions{
		ListsDir:        listsDir,
		DefaultArch:     dpkgArch(),
		DefaultKeyrings: defaultKeyrings,
		AllowUnverified: allowUnverified,
		Client:          httpclient.New(httpclient.Options{UserAgent: userAgent, Logger: logger.Slog()}),
		Logger:          logger.Slog(),
	})

	if n > 0 {
		c := Reload()
		logger.Info(i18n.T("logger.apthost.info.cache_reloaded"), "packages", c.PackageCount(),
			"capabilities", c.CapabilityCount())
	}

	return n, err
}
