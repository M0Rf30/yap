package core

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/M0Rf30/yap/v2/pkg/constants"
	"github.com/M0Rf30/yap/v2/pkg/pkgbuild"
)

func TestNewBasePackageManager(t *testing.T) {
	pkg := &pkgbuild.PKGBUILD{
		PkgName: "test-package",
		PkgVer:  "1.0.0",
	}
	config := &Config{
		Name:        "test",
		InstallCmd:  "test-install",
		InstallArgs: []string{"install"},
	}

	bpm := NewBasePackageManager(pkg, config)

	if bpm.PKGBUILD != pkg {
		t.Fatal("PKGBUILD should be set correctly")
	}

	if bpm.Config != config {
		t.Fatal("Config should be set correctly")
	}
}

func TestBuildPackageName(t *testing.T) {
	pkg := &pkgbuild.PKGBUILD{
		PkgName:      "test-package",
		PkgVer:       "1.0.0",
		PkgRel:       "1",
		ArchComputed: "x86_64",
		Epoch:        "",
	}
	config := &Config{}
	bpm := NewBasePackageManager(pkg, config)

	tests := []struct {
		format   string
		expected string
	}{
		{constants.FormatAPK, "test-package-1.0.0-r1.x86_64.apk"},
		{constants.FormatDEB, "test-package_1.0.0-1_x86_64.deb"},
		{constants.FormatRPM, "test-package-1.0.0-1.x86_64.rpm"},
		{constants.FormatPacman, "test-package-1.0.0-1-x86_64.pkg.tar.zst"},
		{"unknown", "test-package-1.0.0-1-x86_64"},
	}

	for _, test := range tests {
		result := bpm.BuildPackageName(test.format)
		if result != test.expected {
			t.Fatalf("BuildPackageName(%s) = %s, expected %s", test.format, result, test.expected)
		}
	}
}

func TestBuildPackageNameWithEpoch(t *testing.T) {
	pkg := &pkgbuild.PKGBUILD{
		PkgName:      "test-package",
		PkgVer:       "1.0.0",
		PkgRel:       "1",
		ArchComputed: "x86_64",
		Epoch:        "2",
	}
	config := &Config{}
	bpm := NewBasePackageManager(pkg, config)

	tests := []struct {
		format   string
		expected string
	}{
		{constants.FormatRPM, "test-package-2:1.0.0-1.x86_64.rpm"},
		{constants.FormatPacman, "test-package-2:1.0.0-1-x86_64.pkg.tar.zst"},
		{constants.FormatAPK, "test-package-1.0.0-r1.x86_64.apk"}, // APK doesn't use epoch in filename
		{constants.FormatDEB, "test-package_1.0.0-1_x86_64.deb"},  // DEB doesn't use epoch in filename
	}

	for _, test := range tests {
		result := bpm.BuildPackageName(test.format)
		if result != test.expected {
			t.Fatalf("BuildPackageName(%s) with epoch = %s, expected %s", test.format, result, test.expected)
		}
	}
}

func TestValidateArtifactsPath(t *testing.T) {
	pkg := &pkgbuild.PKGBUILD{}
	config := &Config{}
	bpm := NewBasePackageManager(pkg, config)

	// Test with existing directory
	tempDir := t.TempDir()

	err := bpm.ValidateArtifactsPath(tempDir)
	if err != nil {
		t.Fatalf("ValidateArtifactsPath should not fail for existing directory: %v", err)
	}

	// Test with non-existent directory
	nonExistentPath := filepath.Join(tempDir, "non-existent")

	err = bpm.ValidateArtifactsPath(nonExistentPath)
	if err == nil {
		t.Fatal("ValidateArtifactsPath should fail for non-existent directory")
	}
}

func TestSetComputedFields(t *testing.T) {
	pkg := &pkgbuild.PKGBUILD{
		ArchComputed: "x86_64",
		Section:      "devel",
	}
	config := &Config{
		ArchMap: map[string]string{
			"x86_64": "amd64",
		},
		GroupMap: map[string]string{
			"devel": "Development/Tools",
		},
	}
	bpm := NewBasePackageManager(pkg, config)

	bpm.SetComputedFields()

	if pkg.ArchComputed != "amd64" {
		t.Fatalf("Expected ArchComputed to be mapped to 'amd64', got '%s'", pkg.ArchComputed)
	}

	if pkg.Section != "Development/Tools" {
		t.Fatalf("Expected Section to be mapped to 'Development/Tools', got '%s'", pkg.Section)
	}
}

func TestSetComputedFieldsNoMapping(t *testing.T) {
	pkg := &pkgbuild.PKGBUILD{
		ArchComputed: "unknown-arch",
		Section:      "unknown-section",
	}
	config := &Config{
		ArchMap:  map[string]string{},
		GroupMap: map[string]string{},
	}
	bpm := NewBasePackageManager(pkg, config)

	originalArch := pkg.ArchComputed
	originalSection := pkg.Section

	bpm.SetComputedFields()

	// Should remain unchanged if no mapping exists
	if pkg.ArchComputed != originalArch {
		t.Fatalf("ArchComputed should remain unchanged when no mapping exists, got '%s'", pkg.ArchComputed)
	}

	if pkg.Section != originalSection {
		t.Fatalf("Section should remain unchanged when no mapping exists, got '%s'", pkg.Section)
	}
}

func TestSetInstalledSize(t *testing.T) {
	tempDir := t.TempDir()

	// Create some test files
	testFile1 := filepath.Join(tempDir, "file1.txt")

	err := os.WriteFile(testFile1, make([]byte, 1024), 0o644) // 1KB file
	if err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	testFile2 := filepath.Join(tempDir, "file2.txt")

	err = os.WriteFile(testFile2, make([]byte, 2048), 0o644) // 2KB file
	if err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	pkg := &pkgbuild.PKGBUILD{
		PackageDir: tempDir,
	}
	config := &Config{}
	bpm := NewBasePackageManager(pkg, config)

	err = bpm.SetInstalledSize()
	if err != nil {
		t.Fatalf("SetInstalledSize failed: %v", err)
	}

	expectedSize := int64(3) // 3KB total, converted to KB
	if pkg.InstalledSize != expectedSize {
		t.Fatalf("Expected InstalledSize %d, got %d", expectedSize, pkg.InstalledSize)
	}
}

func TestSetInstalledSizeNonExistentDir(t *testing.T) {
	pkg := &pkgbuild.PKGBUILD{
		PackageDir: "/non/existent/directory",
	}
	config := &Config{}
	bpm := NewBasePackageManager(pkg, config)

	err := bpm.SetInstalledSize()
	if err == nil {
		t.Fatal("SetInstalledSize should fail for non-existent directory")
	}
}

func TestLogPackageCreated(t *testing.T) {
	pkg := &pkgbuild.PKGBUILD{
		PkgName: "test-package",
		PkgVer:  "1.0.0",
		PkgRel:  "1",
	}
	config := &Config{}
	bpm := NewBasePackageManager(pkg, config)

	// This should not panic or error
	bpm.LogPackageCreated("/path/to/artifact.pkg")
}

func TestPrepareCommon(t *testing.T) {
	// Create a mock PKGBUILD with GetDepends method
	pkg := &pkgbuild.PKGBUILD{
		PkgName: "test-package",
	}
	config := &Config{
		InstallCmd:  "echo", // Use echo to avoid actual package installation
		InstallArgs: []string{},
	}
	bpm := NewBasePackageManager(pkg, config)

	// Test with empty dependencies (should not fail)
	err := bpm.PrepareCommon([]string{})
	if err != nil {
		t.Fatalf("PrepareCommon should not fail with empty dependencies: %v", err)
	}
}

func TestUpdateCommon(t *testing.T) {
	pkg := &pkgbuild.PKGBUILD{}

	// Test with config that has update args
	configWithUpdate := &Config{
		UpdateArgs: []string{"update"},
	}
	bpm := NewBasePackageManager(pkg, configWithUpdate)

	// This will likely fail but tests the method structure
	err := bpm.UpdateCommon()
	// We don't assert on error since this depends on actual package manager availability
	_ = err

	// Test with config that has no update args
	configNoUpdate := &Config{
		UpdateArgs: []string{},
	}
	bpm = NewBasePackageManager(pkg, configNoUpdate)

	err = bpm.UpdateCommon()
	if err != nil {
		t.Fatalf("UpdateCommon should not fail when no update args are specified: %v", err)
	}
}
