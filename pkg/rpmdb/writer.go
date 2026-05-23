package rpmdb

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	rpmutils "github.com/sassoftware/go-rpmutils"

	yapErrors "github.com/M0Rf30/yap/v2/pkg/errors"
	"github.com/M0Rf30/yap/v2/pkg/logger"
	rpmdbgen "github.com/M0Rf30/yap/v2/pkg/rpmdb/db"
)

// Writer extends the rpmdb with insert support for Fedora 38+/RHEL 9+/openSUSE.
// It is NOT safe for concurrent use; callers must serialize access.
type Writer struct {
	db      *sql.DB
	queries *rpmdbgen.Queries
	path    string
}

// ErrSchemaMismatch is returned when the database schema doesn't match the
// expected Fedora rpmdb schema.
var ErrSchemaMismatch = errors.New("rpmdb: schema mismatch")

// ErrPopulated is returned when attempting to write to a database that already
// contains installed packages. Only fresh databases are supported in v1.
var ErrPopulated = errors.New("rpmdb: database already populated")

// OpenWriter opens a writable handle to the RPM SQLite database at dbPath.
// It validates the schema and ensures the database is empty (no pre-existing
// packages). Returns ErrSchemaMismatch if the schema is invalid, ErrPopulated
// if packages already exist, or a wrapped error for other issues.
//
// The caller must call Close() to release the database handle.
func OpenWriter(ctx context.Context, dbPath string) (*Writer, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	// Ensure parent directory exists
	dir := filepath.Dir(dbPath)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, yapErrors.Wrap(err, yapErrors.ErrTypeFileSystem,
			"failed to create rpmdb directory").
			WithContext("path", dbPath).
			WithOperation("OpenWriter")
	}

	// Open database in read-write mode
	dsn := "file:" + dbPath + "?mode=rwc&_txlock=deferred"

	dbConn, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, yapErrors.Wrap(err, yapErrors.ErrTypeFileSystem,
			"failed to open rpmdb").
			WithContext("path", dbPath).
			WithOperation("OpenWriter")
	}

	// Ping to ensure connection works
	if err := dbConn.PingContext(ctx); err != nil {
		_ = dbConn.Close()

		return nil, yapErrors.Wrap(err, yapErrors.ErrTypeFileSystem,
			"failed to ping rpmdb").
			WithContext("path", dbPath).
			WithOperation("OpenWriter")
	}

	w := &Writer{
		db:      dbConn,
		queries: rpmdbgen.New(dbConn),
		path:    dbPath,
	}

	// Check if database is already initialized
	exists, err := w.tableExists(ctx, "Packages")
	if err != nil {
		_ = dbConn.Close()
		return nil, err
	}

	if !exists {
		// Initialize schema
		if err := w.initSchema(ctx); err != nil {
			_ = dbConn.Close()
			return nil, err
		}
	} else {
		// Validate schema and check if populated
		if err := w.validateSchema(ctx); err != nil {
			_ = dbConn.Close()
			return nil, err
		}

		if err := w.checkPopulated(ctx); err != nil {
			_ = dbConn.Close()
			return nil, err
		}
	}

	return w, nil
}

// Close closes the database handle.
func (w *Writer) Close() error {
	if w.db == nil {
		return nil
	}

	return w.db.Close()
}

// Install inserts an installed-package record from a parsed *rpmutils.Rpm and
// the list of files actually placed on disk by the extractor.
//
// The operation is atomic: either all tables are updated or none are.
// Returns a wrapped error if the operation fails.
//
//nolint:gocyclo,cyclop // install pipeline orchestration
func (w *Writer) Install(ctx context.Context, rpm *rpmutils.Rpm, files []InstalledFile) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	// Extract package name for logging
	name, _ := rpm.Header.GetString(rpmutils.NAME)

	// Serialize the header blob
	headerBlob, err := serializeHeader(rpm, files)
	if err != nil {
		return yapErrors.Wrap(err, yapErrors.ErrTypePackaging,
			"failed to serialize RPM header").
			WithContext("package", name).
			WithOperation("Install")
	}

	// Begin transaction
	tx, err := w.db.BeginTx(ctx, nil)
	if err != nil {
		return yapErrors.Wrap(err, yapErrors.ErrTypeInternal,
			"failed to begin transaction").
			WithContext("package", name).
			WithOperation("Install")
	}
	defer func() {
		_ = tx.Rollback()
	}()

	// Create queries bound to transaction
	qtx := w.queries.WithTx(tx)

	// Get next hnum (header number)
	maxHnumVal, err := qtx.GetMaxHnum(ctx)
	if err != nil {
		return yapErrors.Wrap(err, yapErrors.ErrTypeInternal,
			"failed to get max hnum").
			WithContext("package", name).
			WithOperation("Install")
	}

	var hnum int64

	if maxHnumVal != nil {
		// Convert interface{} to int64
		switch v := maxHnumVal.(type) {
		case int64:
			hnum = v + 1
		case float64:
			hnum = int64(v) + 1
		default:
			hnum = 1
		}
	} else {
		hnum = 1
	}

	// Insert into Packages table
	if err := qtx.InsertPackage(ctx, rpmdbgen.InsertPackageParams{
		Hnum: hnum,
		Blob: headerBlob,
	}); err != nil {
		return yapErrors.Wrap(err, yapErrors.ErrTypeInternal,
			"failed to insert package").
			WithContext("package", name).
			WithContext("hnum", hnum).
			WithOperation("Install")
	}

	// Insert into Name table
	if err := qtx.InsertName(ctx, rpmdbgen.InsertNameParams{
		Name: name,
		Hnum: hnum,
	}); err != nil {
		return yapErrors.Wrap(err, yapErrors.ErrTypeInternal,
			"failed to insert name").
			WithContext("package", name).
			WithContext("hnum", hnum).
			WithOperation("Install")
	}

	// Insert provides
	provides, _ := rpm.Header.GetStrings(rpmutils.PROVIDENAME)
	for idx, prov := range provides {
		if err := qtx.InsertProvidename(ctx, rpmdbgen.InsertProvidenameParams{
			Key:  prov,
			Hnum: hnum,
			Idx:  int64(idx),
		}); err != nil {
			return yapErrors.Wrap(err, yapErrors.ErrTypeInternal,
				"failed to insert provide").
				WithContext("package", name).
				WithContext("provide", prov).
				WithOperation("Install")
		}
	}

	// Insert requires
	requires, _ := rpm.Header.GetStrings(rpmutils.REQUIRENAME)
	for idx, req := range requires {
		if err := qtx.InsertRequirename(ctx, rpmdbgen.InsertRequirenameParams{
			Key:  req,
			Hnum: hnum,
			Idx:  int64(idx),
		}); err != nil {
			return yapErrors.Wrap(err, yapErrors.ErrTypeInternal,
				"failed to insert require").
				WithContext("package", name).
				WithContext("require", req).
				WithOperation("Install")
		}
	}

	// Insert conflicts
	conflicts, _ := rpm.Header.GetStrings(rpmutils.CONFLICTNAME)
	for idx, conf := range conflicts {
		if err := qtx.InsertConflictname(ctx, rpmdbgen.InsertConflictnameParams{
			Key:  conf,
			Hnum: hnum,
			Idx:  int64(idx),
		}); err != nil {
			return yapErrors.Wrap(err, yapErrors.ErrTypeInternal,
				"failed to insert conflict").
				WithContext("package", name).
				WithContext("conflict", conf).
				WithOperation("Install")
		}
	}

	// Insert obsoletes
	obsoletes, _ := rpm.Header.GetStrings(rpmutils.OBSOLETENAME)
	for idx, obs := range obsoletes {
		if err := qtx.InsertObsoletename(ctx, rpmdbgen.InsertObsoletenameParams{
			Key:  obs,
			Hnum: hnum,
			Idx:  int64(idx),
		}); err != nil {
			return yapErrors.Wrap(err, yapErrors.ErrTypeInternal,
				"failed to insert obsolete").
				WithContext("package", name).
				WithContext("obsolete", obs).
				WithOperation("Install")
		}
	}

	// Insert basenames (file list)
	for idx := range files {
		f := &files[idx]
		basename := basenameFromPath(f.Path)

		if err := qtx.InsertBasenames(ctx, rpmdbgen.InsertBasenamesParams{
			Key:  basename,
			Hnum: hnum,
			Idx:  int64(idx),
		}); err != nil {
			return yapErrors.Wrap(err, yapErrors.ErrTypeInternal,
				"failed to insert basename").
				WithContext("package", name).
				WithContext("file", f.Path).
				WithOperation("Install")
		}
	}

	// Insert dirnames (directory list)
	dirMap := make(map[string]int64)

	for i := range files {
		f := &files[i]
		dir := dirFromPath(f.Path)

		if _, ok := dirMap[dir]; !ok {
			idx := int64(len(dirMap))

			dirMap[dir] = idx
			if err := qtx.InsertDirnames(ctx, rpmdbgen.InsertDirnamesParams{
				Key:  dir,
				Hnum: hnum,
				Idx:  idx,
			}); err != nil {
				return yapErrors.Wrap(err, yapErrors.ErrTypeInternal,
					"failed to insert dirname").
					WithContext("package", name).
					WithContext("dir", dir).
					WithOperation("Install")
			}
		}
	}

	// Insert file digests
	for idx := range files {
		f := &files[idx]
		if f.SHA256 != "" {
			if err := qtx.InsertFiledigests(ctx, rpmdbgen.InsertFiledigestsParams{
				Key:  f.SHA256,
				Hnum: hnum,
				Idx:  int64(idx),
			}); err != nil {
				return yapErrors.Wrap(err, yapErrors.ErrTypeInternal,
					"failed to insert filedigest").
					WithContext("package", name).
					WithContext("file", f.Path).
					WithOperation("Install")
			}
		}
	}

	// Commit transaction
	if err := tx.Commit(); err != nil {
		return yapErrors.Wrap(err, yapErrors.ErrTypeInternal,
			"failed to commit transaction").
			WithContext("package", name).
			WithOperation("Install")
	}

	logger.Info("Installed package in rpmdb", "package", name, "hnum", hnum, "files", len(files))

	return nil
}

// tableExists checks if a table exists in the database.
func (w *Writer) tableExists(ctx context.Context, tableName string) (bool, error) {
	var count int

	err := w.db.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name=?",
		tableName).Scan(&count)
	if err != nil {
		return false, yapErrors.Wrap(err, yapErrors.ErrTypeInternal,
			"failed to check table existence").
			WithContext("table", tableName).
			WithOperation("tableExists")
	}

	return count > 0, nil
}

// initSchema initializes the database with the full rpmdb schema.
func (w *Writer) initSchema(ctx context.Context) error {
	// Inline schema to avoid file path resolution issues
	schema := `-- Full RPM 4.16+ SQLite schema (Fedora 38+, RHEL 9+, openSUSE Tumbleweed)
-- Canonical schema used by both readers and writers.

CREATE TABLE Packages (
    hnum INTEGER NOT NULL PRIMARY KEY,
    blob BLOB NOT NULL
);

CREATE TABLE Name (
    name TEXT NOT NULL,
    hnum INTEGER NOT NULL REFERENCES Packages(hnum)
);

CREATE INDEX nameindex ON Name (name ASC);

CREATE TABLE Providename (
    key TEXT NOT NULL,
    hnum INTEGER NOT NULL REFERENCES Packages(hnum),
    idx INTEGER NOT NULL
);

CREATE INDEX providenameindex ON Providename (key ASC);

CREATE TABLE Requirename (
    key TEXT NOT NULL,
    hnum INTEGER NOT NULL REFERENCES Packages(hnum),
    idx INTEGER NOT NULL
);

CREATE INDEX requirenameindex ON Requirename (key ASC);

CREATE TABLE Conflictname (
    key TEXT NOT NULL,
    hnum INTEGER NOT NULL REFERENCES Packages(hnum),
    idx INTEGER NOT NULL
);

CREATE INDEX conflictnameindex ON Conflictname (key ASC);

CREATE TABLE Obsoletename (
    key TEXT NOT NULL,
    hnum INTEGER NOT NULL REFERENCES Packages(hnum),
    idx INTEGER NOT NULL
);

CREATE INDEX obsoletenameindex ON Obsoletename (key ASC);

CREATE TABLE Basenames (
    key TEXT NOT NULL,
    hnum INTEGER NOT NULL REFERENCES Packages(hnum),
    idx INTEGER NOT NULL
);

CREATE INDEX basenamesindex ON Basenames (key ASC);

CREATE TABLE Dirnames (
    key TEXT NOT NULL,
    hnum INTEGER NOT NULL REFERENCES Packages(hnum),
    idx INTEGER NOT NULL
);

CREATE INDEX dirnamesindex ON Dirnames (key ASC);

CREATE TABLE Filedigests (
    key TEXT NOT NULL,
    hnum INTEGER NOT NULL REFERENCES Packages(hnum),
    idx INTEGER NOT NULL
);

CREATE INDEX filedigestsindex ON Filedigests (key ASC);

CREATE TABLE Triggername (
    key TEXT NOT NULL,
    hnum INTEGER NOT NULL REFERENCES Packages(hnum),
    idx INTEGER NOT NULL
);

CREATE INDEX triggernameindex ON Triggername (key ASC);

CREATE TABLE Sha1header (
    key TEXT NOT NULL,
    hnum INTEGER NOT NULL REFERENCES Packages(hnum),
    idx INTEGER NOT NULL
);

CREATE INDEX sha1headerindex ON Sha1header (key ASC);

CREATE TABLE Installtid (
    key TEXT NOT NULL,
    hnum INTEGER NOT NULL REFERENCES Packages(hnum),
    idx INTEGER NOT NULL
);

CREATE INDEX installtidindex ON Installtid (key ASC);

CREATE TABLE Sigmd5 (
    key TEXT NOT NULL,
    hnum INTEGER NOT NULL REFERENCES Packages(hnum),
    idx INTEGER NOT NULL
);

CREATE INDEX sigmd5index ON Sigmd5 (key ASC);`

	// Execute schema
	_, err := w.db.ExecContext(ctx, schema)
	if err != nil {
		return yapErrors.Wrap(err, yapErrors.ErrTypeInternal,
			"failed to initialize schema").
			WithContext("path", w.path).
			WithOperation("initSchema")
	}

	logger.Debug("Initialized rpmdb schema", "path", w.path)

	return nil
}

// validateSchema checks that the database has the expected tables.
func (w *Writer) validateSchema(ctx context.Context) error {
	requiredTables := []string{
		"Packages", "Name", "Providename", "Requirename",
		"Conflictname", "Obsoletename", "Basenames", "Dirnames",
		"Filedigests", "Triggername", "Sha1header", "Installtid", "Sigmd5",
	}

	for _, table := range requiredTables {
		exists, err := w.tableExists(ctx, table)
		if err != nil {
			return err
		}

		if !exists {
			return yapErrors.New(yapErrors.ErrTypeInternal,
				fmt.Sprintf("required table %s not found", table)).
				WithContext("path", w.path).
				WithOperation("validateSchema")
		}
	}

	return nil
}

// checkPopulated returns ErrPopulated if the Packages table has any rows.
func (w *Writer) checkPopulated(ctx context.Context) error {
	count, err := w.queries.CountPackages(ctx)
	if err != nil {
		return yapErrors.Wrap(err, yapErrors.ErrTypeInternal,
			"failed to check if database is populated").
			WithContext("path", w.path).
			WithOperation("checkPopulated")
	}

	if count > 0 {
		return ErrPopulated
	}

	return nil
}
