// SPDX-FileCopyrightText: 2021-2026 Gianluca Boiano
// SPDX-License-Identifier: GPL-3.0-only

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
			expected: []string{"package (>= 1.0)", "other (<< 2.0)"},
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
