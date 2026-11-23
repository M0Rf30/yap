// SPDX-FileCopyrightText: 2021-2026 Gianluca Boiano
// SPDX-License-Identifier: GPL-3.0-only

package dnfinstall

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

	pkgrpm "github.com/M0Rf30/pkgformat/rpm"
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
	dnfConfFile = "/etc/dnf/dnf.conf"
	osReleaseFn = "/etc/os-release"
)

// newHostClient returns a dnf client reading the host's repository files and
// metadata cache, with the host's rpmdb telling it what is installed.
func newHostClient() *dnf.Client {
	opts := dnf.Options{
		RepoDir:    yumRepoDir,
		CacheDir:   dnfCacheDir,
		VarsDirs:   hostVarsDirs,
		Releasever: readReleasever(),
		Installed:  hostInstalled{},
		Logger:     logger.Slog(),
	}

	// The host's dnf.conf (excludes, gpgcheck and repository defaults) applies
	// when there is one; a minimal image has none and gets dnf's defaults.
	if _, err := os.Stat(dnfConfFile); err == nil {
		opts.ConfigFile = dnfConfFile
	}

	return dnf.New(opts)
}

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
		// --skip-sync keeps an existing cache as is, but a repository that was
		// never fetched (a fresh image) has nothing to resolve against: every
		// requirement would come back unresolvable.
		if ids := uncachedRepos(hostState.client); len(ids) > 0 {
			logger.Info(i18n.T("logger.dnfinstall.info.fetching_uncached_repos"),
				"repos", strings.Join(ids, ", "))

			if err := hostState.client.Update(ctx); err != nil {
				return nil, nil, err
			}
		}

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

	if err := hostState.client.Update(ctx); err != nil {
		return err
	}

	warnUnavailableRepos(hostState.client)

	return nil
}

// warnUnavailableRepos reports the enabled repositories that Update could not
// refresh. The dnf client logs each failing mirror and skips a repository whose
// mirrors all answer with an HTTP 4xx; that leaves the repository out of the
// index, so a package only it carries cannot be resolved. A repository whose
// metadata is missing or past its expiry after a refresh is such a repository.
func warnUnavailableRepos(client *dnf.Client) {
	if ids := unavailableRepos(client); len(ids) > 0 {
		logger.Warn(i18n.T("logger.dnfinstall.warn.repositories_unavailable"),
			"count", len(ids), "repos", strings.Join(ids, ", "))
	}
}

// unavailableRepos returns the ids of the enabled repositories whose cached
// metadata is missing or expired.
func unavailableRepos(client *dnf.Client) []string {
	repos, err := client.Repos()
	if err != nil {
		return nil
	}

	var ids []string

	for i := range repos {
		repo := &repos[i]
		if repo.Enabled && (len(repo.BaseURLs) > 0 || repo.MirrorList != "") && client.Expired(repo) {
			ids = append(ids, repo.ID)
		}
	}

	return ids
}

// uncachedRepos returns the ids of the enabled remote repositories that have
// no repomd.xml in the cache at all, under <cache>/<id> or dnf's
// <cache>/<id>-<hash>. Expired metadata does not count: it still resolves.
func uncachedRepos(client *dnf.Client) []string {
	repos, err := client.Repos()
	if err != nil {
		return nil
	}

	var ids []string

	for i := range repos {
		repo := &repos[i]
		if !repo.Enabled || (len(repo.BaseURLs) == 0 && repo.MirrorList == "") {
			continue
		}

		dirs, _ := filepath.Glob(filepath.Join(dnfCacheDir, repo.ID+"-*"))
		dirs = append(dirs, filepath.Join(dnfCacheDir, repo.ID))

		cached := false

		for _, d := range dirs {
			if _, err := os.Stat(filepath.Join(d, "repodata", "repomd.xml")); err == nil {
				cached = true

				break
			}
		}

		if !cached {
			ids = append(ids, repo.ID)
		}
	}

	return ids
}

// hostInstalled answers the dnf resolver from the host's RPM database, in
// whichever format the host keeps it (SQLite, ndb, BerkeleyDB), and from
// "rpm -qa" when the host has none. A host with no readable database has
// nothing installed as far as the resolver is concerned.
type hostInstalled struct{}

// InstalledPackages implements dnf.Installed.
func (hostInstalled) InstalledPackages(ctx context.Context) ([]*pkgrpm.Package, error) {
	pkgs, err := rpmdb.InstalledPackages(ctx)
	if err == nil {
		return pkgs, nil
	}

	if !errors.Is(err, rpmdb.ErrNoDatabase) {
		return nil, nil //nolint:nilerr // an unreadable database reads as nothing installed
	}

	logger.Debug(i18n.T("logger.dnfinstall.debug.no_rpmdb_falling"))

	return rpmQuery(ctx), nil
}

// rpmQueryFormat prints one line per package, "name epoch version release
// arch" tab-separated, followed by one tab-indented "name version" line per
// capability it provides.
const rpmQueryFormat = "%{NAME}\t%{EPOCHNUM}\t%{VERSION}\t%{RELEASE}\t%{ARCH}\n" +
	"[\t%{PROVIDENAME}\t%{PROVIDEVERSION}\n]"

// rpmQuery lists the installed packages "rpm -qa" reports.
func rpmQuery(ctx context.Context) []*pkgrpm.Package {
	out, err := exec.CommandContext(ctx, "rpm", "-qa", "--queryformat", rpmQueryFormat).Output()
	if err != nil {
		return nil
	}

	return parseRPMQuery(string(out))
}

// parseRPMQuery reads the output of rpmQueryFormat.
func parseRPMQuery(out string) []*pkgrpm.Package {
	var pkgs []*pkgrpm.Package

	for line := range strings.SplitSeq(out, "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}

		fields := strings.Split(line, "\t")

		if fields[0] != "" {
			pkgs = append(pkgs, newQueriedPackage(fields))

			continue
		}

		if len(pkgs) == 0 || len(fields) < 2 {
			continue
		}

		pkg := pkgs[len(pkgs)-1]
		pkg.Provides = append(pkg.Provides, queriedProvide(fields[1:]))
	}

	return pkgs
}

// newQueriedPackage builds a package from the name, epoch, version, release and
// arch fields of a query line. rpm lists the package's own name among its
// provides, so none is added here.
func newQueriedPackage(fields []string) *pkgrpm.Package {
	pkg := &pkgrpm.Package{Name: fields[0]}

	if len(fields) >= 5 {
		epoch, _ := strconv.ParseUint(fields[1], 10, 32)
		pkg.Epoch = uint32(epoch)
		pkg.Version, pkg.Release, pkg.Arch = fields[2], fields[3], fields[4]
	}

	return pkg
}

// queriedProvide builds a provide from its name and optional "[epoch:]version-
// release" fields; a provide with a version is an equality.
func queriedProvide(fields []string) pkgrpm.Dependency {
	dep := pkgrpm.Dependency{Name: fields[0]}

	if len(fields) < 2 || fields[1] == "" {
		return dep
	}

	dep.Flags = pkgrpm.SenseEqual
	dep.Version = fields[1]

	if epoch, version, ok := strings.Cut(fields[1], ":"); ok {
		if n, err := strconv.ParseUint(epoch, 10, 32); err == nil {
			dep.Epoch = uint32(n)
			dep.Version = version
		}
	}

	return dep
}
