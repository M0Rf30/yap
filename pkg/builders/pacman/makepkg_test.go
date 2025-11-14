package pacman

import (
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

	err = pkg.BuildPackage(artifactsDir, "")
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

	err = pkg.BuildPackage(artifactsDir, "")
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

	err = pkg.PrepareFakeroot(artifactsDir, "")
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

	// Check that install script was created
	installPath := filepath.Join(startDir, "test-package.install")
	if _, err := os.Stat(installPath); os.IsNotExist(err) {
		t.Error("Install script was not created")
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

	err = pkg.PrepareFakeroot(artifactsDir, "")
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

func TestInstall(t *testing.T) {
	// Skip if not running as root and no sudo available (would prompt for password)
	if os.Geteuid() != 0 && os.Getenv("SUDO_USER") == "" && os.Getenv("CI") == "" {
		t.Skip("Skipping Install test - requires sudo privileges or CI environment")
	}

	pkgBuild := createTestPKGBUILD()
	pkg := NewBuilder(pkgBuild)

	tempDir, err := os.MkdirTemp("", "pacman-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}

	defer func() { _ = os.RemoveAll(tempDir) }()

	// This will likely fail since pacman isn't available, but we test the method call
	err = pkg.Install(tempDir)
	if err == nil {
		t.Log("Install succeeded (unexpected in test environment)")
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
	err := pkg.Prepare(makeDepends, "")
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

	err := pkg.PrepareEnvironment(false, "")
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

	err := pkg.PrepareEnvironment(true, "")
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

	err := pkg.Update()
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
