package rpm

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	rpmpack "github.com/google/rpmpack"

	"github.com/M0Rf30/yap/v2/pkg/builders/common"
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
		Copyright:    []string{"Copyright 2023"},
		URL:          "https://example.com",
		Section:      "misc",
		Depends:      []string{"dependency1>=1.0", "dependency2<2.0"},
		MakeDepends:  []string{"make", "gcc"},
		OptDepends:   []string{"optional>=1.0"},
		Replaces:     []string{"old-package"},
		Provides:     []string{"virtual-package"},
		Conflicts:    []string{"conflicting-package"},
		Backup:       []string{"etc/config.conf", "/etc/other.conf"},
		PreTrans:     "echo 'pre-transaction'",
		PreInst:      "echo 'pre-install'",
		PostInst:     "echo 'post-install'",
		PreRm:        "echo 'pre-remove'",
		PostRm:       "echo 'post-remove'",
		PostTrans:    "echo 'post-transaction'",
		Codename:     "fc38",
		Distro:       "fedora",
		StripEnabled: false,
	}
}

func TestBuildPackage(t *testing.T) {
	pkgBuild := createTestPKGBUILD()
	rpm := &RPM{BaseBuilder: common.NewBaseBuilder(pkgBuild, "rpm")}

	tempDir, err := os.MkdirTemp("", "rpm-test")
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

	rpm.PKGBUILD.PackageDir = packageDir

	artifactsDir := filepath.Join(tempDir, "artifacts")

	err = os.MkdirAll(artifactsDir, 0o755)
	if err != nil {
		t.Fatalf("Failed to create artifacts dir: %v", err)
	}

	err = rpm.BuildPackage(artifactsDir, "")
	if err != nil {
		t.Errorf("BuildPackage failed: %v", err)
	}

	// Check that package file was created
	expectedPkgName := "test-package-1.0.0-1.x86_64.rpm"

	pkgFilePath := filepath.Join(artifactsDir, expectedPkgName)
	if _, err := os.Stat(pkgFilePath); os.IsNotExist(err) {
		t.Errorf("Package file was not created: %s", pkgFilePath)
	}
}

func TestBuildPackageWithoutEpoch(t *testing.T) {
	pkgBuild := createTestPKGBUILD()
	pkgBuild.Epoch = "" // Remove epoch
	rpm := &RPM{BaseBuilder: common.NewBaseBuilder(pkgBuild, "rpm")}

	tempDir, err := os.MkdirTemp("", "rpm-test")
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

	rpm.PKGBUILD.PackageDir = packageDir

	artifactsDir := filepath.Join(tempDir, "artifacts")

	err = os.MkdirAll(artifactsDir, 0o755)
	if err != nil {
		t.Fatalf("Failed to create artifacts dir: %v", err)
	}

	err = rpm.BuildPackage(artifactsDir, "")
	if err != nil {
		t.Errorf("BuildPackage failed: %v", err)
	}
}

func TestPrepareFakeroot(t *testing.T) {
	pkgBuild := createTestPKGBUILD()
	rpm := &RPM{BaseBuilder: common.NewBaseBuilder(pkgBuild, "rpm")}

	tempDir, err := os.MkdirTemp("", "rpm-test")
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

	rpm.PKGBUILD.PackageDir = packageDir

	err = rpm.PrepareFakeroot(tempDir, "")
	if err != nil {
		t.Errorf("PrepareFakeroot failed: %v", err)
	}

	// Check that section was processed
	if rpm.PKGBUILD.Section == "misc" {
		t.Error("Section was not processed")
	}
}

func TestInstall(t *testing.T) {
	// Skip if not running as root and no sudo available (would prompt for password)
	if os.Geteuid() != 0 && os.Getenv("SUDO_USER") == "" && os.Getenv("CI") == "" {
		t.Skip("Skipping Install test - requires sudo privileges or CI environment")
	}

	pkgBuild := createTestPKGBUILD()
	rpm := &RPM{BaseBuilder: common.NewBaseBuilder(pkgBuild, "rpm")}

	tempDir, err := os.MkdirTemp("", "rpm-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}

	defer func() { _ = os.RemoveAll(tempDir) }()

	// This will likely fail since dnf isn't available, but we test the method call
	err = rpm.Install(tempDir)
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
	rpm := &RPM{BaseBuilder: common.NewBaseBuilder(pkgBuild, "rpm")}

	makeDepends := []string{"make", "gcc"}
	err := rpm.Prepare(makeDepends, "")
	// This will likely fail since dnf isn't available, but we test the method call
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
	rpm := &RPM{BaseBuilder: common.NewBaseBuilder(pkgBuild, "rpm")}

	err := rpm.PrepareEnvironment(false, "")
	// This will likely fail since dnf isn't available, but we test the method call
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
	rpm := &RPM{BaseBuilder: common.NewBaseBuilder(pkgBuild, "rpm")}

	err := rpm.PrepareEnvironment(true, "")
	// This will likely fail since dnf isn't available, but we test the method call
	if err == nil {
		t.Log("PrepareEnvironment with golang succeeded (unexpected in test environment)")
	}
}

func TestUpdate(t *testing.T) {
	// Skip if not running as root and no sudo available (would prompt for password)
	if os.Geteuid() != 0 && os.Getenv("SUDO_USER") == "" && os.Getenv("CI") == "" {
		t.Skip("Skipping Update test - requires sudo privileges or CI environment")
	}

	pkgBuild := createTestPKGBUILD()
	rpm := &RPM{BaseBuilder: common.NewBaseBuilder(pkgBuild, "rpm")}

	err := rpm.Update()
	// This will likely fail since dnf isn't available, but we test the method call
	if err == nil {
		t.Log("Update succeeded (unexpected in test environment)")
	}
}

func TestGetGroup(t *testing.T) {
	pkgBuild := createTestPKGBUILD()
	rpm := &RPM{BaseBuilder: common.NewBaseBuilder(pkgBuild, "rpm")}

	originalSection := rpm.PKGBUILD.Section
	rpm.getGroup()

	// The section should be processed (may be the same if not in the map)
	if rpm.PKGBUILD.Section == "" {
		t.Error("Section was cleared")
	}

	t.Logf("Original section: %s, Processed section: %s", originalSection, rpm.PKGBUILD.Section)
}

func TestGetRelease(t *testing.T) {
	pkgBuild := createTestPKGBUILD()
	rpm := &RPM{BaseBuilder: common.NewBaseBuilder(pkgBuild, "rpm")}

	originalRel := rpm.PKGBUILD.PkgRel
	rpm.getRelease()

	// Release should be modified if codename is present
	if rpm.PKGBUILD.Codename != "" && rpm.PKGBUILD.PkgRel == originalRel {
		t.Error("Release was not modified despite having codename")
	}
}

func TestGetReleaseWithoutCodename(t *testing.T) {
	pkgBuild := createTestPKGBUILD()
	pkgBuild.Codename = "" // Remove codename
	rpm := &RPM{BaseBuilder: common.NewBaseBuilder(pkgBuild, "rpm")}

	originalRel := rpm.PKGBUILD.PkgRel
	rpm.getRelease()

	// Release should not be modified without codename
	if rpm.PKGBUILD.PkgRel != originalRel {
		t.Error("Release was modified without codename")
	}
}

func TestPrepareBackupFiles(t *testing.T) {
	pkgBuild := createTestPKGBUILD()
	rpm := &RPM{BaseBuilder: common.NewBaseBuilder(pkgBuild, "rpm")}

	backupFiles := rpm.PrepareBackupFilePaths()

	if len(backupFiles) != len(rpm.PKGBUILD.Backup) {
		t.Errorf("Expected %d backup files, got %d", len(rpm.PKGBUILD.Backup), len(backupFiles))
	}

	// All backup files should have leading slash
	for _, file := range backupFiles {
		if !strings.HasPrefix(file, "/") {
			t.Errorf("Backup file %s doesn't have leading slash", file)
		}
	}

	// Check specific files
	expectedFiles := []string{"/etc/config.conf", "/etc/other.conf"}
	for i, expected := range expectedFiles {
		if i < len(backupFiles) && backupFiles[i] != expected {
			t.Errorf("Expected backup file %s, got %s", expected, backupFiles[i])
		}
	}
}

func TestProcessDepends(t *testing.T) {
	testCases := []struct {
		input    []string
		expected []string
	}{
		{
			input:    []string{"package>=1.0", "other<2.0"},
			expected: []string{"package >= 1.0", "other < 2.0"},
		},
		{
			input:    []string{"simple"},
			expected: []string{"simple"},
		},
		{
			input:    []string{"package=1.0"},
			expected: []string{"package = 1.0"},
		},
	}

	rpm := &RPM{BaseBuilder: common.NewBaseBuilder(createTestPKGBUILD(), "rpm")}
	for _, tc := range testCases {
		result := rpm.processDepends(tc.input)
		if result == nil {
			t.Error("processDepends returned nil")
			continue
		}

		if len(result) != len(tc.expected) {
			t.Errorf("Expected %d items, got %d", len(tc.expected), len(result))
			continue
		}

		// Convert relations back to strings for comparison
		for i, expected := range tc.expected {
			if i < len(result) {
				resultStr := result[i].String()
				if !strings.Contains(resultStr, strings.Fields(expected)[0]) {
					t.Errorf("Expected relation containing %s, got %s", expected, resultStr)
				}
			}
		}
	}
}

func TestCreateFilesInsideRPM(t *testing.T) {
	pkgBuild := createTestPKGBUILD()
	rpm := &RPM{BaseBuilder: common.NewBaseBuilder(pkgBuild, "rpm")}

	tempDir, err := os.MkdirTemp("", "rpm-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}

	defer func() { _ = os.RemoveAll(tempDir) }()

	// Create package directory with content
	packageDir := filepath.Join(tempDir, "package")

	err = os.MkdirAll(packageDir, 0o755)
	if err != nil {
		t.Fatalf("Failed to create package dir: %v", err)
	}

	// Create a test file
	testFile := filepath.Join(packageDir, "test.txt")

	err = os.WriteFile(testFile, []byte("test content"), 0o644)
	if err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	rpm.PKGBUILD.PackageDir = packageDir

	// Create RPM object
	rpmObj, _ := rpmpack.NewRPM(rpmpack.RPMMetaData{
		Name:    "test",
		Version: "1.0",
		Release: "1",
		Arch:    "x86_64",
	})

	err = rpm.createFilesInsideRPM(rpmObj)
	if err != nil {
		t.Errorf("createFilesInsideRPM failed: %v", err)
	}
}

func TestAddScriptlets(t *testing.T) {
	pkgBuild := createTestPKGBUILD()
	rpm := &RPM{BaseBuilder: common.NewBaseBuilder(pkgBuild, "rpm")}

	// Create RPM object
	rpmObj, _ := rpmpack.NewRPM(rpmpack.RPMMetaData{
		Name:    "test",
		Version: "1.0",
		Release: "1",
		Arch:    "x86_64",
	})

	rpm.addScriptlets(rpmObj)

	// We can't easily verify that scriptlets were added without accessing private fields,
	// but we can verify the method doesn't panic
	t.Log("addScriptlets completed without error")
}

func TestAsRPMDirectory(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "rpm-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}

	defer func() { _ = os.RemoveAll(tempDir) }()

	// Create a test directory
	testDir := filepath.Join(tempDir, "testdir")

	err = os.MkdirAll(testDir, 0o755)
	if err != nil {
		t.Fatalf("Failed to create test dir: %v", err)
	}

	entry := &files.Entry{
		Source:      testDir,
		Destination: "/test/dir",
		Type:        files.TypeDir,
	}

	rpmFile := asRPMDirectory(entry)

	if rpmFile.Name != "/test/dir" {
		t.Errorf("Expected name /test/dir, got %s", rpmFile.Name)
	}

	if rpmFile.Owner != "root" {
		t.Errorf("Expected owner root, got %s", rpmFile.Owner)
	}

	if rpmFile.Group != "root" {
		t.Errorf("Expected group root, got %s", rpmFile.Group)
	}
}

func TestAsRPMFile(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "rpm-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}

	defer func() { _ = os.RemoveAll(tempDir) }()

	// Create a test file
	testFile := filepath.Join(tempDir, "test.txt")
	content := []byte("test content")

	err = os.WriteFile(testFile, content, 0o644)
	if err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	entry := &files.Entry{
		Source:      testFile,
		Destination: "/test/file.txt",
		Type:        files.TypeFile,
	}

	rpmFile, err := asRPMFile(entry, rpmpack.GenericFile)
	if err != nil {
		t.Errorf("asRPMFile failed: %v", err)
	}

	if rpmFile.Name != "/test/file.txt" {
		t.Errorf("Expected name /test/file.txt, got %s", rpmFile.Name)
	}

	if !bytes.Equal(rpmFile.Body, content) {
		t.Errorf("Expected body %s, got %s", string(content), string(rpmFile.Body))
	}
}

func TestAsRPMSymlink(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "rpm-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}

	defer func() { _ = os.RemoveAll(tempDir) }()

	// Create a test file and symlink
	testFile := filepath.Join(tempDir, "test.txt")

	err = os.WriteFile(testFile, []byte("content"), 0o644)
	if err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	testLink := filepath.Join(tempDir, "test.link")

	err = os.Symlink(testFile, testLink)
	if err != nil {
		t.Fatalf("Failed to create symlink: %v", err)
	}

	entry := &files.Entry{
		Source:      testLink,
		Destination: "/test/link",
		Type:        files.TypeSymlink,
	}

	rpmFile := asRPMSymlink(entry)

	if rpmFile.Name != "/test/link" {
		t.Errorf("Expected name /test/link, got %s", rpmFile.Name)
	}

	if rpmFile.Mode != uint(files.TagLink) {
		t.Errorf("Expected mode %d, got %d", files.TagLink, rpmFile.Mode)
	}
}

func TestCreateRPMFile(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "rpm-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}

	defer func() { _ = os.RemoveAll(tempDir) }()

	// Test directory
	testDir := filepath.Join(tempDir, "testdir")

	err = os.MkdirAll(testDir, 0o755)
	if err != nil {
		t.Fatalf("Failed to create test dir: %v", err)
	}

	dirEntry := &files.Entry{
		Source:      testDir,
		Destination: "/test/dir",
		Type:        files.TypeDir,
	}

	rpmFile, err := createRPMFile(dirEntry)
	if err != nil {
		t.Errorf("createRPMFile failed for directory: %v", err)
	}

	if rpmFile == nil {
		t.Error("createRPMFile returned nil for directory")
	}

	// Test regular file
	testFile := filepath.Join(tempDir, "test.txt")

	err = os.WriteFile(testFile, []byte("content"), 0o644)
	if err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	fileEntry := &files.Entry{
		Source:      testFile,
		Destination: "/test/file.txt",
		Type:        files.TypeFile,
	}

	rpmFile, err = createRPMFile(fileEntry)
	if err != nil {
		t.Errorf("createRPMFile failed for file: %v", err)
	}

	if rpmFile == nil {
		t.Error("createRPMFile returned nil for file")
	}
}

func TestExtractFileModTimeUint32(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "rpm-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}

	defer func() { _ = os.RemoveAll(tempDir) }()

	testFile := filepath.Join(tempDir, "test.txt")

	err = os.WriteFile(testFile, []byte("content"), 0o644)
	if err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	fileInfo, err := os.Stat(testFile)
	if err != nil {
		t.Fatalf("Failed to stat test file: %v", err)
	}

	modTime := extractFileModTimeUint32(fileInfo)
	expectedTime := uint32(fileInfo.ModTime().Unix())

	if modTime != expectedTime {
		t.Errorf("Expected mod time %d, got %d", expectedTime, modTime)
	}
}
