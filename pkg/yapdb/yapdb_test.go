package yapdb //nolint:testpackage // tests access unexported db.path field

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"
)

func TestOpenAndClose(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	db, err := Open(context.Background(), dbPath)
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer func() { _ = db.Close() }()

	if db.path != dbPath {
		t.Errorf("expected path %q, got %q", dbPath, db.path)
	}
}

func TestDefaultPath(t *testing.T) {
	tests := []struct {
		rootDir  string
		expected string
	}{
		{"", "/var/lib/yap/installed.db"},
		{"/", "/var/lib/yap/installed.db"},
		{"/root", "/root/var/lib/yap/installed.db"},
		{"/tmp/fakeroot", "/tmp/fakeroot/var/lib/yap/installed.db"},
	}

	for _, tt := range tests {
		got := DefaultPath(tt.rootDir)
		if got != tt.expected {
			t.Errorf("DefaultPath(%q) = %q, want %q", tt.rootDir, got, tt.expected)
		}
	}
}

func TestInsertAndLookup(t *testing.T) {
	db := setupTestDB(t)
	defer func() { _ = db.Close() }()

	ctx := context.Background()

	pkg := Package{
		Name:        "test-pkg",
		Epoch:       "1",
		Version:     "1.0.0",
		Release:     "1",
		Arch:        "x86_64",
		Format:      "rpm",
		Summary:     "Test package",
		InstallTime: time.Now(),
		Files: []File{
			{
				Path:      "/usr/bin/test",
				Mode:      0o755,
				IsDir:     false,
				IsSymlink: false,
				SHA256:    "abc123",
			},
			{
				Path:      "/etc/test.conf",
				Mode:      0o644,
				IsDir:     false,
				IsSymlink: false,
				SHA256:    "def456",
			},
		},
		Caps: []Capability{
			{
				Kind:    "provide",
				Name:    "test-pkg",
				Flags:   0,
				Version: "1.0.0",
			},
			{
				Kind:    "require",
				Name:    "libc",
				Flags:   0,
				Version: "",
			},
		},
	}

	if err := db.Insert(ctx, &pkg); err != nil {
		t.Fatalf("Insert failed: %v", err)
	}

	// Lookup the package.
	found, err := db.LookupByName(ctx, "test-pkg", "x86_64")
	if err != nil {
		t.Fatalf("LookupByName failed: %v", err)
	}

	if found == nil {
		t.Fatal("expected package to be found")
	}

	if found.Name != "test-pkg" {
		t.Errorf("expected name test-pkg, got %s", found.Name)
	}

	if found.Version != "1.0.0" {
		t.Errorf("expected version 1.0.0, got %s", found.Version)
	}

	if len(found.Files) != 2 {
		t.Errorf("expected 2 files, got %d", len(found.Files))
	}

	if len(found.Caps) != 2 {
		t.Errorf("expected 2 capabilities, got %d", len(found.Caps))
	}
}

func TestInsertIdempotent(t *testing.T) {
	db := setupTestDB(t)
	defer func() { _ = db.Close() }()

	ctx := context.Background()

	pkg := Package{
		Name:        "test-pkg",
		Epoch:       "1",
		Version:     "1.0.0",
		Release:     "1",
		Arch:        "x86_64",
		Format:      "rpm",
		Summary:     "Test package",
		InstallTime: time.Now(),
		Files: []File{
			{Path: "/usr/bin/test", Mode: 0o755, SHA256: "abc123"},
		},
	}

	// Insert first time.
	if err := db.Insert(ctx, &pkg); err != nil {
		t.Fatalf("first Insert failed: %v", err)
	}

	// Insert again with different version (should replace).
	pkg.Version = "2.0.0"
	pkg.Files = []File{
		{Path: "/usr/bin/test", Mode: 0o755, SHA256: "xyz789"},
		{Path: "/usr/lib/test.so", Mode: 0o755, SHA256: "def456"},
	}

	if err := db.Insert(ctx, &pkg); err != nil {
		t.Fatalf("second Insert failed: %v", err)
	}

	// Verify the new version is stored.
	found, err := db.LookupByName(ctx, "test-pkg", "x86_64")
	if err != nil {
		t.Fatalf("LookupByName failed: %v", err)
	}

	if found.Version != "2.0.0" {
		t.Errorf("expected version 2.0.0, got %s", found.Version)
	}

	if len(found.Files) != 2 {
		t.Errorf("expected 2 files after replace, got %d", len(found.Files))
	}
}

func TestIsInstalled(t *testing.T) {
	db := setupTestDB(t)
	defer func() { _ = db.Close() }()

	ctx := context.Background()

	pkg := Package{
		Name:        "test-pkg",
		Version:     "1.0.0",
		Release:     "1",
		Arch:        "x86_64",
		Format:      "rpm",
		InstallTime: time.Now(),
	}

	if err := db.Insert(ctx, &pkg); err != nil {
		t.Fatalf("Insert failed: %v", err)
	}

	installed, err := db.IsInstalled(ctx, "test-pkg")
	if err != nil {
		t.Fatalf("IsInstalled failed: %v", err)
	}

	if !installed {
		t.Error("expected package to be installed")
	}

	installed, err = db.IsInstalled(ctx, "nonexistent")
	if err != nil {
		t.Fatalf("IsInstalled failed: %v", err)
	}

	if installed {
		t.Error("expected nonexistent package to not be installed")
	}
}

func TestProvidersOf(t *testing.T) {
	db := setupTestDB(t)
	defer func() { _ = db.Close() }()

	ctx := context.Background()

	pkg1 := Package{
		Name:        "pkg1",
		Version:     "1.0.0",
		Release:     "1",
		Arch:        "x86_64",
		Format:      "rpm",
		InstallTime: time.Now(),
		Caps: []Capability{
			{Kind: "provide", Name: "libfoo", Version: "1.0"},
		},
	}

	pkg2 := Package{
		Name:        "pkg2",
		Version:     "2.0.0",
		Release:     "1",
		Arch:        "x86_64",
		Format:      "rpm",
		InstallTime: time.Now(),
		Caps: []Capability{
			{Kind: "provide", Name: "libfoo", Version: "2.0"},
		},
	}

	if err := db.Insert(ctx, &pkg1); err != nil {
		t.Fatalf("Insert pkg1 failed: %v", err)
	}

	if err := db.Insert(ctx, &pkg2); err != nil {
		t.Fatalf("Insert pkg2 failed: %v", err)
	}

	providers, err := db.ProvidersOf(ctx, "libfoo")
	if err != nil {
		t.Fatalf("ProvidersOf failed: %v", err)
	}

	if len(providers) != 2 {
		t.Errorf("expected 2 providers, got %d", len(providers))
	}

	// Check that both pkg1 and pkg2 are in the list.
	found := make(map[string]bool)
	for _, p := range providers {
		found[p] = true
	}

	if !found["pkg1"] || !found["pkg2"] {
		t.Errorf("expected pkg1 and pkg2 in providers, got %v", providers)
	}
}

func TestList(t *testing.T) {
	db := setupTestDB(t)
	defer func() { _ = db.Close() }()

	ctx := context.Background()

	pkg1 := Package{
		Name:        "pkg1",
		Version:     "1.0.0",
		Release:     "1",
		Arch:        "x86_64",
		Format:      "rpm",
		InstallTime: time.Now(),
	}

	pkg2 := Package{
		Name:        "pkg2",
		Version:     "2.0.0",
		Release:     "1",
		Arch:        "aarch64",
		Format:      "rpm",
		InstallTime: time.Now(),
	}

	if err := db.Insert(ctx, &pkg1); err != nil {
		t.Fatalf("Insert pkg1 failed: %v", err)
	}

	if err := db.Insert(ctx, &pkg2); err != nil {
		t.Fatalf("Insert pkg2 failed: %v", err)
	}

	packages, err := db.List(ctx)
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}

	if len(packages) != 2 {
		t.Errorf("expected 2 packages, got %d", len(packages))
	}

	// Verify order (should be sorted by name).
	if packages[0].Name != "pkg1" || packages[1].Name != "pkg2" {
		t.Errorf("expected packages in order [pkg1, pkg2], got [%s, %s]",
			packages[0].Name, packages[1].Name)
	}
}

func TestRemove(t *testing.T) {
	db := setupTestDB(t)
	defer func() { _ = db.Close() }()

	ctx := context.Background()

	pkg := Package{
		Name:        "test-pkg",
		Version:     "1.0.0",
		Release:     "1",
		Arch:        "x86_64",
		Format:      "rpm",
		InstallTime: time.Now(),
		Files: []File{
			{Path: "/usr/bin/test", Mode: 0o755},
		},
	}

	if err := db.Insert(ctx, &pkg); err != nil {
		t.Fatalf("Insert failed: %v", err)
	}

	// Verify it's installed.
	installed, err := db.IsInstalled(ctx, "test-pkg")
	if err != nil {
		t.Fatalf("IsInstalled failed: %v", err)
	}

	if !installed {
		t.Fatal("expected package to be installed")
	}

	// Remove it.
	if err := db.Remove(ctx, "test-pkg", "x86_64"); err != nil {
		t.Fatalf("Remove failed: %v", err)
	}

	// Verify it's gone.
	installed, err = db.IsInstalled(ctx, "test-pkg")
	if err != nil {
		t.Fatalf("IsInstalled failed: %v", err)
	}

	if installed {
		t.Error("expected package to be removed")
	}

	// Verify files are cascaded.
	found, err := db.LookupByName(ctx, "test-pkg", "x86_64")
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("LookupByName: expected ErrNotFound, got err=%v found=%v", err, found)
	}

	if found != nil {
		t.Error("expected package to be nil after removal")
	}
}

func TestConcurrentOpenClose(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	// Open and close multiple times.
	for i := range 3 {
		db, err := Open(context.Background(), dbPath)
		if err != nil {
			t.Fatalf("Open iteration %d failed: %v", i, err)
		}

		if err := db.Close(); err != nil {
			t.Fatalf("Close iteration %d failed: %v", i, err)
		}
	}
}

func TestLookupNonexistent(t *testing.T) {
	db := setupTestDB(t)
	defer func() { _ = db.Close() }()

	ctx := context.Background()

	found, err := db.LookupByName(ctx, "nonexistent", "x86_64")
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("LookupByName: expected ErrNotFound, got err=%v found=%v", err, found)
	}

	if found != nil {
		t.Error("expected nil for nonexistent package")
	}
}

func TestMultipleArchitectures(t *testing.T) {
	db := setupTestDB(t)
	defer func() { _ = db.Close() }()

	ctx := context.Background()

	pkg1 := Package{
		Name:        "test-pkg",
		Version:     "1.0.0",
		Release:     "1",
		Arch:        "x86_64",
		Format:      "rpm",
		InstallTime: time.Now(),
	}

	pkg2 := Package{
		Name:        "test-pkg",
		Version:     "1.0.0",
		Release:     "1",
		Arch:        "aarch64",
		Format:      "rpm",
		InstallTime: time.Now(),
	}

	if err := db.Insert(ctx, &pkg1); err != nil {
		t.Fatalf("Insert pkg1 failed: %v", err)
	}

	if err := db.Insert(ctx, &pkg2); err != nil {
		t.Fatalf("Insert pkg2 failed: %v", err)
	}

	// Both should be findable.
	found1, err := db.LookupByName(ctx, "test-pkg", "x86_64")
	if err != nil {
		t.Fatalf("LookupByName x86_64 failed: %v", err)
	}

	if found1 == nil {
		t.Fatal("expected x86_64 package to be found")
	}

	found2, err := db.LookupByName(ctx, "test-pkg", "aarch64")
	if err != nil {
		t.Fatalf("LookupByName aarch64 failed: %v", err)
	}

	if found2 == nil {
		t.Fatal("expected aarch64 package to be found")
	}

	// Remove one shouldn't affect the other.
	if err := db.Remove(ctx, "test-pkg", "x86_64"); err != nil {
		t.Fatalf("Remove failed: %v", err)
	}

	found1, err = db.LookupByName(ctx, "test-pkg", "x86_64")
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("LookupByName x86_64 after remove: expected ErrNotFound, got err=%v", err)
	}

	if found1 != nil {
		t.Error("expected x86_64 package to be removed")
	}

	found2, err = db.LookupByName(ctx, "test-pkg", "aarch64")
	if err != nil {
		t.Fatalf("LookupByName aarch64 after remove failed: %v", err)
	}

	if found2 == nil {
		t.Error("expected aarch64 package to still exist")
	}
}

// setupTestDB creates a temporary database for testing.
func setupTestDB(t *testing.T) *DB {
	t.Helper()

	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	db, err := Open(context.Background(), dbPath)
	if err != nil {
		t.Fatalf("setupTestDB: Open failed: %v", err)
	}

	return db
}
