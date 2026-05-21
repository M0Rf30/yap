// Package rpmdb reads the RPM SQLite database used by Fedora 33+, RHEL
// 9+, Rocky 9+, AlmaLinux 9+, and openSUSE 15.5+.
//
// Open caches one read-only handle for the lifetime of the process and
// answers installed-package queries via indexed lookups. Legacy
// BerkeleyDB-based hosts (RHEL 8 and earlier) return ErrLegacyDB; the
// caller is expected to fall back to `rpm -q`.
package rpmdb

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"sync"

	_ "modernc.org/sqlite" // CGO-free SQLite driver

	rpmdbgen "github.com/M0Rf30/yap/v2/pkg/rpmdb/db"
)

// ErrLegacyDB is returned when no SQLite database exists at any known path.
// The caller should fall back to subprocess (rpm -q) for legacy BDB hosts.
var ErrLegacyDB = errors.New("rpmdb: no SQLite RPM database found (legacy BDB host?)")

var rpmDBPaths = []string{
	"/var/lib/rpm/rpmdb.sqlite",
	"/usr/lib/sysimage/rpm/rpmdb.sqlite",
}

// findDBPath returns the first existing SQLite database path, or "" if none.
func findDBPath() string {
	for _, p := range rpmDBPaths {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}

	return ""
}

// DB wraps a read-only handle to the RPM SQLite database.
// Safe for concurrent use.
type DB struct {
	sqldb   *sql.DB
	queries *rpmdbgen.Queries
}

var (
	globalMu sync.Mutex
	globalDB *DB
)

// Open returns a process-wide DB handle. The DB is opened in read-only
// mode and cached. Returns ErrLegacyDB when no SQLite database exists at
// any known path.
//
// Unlike sync.Once-based singletons, this implementation re-attempts the
// open on every call when no cached handle is currently valid. A previous
// "no DB found" result therefore won't poison the process after the host
// runs `rpm --rebuilddb` and the SQLite file finally appears.
func Open() (*DB, error) {
	globalMu.Lock()
	defer globalMu.Unlock()

	if globalDB != nil {
		return globalDB, nil
	}

	path := findDBPath()
	if path == "" {
		return nil, ErrLegacyDB
	}

	// mode=ro: read-only. immutable=1 is unsafe (rpm writes during installs).
	// _txlock=deferred minimises locking with concurrent rpm processes.
	dsn := "file:" + path + "?mode=ro&_txlock=deferred"

	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}

	if err := db.PingContext(context.Background()); err != nil {
		_ = db.Close()
		return nil, err
	}

	globalDB = &DB{
		sqldb:   db,
		queries: rpmdbgen.New(db),
	}

	return globalDB, nil
}

// Close drops the cached DB handle. Tests use this to force a reopen.
// Production code rarely needs to call Close — the OS will release the FD
// on exit — but a long-running daemon that detects an rpm DB rotation can
// invoke Close to force the next Open to re-stat the disk.
func Close() error {
	globalMu.Lock()
	defer globalMu.Unlock()

	if globalDB == nil {
		return nil
	}

	err := globalDB.sqldb.Close()
	globalDB = nil

	return err
}

// IsInstalled reports whether the named package is registered in the RPM DB.
func (d *DB) IsInstalled(ctx context.Context, name string) (bool, error) {
	n, err := d.queries.CountByName(ctx, name)
	if err != nil {
		return false, err
	}

	return n > 0, nil
}

// FilterInstalled returns the subset of names that are NOT installed.
// Single DB-open, N indexed queries, much faster than spawning N subprocesses.
func (d *DB) FilterInstalled(ctx context.Context, names []string) []string {
	missing := make([]string, 0, len(names))
	for _, name := range names {
		installed, err := d.IsInstalled(ctx, name)
		if err != nil || !installed {
			missing = append(missing, name)
		}
	}

	return missing
}

// ListInstalled returns all installed package names. Useful for bulk diffs.
func (d *DB) ListInstalled(ctx context.Context) ([]string, error) {
	return d.queries.ListInstalledNames(ctx)
}
