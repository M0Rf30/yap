// SPDX-FileCopyrightText: 2021-2026 Gianluca Boiano
// SPDX-License-Identifier: GPL-3.0-only

package dnfinstall

import (
	"context"
	"errors"
	"os"
	"os/exec"
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
		VarsDirs:   []string{"/etc/dnf/vars", "/etc/yum/vars"},
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
