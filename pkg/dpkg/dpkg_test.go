package dpkg_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/blakesmith/ar"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/M0Rf30/yap/v2/pkg/dpkg"
	"github.com/M0Rf30/yap/v2/pkg/pkgbuild"
)

func TestDeb_BuildPackage(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		pkgbuild    *pkgbuild.PKGBUILD
		setupFiles  func(packageDir, sourceDir string) error
		wantErr     bool
		expectedPkg string
	}{
		{
			name: "Basic package creation",
			pkgbuild: &pkgbuild.PKGBUILD{
				PkgName:      "test-pkg",
				PkgVer:       "1.0.0",
				PkgRel:       "1",
				PkgDesc:      "Test package description",
				ArchComputed: "x86_64", // This will be mapped to amd64
				Section:      "misc",
				Priority:     "optional",
				Maintainer:   "test@example.com",
				License:      []string{"MIT"},
				StripEnabled: false,
			},
			setupFiles: func(packageDir, _ string) error {
				testFile := filepath.Join(packageDir, "usr", "bin", "test-binary")
				err := os.MkdirAll(filepath.Dir(testFile), 0o755)
				if err != nil {
					return err
				}

				return os.WriteFile(testFile, []byte("test content"), 0o600)
			},
			wantErr:     false,
			expectedPkg: "test-pkg_1.0.0-1_amd64.deb",
		},
		{
			name: "Package with epoch and dependencies",
			pkgbuild: &pkgbuild.PKGBUILD{
				PkgName:      "test-pkg-deps",
				PkgVer:       "2.0.0",
				PkgRel:       "1",
				PkgDesc:      "Test package with dependencies",
				ArchComputed: "any", // This will be mapped to all
				Section:      "libs",
				Priority:     "optional",
				Epoch:        "1",
				Maintainer:   "maintainer@example.com",
				License:      []string{"GPL-2+", "LGPL-3+"},
				Depends:      []string{"libc6>=2.17", "libssl1.1"},
				Provides:     []string{"test-interface"},
				Conflicts:    []string{"old-test-pkg"},
				Replaces:     []string{"legacy-pkg"},
				URL:          "https://example.com",
				Copyright:    []string{"2024 Test Author"},
				StripEnabled: false,
			},
			setupFiles: func(packageDir, _ string) error {
				configFile := filepath.Join(packageDir, "etc", "test-config.conf")
				err := os.MkdirAll(filepath.Dir(configFile), 0o755)
				if err != nil {
					return err
				}

				return os.WriteFile(configFile, []byte("config=value"), 0o600)
			},
			wantErr:     false,
			expectedPkg: "test-pkg-deps_2.0.0-1_all.deb",
		},
		{
			name: "Package with backup files",
			pkgbuild: &pkgbuild.PKGBUILD{
				PkgName:      "test-backup",
				PkgVer:       "1.5.0",
				PkgRel:       "2",
				PkgDesc:      "Test package with backup files",
				ArchComputed: "x86_64", // This will be mapped to amd64
				Section:      "admin",
				Priority:     "optional",
				Maintainer:   "admin@example.com",
				License:      []string{"BSD-2-Clause"},
				Backup:       []string{"etc/test-backup/config.conf", "/var/lib/test-backup/data.db"},
				StripEnabled: false,
			},
			setupFiles: func(packageDir, _ string) error {
				configFile := filepath.Join(packageDir, "etc", "test-backup", "config.conf")
				dataFile := filepath.Join(packageDir, "var", "lib", "test-backup", "data.db")

				err := os.MkdirAll(filepath.Dir(configFile), 0o755)
				if err != nil {
					return err
				}

				err = os.MkdirAll(filepath.Dir(dataFile), 0o755)
				if err != nil {
					return err
				}

				err = os.WriteFile(configFile, []byte("backup config"), 0o600)
				if err != nil {
					return err
				}

				return os.WriteFile(dataFile, []byte("backup data"), 0o600)
			},
			wantErr:     false,
			expectedPkg: "test-backup_1.5.0-2_amd64.deb",
		},
		{
			name: "Package with Ubuntu distro (no codename)",
			pkgbuild: &pkgbuild.PKGBUILD{
				PkgName:      "test-ubuntu",
				PkgVer:       "1.0.0",
				PkgRel:       "1",
				PkgDesc:      "Test package with Ubuntu distro",
				ArchComputed: "x86_64", // This will be mapped to amd64
				Section:      "utils",
				Priority:     "optional",
				Maintainer:   "maintainer@ubuntu.com",
				License:      []string{"GPL-3+"},
				Distro:       "ubuntu",
				StripEnabled: false,
			},
			setupFiles: func(packageDir, _ string) error {
				binFile := filepath.Join(packageDir, "usr", "bin", "test-ubuntu")
				err := os.MkdirAll(filepath.Dir(binFile), 0o755)
				if err != nil {
					return err
				}

				return os.WriteFile(binFile, []byte("#!/bin/bash\necho test-ubuntu"), 0o600)
			},
			wantErr:     false,
			expectedPkg: "test-ubuntu_1.0.0-1ubuntu_amd64.deb",
		},
		{
			name: "Package with Ubuntu jammy codename",
			pkgbuild: &pkgbuild.PKGBUILD{
				PkgName:      "test-jammy",
				PkgVer:       "2.5.0",
				PkgRel:       "1",
				PkgDesc:      "Test package with Ubuntu jammy codename",
				ArchComputed: "x86_64", // This will be mapped to amd64
				Section:      "devel",
				Priority:     "optional",
				Maintainer:   "maintainer@ubuntu.com",
				License:      []string{"MIT"},
				Codename:     "jammy",
				StripEnabled: false,
			},
			setupFiles: func(packageDir, _ string) error {
				libFile := filepath.Join(packageDir, "usr", "lib", "test-jammy", "libtest.so")
				err := os.MkdirAll(filepath.Dir(libFile), 0o755)
				if err != nil {
					return err
				}

				return os.WriteFile(libFile, []byte("test library"), 0o600)
			},
			wantErr:     false,
			expectedPkg: "test-jammy_2.5.0-1jammy_amd64.deb",
		},
		{
			name: "Package with Ubuntu focal codename",
			pkgbuild: &pkgbuild.PKGBUILD{
				PkgName:      "test-focal",
				PkgVer:       "3.2.1",
				PkgRel:       "2",
				PkgDesc:      "Test package with Ubuntu focal codename",
				ArchComputed: "any", // This will be mapped to all
				Section:      "admin",
				Priority:     "optional",
				Maintainer:   "admin@ubuntu.com",
				License:      []string{"Apache-2.0"},
				Codename:     "focal",
				Depends:      []string{"libc6>=2.31", "systemd"},
				StripEnabled: false,
			},
			setupFiles: func(packageDir, _ string) error {
				configFile := filepath.Join(packageDir, "etc", "test-focal", "config.yaml")
				err := os.MkdirAll(filepath.Dir(configFile), 0o755)
				if err != nil {
					return err
				}

				return os.WriteFile(configFile, []byte("version: 3.2.1\nservice: test-focal"), 0o600)
			},
			wantErr:     false,
			expectedPkg: "test-focal_3.2.1-2focal_all.deb",
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			// Create temporary directories
			packageDir := t.TempDir()
			sourceDir := t.TempDir()
			artifactsDir := t.TempDir()

			testCase.pkgbuild.PackageDir = packageDir
			testCase.pkgbuild.SourceDir = sourceDir

			// Setup test files
			err := testCase.setupFiles(packageDir, sourceDir)
			require.NoError(t, err)

			// Create Deb instance
			debPkg := &dpkg.Deb{
				PKGBUILD: testCase.pkgbuild,
			}

			// Prepare fakeroot environment first
			err = debPkg.PrepareFakeroot("")
			require.NoError(t, err)

			// Build the package
			err = debPkg.BuildPackage(artifactsDir)
			if testCase.wantErr {
				assert.Error(t, err)

				return
			}

			require.NoError(t, err)

			// Verify package file was created
			expectedPath := filepath.Join(artifactsDir, testCase.expectedPkg)
			assert.FileExists(t, expectedPath)

			// Verify file is not empty
			stat, err := os.Stat(expectedPath)
			require.NoError(t, err)
			assert.Positive(t, stat.Size())
		})
	}
}

func TestDeb_PrepareFakeroot(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		pkgbuild     *pkgbuild.PKGBUILD
		setupFiles   func(packageDir string) error
		wantErr      bool
		expectedArch string
	}{
		{
			name: "Without stripping",
			pkgbuild: &pkgbuild.PKGBUILD{
				PkgName:      "test-pkg",
				ArchComputed: "x86_64",
				Section:      "misc",
				Priority:     "optional",
				StripEnabled: false,
			},
			setupFiles: func(packageDir string) error {
				return os.MkdirAll(packageDir, 0o755)
			},
			wantErr:      false,
			expectedArch: "amd64",
		},
		{
			name: "Architecture mapping",
			pkgbuild: &pkgbuild.PKGBUILD{
				PkgName:      "test-pkg-any",
				ArchComputed: "any",
				Section:      "devel",
				Priority:     "optional",
				StripEnabled: false,
			},
			setupFiles: func(packageDir string) error {
				return os.MkdirAll(packageDir, 0o755)
			},
			wantErr:      false,
			expectedArch: "all",
		},
		{
			name: "Release handling with codename",
			pkgbuild: &pkgbuild.PKGBUILD{
				PkgName:      "test-release",
				PkgRel:       "1",
				ArchComputed: "aarch64",
				Section:      "libs",
				Priority:     "optional",
				Codename:     "jammy",
				StripEnabled: false,
			},
			setupFiles: func(packageDir string) error {
				return os.MkdirAll(packageDir, 0o755)
			},
			wantErr:      false,
			expectedArch: "arm64",
		},
		{
			name: "Release handling with distro",
			pkgbuild: &pkgbuild.PKGBUILD{
				PkgName:      "test-distro",
				PkgRel:       "2",
				ArchComputed: "i686",
				Section:      "utils",
				Priority:     "optional",
				Distro:       "ubuntu",
				StripEnabled: false,
			},
			setupFiles: func(packageDir string) error {
				return os.MkdirAll(packageDir, 0o755)
			},
			wantErr:      false,
			expectedArch: "386",
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			packageDir := t.TempDir()
			sourceDir := t.TempDir()

			testCase.pkgbuild.PackageDir = packageDir
			testCase.pkgbuild.SourceDir = sourceDir

			err := testCase.setupFiles(packageDir)
			require.NoError(t, err)

			debPkg := &dpkg.Deb{
				PKGBUILD: testCase.pkgbuild,
			}

			err = debPkg.PrepareFakeroot("")
			if testCase.wantErr {
				assert.Error(t, err)

				return
			}

			require.NoError(t, err)

			// Verify architecture was mapped correctly
			assert.Equal(t, testCase.expectedArch, debPkg.PKGBUILD.ArchComputed)

			// Verify DEBIAN directory was created
			debDir := filepath.Join(packageDir, "DEBIAN")
			assert.DirExists(t, debDir)

			// Verify control file was created
			controlFile := filepath.Join(debDir, "control")
			assert.FileExists(t, controlFile)
		})
	}
}

func TestDeb_Update(t *testing.T) {
	t.Parallel()

	// Mock PKGBUILD with Update method
	mockPKGBUILD := &pkgbuild.PKGBUILD{}

	debPkg := &dpkg.Deb{
		PKGBUILD: mockPKGBUILD,
	}

	// This will call the actual method but won't execute apt-get in test environment
	// The test verifies the method exists and doesn't panic
	err := debPkg.Update()
	// We expect this to potentially error in test environment, but not panic
	// The important thing is that the method exists and is callable
	_ = err // Error is expected in test environment without apt-get
}

func TestDeb_ProcessDepends(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		depends  []string
		expected []string
	}{
		{
			name:     "Simple dependency without version",
			depends:  []string{"libc6"},
			expected: []string{"libc6"},
		},
		{
			name:     "Dependency with version constraint",
			depends:  []string{"libc6>=2.17"},
			expected: []string{"libc6 (>= 2.17)"},
		},
		{
			name:     "Multiple dependencies with various constraints",
			depends:  []string{"libc6>=2.17", "libssl1.1>1.0", "zlib1g=1.2.8", "curl<7.68"},
			expected: []string{"libc6 (>= 2.17)", "libssl1.1 (> 1.0)", "zlib1g (= 1.2.8)", "curl (< 7.68)"},
		},
		{
			name:     "Dependencies with equality and less-equal",
			depends:  []string{"pkg1<=1.0", "pkg2=2.0"},
			expected: []string{"pkg1<=1.0", "pkg2 (= 2.0)"}, // Note: <= is not handled by the regex pattern
		},
		{
			name:     "Mixed dependencies",
			depends:  []string{"simple-pkg", "versioned>=1.0", "exact=2.0"},
			expected: []string{"simple-pkg", "versioned (>= 1.0)", "exact (= 2.0)"},
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			debPkg := &dpkg.Deb{
				PKGBUILD: &pkgbuild.PKGBUILD{},
			}

			// Test processDepends indirectly by setting up a package and checking the result
			packageDir := t.TempDir()
			sourceDir := t.TempDir()

			debPkg.PKGBUILD.PackageDir = packageDir
			debPkg.PKGBUILD.SourceDir = sourceDir
			debPkg.PKGBUILD.Depends = testCase.depends
			debPkg.PKGBUILD.ArchComputed = "amd64"
			debPkg.PKGBUILD.Section = "test"
			debPkg.PKGBUILD.Priority = "optional"

			err := os.MkdirAll(packageDir, 0o755)
			require.NoError(t, err)

			err = debPkg.PrepareFakeroot("")
			require.NoError(t, err)

			// Check that dependencies were processed correctly
			assert.Equal(t, testCase.expected, debPkg.PKGBUILD.Depends)
		})
	}
}

func TestDeb_GetRelease(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		pkgRel         string
		codename       string
		distro         string
		expectedPkgRel string
	}{
		{
			name:           "With codename",
			pkgRel:         "1",
			codename:       "jammy",
			distro:         "ubuntu",
			expectedPkgRel: "1jammy",
		},
		{
			name:           "With distro, no codename",
			pkgRel:         "2",
			codename:       "",
			distro:         "debian",
			expectedPkgRel: "2debian",
		},
		{
			name:           "Empty codename and distro",
			pkgRel:         "3",
			codename:       "",
			distro:         "",
			expectedPkgRel: "3",
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			debPkg := &dpkg.Deb{
				PKGBUILD: &pkgbuild.PKGBUILD{
					PkgRel:   testCase.pkgRel,
					Codename: testCase.codename,
					Distro:   testCase.distro,
				},
			}

			// Test getRelease indirectly through PrepareFakeroot
			packageDir := t.TempDir()
			sourceDir := t.TempDir()

			debPkg.PKGBUILD.PackageDir = packageDir
			debPkg.PKGBUILD.SourceDir = sourceDir
			debPkg.PKGBUILD.ArchComputed = "amd64"
			debPkg.PKGBUILD.Section = "test"
			debPkg.PKGBUILD.Priority = "optional"

			err := os.MkdirAll(packageDir, 0o755)
			require.NoError(t, err)

			err = debPkg.PrepareFakeroot("")
			require.NoError(t, err)

			assert.Equal(t, testCase.expectedPkgRel, debPkg.PKGBUILD.PkgRel)
		})
	}
}

func TestDeb_ScriptletsHandling(t *testing.T) {
	t.Parallel()

	packageDir := t.TempDir()
	sourceDir := t.TempDir()
	artifactsDir := t.TempDir()

	err := os.MkdirAll(packageDir, 0o755)
	require.NoError(t, err)

	pkgBuild := &pkgbuild.PKGBUILD{
		PkgName:      "test-scripts",
		PkgVer:       "1.0.0",
		PkgRel:       "1",
		PkgDesc:      "Test scriptlets",
		ArchComputed: "x86_64", // This will be mapped to amd64
		Section:      "test",
		Priority:     "optional",
		Maintainer:   "test@example.com",
		License:      []string{"MIT"},
		PreInst:      "echo 'PreInst script'",
		PostInst:     "echo 'PostInst script'",
		PreRm:        "echo 'PreRm script'",
		PostRm:       "echo 'PostRm script'",
		PackageDir:   packageDir,
		SourceDir:    sourceDir,
		StripEnabled: false,
	}

	debPkg := &dpkg.Deb{
		PKGBUILD: pkgBuild,
	}

	err = debPkg.PrepareFakeroot("")
	require.NoError(t, err)

	// Verify scriptlets were created
	debDir := filepath.Join(packageDir, "DEBIAN")

	preinst := filepath.Join(debDir, "preinst")
	postinst := filepath.Join(debDir, "postinst")
	prerm := filepath.Join(debDir, "prerm")
	postrm := filepath.Join(debDir, "postrm")

	assert.FileExists(t, preinst)
	assert.FileExists(t, postinst)
	assert.FileExists(t, prerm)
	assert.FileExists(t, postrm)

	// Verify scripts have correct permissions
	for _, script := range []string{preinst, postinst, prerm, postrm} {
		info, err := os.Stat(script)
		require.NoError(t, err)
		assert.Equal(t, os.FileMode(0o755), info.Mode().Perm())
	}

	// Build package to ensure scriptlets don't cause errors
	err = debPkg.BuildPackage(artifactsDir)
	require.NoError(t, err)

	expectedPath := filepath.Join(artifactsDir, "test-scripts_1.0.0-1_amd64.deb")
	assert.FileExists(t, expectedPath)
}

func TestDeb_ConfFilesHandling(t *testing.T) {
	t.Parallel()

	packageDir := t.TempDir()
	sourceDir := t.TempDir()
	artifactsDir := t.TempDir()

	// Create config files
	configDir := filepath.Join(packageDir, "etc", "myapp")
	err := os.MkdirAll(configDir, 0o755)
	require.NoError(t, err)

	configFile := filepath.Join(configDir, "config.conf")
	err = os.WriteFile(configFile, []byte("config content"), 0o600)
	require.NoError(t, err)

	pkgBuild := &pkgbuild.PKGBUILD{
		PkgName:      "test-conffiles",
		PkgVer:       "1.0.0",
		PkgRel:       "1",
		PkgDesc:      "Test configuration files",
		ArchComputed: "x86_64", // This will be mapped to amd64
		Section:      "admin",
		Priority:     "optional",
		Maintainer:   "admin@example.com",
		License:      []string{"MIT"},
		Backup:       []string{"etc/myapp/config.conf", "/etc/myapp/other.conf"},
		PackageDir:   packageDir,
		SourceDir:    sourceDir,
		StripEnabled: false,
	}

	debPkg := &dpkg.Deb{
		PKGBUILD: pkgBuild,
	}

	err = debPkg.PrepareFakeroot("")
	require.NoError(t, err)

	// Verify conffiles was created
	debDir := filepath.Join(packageDir, "DEBIAN")
	conffiles := filepath.Join(debDir, "conffiles")
	assert.FileExists(t, conffiles)

	// Read and verify conffiles content
	content, err := os.ReadFile(conffiles)
	require.NoError(t, err)

	contentStr := string(content)
	assert.Contains(t, contentStr, "/etc/myapp/config.conf")
	assert.Contains(t, contentStr, "/etc/myapp/other.conf")

	// Build package to ensure conffiles work correctly
	err = debPkg.BuildPackage(artifactsDir)
	require.NoError(t, err)

	expectedPath := filepath.Join(artifactsDir, "test-conffiles_1.0.0-1_amd64.deb")
	assert.FileExists(t, expectedPath)
}

func TestDeb_CopyrightFile(t *testing.T) {
	t.Parallel()

	packageDir := t.TempDir()
	sourceDir := t.TempDir()

	pkgBuild := &pkgbuild.PKGBUILD{
		PkgName:      "test-copyright",
		PkgVer:       "1.0.0",
		PkgRel:       "1",
		PkgDesc:      "Test copyright file",
		ArchComputed: "x86_64", // This will be mapped to amd64
		Section:      "misc",
		Priority:     "optional",
		Maintainer:   "test@example.com",
		License:      []string{"MIT", "GPL-2+"},
		Copyright:    []string{"2024 Test Author", "2024 Another Author"},
		URL:          "https://example.com",
		PackageDir:   packageDir,
		SourceDir:    sourceDir,
		StripEnabled: false,
	}

	debPkg := &dpkg.Deb{
		PKGBUILD: pkgBuild,
	}

	err := os.MkdirAll(packageDir, 0o755)
	require.NoError(t, err)

	err = debPkg.PrepareFakeroot("")
	require.NoError(t, err)

	// Verify copyright file was created
	debDir := filepath.Join(packageDir, "DEBIAN")
	copyrightFile := filepath.Join(debDir, "copyright")
	assert.FileExists(t, copyrightFile)

	// Read and verify copyright content
	content, err := os.ReadFile(copyrightFile)
	require.NoError(t, err)

	contentStr := string(content)
	assert.Contains(t, contentStr, "test-copyright")
	assert.Contains(t, contentStr, "test@example.com")
	assert.Contains(t, contentStr, "https://example.com")
	assert.Contains(t, contentStr, "2024 Test Author")
	assert.Contains(t, contentStr, "MIT")
	assert.Contains(t, contentStr, "GPL-2+")
}

func TestAddArFile(t *testing.T) {
	t.Parallel()

	// Create a temporary file to write the archive
	tempFile := filepath.Join(t.TempDir(), "test.ar")
	file, err := os.Create(tempFile)
	require.NoError(t, err)

	defer func() {
		if err := file.Close(); err != nil {
			t.Logf("failed to close file: %v", err)
		}
	}()

	writer := ar.NewWriter(file)
	err = writer.WriteGlobalHeader()
	require.NoError(t, err)

	// This tests the addArFile function indirectly through package creation
	// since addArFile is not exported
	packageDir := t.TempDir()
	sourceDir := t.TempDir()
	artifactsDir := t.TempDir()

	err = os.MkdirAll(packageDir, 0o755)
	require.NoError(t, err)

	pkgBuild := &pkgbuild.PKGBUILD{
		PkgName:      "test-ar",
		PkgVer:       "1.0.0",
		PkgRel:       "1",
		PkgDesc:      "Test AR file creation",
		ArchComputed: "x86_64", // This will be mapped to amd64
		Section:      "test",
		Priority:     "optional",
		PackageDir:   packageDir,
		SourceDir:    sourceDir,
		StripEnabled: false,
	}

	debPkg := &dpkg.Deb{
		PKGBUILD: pkgBuild,
	}

	err = debPkg.PrepareFakeroot("")
	require.NoError(t, err)

	err = debPkg.BuildPackage(artifactsDir)
	require.NoError(t, err)

	// Verify the .deb file was created (which uses addArFile internally)
	expectedPath := filepath.Join(artifactsDir, "test-ar_1.0.0-1_amd64.deb")
	assert.FileExists(t, expectedPath)
}

func TestGetModTime(t *testing.T) {
	t.Parallel()

	// The getModTime function is not exported, so we test it indirectly
	// by creating a package that would use it and verifying it doesn't crash
	packageDir := t.TempDir()
	sourceDir := t.TempDir()
	artifactsDir := t.TempDir()

	err := os.MkdirAll(packageDir, 0o755)
	require.NoError(t, err)

	pkgBuild := &pkgbuild.PKGBUILD{
		PkgName:      "test-modtime",
		PkgVer:       "1.0.0",
		PkgRel:       "1",
		PkgDesc:      "Test modification time",
		ArchComputed: "x86_64", // This will be mapped to amd64
		Section:      "test",
		Priority:     "optional",
		PackageDir:   packageDir,
		SourceDir:    sourceDir,
		StripEnabled: false,
	}

	debPkg := &dpkg.Deb{
		PKGBUILD: pkgBuild,
	}

	err = debPkg.PrepareFakeroot("")
	require.NoError(t, err)

	err = debPkg.BuildPackage(artifactsDir)
	require.NoError(t, err)

	// Verify package was created (getModTime didn't cause issues)
	expectedPath := filepath.Join(artifactsDir, "test-modtime_1.0.0-1_amd64.deb")
	assert.FileExists(t, expectedPath)

	// Verify the file has a reasonable modification time
	stat, err := os.Stat(expectedPath)
	require.NoError(t, err)

	now := time.Now()
	timeDiff := now.Sub(stat.ModTime())
	// Should be created within the last minute
	assert.Less(t, timeDiff, time.Minute, "Package modification time should be recent")
}

func TestDeb_ArchMapping(t *testing.T) {
	t.Parallel()

	tests := []struct {
		inputArch  string
		outputArch string
	}{
		{"x86_64", "amd64"},
		{"i686", "386"},
		{"aarch64", "arm64"},
		{"armv7h", "arm7"},
		{"armv6h", "arm6"},
		{"arm", "arm5"},
		{"any", "all"},
	}

	for _, testCase := range tests {
		t.Run(testCase.inputArch, func(t *testing.T) {
			t.Parallel()

			packageDir := t.TempDir()
			sourceDir := t.TempDir()

			debPkg := &dpkg.Deb{
				PKGBUILD: &pkgbuild.PKGBUILD{
					ArchComputed: testCase.inputArch,
					PackageDir:   packageDir,
					SourceDir:    sourceDir,
					Section:      "test",
					Priority:     "optional",
					StripEnabled: false,
				},
			}

			err := os.MkdirAll(packageDir, 0o755)
			require.NoError(t, err)

			err = debPkg.PrepareFakeroot("")
			require.NoError(t, err)

			assert.Equal(t, testCase.outputArch, debPkg.PKGBUILD.ArchComputed)
		})
	}
}
