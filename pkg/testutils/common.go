// Package testutils provides common testing utilities for package managers.
package testutils

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/M0Rf30/yap/v2/pkg/pkgbuild"
)

// TestPKGBUILD creates a minimal PKGBUILD for testing.
func TestPKGBUILD(t *testing.T) *pkgbuild.PKGBUILD {
	t.Helper()

	tempDir := t.TempDir()
	packageDir := filepath.Join(tempDir, "package")
	err := os.MkdirAll(packageDir, 0o755)
	require.NoError(t, err)

	return &pkgbuild.PKGBUILD{
		PkgName:      "test-package",
		PkgVer:       "1.0.0",
		PkgRel:       "1",
		PkgDesc:      "Test package description",
		ArchComputed: "x86_64",
		Home:         tempDir,
		StartDir:     tempDir,
		PackageDir:   packageDir,
		SourceDir:    filepath.Join(tempDir, "src"),
		Maintainer:   "Test Maintainer <test@example.com>",
		URL:          "https://example.com",
		License:      []string{"MIT"},
		Section:      "utils",
	}
}

// TestPackageManagerBehavior tests common behavior across all package managers.
func TestPackageManagerBehavior(t *testing.T, packageManager interface {
	BuildPackage(string) error
	Install(string) error
	Prepare([]string) error
	PrepareEnvironment(bool) error
	PrepareFakeroot(string) error
	Update() error
}) {
	t.Helper()

	// Test that methods don't panic with nil inputs
	t.Run("NilSafety", func(t *testing.T) {
		artifactsPath := t.TempDir()

		// These should not panic even with empty/nil inputs
		assert.NotPanics(t, func() {
			_ = packageManager.Prepare(nil)
		})

		assert.NotPanics(t, func() {
			_ = packageManager.Prepare([]string{})
		})

		assert.NotPanics(t, func() {
			_ = packageManager.PrepareEnvironment(false)
		})

		assert.NotPanics(t, func() {
			_ = packageManager.Update()
		})

		assert.NotPanics(t, func() {
			_ = packageManager.PrepareFakeroot(artifactsPath)
		})
	})

	t.Run("PathValidation", func(t *testing.T) {
		// Test with invalid paths
		err := packageManager.BuildPackage("/nonexistent/path")
		assert.Error(t, err, "Should error with nonexistent path")

		err = packageManager.Install("/nonexistent/file.pkg")
		assert.Error(t, err, "Should error with nonexistent file")
	})
}

// CreateTestFile creates a test file with specified content.
func CreateTestFile(t *testing.T, dir, filename, content string) string {
	t.Helper()

	filePath := filepath.Join(dir, filename)
	err := os.WriteFile(filePath, []byte(content), 0o600)
	require.NoError(t, err)

	return filePath
}

// CreateTestDir creates a test directory structure.
func CreateTestDir(t *testing.T, baseDir string, subdirs ...string) string {
	t.Helper()

	fullPath := filepath.Join(append([]string{baseDir}, subdirs...)...)
	err := os.MkdirAll(fullPath, 0o755)
	require.NoError(t, err)

	return fullPath
}

// CreateTestSymlink creates a test symlink.
func CreateTestSymlink(t *testing.T, dir, linkName, target string) string {
	t.Helper()

	linkPath := filepath.Join(dir, linkName)
	err := os.Symlink(target, linkPath)
	require.NoError(t, err)

	return linkPath
}

// AssertPackageFiles verifies that expected package files exist.
func AssertPackageFiles(t *testing.T, artifactsPath, packageName string) {
	t.Helper()

	packagePath := filepath.Join(artifactsPath, packageName)
	_, err := os.Stat(packagePath)
	assert.NoError(t, err, "Package file should exist: %s", packagePath)

	// Check that file is not empty
	stat, err := os.Stat(packagePath)
	if err == nil {
		assert.Greater(t, stat.Size(), int64(0), "Package file should not be empty")
	}
}

// SkipIfNoRoot skips the test if not running as root (for installation tests).
func SkipIfNoRoot(t *testing.T) {
	t.Helper()

	if os.Geteuid() != 0 {
		t.Skip("Skipping test: requires root privileges")
	}
}

// SkipIfMissingCommand skips the test if a required command is not available.
func SkipIfMissingCommand(t *testing.T, command string) {
	t.Helper()

	_, err := os.Stat("/usr/bin/" + command)
	if os.IsNotExist(err) {
		_, err = os.Stat("/bin/" + command)
		if os.IsNotExist(err) {
			t.Skipf("Skipping test: command %s not found", command)
		}
	}
}
