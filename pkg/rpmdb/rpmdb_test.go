package rpmdb //nolint:testpackage

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"

	_ "modernc.org/sqlite"
)

// setupTestDB creates an ephemeral SQLite database with the RPM schema and
// some test packages, then overrides findDBPath via the rpmDBPaths variable.
func setupTestDB(t *testing.T) (dbPath string, cleanup func()) { //nolint:gocritic,unparam
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "rpmdb.sqlite")

	db, err := sql.Open("sqlite", "file:"+path)
	if err != nil {
		t.Fatal(err)
	}

	schema, err := os.ReadFile("schema.sql")
	if err != nil {
		t.Fatal(err)
	}

	if _, err := db.ExecContext(context.Background(), string(schema)); err != nil {
		t.Fatal(err)
	}
	// Seed.
	_, _ = db.ExecContext(context.Background(), "INSERT INTO Packages (hnum, blob) VALUES (1, X'00')")
	_, _ = db.ExecContext(context.Background(), "INSERT INTO Packages (hnum, blob) VALUES (2, X'00')")
	_, _ = db.ExecContext(context.Background(), "INSERT INTO Name (name, hnum) VALUES ('gcc', 1)")
	_, _ = db.ExecContext(context.Background(), "INSERT INTO Name (name, hnum) VALUES ('make', 2)")
	_ = db.Close()

	// Override the singleton search path.
	origPaths := rpmDBPaths
	rpmDBPaths = []string{path}
	// Reset the singleton state so Open re-runs.
	globalOnce = sync.Once{}
	globalDB = nil
	globalErr = nil

	return path, func() {
		rpmDBPaths = origPaths
		globalOnce = sync.Once{}
		globalDB = nil
		globalErr = nil
	}
}

func TestIsInstalled(t *testing.T) {
	_, cleanup := setupTestDB(t)
	defer cleanup()

	db, err := Open()
	if err != nil {
		t.Fatal(err)
	}

	ok, err := db.IsInstalled(context.Background(), "gcc")
	if err != nil {
		t.Fatal(err)
	}

	if !ok {
		t.Fatal("expected gcc to be installed")
	}

	ok, err = db.IsInstalled(context.Background(), "nonexistent")
	if err != nil {
		t.Fatal(err)
	}

	if ok {
		t.Fatal("expected nonexistent to NOT be installed")
	}
}

func TestFilterInstalled(t *testing.T) {
	_, cleanup := setupTestDB(t)
	defer cleanup()

	db, err := Open()
	if err != nil {
		t.Fatal(err)
	}

	missing := db.FilterInstalled(context.Background(), []string{"gcc", "missing-pkg", "make"})
	if len(missing) != 1 || missing[0] != "missing-pkg" {
		t.Fatalf("expected [missing-pkg], got %v", missing)
	}
}

func TestListInstalled(t *testing.T) {
	_, cleanup := setupTestDB(t)
	defer cleanup()

	db, err := Open()
	if err != nil {
		t.Fatal(err)
	}

	names, err := db.ListInstalled(context.Background())
	if err != nil {
		t.Fatal(err)
	}

	if len(names) != 2 || names[0] != "gcc" || names[1] != "make" {
		t.Fatalf("expected [gcc, make], got %v", names)
	}
}

func TestLegacyDB(t *testing.T) {
	origPaths := rpmDBPaths
	rpmDBPaths = []string{"/nonexistent/path"}
	globalOnce = sync.Once{}
	globalDB = nil
	globalErr = nil

	defer func() {
		rpmDBPaths = origPaths
		globalOnce = sync.Once{}
		globalDB = nil
		globalErr = nil
	}()

	_, err := Open()
	if !errors.Is(err, ErrLegacyDB) {
		t.Fatalf("expected ErrLegacyDB, got %v", err)
	}
}
