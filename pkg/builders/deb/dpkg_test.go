package deb

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/M0Rf30/yap/v2/pkg/constants"
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
	pkg := NewBuilder(pkgBuild, "")

	if pkg.PKGBUILD != pkgBuild {
		t.Error("PKGBUILD not set correctly")
	}
}

func TestBuildPackage(t *testing.T) {
	pkgBuild := createTestPKGBUILD()
	pkg := NewBuilder(pkgBuild, "")

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

	_, err = pkg.BuildPackage(context.Background(), artifactsDir, "")
	if err != nil {
		t.Errorf("BuildPackage failed: %v", err)
	}
}

func TestPrepare(t *testing.T) {
	// Skip if not running as root and no sudo available (would prompt for password)
	if os.Geteuid() != 0 && os.Getenv("SUDO_USER") == "" && os.Getenv("CI") == "" {
		t.Skip("Skipping Prepare test - requires sudo privileges or CI environment")
	}

	pkgBuild := createTestPKGBUILD()
	pkg := NewBuilder(pkgBuild, "")

	makeDepends := []string{"make", "gcc"}
	err := pkg.Prepare(context.Background(), makeDepends, "")
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
	pkg := NewBuilder(pkgBuild, "")

	err := pkg.PrepareEnvironment(context.Background(), false, "")
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
	pkg := NewBuilder(pkgBuild, "")

	err := pkg.PrepareEnvironment(context.Background(), true, "")
	// This will likely fail since apt-get isn't available, but we test the method call
	if err == nil {
		t.Log("PrepareEnvironment with golang succeeded (unexpected in test environment)")
	}
}

func TestPrepareFakeroot(t *testing.T) {
	pkgBuild := createTestPKGBUILD()
	pkg := NewBuilder(pkgBuild, "")

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

	err = pkg.PrepareFakeroot(context.Background(), tempDir, "")
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
	pkg := NewBuilder(pkgBuild, "")

	err := pkg.Update(context.Background())
	// This will likely fail since apt-get isn't available, but we test the method call
	if err == nil {
		t.Log("Update succeeded (unexpected in test environment)")
	}
}

func TestGetRelease(t *testing.T) {
	pkgBuild := createTestPKGBUILD()
	pkg := NewBuilder(pkgBuild, "")

	originalRel := pkg.PKGBUILD.PkgRel
	pkg.FormatRelease(map[string]string{})

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
	pkg := NewBuilder(pkgBuild, "")

	originalRel := pkg.PKGBUILD.PkgRel
	pkg.FormatRelease(map[string]string{})

	if !strings.Contains(pkg.PKGBUILD.PkgRel, pkg.PKGBUILD.Distro) {
		t.Error("Distro was not added to release")
	}

	if pkg.PKGBUILD.PkgRel == originalRel {
		t.Error("Release was not modified")
	}
}

// TestBuildPackageNameGenericDistroSuffix locks issue #202/#214: `yap build
// ubuntu` must produce a generic "1ubuntu" release suffix in the artifact
// filename regardless of the host/container codename, while an explicit
// codename still produces a release-qualified suffix.
func TestBuildPackageNameGenericDistroSuffix(t *testing.T) {
	pkgBuild := createTestPKGBUILD()
	pkgBuild.PkgName = "hello214"
	pkgBuild.PkgVer = "0.10.1"
	pkgBuild.PkgRel = "1"
	pkgBuild.Distro = "ubuntu"
	pkgBuild.Codename = ""

	pkg := NewBuilder(pkgBuild, "")
	pkg.FormatRelease(map[string]string{})

	got := pkg.BuildPackageName(constants.ExtDEB)

	want := "hello214_0.10.1-1ubuntu_x86_64.deb"
	if got != want {
		t.Errorf("BuildPackageName() = %q, want %q", got, want)
	}
}

func TestBuildPackageNameReleaseQualifiedCodename(t *testing.T) {
	pkgBuild := createTestPKGBUILD()
	pkgBuild.PkgName = "hello214"
	pkgBuild.PkgVer = "0.10.1"
	pkgBuild.PkgRel = "1"
	pkgBuild.Distro = "ubuntu"
	pkgBuild.Codename = "jammy"

	pkg := NewBuilder(pkgBuild, "")
	pkg.FormatRelease(map[string]string{})

	got := pkg.BuildPackageName(constants.ExtDEB)

	want := "hello214_0.10.1-1jammy_x86_64.deb"
	if got != want {
		t.Errorf("BuildPackageName() = %q, want %q", got, want)
	}
}

func TestProcessDepends(t *testing.T) {
	pkgBuild := createTestPKGBUILD()
	pkg := NewBuilder(pkgBuild, "")

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
	pkg := NewBuilder(pkgBuild, "")

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
	pkg := NewBuilder(pkgBuild, "")

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
	pkg := NewBuilder(pkgBuild, "")

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

// TestAddScriptlets locks Debian Policy §6.1: every maintainer script dpkg
// executes directly must begin with a "#!" interpreter line. preinst and
// postinst get maintainerScriptShebang prepended since their PKGBUILD bodies
// carry no interpreter of their own; prerm and postrm already start with
// "#!/bin/bash" via removeHeader, so they are left as-is.
func TestAddScriptlets(t *testing.T) {
	pkgBuild := createTestPKGBUILD()
	pkg := NewBuilder(pkgBuild, "")

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

	expected := map[string]string{
		preinstScript:  maintainerScriptShebang + pkgBuild.PreInst,
		postinstScript: maintainerScriptShebang + pkgBuild.PostInst,
		prermScript:    removeHeader + pkgBuild.PreRm,
		postrmScript:   removeHeader + pkgBuild.PostRm,
	}

	for script, want := range expected {
		scriptPath := filepath.Join(tempDir, script)

		info, err := os.Stat(scriptPath)
		if os.IsNotExist(err) {
			t.Errorf("Script %s was not created", script)

			continue
		}

		if info.Mode().Perm() != 0o755 {
			t.Errorf("Script %s has mode %o, want 0755", script, info.Mode().Perm())
		}

		content, err := os.ReadFile(scriptPath)
		if err != nil {
			t.Errorf("Failed to read %s: %v", script, err)

			continue
		}

		if string(content) != want {
			t.Errorf("Script %s content = %q, want %q", script, content, want)
		}

		firstLine := strings.SplitN(string(content), "\n", 2)[0]
		if firstLine != "#!/bin/sh" && firstLine != "#!/bin/bash" {
			t.Errorf("Script %s first line = %q, want an interpreter line", script, firstLine)
		}
	}
}

// TestAddScriptletsShebangIdempotent locks that a PKGBUILD scriptlet body
// which already declares its own interpreter is preserved byte-for-byte:
// addScriptlets must not prepend a second "#!" line on top of it.
func TestAddScriptletsShebangIdempotent(t *testing.T) {
	pkgBuild := createTestPKGBUILD()
	pkgBuild.PreInst = "#!/bin/bash\necho 'custom interpreter'"
	pkgBuild.PostInst = "#!/bin/bash\necho 'custom interpreter'"
	pkg := NewBuilder(pkgBuild, "")

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

	for _, script := range []string{preinstScript, postinstScript} {
		scriptPath := filepath.Join(tempDir, script)

		info, err := os.Stat(scriptPath)
		if err != nil {
			t.Fatalf("Failed to stat %s: %v", script, err)
		}

		if info.Mode().Perm() != 0o755 {
			t.Errorf("Script %s has mode %o, want 0755", script, info.Mode().Perm())
		}

		content, err := os.ReadFile(scriptPath)
		if err != nil {
			t.Fatalf("Failed to read %s: %v", script, err)
		}

		got := string(content)

		firstLine := strings.SplitN(got, "\n", 2)[0]
		if firstLine != "#!/bin/bash" {
			t.Errorf("Script %s first line = %q, want %q", script, firstLine, "#!/bin/bash")
		}

		if got != pkgBuild.PreInst && got != pkgBuild.PostInst {
			t.Errorf("Script %s content = %q, want body preserved unchanged", script, got)
		}

		if count := strings.Count(got, "#!"); count != 1 {
			t.Errorf("Script %s contains %d \"#!\" occurrences, want exactly 1", script, count)
		}
	}
}

func TestCreateChangelogFile(t *testing.T) {
	pkgBuild := createTestPKGBUILD()

	tempDir, err := os.MkdirTemp("", "deb-changelog-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}

	defer func() { _ = os.RemoveAll(tempDir) }()

	// Create a changelog file
	changelogPath := filepath.Join(tempDir, "CHANGELOG.md")
	changelogContent := "# Changelog\n\n## Version 1.0\n- Initial release\n"

	err = os.WriteFile(changelogPath, []byte(changelogContent), 0o644)
	if err != nil {
		t.Fatalf("Failed to create changelog file: %v", err)
	}

	// Create package directory
	packageDir := filepath.Join(tempDir, "package")

	err = os.MkdirAll(packageDir, 0o755)
	if err != nil {
		t.Fatalf("Failed to create package dir: %v", err)
	}

	pkgBuild.StartDir = tempDir
	pkgBuild.Changelog = "CHANGELOG.md"
	pkgBuild.PackageDir = packageDir

	pkg := NewBuilder(pkgBuild, "")

	err = pkg.createChangelogFile()
	if err != nil {
		t.Errorf("createChangelogFile failed: %v", err)
	}

	// Check that the changelog file was created
	expectedPath := filepath.Join(packageDir, "usr", "share", "doc",
		pkgBuild.PkgName, "changelog.Debian.gz")
	if _, err := os.Stat(expectedPath); os.IsNotExist(err) {
		t.Errorf("Changelog file was not created at %s", expectedPath)
	}
}

func TestCreateChangelogFile_NoChangelog(t *testing.T) {
	pkgBuild := createTestPKGBUILD()

	tempDir, err := os.MkdirTemp("", "deb-no-changelog-test")
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

	pkgBuild.StartDir = tempDir
	pkgBuild.Changelog = ""
	pkgBuild.PackageDir = packageDir

	pkg := NewBuilder(pkgBuild, "")

	err = pkg.createChangelogFile()
	if err != nil {
		t.Errorf("createChangelogFile failed: %v", err)
	}

	// Check that no changelog directory was created
	docDir := filepath.Join(packageDir, "usr", "share", "doc")
	if _, err := os.Stat(docDir); err == nil {
		t.Errorf("Doc directory should not be created when changelog is empty")
	}
}
