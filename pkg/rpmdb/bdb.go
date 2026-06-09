package rpmdb

import (
	"context"
	"errors"
	"os"

	"github.com/knqyf263/go-rpmdb/pkg/bdb"
)

// ErrNoBDB is returned by OpenLegacy when no BerkeleyDB RPM database
// exists at any known path.
var ErrNoBDB = errors.New("rpmdb: no BerkeleyDB RPM database found")

// bdbPaths lists the known locations of the BerkeleyDB Packages database
// (RHEL/Rocky/CentOS 8 and earlier).
var bdbPaths = []string{
	"/var/lib/rpm/Packages",
	"/usr/lib/sysimage/rpm/Packages",
}

// LegacyDB reads the BerkeleyDB-format RPM database natively via a
// pure-Go BDB hash reader — no `rpm` subprocess. It complements DB
// (the SQLite reader) on hosts that return ErrLegacyDB from Open.
//
// Unlike DB there is no process-wide cache: the BDB file is scanned
// sequentially per query, which is fine for the install-time call sites
// (one scan per resolution pass).
type LegacyDB struct {
	path string
}

// OpenLegacy returns a LegacyDB if a BerkeleyDB Packages database exists
// at a known path, or ErrNoBDB.
func OpenLegacy() (*LegacyDB, error) {
	for _, p := range bdbPaths {
		if fi, err := os.Stat(p); err == nil && fi.Mode().IsRegular() {
			return &LegacyDB{path: p}, nil
		}
	}

	return nil, ErrNoBDB
}

// OpenLegacyAt returns a LegacyDB reading the BerkeleyDB Packages database
// at an explicit path. Used by tests and tooling; production code goes
// through OpenLegacy's path discovery.
func OpenLegacyAt(path string) (*LegacyDB, error) {
	fi, err := os.Stat(path)
	if err != nil || !fi.Mode().IsRegular() {
		return nil, ErrNoBDB
	}

	return &LegacyDB{path: path}, nil
}

// eachHeader scans every header image in the BDB Packages database and
// calls fn for each successfully parsed header. Malformed individual
// blobs are skipped (the BDB also stores non-header bookkeeping values);
// reader-level errors abort the scan.
func (d *LegacyDB) eachHeader(ctx context.Context, fn func(headerInfo)) error {
	db, err := bdb.Open(d.path)
	if err != nil {
		return err
	}
	defer func() { _ = db.Close() }()

	for entry := range db.Read() {
		if entry.Err != nil {
			return entry.Err
		}

		if err := ctx.Err(); err != nil {
			return err
		}

		info, err := parseHeaderBlob(entry.Value)
		if err != nil {
			continue // bookkeeping blob or corrupt record — skip
		}

		fn(info)
	}

	return nil
}

// ListInstalled returns all installed package names.
func (d *LegacyDB) ListInstalled(ctx context.Context) ([]string, error) {
	var names []string

	err := d.eachHeader(ctx, func(info headerInfo) {
		names = append(names, info.Name)
	})
	if err != nil {
		return nil, err
	}

	return names, nil
}

// ListInstalledProvides returns the capabilities (Provides) of all
// installed packages, including each package's own name.
func (d *LegacyDB) ListInstalledProvides(ctx context.Context) ([]string, error) {
	var provides []string

	err := d.eachHeader(ctx, func(info headerInfo) {
		provides = append(provides, info.Name)
		provides = append(provides, info.Provides...)
	})
	if err != nil {
		return nil, err
	}

	return provides, nil
}

// FilterInstalled returns the subset of names that are NOT installed.
// One sequential DB scan regardless of len(names).
func (d *LegacyDB) FilterInstalled(ctx context.Context, names []string) ([]string, error) {
	installed := make(map[string]bool)

	err := d.eachHeader(ctx, func(info headerInfo) {
		installed[info.Name] = true
	})
	if err != nil {
		return nil, err
	}

	missing := make([]string, 0, len(names))

	for _, name := range names {
		if !installed[name] {
			missing = append(missing, name)
		}
	}

	return missing, nil
}
