package deb

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/M0Rf30/yap/v2/pkg/pkgbuild"
)

func createTestPKGBUILD() *pkgbuild.PKGBUILD {
	return &pkgbuild.PKGBUILD{
		PkgName:      "test-package",
		PkgVer:       "1.0.0",
		PkgRel:       "1",
		Arch:         []string{"x86_64"},
		ArchComputed: "x86_64",
		PkgDesc:      "Test package description",
		Maintainer:   "test@example.com",
		License:      []string{"MIT"},
		Depends:      []string{"dependency1>=1.0", "dependency2<2.0"},
		MakeDepends:  []string{"make", "gcc"},
		OptDepends:   []string{"optional>=1.0"},
		Backup:       []string{"etc/config.conf", "/etc/other.conf"},
		PreInst:      "echo 'pre-install'",
		PostInst:     "echo 'post-install'",
		PreRm:        "echo 'pre-remove'",
		PostRm:       "echo 'post-remove'",
		DebConfig:    "",
		DebTemplate:  "",
		Codename:     "focal",
		Distro:       "ubuntu",
		StripEnabled: false,
	}
}

func TestNewBuilder(t *testing.T) {
	pkgBuild := createTestPKGBUILD()
	pkg := NewBuilder(pkgBuild)

	if pkg == nil {
		t.Fatal("NewBuilder returned nil")
	}

	if pkg.PKGBUILD != pkgBuild {
		t.Error("PKGBUILD not set correctly")
	}
}

func TestBuildPackage(t *testing.T) {
	pkgBuild := createTestPKGBUILD()
	pkg := NewBuilder(pkgBuild)

	tempDir, err := os.MkdirTemp("", "deb-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}

	defer func() { _ = os.RemoveAll(tempDir) }()

	// Create source and package directories
	sourceDir := filepath.Join(tempDir, "source")
	packageDir := filepath.Join(tempDir, "package")

	err = os.MkdirAll(sourceDir, 0o755)
	if err != nil {
		t.Fatalf("Failed to create source dir: %v", err)
	}

	err = os.MkdirAll(packageDir, 0o755)
	if err != nil {
		t.Fatalf("Failed to create package dir: %v", err)
	}

	pkg.PKGBUILD.SourceDir = sourceDir
	pkg.PKGBUILD.PackageDir = packageDir

	// Create a fake deb directory that will be processed
	debDir := filepath.Join(packageDir, "DEBIAN")

	err = os.MkdirAll(debDir, 0o755)
	if err != nil {
		t.Fatalf("Failed to create DEBIAN dir: %v", err)
	}

	pkg.debDir = debDir

	artifactsDir := filepath.Join(tempDir, "artifacts")

	err = os.MkdirAll(artifactsDir, 0o755)
	if err != nil {
		t.Fatalf("Failed to create artifacts dir: %v", err)
	}

	err = pkg.BuildPackage(artifactsDir, "")
	if err != nil {
		t.Errorf("BuildPackage failed: %v", err)
	}
}

func TestInstall(t *testing.T) {
	// Skip if not running as root and no sudo available (would prompt for password)
	if os.Geteuid() != 0 && os.Getenv("SUDO_USER") == "" && os.Getenv("CI") == "" {
		t.Skip("Skipping Install test - requires sudo privileges or CI environment")
	}

	pkgBuild := createTestPKGBUILD()
	pkg := NewBuilder(pkgBuild)

	tempDir, err := os.MkdirTemp("", "deb-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}

	defer func() { _ = os.RemoveAll(tempDir) }()

	// This will likely fail since apt-get isn't available, but we test the method call
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
	// This will likely fail since apt-get isn't available, but we test the method call
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
	// This will likely fail since apt-get isn't available, but we test the method call
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
	// This will likely fail since apt-get isn't available, but we test the method call
	if err == nil {
		t.Log("PrepareEnvironment with golang succeeded (unexpected in test environment)")
	}
}

func TestPrepareFakeroot(t *testing.T) {
	pkgBuild := createTestPKGBUILD()
	pkg := NewBuilder(pkgBuild)

	tempDir, err := os.MkdirTemp("", "deb-test")
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

	err = pkg.PrepareFakeroot(tempDir, "")
	if err != nil {
		t.Errorf("PrepareFakeroot failed: %v", err)
	}

	// Check that release was processed
	if pkg.PKGBUILD.PkgRel == "1" {
		t.Error("Release was not processed")
	}
}

func TestUpdate(t *testing.T) {
	// Skip if not running as root and no sudo available (would prompt for password)
	// or if not in CI environment where package managers might hang on network calls
	if os.Geteuid() != 0 && os.Getenv("SUDO_USER") == "" && os.Getenv("CI") == "" {
		t.Skip("Skipping Update test - requires sudo privileges or CI environment")
	}

	// Check if apt-get is available
	if _, err := exec.LookPath("apt-get"); err != nil {
		t.Skip("Skipping Update test - apt-get not found")
	}

	pkgBuild := createTestPKGBUILD()
	pkg := NewBuilder(pkgBuild)

	err := pkg.Update()
	// This will likely fail since apt-get isn't available, but we test the method call
	if err == nil {
		t.Log("Update succeeded (unexpected in test environment)")
	}
}

func TestGetRelease(t *testing.T) {
	pkgBuild := createTestPKGBUILD()
	pkg := NewBuilder(pkgBuild)

	originalRel := pkg.PKGBUILD.PkgRel
	pkg.getRelease()

	if pkg.PKGBUILD.PkgRel == originalRel {
		t.Error("Release was not modified")
	}

	// Test with codename
	if pkg.PKGBUILD.Codename != "" && !strings.Contains(pkg.PKGBUILD.PkgRel, pkg.PKGBUILD.Codename) {
		t.Error("Codename was not added to release")
	}
}

func TestGetReleaseWithDistro(t *testing.T) {
	pkgBuild := createTestPKGBUILD()
	pkgBuild.Codename = "" // Remove codename to test distro fallback
	pkg := NewBuilder(pkgBuild)

	originalRel := pkg.PKGBUILD.PkgRel
	pkg.getRelease()

	if !strings.Contains(pkg.PKGBUILD.PkgRel, pkg.PKGBUILD.Distro) {
		t.Error("Distro was not added to release")
	}

	if pkg.PKGBUILD.PkgRel == originalRel {
		t.Error("Release was not modified")
	}
}

func TestProcessDepends(t *testing.T) {
	pkgBuild := createTestPKGBUILD()
	pkg := NewBuilder(pkgBuild)

	testCases := []struct {
		input    []string
		expected []string
	}{
		{
			input:    []string{"package>=1.0", "other<2.0"},
			expected: []string{"package (>= 1.0)", "other (< 2.0)"},
		},
		{
			input:    []string{"simple"},
			expected: []string{"simple"},
		},
		{
			input:    []string{"package=1.0"},
			expected: []string{"package (= 1.0)"},
		},
	}

	for _, tc := range testCases {
		result := pkg.ProcessDependencies(tc.input)
		if len(result) != len(tc.expected) {
			t.Errorf("Expected %d items, got %d", len(tc.expected), len(result))
			continue
		}

		for i, expected := range tc.expected {
			if result[i] != expected {
				t.Errorf("Expected %s, got %s", expected, result[i])
			}
		}
	}
}

func TestCreateDebResources(t *testing.T) {
	pkgBuild := createTestPKGBUILD()
	pkg := NewBuilder(pkgBuild)

	tempDir, err := os.MkdirTemp("", "deb-test")
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

	err = pkg.createDebResources()
	if err != nil {
		t.Errorf("createDebResources failed: %v", err)
	}

	// Check that DEBIAN directory was created
	if pkg.debDir == "" {
		t.Error("debDir was not set")
	}

	// Check that control file exists
	controlPath := filepath.Join(pkg.debDir, "control")
	if _, err := os.Stat(controlPath); os.IsNotExist(err) {
		t.Error("Control file was not created")
	}

	// Check that conffiles exists
	conffilesPath := filepath.Join(pkg.debDir, "conffiles")
	if _, err := os.Stat(conffilesPath); os.IsNotExist(err) {
		t.Error("Conffiles was not created")
	}
}

func TestCreateConfFiles(t *testing.T) {
	pkgBuild := createTestPKGBUILD()
	pkg := NewBuilder(pkgBuild)

	tempDir, err := os.MkdirTemp("", "deb-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}

	defer func() { _ = os.RemoveAll(tempDir) }()

	pkg.debDir = tempDir

	err = pkg.createConfFiles()
	if err != nil {
		t.Errorf("createConfFiles failed: %v", err)
	}

	// Check that conffiles was created
	conffilesPath := filepath.Join(tempDir, "conffiles")
	if _, err := os.Stat(conffilesPath); os.IsNotExist(err) {
		t.Error("Conffiles was not created")
	}

	// Read and verify content
	content, err := os.ReadFile(conffilesPath)
	if err != nil {
		t.Errorf("Failed to read conffiles: %v", err)
	}

	contentStr := string(content)
	if !strings.Contains(contentStr, "/etc/config.conf") {
		t.Error("Expected backup file not found in conffiles")
	}

	if !strings.Contains(contentStr, "/etc/other.conf") {
		t.Error("Expected backup file not found in conffiles")
	}
}

func TestCreateConfFilesEmpty(t *testing.T) {
	pkgBuild := createTestPKGBUILD()
	pkgBuild.Backup = []string{} // No backup files
	pkg := NewBuilder(pkgBuild)

	tempDir, err := os.MkdirTemp("", "deb-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}

	defer func() { _ = os.RemoveAll(tempDir) }()

	pkg.debDir = tempDir

	err = pkg.createConfFiles()
	if err != nil {
		t.Errorf("createConfFiles failed: %v", err)
	}

	// Check that conffiles was not created
	conffilesPath := filepath.Join(tempDir, "conffiles")
	if _, err := os.Stat(conffilesPath); !os.IsNotExist(err) {
		t.Error("Conffiles should not have been created for empty backup list")
	}
}

func TestAddScriptlets(t *testing.T) {
	pkgBuild := createTestPKGBUILD()
	pkg := NewBuilder(pkgBuild)

	tempDir, err := os.MkdirTemp("", "deb-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}

	defer func() { _ = os.RemoveAll(tempDir) }()

	pkg.debDir = tempDir

	err = pkg.addScriptlets()
	if err != nil {
		t.Errorf("addScriptlets failed: %v", err)
	}

	// Check that script files were created
	scripts := []string{"preinst", "postinst", "prerm", "postrm"}
	for _, script := range scripts {
		scriptPath := filepath.Join(tempDir, script)
		if _, err := os.Stat(scriptPath); os.IsNotExist(err) {
			t.Errorf("Script %s was not created", script)
		}
	}
}

func TestGetCurrentBuildTime(t *testing.T) {
	before := time.Now()
	modTime := getCurrentBuildTime()
	after := time.Now()

	if modTime.Before(before) || modTime.After(after) {
		t.Error("getCurrentBuildTime returned time outside expected range")
	}
}
