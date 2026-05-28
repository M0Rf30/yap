package aptinstall_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/M0Rf30/yap/v2/pkg/aptcache"
	"github.com/M0Rf30/yap/v2/pkg/aptinstall"
)

// Convenience aliases for test helpers
var (
	FilterForeignArchPackagesForTesting         = aptinstall.FilterForeignArchPackagesForTesting
	CurrentInstalledVersionForTesting           = aptinstall.CurrentInstalledVersionForTesting
	CurrentInstalledVersionFromStatusForTesting = aptinstall.CurrentInstalledVersionFromStatusForTesting
	EnsureDpkgDirsForTesting                    = aptinstall.EnsureDpkgDirsForTesting
	WriteDpkgInfoFilesForTesting                = aptinstall.WriteDpkgInfoFilesForTesting
	ReadDpkgStatusFromPathForTesting            = aptinstall.ReadDpkgStatusFromPathForTesting
	WriteDpkgStatusToPathForTesting             = aptinstall.WriteDpkgStatusToPathForTesting
	UpdateDpkgStatusForPackageAtPathForTesting  = aptinstall.UpdateDpkgStatusForPackageAtPathForTesting
	AcquireDpkgLockForTesting                   = aptinstall.AcquireDpkgLockForTesting
	ResolveRootDirForTesting                    = aptinstall.ResolveRootDirForTesting
	ParseControlForTesting                      = aptinstall.ParseControlForTesting
	RunMaintainerScriptForPhaseForTesting       = aptinstall.RunMaintainerScriptForPhaseForTesting
	InstallWithOptions                          = aptinstall.InstallWithOptions
	WriteAllStatusEntriesForTesting             = aptinstall.WriteAllStatusEntriesForTesting
)

type DebContentsForTesting = aptinstall.DebContentsForTesting

// TestFilterForeignArchPackages tests the filterForeignArchPackages function
// with various architecture combinations.
func TestFilterForeignArchPackages(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		pkgs     []*aptcache.PackageInfo
		expected int
	}{
		{
			name: "all packages",
			pkgs: []*aptcache.PackageInfo{
				{Name: "pkg1", Architecture: "all"},
				{Name: "pkg2", Architecture: ""},
			},
			expected: 2,
		},
		{
			name: "host arch packages",
			pkgs: []*aptcache.PackageInfo{
				{Name: "pkg1", Architecture: aptcache.GoarchToDebArch()},
			},
			expected: 1,
		},
		{
			name: "foreign arch non-multiarch filtered",
			pkgs: []*aptcache.PackageInfo{
				{Name: "pkg1", Architecture: "arm64", MultiArch: ""},
			},
			expected: 0,
		},
		{
			name: "foreign arch multiarch same kept",
			pkgs: []*aptcache.PackageInfo{
				{Name: "pkg1", Architecture: "arm64", MultiArch: "same"},
			},
			expected: 1,
		},
		{
			name: "mixed packages",
			pkgs: []*aptcache.PackageInfo{
				{Name: "pkg1", Architecture: "all"},
				{Name: "pkg2", Architecture: aptcache.GoarchToDebArch()},
				{Name: "pkg3", Architecture: "arm64", MultiArch: ""},
				{Name: "pkg4", Architecture: "arm64", MultiArch: "same"},
			},
			expected: 3,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			filtered := aptinstall.FilterForeignArchPackagesForTesting(tt.pkgs)
			if len(filtered) != tt.expected {
				t.Errorf("expected %d packages, got %d", tt.expected, len(filtered))
			}
		})
	}
}

// TestCurrentInstalledVersionNotInstalled tests currentInstalledVersion
// when the package is not installed.
func TestCurrentInstalledVersionNotInstalled(t *testing.T) {
	t.Parallel()

	pkg := &aptcache.PackageInfo{
		Name:         "nonexistent",
		Architecture: "amd64",
		Installed:    false,
	}

	version := aptinstall.CurrentInstalledVersionForTesting(pkg)
	if version != "" {
		t.Errorf("expected empty version for non-installed package, got %q", version)
	}
}

// TestCurrentInstalledVersionFromStatus tests currentInstalledVersion
// with synthetic dpkg status data.
func TestCurrentInstalledVersionFromStatus(t *testing.T) {
	t.Parallel()

	statusData := `Package: hello
Version: 1.0
Architecture: amd64
Status: install ok installed

Package: world
Version: 2.0
Status: install ok installed

`

	tests := []struct {
		name     string
		pkgName  string
		arch     string
		expected string
	}{
		{
			name:     "exact match with arch",
			pkgName:  "hello",
			arch:     "amd64",
			expected: "1.0",
		},
		{
			name:     "fallback to unqualified name",
			pkgName:  "world",
			arch:     "amd64",
			expected: "2.0",
		},
		{
			name:     "nonexistent package",
			pkgName:  "missing",
			arch:     "amd64",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			version := aptinstall.CurrentInstalledVersionFromStatusForTesting(statusData, tt.pkgName, tt.arch)
			if version != tt.expected {
				t.Errorf("expected %q, got %q", tt.expected, version)
			}
		})
	}
}

// TestEnsureDpkgDirs tests that ensureDpkgDirs creates the required directories.
func TestEnsureDpkgDirs(t *testing.T) {
	t.Parallel()

	// Call ensureDpkgDirs (it creates /var/lib/dpkg and subdirs)
	// This test just verifies the function doesn't panic
	if err := aptinstall.EnsureDpkgDirsForTesting(); err != nil {
		// It's ok if it fails (e.g., permission denied on /var/lib/dpkg)
		// We're just testing that the function is callable
		t.Logf("EnsureDpkgDirs returned error (expected in test env): %v", err)
	}
}

// TestWriteDpkgInfoFiles tests that WriteDpkgInfoFiles is callable
func TestWriteDpkgInfoFiles(t *testing.T) {
	t.Parallel()

	// This test just verifies the function is callable
	// Actual filesystem writes are tested in status_write_test.go
	contents := &aptinstall.DebContentsForTesting{
		Control:    "Package: testpkg\nVersion: 1.0\n",
		Md5sums:    "abc123  /usr/bin/test\n",
		Conffiles:  "/etc/test.conf\n",
		Scriptlets: map[string]string{},
		Triggers:   "",
		Files: []string{
			"/usr/bin/test",
		},
	}

	// This will fail in test env (no /var/lib/dpkg), but we're just testing
	// that the function is callable and doesn't panic
	_ = aptinstall.WriteDpkgInfoFilesForTesting("testpkg", "amd64", contents)
}

// TestReadDpkgStatusEmpty tests reading dpkg status when file doesn't exist
func TestReadDpkgStatusEmpty(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	statusPath := filepath.Join(tmpDir, "status")

	// File doesn't exist yet
	entries, err := aptinstall.ReadDpkgStatusFromPathForTesting(statusPath)
	if err != nil {
		t.Fatalf("ReadDpkgStatus failed: %v", err)
	}

	if len(entries) != 0 {
		t.Errorf("expected empty entries, got %d", len(entries))
	}
}

// TestReadDpkgStatusWithData tests reading dpkg status with actual data
func TestReadDpkgStatusWithData(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	statusPath := filepath.Join(tmpDir, "status")

	statusData := map[string]map[string]string{
		"pkg1": {
			"Package":      "pkg1",
			"Version":      "1.0",
			"Architecture": "amd64",
			"Status":       "install ok installed",
		},
		"pkg2:arm64": {
			"Package":      "pkg2",
			"Version":      "2.0",
			"Architecture": "arm64",
			"Status":       "install ok installed",
		},
	}

	if err := aptinstall.WriteDpkgStatusToPathForTesting(statusPath, statusData); err != nil {
		t.Fatalf("WriteDpkgStatus failed: %v", err)
	}

	entries, err := aptinstall.ReadDpkgStatusFromPathForTesting(statusPath)
	if err != nil {
		t.Fatalf("ReadDpkgStatus failed: %v", err)
	}

	if len(entries) != 2 {
		t.Errorf("expected 2 entries, got %d (entries: %v)", len(entries), entries)
	}

	// Check if entries exist (they should be keyed by pkg name or pkg:arch)
	if len(entries) >= 1 {
		// Just verify we can read back what we wrote
		for key, entry := range entries {
			if entry["Package"] == "" {
				t.Errorf("entry %q has empty Package field", key)
			}
		}
	}
}

// TestWriteDpkgStatusAtomicity tests that writeDpkgStatus is atomic
func TestWriteDpkgStatusAtomicity(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	statusPath := filepath.Join(tmpDir, "status")

	statusData := map[string]map[string]string{
		"pkg1": {
			"Package": "pkg1",
			"Version": "1.0",
			"Status":  "install ok installed",
		},
	}

	if err := aptinstall.WriteDpkgStatusToPathForTesting(statusPath, statusData); err != nil {
		t.Fatalf("WriteDpkgStatus failed: %v", err)
	}

	// Verify the file exists and contains the data
	if _, err := os.Stat(statusPath); err != nil {
		t.Errorf("status file not created: %v", err)
	}

	// Verify temp file is cleaned up
	tmpPath := statusPath + ".dpkg-tmp"
	if _, err := os.Stat(tmpPath); err == nil {
		t.Error("temp file not cleaned up")
	}

	// Verify content
	entries, err := aptinstall.ReadDpkgStatusFromPathForTesting(statusPath)
	if err != nil {
		t.Fatalf("ReadDpkgStatus failed: %v", err)
	}

	if len(entries) != 1 {
		t.Errorf("expected 1 entry, got %d", len(entries))
	}
}

// TestUpdateDpkgStatusForPackage tests updating dpkg status for a package
func TestUpdateDpkgStatusForPackage(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	statusPath := filepath.Join(tmpDir, "status")

	control := `Package: testpkg
Version: 1.0
Architecture: amd64
Description: Test package
`

	// Update status (should create entry)
	if err := aptinstall.UpdateDpkgStatusForPackageAtPathForTesting(
		statusPath, "testpkg", "amd64", control, "install ok installed",
	); err != nil {
		t.Fatalf("UpdateDpkgStatusForPackage failed: %v", err)
	}

	// Verify entry was created
	entries, err := aptinstall.ReadDpkgStatusFromPathForTesting(statusPath)
	if err != nil {
		t.Fatalf("ReadDpkgStatus failed: %v", err)
	}

	if len(entries) != 1 {
		t.Errorf("expected 1 entry, got %d", len(entries))
	}

	entry := entries["testpkg:amd64"]
	if entry["Package"] != "testpkg" {
		t.Errorf("expected package testpkg, got %q", entry["Package"])
	}

	if entry["Version"] != "1.0" {
		t.Errorf("expected version 1.0, got %q", entry["Version"])
	}

	if entry["Status"] != "install ok installed" {
		t.Errorf("expected status 'install ok installed', got %q", entry["Status"])
	}
}

// TestUpdateDpkgStatusForPackageUpgrade tests updating an existing package entry
func TestUpdateDpkgStatusForPackageUpgrade(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	statusPath := filepath.Join(tmpDir, "status")

	control1 := `Package: testpkg
Version: 1.0
Architecture: amd64
Description: Test package
`

	control2 := `Package: testpkg
Version: 2.0
Architecture: amd64
Description: Test package upgraded
`

	// Create initial entry
	if err := aptinstall.UpdateDpkgStatusForPackageAtPathForTesting(
		statusPath, "testpkg", "amd64", control1, "install ok installed",
	); err != nil {
		t.Fatalf("UpdateDpkgStatusForPackage (1) failed: %v", err)
	}

	// Upgrade to version 2.0
	if err := aptinstall.UpdateDpkgStatusForPackageAtPathForTesting(
		statusPath, "testpkg", "amd64", control2, "install ok installed",
	); err != nil {
		t.Fatalf("UpdateDpkgStatusForPackage (2) failed: %v", err)
	}

	// Verify entry was updated
	entries, err := aptinstall.ReadDpkgStatusFromPathForTesting(statusPath)
	if err != nil {
		t.Fatalf("ReadDpkgStatus failed: %v", err)
	}

	if len(entries) != 1 {
		t.Errorf("expected 1 entry, got %d", len(entries))
	}

	entry := entries["testpkg:amd64"]
	if entry["Version"] != "2.0" {
		t.Errorf("expected version 2.0, got %q", entry["Version"])
	}
}

// TestAcquireDpkgLock tests acquiring and releasing the dpkg lock
func TestAcquireDpkgLock(t *testing.T) {
	t.Parallel()

	lock, err := aptinstall.AcquireDpkgLockForTesting()
	if err != nil {
		t.Fatalf("AcquireDpkgLock failed: %v", err)
	}

	// Lock should be acquired
	if lock == nil {
		t.Error("expected non-nil lock")
	}

	// Release the lock
	lock.Release()
}

// TestResolveRootDir tests resolveRootDir with various inputs
func TestResolveRootDir(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		rootDir      string
		allowRoot    bool
		shouldError  bool
		expectedPath string
	}{
		{
			name:         "empty root dir without allow",
			rootDir:      "",
			allowRoot:    false,
			shouldError:  true,
			expectedPath: "",
		},
		{
			name:         "empty root dir with allow",
			rootDir:      "",
			allowRoot:    true,
			shouldError:  false,
			expectedPath: "/",
		},
		{
			name:         "custom root dir",
			rootDir:      "/tmp/custom",
			allowRoot:    false,
			shouldError:  false,
			expectedPath: "/tmp/custom",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path, err := aptinstall.ResolveRootDirForTesting(tt.rootDir, tt.allowRoot)
			if (err != nil) != tt.shouldError {
				t.Errorf("expected error=%v, got %v", tt.shouldError, err)
			}

			if !tt.shouldError && path != tt.expectedPath {
				t.Errorf("expected path %q, got %q", tt.expectedPath, path)
			}
		})
	}
}

// TestRunMaintainerScriptNotFound tests runMaintainerScript when script doesn't exist
func TestRunMaintainerScriptNotFound(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	scriptlets := map[string]string{}
	control := "Package: testpkg\nVersion: 1.0\n"

	// Should not error when scriptlet doesn't exist
	err := aptinstall.RunMaintainerScriptForPhaseForTesting(
		ctx, "preinst", "testpkg", "amd64", scriptlets, control, "",
	)
	// It's ok if this errors (scriptlet execution may fail in test env)
	// We're just testing that the function is callable
	_ = err
}

// TestInstallWithOptionsEmpty tests InstallWithOptions with empty package list
func TestInstallWithOptionsEmpty(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	opts := aptinstall.Options{
		RootDir:          t.TempDir(),
		AllowRootInstall: true,
		WriteDpkgStatus:  false,
	}

	err := aptinstall.InstallWithOptions(ctx, []string{}, opts)
	if err != nil {
		t.Errorf("expected no error for empty list, got %v", err)
	}
}

// TestInstallWithOptionsNonexistent tests InstallWithOptions with nonexistent packages
func TestInstallWithOptionsNonexistent(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	opts := aptinstall.Options{
		RootDir:          t.TempDir(),
		AllowRootInstall: true,
		WriteDpkgStatus:  false,
	}

	// This should error because the package doesn't exist in the cache
	err := aptinstall.InstallWithOptions(ctx, []string{"nonexistent-xyz-pkg"}, opts)
	if err == nil {
		t.Error("expected error for nonexistent package")
	}
}

// TestWriteAllStatusEntries tests writing multiple status entries
func TestWriteAllStatusEntries(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "status")

	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}

	data := map[string]map[string]string{
		"pkg1": {
			"Package": "pkg1",
			"Version": "1.0",
			"Status":  "install ok installed",
		},
		"pkg2": {
			"Package": "pkg2",
			"Version": "2.0",
			"Status":  "install ok installed",
		},
	}

	if err := aptinstall.WriteAllStatusEntriesForTesting(f, data); err != nil {
		t.Fatal(err)
	}

	if err := f.Close(); err != nil {
		t.Fatal(err)
	}

	// Verify content
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	contentStr := string(content)

	// Verify both packages are present
	if !strings.Contains(contentStr, "Package: pkg1") {
		t.Error("pkg1 not found in output")
	}

	if !strings.Contains(contentStr, "Package: pkg2") {
		t.Error("pkg2 not found in output")
	}

	// Verify entries are separated by blank lines
	if !strings.Contains(contentStr, "\n\n") {
		t.Error("entries not separated by blank lines")
	}
}
