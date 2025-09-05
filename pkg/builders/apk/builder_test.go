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

	err = builder.BuildPackage(tempDir)
	if err != nil {
		t.Errorf("BuildPackage failed: %v", err)
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

	// Check that build date was set
	if builder.PKGBUILD.BuildDate == 0 {
		t.Error("BuildDate was not set")
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

	pkgFilePath := filepath.Join(tempDir, "test.apk")

	err = builder.createAPKPackage(pkgFilePath, tempDir)
	if err != nil {
		t.Errorf("createAPKPackage failed: %v", err)
	}
}

func TestCreatePkgInfo(t *testing.T) {
	pkgBuild := createTestPKGBUILD()
	builder := NewBuilder(pkgBuild)

	err := builder.createPkgInfo()
	if err != nil {
		t.Errorf("createPkgInfo failed: %v", err)
	}
}

func TestCreateInstallScript(t *testing.T) {
	pkgBuild := createTestPKGBUILD()
	builder := NewBuilder(pkgBuild)

	err := builder.createInstallScript()
	if err != nil {
		t.Errorf("createInstallScript failed: %v", err)
	}
}
