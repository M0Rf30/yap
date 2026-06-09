package yapdb

import (
	"context"
	"database/sql"
	stderrors "errors"
	"os"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite" // registers sqlite driver with database/sql

	"github.com/M0Rf30/yap/v2/pkg/errors"
	db "github.com/M0Rf30/yap/v2/pkg/yapdb/db"
)

// defaultDBPath is the canonical absolute location of the YAP state database.
const defaultDBPath = "/var/lib/yap/installed.db"

// dbRelPath is the relative path under <rootDir> for non-root installs.
const dbRelPath = "var/lib/yap/installed.db"

// ErrNotFound is returned by lookup methods when the requested record does
// not exist in the database.
var ErrNotFound = stderrors.New("yapdb: package not found")

// DB wraps the sqlc-generated queries and provides a high-level API
// for managing the YAP installed package registry.
type DB struct {
	sqlDB   *sql.DB
	queries *db.Queries
	path    string
}

// Package represents an installed package record.
type Package struct {
	Name        string
	Epoch       string
	Version     string
	Release     string
	Arch        string
	Format      string // "rpm" | "deb" | "apk" | "pacman"
	Summary     string
	InstallTime time.Time
	Files       []File
	Caps        []Capability
}

// File represents a file placed on disk by a package.
type File struct {
	Path       string
	Mode       os.FileMode
	IsDir      bool
	IsSymlink  bool
	LinkTarget string
	SHA256     string
}

// Capability represents a provides/requires/conflicts/obsoletes entry.
type Capability struct {
	Kind    string // "provide" | "require" | "obsolete" | "conflict"
	Name    string
	Flags   int
	Version string
}

// Open opens or creates the state DB at the given path.
// If path is empty, uses DefaultPath("").
// Auto-creates parent directories and initializes schema if needed.
func Open(ctx context.Context, path string) (*DB, error) {
	if path == "" {
		path = DefaultPath("")
	}

	// Ensure parent directory exists.
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, errors.Wrap(err, errors.ErrTypeFileSystem, "failed to create yapdb directory").
			WithOperation("Open").
			WithContext("path", path)
	}

	// Open or create the SQLite database.
	sqlDB, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, errors.Wrap(err, errors.ErrTypeFileSystem, "failed to open yapdb").
			WithOperation("Open").
			WithContext("path", path)
	}

	// Test the connection.
	if err := sqlDB.PingContext(ctx); err != nil {
		_ = sqlDB.Close()

		return nil, errors.Wrap(err, errors.ErrTypeFileSystem, "failed to ping yapdb").
			WithOperation("Open").
			WithContext("path", path)
	}

	d := &DB{
		sqlDB:   sqlDB,
		queries: db.New(sqlDB),
		path:    path,
	}

	// Initialize schema if this is a new database.
	if err := d.initSchema(ctx); err != nil {
		_ = d.Close()
		return nil, err
	}

	return d, nil
}

// DefaultPath returns the canonical state DB path for a given rootDir.
// If rootDir is empty or "/", uses /var/lib/yap/installed.db.
// Otherwise uses <rootDir>/var/lib/yap/installed.db.
func DefaultPath(rootDir string) string {
	if rootDir == "" || rootDir == "/" {
		return defaultDBPath
	}

	return filepath.Join(rootDir, dbRelPath)
}

// RecordInstalled opens the state DB under rootDir, inserts pkg, and
// closes the handle. This is the shared tail of every per-format
// installer (deb, rpm); metadata assembly stays format-specific, the
// open/insert/close transaction does not.
func RecordInstalled(ctx context.Context, rootDir string, pkg *Package) error {
	handle, err := Open(ctx, DefaultPath(rootDir))
	if err != nil {
		return errors.Wrap(err, errors.ErrTypeFileSystem, "failed to open yapdb").
			WithOperation("RecordInstalled").
			WithContext("package", pkg.Name)
	}
	defer func() { _ = handle.Close() }()

	if err := handle.Insert(ctx, pkg); err != nil {
		return errors.Wrap(err, errors.ErrTypeFileSystem, "failed to insert package into yapdb").
			WithOperation("RecordInstalled").
			WithContext("package", pkg.Name)
	}

	return nil
}

// initSchema initializes the database schema if it doesn't exist.
func (d *DB) initSchema(ctx context.Context) error {
	// Check if meta table exists.
	var exists bool

	err := d.sqlDB.QueryRowContext(ctx,
		"SELECT EXISTS(SELECT 1 FROM sqlite_master WHERE type='table' AND name='meta')").
		Scan(&exists)
	if err != nil {
		return errors.Wrap(err, errors.ErrTypeFileSystem, "failed to check schema").
			WithOperation("initSchema")
	}

	if exists {
		// Schema already initialized.
		return nil
	}

	// Read and execute schema.sql.
	schemaSQL := `
CREATE TABLE packages (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    name        TEXT NOT NULL,
    epoch       TEXT NOT NULL DEFAULT '',
    version     TEXT NOT NULL,
    release     TEXT NOT NULL,
    arch        TEXT NOT NULL,
    format      TEXT NOT NULL,
    install_time INTEGER NOT NULL,
    summary     TEXT NOT NULL DEFAULT ''
);
CREATE UNIQUE INDEX packages_name_arch ON packages (name, arch);

CREATE TABLE files (
    package_id  INTEGER NOT NULL REFERENCES packages(id) ON DELETE CASCADE,
    path        TEXT NOT NULL,
    mode        INTEGER NOT NULL,
    is_dir      INTEGER NOT NULL DEFAULT 0,
    is_symlink  INTEGER NOT NULL DEFAULT 0,
    link_target TEXT NOT NULL DEFAULT '',
    sha256      TEXT NOT NULL DEFAULT ''
);
CREATE INDEX files_package ON files (package_id);
CREATE INDEX files_path ON files (path);

CREATE TABLE caps (
    package_id  INTEGER NOT NULL REFERENCES packages(id) ON DELETE CASCADE,
    kind        TEXT NOT NULL,
    name        TEXT NOT NULL,
    flags       INTEGER NOT NULL DEFAULT 0,
    version     TEXT NOT NULL DEFAULT ''
);
CREATE INDEX caps_name ON caps (name);
CREATE INDEX caps_package ON caps (package_id);

CREATE TABLE meta (
    key   TEXT PRIMARY KEY,
    value TEXT NOT NULL
);
INSERT INTO meta (key, value) VALUES ('schema_version', '1');
`

	if _, err := d.sqlDB.ExecContext(ctx, schemaSQL); err != nil {
		return errors.Wrap(err, errors.ErrTypeFileSystem, "failed to initialize schema").
			WithOperation("initSchema")
	}

	return nil
}

// Insert adds or replaces an installed package record.
// If a package with the same name+arch already exists, it is replaced atomically
// (old files and caps are deleted via CASCADE).
func (d *DB) Insert(ctx context.Context, p *Package) error {
	tx, err := d.sqlDB.BeginTx(ctx, nil)
	if err != nil {
		return errors.Wrap(err, errors.ErrTypeFileSystem, "failed to begin transaction").
			WithOperation("Insert")
	}
	defer func() { _ = tx.Rollback() }()

	queries := db.New(tx)

	// Delete any existing package with the same name+arch.
	if err := queries.DeletePackageByNameArch(ctx, db.DeletePackageByNameArchParams{
		Name: p.Name,
		Arch: p.Arch,
	}); err != nil {
		return errors.Wrap(err, errors.ErrTypeFileSystem, "failed to delete existing package").
			WithOperation("Insert").
			WithContext("package", p.Name)
	}

	// Insert the new package.
	pkgID, err := queries.InsertPackage(ctx, db.InsertPackageParams{
		Name:        p.Name,
		Epoch:       p.Epoch,
		Version:     p.Version,
		Release:     p.Release,
		Arch:        p.Arch,
		Format:      p.Format,
		InstallTime: p.InstallTime.Unix(),
		Summary:     p.Summary,
	})
	if err != nil {
		return errors.Wrap(err, errors.ErrTypeFileSystem, "failed to insert package").
			WithOperation("Insert").
			WithContext("package", p.Name)
	}

	// Insert files.
	for _, f := range p.Files {
		isDir := 0
		if f.IsDir {
			isDir = 1
		}

		isSymlink := 0
		if f.IsSymlink {
			isSymlink = 1
		}

		if err := queries.InsertFile(ctx, db.InsertFileParams{
			PackageID:  pkgID,
			Path:       f.Path,
			Mode:       int64(f.Mode),
			IsDir:      int64(isDir),
			IsSymlink:  int64(isSymlink),
			LinkTarget: f.LinkTarget,
			Sha256:     f.SHA256,
		}); err != nil {
			return errors.Wrap(err, errors.ErrTypeFileSystem, "failed to insert file").
				WithOperation("Insert").
				WithContext("package", p.Name).
				WithContext("path", f.Path)
		}
	}

	// Insert capabilities.
	for _, c := range p.Caps {
		if err := queries.InsertCap(ctx, db.InsertCapParams{
			PackageID: pkgID,
			Kind:      c.Kind,
			Name:      c.Name,
			Flags:     int64(c.Flags),
			Version:   c.Version,
		}); err != nil {
			return errors.Wrap(err, errors.ErrTypeFileSystem, "failed to insert capability").
				WithOperation("Insert").
				WithContext("package", p.Name).
				WithContext("capability", c.Name)
		}
	}

	if err := tx.Commit(); err != nil {
		return errors.Wrap(err, errors.ErrTypeFileSystem, "failed to commit transaction").
			WithOperation("Insert")
	}

	return nil
}

// Remove deletes a package record by name and arch (cascades to files and caps).
func (d *DB) Remove(ctx context.Context, name, arch string) error {
	if err := d.queries.DeletePackageByNameArch(ctx, db.DeletePackageByNameArchParams{
		Name: name,
		Arch: arch,
	}); err != nil {
		return errors.Wrap(err, errors.ErrTypeFileSystem, "failed to remove package").
			WithOperation("Remove").
			WithContext("package", name)
	}

	return nil
}

// IsInstalled reports whether a package is recorded by name.
func (d *DB) IsInstalled(ctx context.Context, name string) (bool, error) {
	exists, err := d.queries.IsInstalledByName(ctx, name)
	if err != nil {
		return false, errors.Wrap(err, errors.ErrTypeFileSystem, "failed to check installation").
			WithOperation("IsInstalled").
			WithContext("package", name)
	}

	return exists, nil
}

// ProvidersOf returns package names that provide the given capability.
func (d *DB) ProvidersOf(ctx context.Context, capName string) ([]string, error) {
	rows, err := d.queries.LookupProviders(ctx, capName)
	if err != nil {
		return nil, errors.Wrap(err, errors.ErrTypeFileSystem, "failed to lookup providers").
			WithOperation("ProvidersOf").
			WithContext("capability", capName)
	}

	providers := make([]string, 0, len(rows))
	for i := range rows {
		providers = append(providers, rows[i].Name)
	}

	return providers, nil
}

// List returns all installed packages (without files/caps to keep it cheap).
func (d *DB) List(ctx context.Context) ([]Package, error) {
	rows, err := d.queries.ListPackages(ctx)
	if err != nil {
		return nil, errors.Wrap(err, errors.ErrTypeFileSystem, "failed to list packages").
			WithOperation("List")
	}

	packages := make([]Package, 0, len(rows))
	for i := range rows {
		row := &rows[i]
		packages = append(packages, Package{
			Name:        row.Name,
			Epoch:       row.Epoch,
			Version:     row.Version,
			Release:     row.Release,
			Arch:        row.Arch,
			Format:      row.Format,
			Summary:     row.Summary,
			InstallTime: time.Unix(row.InstallTime, 0),
		})
	}

	return packages, nil
}

// LookupByName returns the package record with files and caps.
// Returns (nil, ErrNotFound) if no package matches name+arch.
func (d *DB) LookupByName(ctx context.Context, name, arch string) (*Package, error) {
	row, err := d.queries.LookupPackageByNameArch(ctx, db.LookupPackageByNameArchParams{
		Name: name,
		Arch: arch,
	})
	if stderrors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}

	if err != nil {
		return nil, errors.Wrap(err, errors.ErrTypeFileSystem, "failed to lookup package").
			WithOperation("LookupByName").
			WithContext("package", name)
	}

	pkg := &Package{
		Name:        row.Name,
		Epoch:       row.Epoch,
		Version:     row.Version,
		Release:     row.Release,
		Arch:        row.Arch,
		Format:      row.Format,
		Summary:     row.Summary,
		InstallTime: time.Unix(row.InstallTime, 0),
	}

	// Load files.
	fileRows, err := d.queries.FilesByPackage(ctx, row.ID)
	if err != nil {
		return nil, errors.Wrap(err, errors.ErrTypeFileSystem, "failed to load files").
			WithOperation("LookupByName").
			WithContext("package", name)
	}

	for _, f := range fileRows {
		pkg.Files = append(pkg.Files, File{
			Path:       f.Path,
			Mode:       os.FileMode(f.Mode), //nolint:gosec
			IsDir:      f.IsDir != 0,
			IsSymlink:  f.IsSymlink != 0,
			LinkTarget: f.LinkTarget,
			SHA256:     f.Sha256,
		})
	}

	// Load capabilities.
	capRows, err := d.queries.CapsByPackage(ctx, row.ID)
	if err != nil {
		return nil, errors.Wrap(err, errors.ErrTypeFileSystem, "failed to load capabilities").
			WithOperation("LookupByName").
			WithContext("package", name)
	}

	for _, c := range capRows {
		pkg.Caps = append(pkg.Caps, Capability{
			Kind:    c.Kind,
			Name:    c.Name,
			Flags:   int(c.Flags),
			Version: c.Version,
		})
	}

	return pkg, nil
}

// Close releases the DB handle.
func (d *DB) Close() error {
	if d.sqlDB != nil {
		if err := d.sqlDB.Close(); err != nil {
			return errors.Wrap(err, errors.ErrTypeFileSystem, "failed to close yapdb").
				WithOperation("Close")
		}
	}

	return nil
}
