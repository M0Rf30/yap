// Package rpmdb provides a pure-Go reader for the RPM SQLite database used
// by Fedora 33+, RHEL 9+, Rocky 9+, AlmaLinux 9+, and openSUSE 15.5+.
//
// It replaces "rpm -q <pkg>" subprocess calls for installed-package checks.
// Legacy BerkeleyDB-based systems (RHEL 8 and earlier) are unsupported and
// callers must fall back to subprocess on those hosts; ErrLegacyDB indicates
// the legacy format was detected.
package rpmdb

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"sync"

	_ "modernc.org/sqlite" // pure-Go SQLite driver

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
	globalOnce sync.Once
	globalDB   *DB
	globalErr  error
)

// Open returns a process-wide singleton DB handle. The DB is opened in
// read-only mode and reused across calls. Returns ErrLegacyDB when the
// host has no SQLite RPM database.
func Open() (*DB, error) {
	globalOnce.Do(func() {
		path := findDBPath()
		if path == "" {
			globalErr = ErrLegacyDB
			return
		}
		// mode=ro: read-only. immutable=1 is unsafe (rpm writes during installs).
		// _txlock=deferred minimises locking with concurrent rpm processes.
		dsn := "file:" + path + "?mode=ro&_txlock=deferred"

		db, err := sql.Open("sqlite", dsn)
		if err != nil {
			globalErr = err
			return
		}

		if err := db.PingContext(context.Background()); err != nil {
			_ = db.Close()
			globalErr = err

			return
		}

		globalDB = &DB{
			sqldb:   db,
			queries: rpmdbgen.New(db),
		}
	})

	return globalDB, globalErr
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
