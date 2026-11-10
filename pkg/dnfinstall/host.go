// SPDX-FileCopyrightText: 2021-2026 Gianluca Boiano
// SPDX-License-Identifier: GPL-3.0-only

package dnfinstall

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"strings"
	"sync"

	"github.com/M0Rf30/pkgformat/rpm/dnf"

	"github.com/M0Rf30/yap/v2/pkg/i18n"
	"github.com/M0Rf30/yap/v2/pkg/logger"
	"github.com/M0Rf30/yap/v2/pkg/rpmdb"
)

// Host locations of the dnf configuration and metadata cache. The repository
// client itself (github.com/M0Rf30/pkgformat/rpm/dnf) assumes none of them.
const (
	yumRepoDir  = "/etc/yum.repos.d"
	dnfCacheDir = "/var/cache/dnf"
	dnfVarsDir  = "/etc/dnf/vars"
	osReleaseFn = "/etc/os-release"
)

// newHostClient returns a dnf client reading the host's repository files and
// metadata cache, with the host's rpmdb telling it what is installed.
func newHostClient() *dnf.Client {
	return dnf.New(dnf.Options{
		RepoDir:    yumRepoDir,
		CacheDir:   dnfCacheDir,
		VarsDir:    dnfVarsDir,
		Releasever: readReleasever(),
		Installed:  hostInstalled{},
		Logger:     logger.Slog(),
	})
}

// readReleasever returns VERSION_ID from /etc/os-release, cached for the
// process lifetime (the file is static host configuration). It is empty when
// the file cannot be read or the field is absent.
var readReleasever = sync.OnceValue(func() string {
	data, err := os.ReadFile(osReleaseFn)
	if err != nil {
		return ""
	}

	for line := range strings.SplitSeq(string(data), "\n") {
		key, val, ok := strings.Cut(line, "=")
		if !ok || strings.TrimSpace(key) != "VERSION_ID" {
			continue
		}

		return strings.Trim(strings.TrimSpace(val), `"'`)
	}

	return ""
})

// hostState is the process-global repository client and its index, loaded on
// first use and dropped by Update so freshly fetched metadata is visible.
var hostState struct {
	mu     sync.Mutex
	client *dnf.Client
	index  *dnf.Index
}

// hostIndex returns the process-global client and the index of the cached
// repository metadata. On a host without repositories the index is empty.
func hostIndex(ctx context.Context) (*dnf.Client, *dnf.Index, error) {
	hostState.mu.Lock()
	defer hostState.mu.Unlock()

	if hostState.client == nil {
		hostState.client = newHostClient()
	}

	if hostState.index == nil {
		idx, err := hostState.client.Load(ctx)
		if err != nil {
			return nil, nil, err
		}

		hostState.index = idx
	}

	return hostState.client, hostState.index, nil
}

// Update fetches fresh repository metadata for every enabled repository into
// the dnf cache and drops the in-memory index. This replaces "dnf makecache".
func Update(ctx context.Context) error {
	hostState.mu.Lock()
	defer hostState.mu.Unlock()

	if hostState.client == nil {
		hostState.client = newHostClient()
	}

	hostState.index = nil

	return hostState.client.Update(ctx)
}

// hostInstalled answers the dnf resolver from the host's RPM database: the
// SQLite rpmdb, the BerkeleyDB one on legacy hosts (RHEL 8 and earlier), and
// "rpm -qa" as the last resort. A host with no readable database has nothing
// installed as far as the resolver is concerned.
type hostInstalled struct{}

func (hostInstalled) InstalledNames(ctx context.Context) ([]string, error) {
	return queryInstalled(ctx,
		func(db *rpmdb.DB) ([]string, error) { return db.ListInstalled(ctx) },
		func(db *rpmdb.LegacyDB) ([]string, error) { return db.ListInstalled(ctx) },
		"%{NAME}\n"), nil
}

func (hostInstalled) InstalledProvides(ctx context.Context) ([]string, error) {
	return queryInstalled(ctx,
		func(db *rpmdb.DB) ([]string, error) { return db.ListInstalledProvides(ctx) },
		func(db *rpmdb.LegacyDB) ([]string, error) { return db.ListInstalledProvides(ctx) },
		"[%{PROVIDENAME}\n]"), nil
}

func queryInstalled(
	ctx context.Context,
	sqlite func(*rpmdb.DB) ([]string, error),
	legacy func(*rpmdb.LegacyDB) ([]string, error),
	queryFormat string,
) []string {
	db, err := rpmdb.Open()
	if err == nil {
		if out, err := sqlite(db); err == nil {
			return out
		}
	}

	if err != nil && !errors.Is(err, rpmdb.ErrLegacyDB) {
		return nil
	}

	logger.Debug(i18n.T("logger.dnfinstall.debug.legacy_bdb_rpmdb_falling"))

	if ldb, err := rpmdb.OpenLegacy(); err == nil {
		if out, err := legacy(ldb); err == nil {
			return out
		}
	}

	return rpmQuery(ctx, queryFormat)
}

// rpmQuery lists the lines "rpm -qa --queryformat" prints.
func rpmQuery(ctx context.Context, queryFormat string) []string {
	out, err := exec.CommandContext(ctx, "rpm", "-qa", "--queryformat", queryFormat).Output()
	if err != nil {
		return nil
	}

	var lines []string

	for line := range strings.SplitSeq(string(out), "\n") {
		if line = strings.TrimSpace(line); line != "" {
			lines = append(lines, line)
		}
	}

	return lines
}
