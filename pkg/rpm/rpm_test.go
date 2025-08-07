package rpm_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/M0Rf30/yap/pkg/pkgbuild"
	"github.com/M0Rf30/yap/pkg/rpm"
)

func TestRPM_BuildPackage(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		pkgbuild    *pkgbuild.PKGBUILD
		setupFiles  func(packageDir string) error
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
				ArchComputed: "x86_64",
				URL:          "https://example.com",
				License:      []string{"MIT"},
				Copyright:    []string{"2024 Test Author"},
				Maintainer:   "test@example.com",
				Section:      "misc",
				PackageDir:   "",
				StripEnabled: false,
			},
			setupFiles: func(packageDir string) error {
				testFile := filepath.Join(packageDir, "usr", "bin", "test-binary")
				err := os.MkdirAll(filepath.Dir(testFile), 0o755)
				if err != nil {
					return err
				}

				return os.WriteFile(testFile, []byte("test content"), 0o600)
			},
			wantErr:     false,
			expectedPkg: "test-pkg-1.0.0-1.x86_64.rpm",
		},
		{
			name: "Package with epoch",
			pkgbuild: &pkgbuild.PKGBUILD{
				PkgName:      "test-pkg-epoch",
				PkgVer:       "2.0.0",
				PkgRel:       "1",
				PkgDesc:      "Test package with epoch",
				ArchComputed: "noarch",
				Epoch:        "1",
				License:      []string{"GPL", "MIT"},
				Section:      "devel",
				PackageDir:   "",
				StripEnabled: false,
			},
			setupFiles: func(packageDir string) error {
				return os.MkdirAll(filepath.Join(packageDir, "etc"), 0o755)
			},
			wantErr:     false,
			expectedPkg: "test-pkg-epoch-2.0.0-1.noarch.rpm",
		},
		{
			name: "Package with dependencies",
			pkgbuild: &pkgbuild.PKGBUILD{
				PkgName:      "test-pkg-deps",
				PkgVer:       "1.5.0",
				PkgRel:       "2",
				PkgDesc:      "Test package with dependencies",
				ArchComputed: "x86_64",
				License:      []string{"Apache-2.0"},
				Depends:      []string{"glibc>=2.17", "openssl"},
				Provides:     []string{"test-interface"},
				Conflicts:    []string{"old-test-pkg"},
				Replaces:     []string{"legacy-pkg"},
				OptDepends:   []string{"optional-lib"},
				Section:      "libs",
				PackageDir:   "",
				StripEnabled: false,
			},
			setupFiles: func(packageDir string) error {
				return os.MkdirAll(packageDir, 0o755)
			},
			wantErr:     false,
			expectedPkg: "test-pkg-deps-1.5.0-2.x86_64.rpm",
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			// Create temporary directories
			packageDir := t.TempDir()
			artifactsDir := t.TempDir()
			testCase.pkgbuild.PackageDir = packageDir

			// Setup test files
			err := testCase.setupFiles(packageDir)
			require.NoError(t, err)

			// Create RPM instance
			rpmPkg := &rpm.RPM{
				PKGBUILD: testCase.pkgbuild,
			}

			// Build the package
			err = rpmPkg.BuildPackage(artifactsDir)
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

func TestRPM_PrepareFakeroot(t *testing.T) {
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
				StripEnabled: false,
				PackageDir:   "",
			},
			setupFiles: func(packageDir string) error {
				return os.MkdirAll(packageDir, 0o755)
			},
			wantErr:      false,
			expectedArch: "x86_64",
		},
		{
			name: "Architecture mapping",
			pkgbuild: &pkgbuild.PKGBUILD{
				PkgName:      "test-pkg-any",
				ArchComputed: "any",
				Section:      "devel",
				StripEnabled: false,
				PackageDir:   "",
			},
			setupFiles: func(packageDir string) error {
				return os.MkdirAll(packageDir, 0o755)
			},
			wantErr:      false,
			expectedArch: "noarch",
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			packageDir := t.TempDir()
			testCase.pkgbuild.PackageDir = packageDir

			err := testCase.setupFiles(packageDir)
			require.NoError(t, err)

			rpmPkg := &rpm.RPM{
				PKGBUILD: testCase.pkgbuild,
			}

			err = rpmPkg.PrepareFakeroot("")
			if testCase.wantErr {
				assert.Error(t, err)

				return
			}

			require.NoError(t, err)

			// Verify architecture was mapped correctly
			assert.Equal(t, testCase.expectedArch, rpmPkg.PKGBUILD.ArchComputed)
		})
	}
}

func TestRPM_Update(t *testing.T) {
	t.Parallel()

	rpmPkg := &rpm.RPM{
		PKGBUILD: &pkgbuild.PKGBUILD{},
	}

	err := rpmPkg.Update()
	require.NoError(t, err)
}

func TestProcessDepends(t *testing.T) {
	t.Parallel()

	// Since processDepends is not exported, we test it indirectly through BuildPackage
	// by checking that dependencies are properly handled
	packageDir := t.TempDir()
	artifactsDir := t.TempDir()

	pkgBuild := &pkgbuild.PKGBUILD{
		PkgName:      "test-deps",
		PkgVer:       "1.0.0",
		PkgRel:       "1",
		PkgDesc:      "Test dependency processing",
		ArchComputed: "x86_64",
		License:      []string{"MIT"},
		Depends:      []string{"glibc>=2.17", "openssl>1.0", "zlib=1.2.8"},
		PackageDir:   packageDir,
		StripEnabled: false,
	}

	err := os.MkdirAll(packageDir, 0o755)
	require.NoError(t, err)

	rpmPkg := &rpm.RPM{
		PKGBUILD: pkgBuild,
	}

	err = rpmPkg.BuildPackage(artifactsDir)
	require.NoError(t, err)

	// Verify package was created (dependency processing didn't fail)
	expectedPath := filepath.Join(artifactsDir, "test-deps-1.0.0-1.x86_64.rpm")
	assert.FileExists(t, expectedPath)
}

func TestRPM_GetGroup(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name            string
		section         string
		expectedSection string
	}{
		{"devel section", "devel", "Development/Tools"},
		{"libs section", "libs", "System Environment/Libraries"},
		{"games section", "games", "Amusements/Games"},
		{"unknown section", "unknown", ""},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			rpmPkg := &rpm.RPM{
				PKGBUILD: &pkgbuild.PKGBUILD{
					Section: testCase.section,
				},
			}

			// This indirectly tests getGroup through PrepareFakeroot
			err := rpmPkg.PrepareFakeroot("")
			require.NoError(t, err)

			assert.Equal(t, testCase.expectedSection, rpmPkg.PKGBUILD.Section)
		})
	}
}

func TestRPM_GetRelease(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		pkgRel         string
		distro         string
		codename       string
		expectedPkgRel string
	}{
		{
			name:           "Fedora with codename",
			pkgRel:         "1",
			distro:         "fedora",
			codename:       "38",
			expectedPkgRel: "1.fc38",
		},
		{
			name:           "RHEL with codename",
			pkgRel:         "2",
			distro:         "rhel",
			codename:       "9",
			expectedPkgRel: "2.el9",
		},
		{
			name:           "No codename",
			pkgRel:         "1",
			distro:         "fedora",
			codename:       "",
			expectedPkgRel: "1",
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			rpmPkg := &rpm.RPM{
				PKGBUILD: &pkgbuild.PKGBUILD{
					PkgRel:   testCase.pkgRel,
					Distro:   testCase.distro,
					Codename: testCase.codename,
				},
			}

			// This indirectly tests getRelease through PrepareFakeroot
			err := rpmPkg.PrepareFakeroot("")
			require.NoError(t, err)

			assert.Equal(t, testCase.expectedPkgRel, rpmPkg.PKGBUILD.PkgRel)
		})
	}
}

func TestRPM_PrepareBackupFiles(t *testing.T) {
	t.Parallel()

	// Test backup files processing indirectly through BuildPackage
	packageDir := t.TempDir()
	artifactsDir := t.TempDir()

	// Create config files
	configDir := filepath.Join(packageDir, "etc", "myapp")
	err := os.MkdirAll(configDir, 0o755)
	require.NoError(t, err)

	configFile := filepath.Join(configDir, "config.conf")
	err = os.WriteFile(configFile, []byte("config content"), 0o600)
	require.NoError(t, err)

	pkgBuild := &pkgbuild.PKGBUILD{
		PkgName:      "test-backup",
		PkgVer:       "1.0.0",
		PkgRel:       "1",
		PkgDesc:      "Test backup files",
		ArchComputed: "x86_64",
		License:      []string{"MIT"},
		Backup:       []string{"etc/myapp/config.conf", "/etc/myapp/other.conf"},
		PackageDir:   packageDir,
		StripEnabled: false,
	}

	rpmPkg := &rpm.RPM{
		PKGBUILD: pkgBuild,
	}

	err = rpmPkg.BuildPackage(artifactsDir)
	require.NoError(t, err)

	// Verify package was created successfully
	expectedPath := filepath.Join(artifactsDir, "test-backup-1.0.0-1.x86_64.rpm")
	assert.FileExists(t, expectedPath)
}

func TestRPM_ScriptletsHandling(t *testing.T) {
	t.Parallel()

	packageDir := t.TempDir()
	artifactsDir := t.TempDir()

	err := os.MkdirAll(packageDir, 0o755)
	require.NoError(t, err)

	pkgBuild := &pkgbuild.PKGBUILD{
		PkgName:      "test-scripts",
		PkgVer:       "1.0.0",
		PkgRel:       "1",
		PkgDesc:      "Test scriptlets",
		ArchComputed: "x86_64",
		License:      []string{"MIT"},
		PreTrans:     "echo 'PreTrans script'",
		PreInst:      "echo 'PreInst script'",
		PostInst:     "echo 'PostInst script'",
		PreRm:        "echo 'PreRm script'",
		PostRm:       "echo 'PostRm script'",
		PostTrans:    "echo 'PostTrans script'",
		PackageDir:   packageDir,
		StripEnabled: false,
	}

	rpmPkg := &rpm.RPM{
		PKGBUILD: pkgBuild,
	}

	err = rpmPkg.BuildPackage(artifactsDir)
	require.NoError(t, err)

	// Verify package was created with scriptlets
	expectedPath := filepath.Join(artifactsDir, "test-scripts-1.0.0-1.x86_64.rpm")
	assert.FileExists(t, expectedPath)
}

func TestRPM_SymlinkHandling(t *testing.T) {
	t.Parallel()

	packageDir := t.TempDir()
	artifactsDir := t.TempDir()

	// Create a target file and a symlink
	targetDir := filepath.Join(packageDir, "usr", "bin")
	err := os.MkdirAll(targetDir, 0o755)
	require.NoError(t, err)

	targetFile := filepath.Join(targetDir, "target-binary")
	err = os.WriteFile(targetFile, []byte("target content"), 0o600)
	require.NoError(t, err)

	symlinkFile := filepath.Join(targetDir, "symlink-binary")
	err = os.Symlink("target-binary", symlinkFile)
	require.NoError(t, err)

	pkgBuild := &pkgbuild.PKGBUILD{
		PkgName:      "test-symlink",
		PkgVer:       "1.0.0",
		PkgRel:       "1",
		PkgDesc:      "Test symlink handling",
		ArchComputed: "x86_64",
		License:      []string{"MIT"},
		PackageDir:   packageDir,
		StripEnabled: false,
	}

	rpmPkg := &rpm.RPM{
		PKGBUILD: pkgBuild,
	}

	err = rpmPkg.BuildPackage(artifactsDir)
	require.NoError(t, err)

	// Verify package was created with symlinks
	expectedPath := filepath.Join(artifactsDir, "test-symlink-1.0.0-1.x86_64.rpm")
	assert.FileExists(t, expectedPath)
}

func TestRPM_EmptyDirectoryHandling(t *testing.T) {
	t.Parallel()

	packageDir := t.TempDir()
	artifactsDir := t.TempDir()

	// Create empty directories
	emptyDir1 := filepath.Join(packageDir, "opt", "empty1")
	emptyDir2 := filepath.Join(packageDir, "var", "empty2")
	err := os.MkdirAll(emptyDir1, 0o755)
	require.NoError(t, err)
	err = os.MkdirAll(emptyDir2, 0o755)
	require.NoError(t, err)

	pkgBuild := &pkgbuild.PKGBUILD{
		PkgName:      "test-empty-dirs",
		PkgVer:       "1.0.0",
		PkgRel:       "1",
		PkgDesc:      "Test empty directory handling",
		ArchComputed: "x86_64",
		License:      []string{"MIT"},
		PackageDir:   packageDir,
		StripEnabled: false,
	}

	rpmPkg := &rpm.RPM{
		PKGBUILD: pkgBuild,
	}

	err = rpmPkg.BuildPackage(artifactsDir)
	require.NoError(t, err)

	// Verify package was created with empty directories
	expectedPath := filepath.Join(artifactsDir, "test-empty-dirs-1.0.0-1.x86_64.rpm")
	assert.FileExists(t, expectedPath)
}

func TestGetModTime(t *testing.T) {
	t.Parallel()

	// Create a temporary file to test modification time
	tempFile := filepath.Join(t.TempDir(), "test-file")
	err := os.WriteFile(tempFile, []byte("test"), 0o600)
	require.NoError(t, err)

	// The function getModTime is not exported, so we test it indirectly
	// by creating a package that would use it and verifying it doesn't crash
	packageDir := t.TempDir()
	artifactsDir := t.TempDir()

	// Copy our test file to package directory
	pkgFile := filepath.Join(packageDir, "test-file")
	err = os.MkdirAll(packageDir, 0o755)
	require.NoError(t, err)

	content, err := os.ReadFile(tempFile)
	require.NoError(t, err)

	err = os.WriteFile(pkgFile, content, 0o600)
	require.NoError(t, err)

	// Set the modification time to ensure it's within valid range
	now := time.Now()
	err = os.Chtimes(pkgFile, now, now)
	require.NoError(t, err)

	pkgBuild := &pkgbuild.PKGBUILD{
		PkgName:      "test-modtime",
		PkgVer:       "1.0.0",
		PkgRel:       "1",
		PkgDesc:      "Test modification time",
		ArchComputed: "x86_64",
		License:      []string{"MIT"},
		PackageDir:   packageDir,
		StripEnabled: false,
	}

	rpmPkg := &rpm.RPM{
		PKGBUILD: pkgBuild,
	}

	err = rpmPkg.BuildPackage(artifactsDir)
	require.NoError(t, err)

	// Verify package was created (getModTime didn't cause issues)
	expectedPath := filepath.Join(artifactsDir, "test-modtime-1.0.0-1.x86_64.rpm")
	assert.FileExists(t, expectedPath)
}
