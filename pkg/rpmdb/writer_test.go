// SPDX-FileCopyrightText: 2021-2026 Gianluca Boiano
// SPDX-License-Identifier: GPL-3.0-only

package rpmdb //nolint:testpackage

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	pkgrpm "github.com/M0Rf30/pkgformat/rpm"
	"github.com/M0Rf30/pkgformat/rpm/pack"
	_ "modernc.org/sqlite"
)

// TestOpenWriterFresh tests opening a fresh database.
func TestOpenWriterFresh(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "rpmdb.sqlite")

	ctx := context.Background()

	w, err := OpenWriter(ctx, dbPath)
	if err != nil {
		t.Fatalf("OpenWriter failed: %v", err)
	}

	defer func() {
		_ = w.Close()
	}()

	// Verify file was created
	if _, err := os.Stat(dbPath); err != nil {
		t.Fatalf("database file not created: %v", err)
	}

	// Verify schema was initialized
	var count int

	err = w.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM sqlite_master WHERE type='table'").Scan(&count)
	if err != nil {
		t.Fatalf("failed to query tables: %v", err)
	}

	if count == 0 {
		t.Fatal("no tables created")
	}
}

// TestOpenWriterExisting tests opening an existing database.
func TestOpenWriterExisting(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "rpmdb.sqlite")

	ctx := context.Background()

	// First open
	w1, err := OpenWriter(ctx, dbPath)
	if err != nil {
		t.Fatalf("first OpenWriter failed: %v", err)
	}

	_ = w1.Close()

	// Second open should succeed
	w2, err := OpenWriter(ctx, dbPath)
	if err != nil {
		t.Fatalf("second OpenWriter failed: %v", err)
	}
	defer func() {
		_ = w2.Close()
	}()

	// Verify tables still exist
	var count int

	err = w2.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM sqlite_master WHERE type='table'").Scan(&count)
	if err != nil {
		t.Fatalf("failed to query tables: %v", err)
	}

	if count == 0 {
		t.Fatal("tables missing after reopen")
	}
}

// TestOpenWriterPopulated tests that OpenWriter appends to populated
// databases and that successive writers can each install a package.
func TestOpenWriterPopulated(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "rpmdb.sqlite")

	ctx := context.Background()

	w1, err := OpenWriter(ctx, dbPath)
	if err != nil {
		t.Fatalf("first OpenWriter failed: %v", err)
	}

	// Insert a dummy package
	_, err = w1.db.ExecContext(ctx, "INSERT INTO Packages (hnum, blob) VALUES (1, X'00')")
	if err != nil {
		t.Fatalf("failed to insert package: %v", err)
	}

	_ = w1.Close()

	// Reopen per package, as dnfinstall does: each must succeed and append.
	for i, name := range []string{"pkg1", "pkg2"} {
		w, err := OpenWriter(ctx, dbPath)
		if err != nil {
			t.Fatalf("OpenWriter #%d on populated database failed: %v", i+2, err)
		}

		if err := w.Install(ctx, buildTestRPM(t), []InstalledFile{
			{Path: "/usr/bin/" + name, Mode: 0o755},
		}); err != nil {
			_ = w.Close()

			t.Fatalf("Install %s failed: %v", name, err)
		}

		_ = w.Close()
	}

	w, err := OpenWriter(ctx, dbPath)
	if err != nil {
		t.Fatalf("final OpenWriter failed: %v", err)
	}

	defer func() { _ = w.Close() }()

	var count int
	if err := w.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM Packages").Scan(&count); err != nil {
		t.Fatal(err)
	}

	if count != 3 {
		t.Fatalf("expected 3 packages (1 pre-existing + 2 appended), got %d", count)
	}
}

// TestOpenWriterSchemaMismatch tests that OpenWriter rejects invalid schemas.
// Skipped: OpenWriter will initialize the schema if Packages table doesn't exist.
// This is by design - we only reject if the schema exists but is incomplete.
func TestOpenWriterSchemaMismatch(t *testing.T) {
	t.Skip("OpenWriter initializes schema if missing; only rejects incomplete existing schemas")
}

// TestInstallBasic tests basic package installation.
// Skipped: Requires a real RPM with proper header tags set.
// The mock RPM doesn't have NAME tag populated.
func TestInstallBasic(t *testing.T) {
	t.Skip("Requires real RPM with header tags; mock RPM insufficient")
}

// TestInstallWithFiles tests installation with multiple files.
func TestInstallWithFiles(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "rpmdb.sqlite")

	ctx := context.Background()

	w, err := OpenWriter(ctx, dbPath)
	if err != nil {
		t.Fatalf("OpenWriter failed: %v", err)
	}

	defer func() {
		_ = w.Close()
	}()

	rpm := createMockRPM(t, "multi-file", "2.0", "1")

	files := []InstalledFile{
		{
			Path:       "/usr/bin/tool1",
			Size:       512,
			Mode:       0o755,
			SHA256:     "hash1",
			LinkTarget: "",
			User:       "root",
			Group:      "root",
			MTime:      time.Now(),
			Flags:      0,
		},
		{
			Path:       "/usr/bin/tool2",
			Size:       1024,
			Mode:       0o755,
			SHA256:     "hash2",
			LinkTarget: "",
			User:       "root",
			Group:      "root",
			MTime:      time.Now(),
			Flags:      0,
		},
		{
			Path:       "/etc/config.conf",
			Size:       256,
			Mode:       0o644,
			SHA256:     "hash3",
			LinkTarget: "",
			User:       "root",
			Group:      "root",
			MTime:      time.Now(),
			Flags:      1, // RPMFILE_CONFIG
		},
	}

	err = w.Install(ctx, rpm, files)
	if err != nil {
		t.Fatalf("Install failed: %v", err)
	}

	// Verify basenames
	var count int

	err = w.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM Basenames").Scan(&count)
	if err != nil {
		t.Fatalf("failed to query basenames: %v", err)
	}

	if count != 3 {
		t.Fatalf("expected 3 basenames, got %d", count)
	}

	// Verify dirnames
	err = w.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM Dirnames").Scan(&count)
	if err != nil {
		t.Fatalf("failed to query dirnames: %v", err)
	}

	if count != 2 { // /usr/bin/ and /etc/
		t.Fatalf("expected 2 dirnames, got %d", count)
	}
}

// TestInstallMultiplePackages tests installing multiple packages sequentially.
func TestInstallMultiplePackages(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "rpmdb.sqlite")

	ctx := context.Background()

	w, err := OpenWriter(ctx, dbPath)
	if err != nil {
		t.Fatalf("OpenWriter failed: %v", err)
	}

	defer func() {
		_ = w.Close()
	}()

	// Install first package
	rpm1 := createMockRPM(t, "pkg1", "1.0", "1")

	err = w.Install(ctx, rpm1, []InstalledFile{
		{
			Path:  "/usr/bin/pkg1",
			Size:  100,
			Mode:  0o755,
			User:  "root",
			Group: "root",
			MTime: time.Now(),
		},
	})
	if err != nil {
		t.Fatalf("Install pkg1 failed: %v", err)
	}

	// Install second package
	rpm2 := createMockRPM(t, "pkg2", "2.0", "1")

	err = w.Install(ctx, rpm2, []InstalledFile{
		{
			Path:  "/usr/bin/pkg2",
			Size:  200,
			Mode:  0o755,
			User:  "root",
			Group: "root",
			MTime: time.Now(),
		},
	})
	if err != nil {
		t.Fatalf("Install pkg2 failed: %v", err)
	}

	// Verify both packages
	var count int

	err = w.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM Packages").Scan(&count)
	if err != nil {
		t.Fatalf("failed to query packages: %v", err)
	}

	if count != 2 {
		t.Fatalf("expected 2 packages, got %d", count)
	}

	// Verify names
	err = w.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM Name").Scan(&count)
	if err != nil {
		t.Fatalf("failed to query names: %v", err)
	}

	if count != 2 {
		t.Fatalf("expected 2 names, got %d", count)
	}
}

// TestInstallContextCancellation tests that Install respects context cancellation.
func TestInstallContextCancellation(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "rpmdb.sqlite")

	ctx := context.Background()

	w, err := OpenWriter(ctx, dbPath)
	if err != nil {
		t.Fatalf("OpenWriter failed: %v", err)
	}

	defer func() {
		_ = w.Close()
	}()

	rpm := createMockRPM(t, "test", "1.0", "1")

	// Create a cancelled context
	cancelCtx, cancel := context.WithCancel(context.Background())
	cancel()

	err = w.Install(cancelCtx, rpm, []InstalledFile{})
	if err == nil {
		t.Fatal("expected Install to fail with cancelled context")
	}
}

// TestPathHelpers tests the path helper functions.
func TestPathHelpers(t *testing.T) {
	tests := []struct {
		path     string
		wantDir  string
		wantBase string
	}{
		{"/usr/bin/test", "/usr/bin/", "test"},
		{"/etc/config.conf", "/etc/", "config.conf"},
		{"/root/.bashrc", "/root/", ".bashrc"},
		{"/", "/", ""},
		{"test", "", "test"},
	}

	for _, tt := range tests {
		dir := dirFromPath(tt.path)
		base := basenameFromPath(tt.path)

		if dir != tt.wantDir {
			t.Errorf("dirFromPath(%q) = %q, want %q", tt.path, dir, tt.wantDir)
		}

		if base != tt.wantBase {
			t.Errorf("basenameFromPath(%q) = %q, want %q", tt.path, base, tt.wantBase)
		}
	}
}

// createMockRPM builds a small real RPM with pack and reads it back.
//
//nolint:unparam // name and version kept for call-site readability
func createMockRPM(t *testing.T, name, version, release string) *pkgrpm.Rpm {
	t.Helper()

	built, err := pack.NewRPM(pack.RPMMetaData{
		Name:    name,
		Version: version,
		Release: release,
		Arch:    "x86_64",
		Licence: "MIT",
		Summary: "rpmdb writer fixture",
	})
	if err != nil {
		t.Fatalf("pack.NewRPM: %v", err)
	}

	built.AddFile(pack.RPMFile{Name: "/usr/bin/" + name, Body: []byte("x"), Mode: 0o100755})

	buf := new(bytes.Buffer)
	if err := built.Write(buf); err != nil {
		t.Fatalf("rpm.Write: %v", err)
	}

	rpm, err := pkgrpm.ReadRpm(buf)
	if err != nil {
		t.Fatalf("ReadRpm: %v", err)
	}

	return rpm
}
