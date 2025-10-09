package apk

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/M0Rf30/yap/v2/pkg/pkgbuild"
)

func createTestPKGBUILD() *pkgbuild.PKGBUILD {
	return &pkgbuild.PKGBUILD{
		PkgName:    "test-package",
		PkgVer:     "1.0.0",
		PkgRel:     "1",
		Arch:       []string{"x86_64"},
		PkgDesc:    "Test package description",
		Maintainer: "test@example.com",
	}
}

func TestNewBuilder(t *testing.T) {
	pkgBuild := createTestPKGBUILD()
	builder := NewBuilder(pkgBuild)

	if builder == nil {
		t.Fatal("NewBuilder returned nil")
	}

	if builder.BaseBuilder == nil {
		t.Fatal("BaseBuilder is nil")
	}
}

func TestBuildPackage(t *testing.T) {
	pkgBuild := createTestPKGBUILD()
	builder := NewBuilder(pkgBuild)

	tempDir, err := os.MkdirTemp("", "apk-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}

	defer func() { _ = os.RemoveAll(tempDir) }()

	// Create a fake package directory with some content
	pkgDir := filepath.Join(tempDir, "pkg")

	err = os.MkdirAll(pkgDir, 0o755)
	if err != nil {
		t.Fatalf("Failed to create package dir: %v", err)
	}

	builder.PKGBUILD.PackageDir = pkgDir
	builder.PKGBUILD.ArchComputed = "x86_64" // Set computed arch before building

	// Create a test file in the package
	testFile := filepath.Join(pkgDir, "testfile")

	err = os.WriteFile(testFile, []byte("test content"), 0o644)
	if err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	err = builder.BuildPackage(tempDir)
	if err != nil {
		t.Errorf("BuildPackage failed: %v", err)
	}

	// Check that the package was created
	// APK format is: name-version-release.arch.apk
	expectedPkg := filepath.Join(tempDir, "test-package-1.0.0-1.x86_64.apk")
	if _, err := os.Stat(expectedPkg); os.IsNotExist(err) {
		t.Errorf("Package file was not created: %s", expectedPkg)
	}
}

func TestPrepareFakeroot(t *testing.T) {
	pkgBuild := createTestPKGBUILD()
	builder := NewBuilder(pkgBuild)

	tempDir, err := os.MkdirTemp("", "apk-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}

	defer func() { _ = os.RemoveAll(tempDir) }()

	// Create a fake package directory
	pkgDir := filepath.Join(tempDir, "pkg")

	err = os.MkdirAll(pkgDir, 0o755)
	if err != nil {
		t.Fatalf("Failed to create package dir: %v", err)
	}

	builder.PKGBUILD.PackageDir = pkgDir

	err = builder.PrepareFakeroot(tempDir)
	if err != nil {
		t.Errorf("PrepareFakeroot failed: %v", err)
	}

	// Build date should be set
	if builder.PKGBUILD.BuildDate == 0 {
		t.Error("BuildDate was not set")
	}

	// Check that .PKGINFO was created
	pkginfoPath := filepath.Join(pkgDir, ".PKGINFO")
	if _, err := os.Stat(pkginfoPath); os.IsNotExist(err) {
		t.Error(".PKGINFO file was not created")
	}
}

func TestPrepareFakerootWithScripts(t *testing.T) {
	pkgBuild := createTestPKGBUILD()
	pkgBuild.PreInst = "echo 'pre-install'"
	pkgBuild.PostInst = "echo 'post-install'"
	builder := NewBuilder(pkgBuild)

	tempDir, err := os.MkdirTemp("", "apk-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}

	defer func() { _ = os.RemoveAll(tempDir) }()

	// Create a fake package directory
	pkgDir := filepath.Join(tempDir, "pkg")

	err = os.MkdirAll(pkgDir, 0o755)
	if err != nil {
		t.Fatalf("Failed to create package dir: %v", err)
	}

	builder.PKGBUILD.PackageDir = pkgDir

	err = builder.PrepareFakeroot(tempDir)
	if err != nil {
		t.Errorf("PrepareFakeroot with scripts failed: %v", err)
	}

	// Check that install script was created
	installPath := filepath.Join(pkgDir, ".install")
	if _, err := os.Stat(installPath); os.IsNotExist(err) {
		t.Error(".install file was not created")
	}
}

func TestInstall(t *testing.T) {
	// Skip if not running as root and no sudo available (would prompt for password)
	if os.Geteuid() != 0 && os.Getenv("SUDO_USER") == "" && os.Getenv("CI") == "" {
		t.Skip("Skipping Install test - requires sudo privileges or CI environment")
	}

	pkgBuild := createTestPKGBUILD()
	builder := NewBuilder(pkgBuild)

	tempDir, err := os.MkdirTemp("", "apk-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}

	defer func() { _ = os.RemoveAll(tempDir) }()

	// This will likely fail since apk isn't installed, but we test the method call
	err = builder.Install(tempDir)
	// We expect this to fail in most test environments, so we just check that it doesn't panic
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
	builder := NewBuilder(pkgBuild)

	makeDepends := []string{"make", "gcc"}
	err := builder.Prepare(makeDepends)
	// This will likely fail since apk isn't installed, but we test the method call
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
	builder := NewBuilder(pkgBuild)

	err := builder.PrepareEnvironment(false)
	// This will likely fail since apk isn't installed, but we test the method call
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
	builder := NewBuilder(pkgBuild)

	err := builder.PrepareEnvironment(true)
	// This will likely fail since apk isn't installed, but we test the method call
	if err == nil {
		t.Log("PrepareEnvironment with golang succeeded (unexpected in test environment)")
	}
}

func TestUpdate(t *testing.T) {
	// Skip if not running as root and no sudo available (would prompt for password)
	if os.Geteuid() != 0 && os.Getenv("SUDO_USER") == "" && os.Getenv("CI") == "" {
		t.Skip("Skipping Update test - requires sudo privileges or CI environment")
	}

	// Check if apk is available
	if _, err := exec.LookPath("apk"); err != nil {
		t.Skip("Skipping Update test - apk not found")
	}

	pkgBuild := createTestPKGBUILD()
	builder := NewBuilder(pkgBuild)

	err := builder.Update()
	// This will likely fail since apk isn't installed, but we test the method call
	if err == nil {
		t.Log("Update succeeded (unexpected in test environment)")
	}
}

func TestCreateAPKPackage(t *testing.T) {
	pkgBuild := createTestPKGBUILD()
	builder := NewBuilder(pkgBuild)

	tempDir, err := os.MkdirTemp("", "apk-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}

	defer func() { _ = os.RemoveAll(tempDir) }()

	// Create a package directory with content
	pkgDir := filepath.Join(tempDir, "pkg")

	err = os.MkdirAll(pkgDir, 0o755)
	if err != nil {
		t.Fatalf("Failed to create package dir: %v", err)
	}

	builder.PKGBUILD.PackageDir = pkgDir

	// Create a test file
	testFile := filepath.Join(pkgDir, "testfile")

	err = os.WriteFile(testFile, []byte("test content"), 0o644)
	if err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	pkgFilePath := filepath.Join(tempDir, "test.apk")

	err = builder.createAPKPackage(pkgFilePath, tempDir)
	if err != nil {
		t.Errorf("createAPKPackage failed: %v", err)
	}

	// Check that package file was created
	if _, err := os.Stat(pkgFilePath); os.IsNotExist(err) {
		t.Error("APK package file was not created")
	}
}

func TestCreatePkgInfo(t *testing.T) {
	pkgBuild := createTestPKGBUILD()
	builder := NewBuilder(pkgBuild)

	tempDir, err := os.MkdirTemp("", "apk-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}

	defer func() { _ = os.RemoveAll(tempDir) }()

	builder.PKGBUILD.PackageDir = tempDir
	builder.PKGBUILD.ArchComputed = "x86_64"

	err = builder.createPkgInfo()
	if err != nil {
		t.Errorf("createPkgInfo failed: %v", err)
	}

	// Check that .PKGINFO was created
	pkginfoPath := filepath.Join(tempDir, ".PKGINFO")
	if _, err := os.Stat(pkginfoPath); os.IsNotExist(err) {
		t.Error(".PKGINFO file was not created")
	}

	// Read and verify content
	content, err := os.ReadFile(pkginfoPath)
	if err != nil {
		t.Fatalf("Failed to read .PKGINFO: %v", err)
	}

	contentStr := string(content)
	if !contains(contentStr, "pkgname = test-package") {
		t.Error(".PKGINFO missing package name")
	}

	if !contains(contentStr, "pkgver = 1.0.0-r1") {
		t.Error(".PKGINFO missing version")
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > len(substr) && containsInner(s, substr))
}

func containsInner(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}

	return false
}

func TestCreateInstallScript(t *testing.T) {
	pkgBuild := createTestPKGBUILD()
	pkgBuild.PreInst = "echo 'pre-install'"
	pkgBuild.PostInst = "echo 'post-install'"
	builder := NewBuilder(pkgBuild)

	tempDir, err := os.MkdirTemp("", "apk-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}

	defer func() { _ = os.RemoveAll(tempDir) }()

	builder.PKGBUILD.PackageDir = tempDir

	err = builder.createInstallScript()
	if err != nil {
		t.Errorf("createInstallScript failed: %v", err)
	}

	// Check that .install was created
	installPath := filepath.Join(tempDir, ".install")
	if _, err := os.Stat(installPath); os.IsNotExist(err) {
		t.Error(".install file was not created")
	}

	// Read and verify content
	content, err := os.ReadFile(installPath)
	if err != nil {
		t.Fatalf("Failed to read .install: %v", err)
	}

	contentStr := string(content)
	if !contains(contentStr, "#!/bin/sh") {
		t.Error(".install missing shebang")
	}

	if !contains(contentStr, "pre_install()") {
		t.Error(".install missing pre_install function")
	}

	if !contains(contentStr, "post_install()") {
		t.Error(".install missing post_install function")
	}
}
