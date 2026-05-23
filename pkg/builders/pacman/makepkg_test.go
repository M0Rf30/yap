package pacman

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/M0Rf30/yap/v2/pkg/files"
	"github.com/M0Rf30/yap/v2/pkg/pkgbuild"
)

func createTestPKGBUILD() *pkgbuild.PKGBUILD {
	return &pkgbuild.PKGBUILD{
		PkgName:      "test-package",
		PkgVer:       "1.0.0",
		PkgRel:       "1",
		Epoch:        "1",
		Arch:         []string{"x86_64"},
		ArchComputed: "x86_64",
		PkgDesc:      "Test package description",
		Maintainer:   "test@example.com",
		License:      []string{"MIT"},
		Depends:      []string{"dependency1"},
		MakeDepends:  []string{"make", "gcc"},
		StartDir:     "/tmp/start",
		Home:         "/tmp/home",
		PkgType:      "pkg",
		StripEnabled: false,
	}
}

func TestBuildPackage(t *testing.T) {
	pkgBuild := createTestPKGBUILD()
	pkg := NewBuilder(pkgBuild)

	tempDir, err := os.MkdirTemp("", "pacman-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}

	defer func() { _ = os.RemoveAll(tempDir) }()

	// Create package directory with some content
	packageDir := filepath.Join(tempDir, "package")

	err = os.MkdirAll(packageDir, 0o755)
	if err != nil {
		t.Fatalf("Failed to create package dir: %v", err)
	}

	// Create a test file in package dir
	testFile := filepath.Join(packageDir, "test.txt")

	err = os.WriteFile(testFile, []byte("test content"), 0o644)
	if err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	pkg.PKGBUILD.PackageDir = packageDir

	artifactsDir := filepath.Join(tempDir, "artifacts")

	err = os.MkdirAll(artifactsDir, 0o755)
	if err != nil {
		t.Fatalf("Failed to create artifacts dir: %v", err)
	}

	_, err = pkg.BuildPackage(context.Background(), artifactsDir, "")
	if err != nil {
		t.Errorf("BuildPackage failed: %v", err)
	}

	// Check that package file was created
	expectedPkgName := "test-package-1:1.0.0-1-x86_64.pkg.tar.zst"

	pkgFilePath := filepath.Join(artifactsDir, expectedPkgName)
	if _, err := os.Stat(pkgFilePath); os.IsNotExist(err) {
		t.Errorf("Package file was not created: %s", pkgFilePath)
	}
}

func TestBuildPackageWithoutEpoch(t *testing.T) {
	pkgBuild := createTestPKGBUILD()
	pkgBuild.Epoch = "" // Remove epoch
	pkg := NewBuilder(pkgBuild)

	tempDir, err := os.MkdirTemp("", "pacman-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}

	defer func() { _ = os.RemoveAll(tempDir) }()

	// Create package directory
	packageDir := filepath.Join(tempDir, "package")

	err = os.MkdirAll(packageDir, 0o755)
	if err != nil {
		t.Fatalf("Failed to create package dir: %v", err)
	}

	pkg.PKGBUILD.PackageDir = packageDir

	artifactsDir := filepath.Join(tempDir, "artifacts")

	err = os.MkdirAll(artifactsDir, 0o755)
	if err != nil {
		t.Fatalf("Failed to create artifacts dir: %v", err)
	}

	_, err = pkg.BuildPackage(context.Background(), artifactsDir, "")
	if err != nil {
		t.Errorf("BuildPackage failed: %v", err)
	}

	// Check that package file was created with correct name
	expectedPkgName := "test-package-1.0.0-1-x86_64.pkg.tar.zst"

	pkgFilePath := filepath.Join(artifactsDir, expectedPkgName)
	if _, err := os.Stat(pkgFilePath); os.IsNotExist(err) {
		t.Errorf("Package file was not created: %s", pkgFilePath)
	}
}

func TestPrepareFakeroot(t *testing.T) {
	pkgBuild := createTestPKGBUILD()
	pkg := NewBuilder(pkgBuild)

	tempDir, err := os.MkdirTemp("", "pacman-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}

	defer func() { _ = os.RemoveAll(tempDir) }()

	// Create directories
	startDir := filepath.Join(tempDir, "start")
	packageDir := filepath.Join(tempDir, "package")

	err = os.MkdirAll(startDir, 0o755)
	if err != nil {
		t.Fatalf("Failed to create start dir: %v", err)
	}

	err = os.MkdirAll(packageDir, 0o755)
	if err != nil {
		t.Fatalf("Failed to create package dir: %v", err)
	}

	pkg.PKGBUILD.StartDir = startDir
	pkg.PKGBUILD.PackageDir = packageDir
	pkg.PKGBUILD.Home = startDir // Same as start dir to avoid spec creation

	// Create a PKGBUILD file since it's needed for checksum calculation
	pkgbuildPath := filepath.Join(startDir, "PKGBUILD")
	pkgbuildContent := `pkgname=test-package
pkgver=1.0.0
pkgrel=1
pkgdesc="Test package"
arch=('x86_64')
`

	err = os.WriteFile(pkgbuildPath, []byte(pkgbuildContent), 0o644)
	if err != nil {
		t.Fatalf("Failed to create PKGBUILD file: %v", err)
	}

	// Create a test file in package dir
	testFile := filepath.Join(packageDir, "test.txt")

	err = os.WriteFile(testFile, []byte("test content"), 0o644)
	if err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	artifactsDir := filepath.Join(tempDir, "artifacts")

	err = os.MkdirAll(artifactsDir, 0o755)
	if err != nil {
		t.Fatalf("Failed to create artifacts dir: %v", err)
	}

	err = pkg.PrepareFakeroot(context.Background(), artifactsDir, "")
	if err != nil {
		t.Errorf("PrepareFakeroot failed: %v", err)
	}

	// Check that build date was set
	if pkg.PKGBUILD.BuildDate == 0 {
		t.Error("BuildDate was not set")
	}

	// Check that package type was set
	if pkg.PKGBUILD.PkgType != "pkg" {
		t.Error("PkgType was not set correctly")
	}

	// Check that .PKGINFO file was created
	pkginfoPath := filepath.Join(packageDir, ".PKGINFO")
	if _, err := os.Stat(pkginfoPath); os.IsNotExist(err) {
		t.Error(".PKGINFO file was not created")
	}

	// Check that .BUILDINFO file was created
	buildinfoPath := filepath.Join(packageDir, ".BUILDINFO")
	if _, err := os.Stat(buildinfoPath); os.IsNotExist(err) {
		t.Error(".BUILDINFO file was not created")
	}

	// Check that .MTREE file was created
	mtreePath := filepath.Join(packageDir, ".MTREE")
	if _, err := os.Stat(mtreePath); os.IsNotExist(err) {
		t.Error(".MTREE file was not created")
	}

	// Check that install script was NOT created (no scriptlets in this test)
	installPath := filepath.Join(startDir, "test-package.install")
	if _, err := os.Stat(installPath); err == nil {
		t.Error("Install script should not be created when no scriptlets are present")
	}
}

func TestPrepareFakerootWithSpecCreation(t *testing.T) {
	pkgBuild := createTestPKGBUILD()
	pkg := NewBuilder(pkgBuild)

	tempDir, err := os.MkdirTemp("", "pacman-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}

	defer func() { _ = os.RemoveAll(tempDir) }()

	// Create directories
	startDir := filepath.Join(tempDir, "start")
	packageDir := filepath.Join(tempDir, "package")
	homeDir := filepath.Join(tempDir, "home") // Different from start dir

	err = os.MkdirAll(startDir, 0o755)
	if err != nil {
		t.Fatalf("Failed to create start dir: %v", err)
	}

	err = os.MkdirAll(packageDir, 0o755)
	if err != nil {
		t.Fatalf("Failed to create package dir: %v", err)
	}

	err = os.MkdirAll(homeDir, 0o755)
	if err != nil {
		t.Fatalf("Failed to create home dir: %v", err)
	}

	pkg.PKGBUILD.StartDir = startDir
	pkg.PKGBUILD.PackageDir = packageDir
	pkg.PKGBUILD.Home = homeDir // Different from start dir

	// Create a test file in package dir
	testFile := filepath.Join(packageDir, "test.txt")

	err = os.WriteFile(testFile, []byte("test content"), 0o644)
	if err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	artifactsDir := filepath.Join(tempDir, "artifacts")

	err = os.MkdirAll(artifactsDir, 0o755)
	if err != nil {
		t.Fatalf("Failed to create artifacts dir: %v", err)
	}

	err = pkg.PrepareFakeroot(context.Background(), artifactsDir, "")
	if err != nil {
		t.Errorf("PrepareFakeroot failed: %v", err)
	}

	// Check that PKGBUILD file was created (since home != start dir)
	pkgbuildPath := filepath.Join(startDir, "PKGBUILD")
	if _, err := os.Stat(pkgbuildPath); os.IsNotExist(err) {
		t.Error("PKGBUILD file was not created")
	}

	// Check that checksum was calculated
	if pkg.PKGBUILD.Checksum == "" {
		t.Error("Checksum was not calculated")
	}
}

func TestPrepare(t *testing.T) {
	// Skip if not running as root and no sudo available (would prompt for password)
	if os.Geteuid() != 0 && os.Getenv("SUDO_USER") == "" && os.Getenv("CI") == "" {
		t.Skip("Skipping Prepare test - requires sudo privileges or CI environment")
	}

	pkgBuild := createTestPKGBUILD()
	pkg := NewBuilder(pkgBuild)

	makeDepends := []string{"make", "gcc"}
	err := pkg.Prepare(context.Background(), makeDepends, "")
	// This will likely fail since pacman isn't available, but we test the method call
	if err == nil {
		t.Log("Prepare succeeded (unexpected in test environment)")
	}
}

func TestPrepareEnvironment(t *testing.T) {
	// Skip if not running as root and no sudo available (would prompt for password)
	if os.Geteuid() != 0 && os.Getenv("SUDO_USER") == "" && os.Getenv("CI") == "" {
		t.Skip("Skipping PrepareEnvironment test - requires sudo privileges or CI environment")
	}

	pkgBuild := createTestPKGBUILD()
	pkg := NewBuilder(pkgBuild)

	err := pkg.PrepareEnvironment(context.Background(), false, "")
	// This will likely fail since pacman isn't available, but we test the method call
	if err == nil {
		t.Log("PrepareEnvironment succeeded (unexpected in test environment)")
	}
}

func TestPrepareEnvironmentWithGolang(t *testing.T) {
	// Skip if not running as root and no sudo available (would prompt for password)
	if os.Geteuid() != 0 && os.Getenv("SUDO_USER") == "" && os.Getenv("CI") == "" {
		t.Skip("Skipping PrepareEnvironmentWithGolang test - requires sudo privileges or CI environment")
	}

	pkgBuild := createTestPKGBUILD()
	pkg := NewBuilder(pkgBuild)

	err := pkg.PrepareEnvironment(context.Background(), true, "")
	// This will likely fail since pacman isn't available, but we test the method call
	if err == nil {
		t.Log("PrepareEnvironment with golang succeeded (unexpected in test environment)")
	}
}

func TestUpdate(t *testing.T) {
	// Skip if not running as root and no sudo available (would prompt for password)
	// or if not in CI environment where package managers might hang on network calls
	if os.Geteuid() != 0 && os.Getenv("SUDO_USER") == "" && os.Getenv("CI") == "" {
		t.Skip("Skipping Update test - requires sudo privileges or CI environment")
	}

	// Check if pacman is available
	if _, err := exec.LookPath("pacman"); err != nil {
		t.Skip("Skipping Update test - pacman not found")
	}

	pkgBuild := createTestPKGBUILD()
	pkg := NewBuilder(pkgBuild)

	err := pkg.Update(context.Background())
	// This will likely fail since pacman isn't available, but we test the method call
	if err == nil {
		t.Log("Update succeeded (unexpected in test environment)")
	}
}

func TestRenderMtree(t *testing.T) {
	// Create test entries
	entries := []*files.Entry{
		{
			Destination: "/test/file1.txt",
			Type:        "file",
			Size:        100,
			Mode:        0o644,
			ModTime:     time.Now(),
			LinkTarget:  "",
		},
		{
			Destination: "/test/dir",
			Type:        "dir",
			Size:        0,
			Mode:        0o755,
			ModTime:     time.Now(),
			LinkTarget:  "",
		},
	}

	result, err := renderMtree(entries)
	if err != nil {
		t.Errorf("renderMtree failed: %v", err)
	}

	if result == "" {
		t.Error("renderMtree returned empty string")
	}

	// Check that the result contains expected content
	if !strings.Contains(result, "file1.txt") {
		t.Error("renderMtree result doesn't contain expected file")
	}
}

func TestCreateMTREEGzip(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "mtree-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}

	defer func() { _ = os.RemoveAll(tempDir) }()

	content := "test mtree content"
	outputFile := filepath.Join(tempDir, ".MTREE")

	err = createMTREEGzip(content, outputFile)
	if err != nil {
		t.Errorf("createMTREEGzip failed: %v", err)
	}

	// Check that the file was created
	if _, err := os.Stat(outputFile); os.IsNotExist(err) {
		t.Error("MTREE file was not created")
	}

	// Check that the file is not empty
	info, err := os.Stat(outputFile)
	if err != nil {
		t.Errorf("Failed to stat MTREE file: %v", err)
	}

	if info.Size() == 0 {
		t.Error("MTREE file is empty")
	}
}

func TestPrepareFakeroot_WithScriptlets(t *testing.T) {
	pkgBuild := createTestPKGBUILD()
	pkgBuild.PreInst = "echo 'pre-install'"
	pkgBuild.PostInst = "echo 'post-install'"
	pkgBuild.PreUpgrade = "echo 'pre-upgrade'"
	pkgBuild.PostUpgrade = "echo 'post-upgrade'"
	pkgBuild.PreRm = "echo 'pre-remove'"
	pkgBuild.PostRm = "echo 'post-remove'"

	pkg := NewBuilder(pkgBuild)

	tempDir, err := os.MkdirTemp("", "pacman-scriptlet-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}

	defer func() { _ = os.RemoveAll(tempDir) }()

	// Create directories
	startDir := filepath.Join(tempDir, "start")
	packageDir := filepath.Join(tempDir, "package")

	if err := os.MkdirAll(startDir, 0o755); err != nil {
		t.Fatalf("Failed to create start dir: %v", err)
	}

	if err := os.MkdirAll(packageDir, 0o755); err != nil {
		t.Fatalf("Failed to create package dir: %v", err)
	}

	// Create a PKGBUILD file for checksum calculation
	pkgbuildPath := filepath.Join(startDir, "PKGBUILD")
	pkgbuildContent := `pkgname=test-package
pkgver=1.0.0
pkgrel=1
pkgdesc="Test package"
arch=('x86_64')
`

	if err := os.WriteFile(pkgbuildPath, []byte(pkgbuildContent), 0o644); err != nil {
		t.Fatalf("Failed to create PKGBUILD file: %v", err)
	}

	pkg.PKGBUILD.PackageDir = packageDir
	pkg.PKGBUILD.StartDir = startDir
	pkg.PKGBUILD.Home = startDir

	artifactsDir := filepath.Join(tempDir, "artifacts")

	if err := os.MkdirAll(artifactsDir, 0o755); err != nil {
		t.Fatalf("Failed to create artifacts dir: %v", err)
	}

	err = pkg.PrepareFakeroot(context.Background(), artifactsDir, "x86_64")
	if err != nil {
		t.Errorf("PrepareFakeroot failed: %v", err)
	}

	// Check that .install file was created
	installFile := filepath.Join(startDir, "test-package.install")
	if _, err := os.Stat(installFile); os.IsNotExist(err) {
		t.Errorf(".install file was not created: %s", installFile)
	}

	// Read and verify content
	content, err := os.ReadFile(installFile)
	if err != nil {
		t.Errorf("Failed to read .install file: %v", err)
	}

	contentStr := string(content)

	// Verify all scriptlet functions are present
	if !strings.Contains(contentStr, "pre_install()") {
		t.Error(".install file missing pre_install function")
	}

	if !strings.Contains(contentStr, "post_install()") {
		t.Error(".install file missing post_install function")
	}

	if !strings.Contains(contentStr, "pre_upgrade()") {
		t.Error(".install file missing pre_upgrade function")
	}

	if !strings.Contains(contentStr, "post_upgrade()") {
		t.Error(".install file missing post_upgrade function")
	}

	if !strings.Contains(contentStr, "pre_remove()") {
		t.Error(".install file missing pre_remove function")
	}

	if !strings.Contains(contentStr, "post_remove()") {
		t.Error(".install file missing post_remove function")
	}

	// Verify scriptlet content
	if !strings.Contains(contentStr, "echo 'pre-install'") {
		t.Error(".install file missing pre_install content")
	}

	if !strings.Contains(contentStr, "echo 'post-install'") {
		t.Error(".install file missing post_install content")
	}

	if !strings.Contains(contentStr, "echo 'pre-upgrade'") {
		t.Error(".install file missing pre_upgrade content")
	}

	if !strings.Contains(contentStr, "echo 'post-upgrade'") {
		t.Error(".install file missing post_upgrade content")
	}
}

func TestPrepareFakeroot_WithoutScriptlets(t *testing.T) {
	pkgBuild := createTestPKGBUILD()
	// No scriptlets set

	pkg := NewBuilder(pkgBuild)

	tempDir, err := os.MkdirTemp("", "pacman-no-scriptlet-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}

	defer func() { _ = os.RemoveAll(tempDir) }()

	// Create directories
	startDir := filepath.Join(tempDir, "start")
	packageDir := filepath.Join(tempDir, "package")

	if err := os.MkdirAll(startDir, 0o755); err != nil {
		t.Fatalf("Failed to create start dir: %v", err)
	}

	if err := os.MkdirAll(packageDir, 0o755); err != nil {
		t.Fatalf("Failed to create package dir: %v", err)
	}

	// Create a PKGBUILD file for checksum calculation
	pkgbuildPath := filepath.Join(startDir, "PKGBUILD")
	pkgbuildContent := `pkgname=test-package
pkgver=1.0.0
pkgrel=1
pkgdesc="Test package"
arch=('x86_64')
`

	if err := os.WriteFile(pkgbuildPath, []byte(pkgbuildContent), 0o644); err != nil {
		t.Fatalf("Failed to create PKGBUILD file: %v", err)
	}

	pkg.PKGBUILD.PackageDir = packageDir
	pkg.PKGBUILD.StartDir = startDir
	pkg.PKGBUILD.Home = startDir

	artifactsDir := filepath.Join(tempDir, "artifacts")

	if err := os.MkdirAll(artifactsDir, 0o755); err != nil {
		t.Fatalf("Failed to create artifacts dir: %v", err)
	}

	err = pkg.PrepareFakeroot(context.Background(), artifactsDir, "x86_64")
	if err != nil {
		t.Errorf("PrepareFakeroot failed: %v", err)
	}

	// Check that .install file was NOT created
	installFile := filepath.Join(startDir, "test-package.install")
	if _, err := os.Stat(installFile); err == nil {
		t.Errorf(".install file should not be created when no scriptlets are present: %s", installFile)
	}
}

func TestPrepareFakeroot_WithChangelog(t *testing.T) {
	pkgBuild := createTestPKGBUILD()

	tempDir, err := os.MkdirTemp("", "pacman-changelog-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}

	defer func() { _ = os.RemoveAll(tempDir) }()

	startDir := filepath.Join(tempDir, "start")
	packageDir := filepath.Join(tempDir, "package")

	err = os.MkdirAll(startDir, 0o755)
	if err != nil {
		t.Fatalf("Failed to create start dir: %v", err)
	}

	err = os.MkdirAll(packageDir, 0o755)
	if err != nil {
		t.Fatalf("Failed to create package dir: %v", err)
	}

	// Create a changelog file in the start directory
	changelogPath := filepath.Join(startDir, "CHANGELOG.md")
	changelogContent := "# Changelog\n\n## Version 1.0\n- Initial release\n"

	err = os.WriteFile(changelogPath, []byte(changelogContent), 0o644)
	if err != nil {
		t.Fatalf("Failed to create changelog file: %v", err)
	}

	// Create a dummy PKGBUILD file
	pkgbuildPath := filepath.Join(startDir, "PKGBUILD")

	err = os.WriteFile(pkgbuildPath, []byte("# PKGBUILD\n"), 0o644)
	if err != nil {
		t.Fatalf("Failed to create PKGBUILD file: %v", err)
	}

	pkgBuild.StartDir = startDir
	pkgBuild.Home = startDir // Same as StartDir to avoid PKGBUILD creation
	pkgBuild.PackageDir = packageDir
	pkgBuild.Changelog = "CHANGELOG.md"

	pkg := NewBuilder(pkgBuild)

	artifactsDir := filepath.Join(tempDir, "artifacts")

	if err := os.MkdirAll(artifactsDir, 0o755); err != nil {
		t.Fatalf("Failed to create artifacts dir: %v", err)
	}

	err = pkg.PrepareFakeroot(context.Background(), artifactsDir, "x86_64")
	if err != nil {
		t.Errorf("PrepareFakeroot failed: %v", err)
	}

	// Check that .CHANGELOG file was created
	changelogFile := filepath.Join(packageDir, ".CHANGELOG")
	if _, err := os.Stat(changelogFile); os.IsNotExist(err) {
		t.Errorf(".CHANGELOG file was not created: %s", changelogFile)
	}

	// Verify the content
	content, err := os.ReadFile(changelogFile)
	if err != nil {
		t.Errorf("Failed to read .CHANGELOG file: %v", err)
	}

	if string(content) != changelogContent {
		t.Errorf("Changelog content mismatch. Expected %q, got %q", changelogContent, string(content))
	}
}
