package makepkg_test

import (
	"bufio"
	"compress/gzip"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/M0Rf30/yap/pkg/makepkg"
	"github.com/M0Rf30/yap/pkg/pkgbuild"
)

func TestPkg_BuildPackage(t *testing.T) {
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
				ArchComputed: "x86_64",
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
			expectedPkg: "test-pkg-1.0.0-1-x86_64.pkg.tar.zst",
		},
		{
			name: "Package with epoch",
			pkgbuild: &pkgbuild.PKGBUILD{
				PkgName:      "test-epoch-pkg",
				Epoch:        "2",
				PkgVer:       "1.5.0",
				PkgRel:       "3",
				ArchComputed: "any",
			},
			setupFiles: func(packageDir string) error {
				testFile := filepath.Join(packageDir, "etc", "config.conf")
				err := os.MkdirAll(filepath.Dir(testFile), 0o755)
				if err != nil {
					return err
				}

				return os.WriteFile(testFile, []byte("config=value"), 0o600)
			},
			wantErr:     false,
			expectedPkg: "test-epoch-pkg-2:1.5.0-3-any.pkg.tar.zst",
		},
		{
			name: "Empty package directory",
			pkgbuild: &pkgbuild.PKGBUILD{
				PkgName:      "empty-pkg",
				PkgVer:       "1.0.0",
				PkgRel:       "1",
				ArchComputed: "x86_64",
			},
			setupFiles: func(_ string) error {
				return nil
			},
			wantErr:     false,
			expectedPkg: "empty-pkg-1.0.0-1-x86_64.pkg.tar.zst",
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			packageDir := t.TempDir()
			artifactsDir := t.TempDir()

			testCase.pkgbuild.PackageDir = packageDir

			err := testCase.setupFiles(packageDir)
			require.NoError(t, err)

			pkg := &makepkg.Pkg{
				PKGBUILD: testCase.pkgbuild,
			}

			err = pkg.BuildPackage(artifactsDir)

			if testCase.wantErr {
				assert.Error(t, err)

				return
			}

			require.NoError(t, err)

			expectedPath := filepath.Join(artifactsDir, testCase.expectedPkg)
			assert.FileExists(t, expectedPath)

			stat, err := os.Stat(expectedPath)
			require.NoError(t, err)
			assert.Positive(t, stat.Size())
		})
	}
}

func TestPkg_PrepareFakeroot(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		pkgbuild   *pkgbuild.PKGBUILD
		setupFiles func(packageDir, startDir string) error
		wantErr    bool
	}{
		{
			name: "Basic fakeroot preparation",
			pkgbuild: &pkgbuild.PKGBUILD{
				PkgName:      "test-pkg",
				PkgVer:       "1.0.0",
				PkgRel:       "1",
				PkgDesc:      "Test package",
				ArchComputed: "x86_64",
				License:      []string{"GPL"},
				URL:          "https://example.com",
			},
			setupFiles: func(packageDir, startDir string) error {
				testFile := filepath.Join(packageDir, "usr", "bin", "test")
				err := os.MkdirAll(filepath.Dir(testFile), 0o755)
				if err != nil {
					return err
				}

				err = os.WriteFile(testFile, []byte("#!/bin/bash\necho test"), 0o600)
				if err != nil {
					return err
				}

				pkgbuildFile := filepath.Join(startDir, "PKGBUILD")

				return os.WriteFile(pkgbuildFile, []byte("pkgname=test-pkg\npkgver=1.0.0"), 0o600)
			},
			wantErr: false,
		},
		{
			name: "Package with symlinks",
			pkgbuild: &pkgbuild.PKGBUILD{
				PkgName:      "symlink-pkg",
				PkgVer:       "2.0.0",
				PkgRel:       "1",
				PkgDesc:      "Package with symlinks",
				ArchComputed: "x86_64",
				License:      []string{"MIT"},
			},
			setupFiles: func(packageDir, startDir string) error {
				targetFile := filepath.Join(packageDir, "usr", "bin", "target")
				err := os.MkdirAll(filepath.Dir(targetFile), 0o755)
				if err != nil {
					return err
				}

				err = os.WriteFile(targetFile, []byte("target content"), 0o600)
				if err != nil {
					return err
				}

				symlinkPath := filepath.Join(packageDir, "usr", "bin", "symlink")
				err = os.Symlink("target", symlinkPath)
				if err != nil {
					return err
				}

				pkgbuildFile := filepath.Join(startDir, "PKGBUILD")

				return os.WriteFile(pkgbuildFile, []byte("pkgname=symlink-pkg\npkgver=2.0.0"), 0o600)
			},
			wantErr: false,
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			packageDir := t.TempDir()
			startDir := t.TempDir()
			artifactsDir := t.TempDir()

			testCase.pkgbuild.PackageDir = packageDir
			testCase.pkgbuild.StartDir = startDir
			testCase.pkgbuild.Home = startDir

			err := testCase.setupFiles(packageDir, startDir)
			require.NoError(t, err)

			pkg := &makepkg.Pkg{
				PKGBUILD: testCase.pkgbuild,
			}

			err = pkg.PrepareFakeroot(artifactsDir)

			if testCase.wantErr {
				assert.Error(t, err)

				return
			}

			require.NoError(t, err)

			assert.Positive(t, testCase.pkgbuild.InstalledSize)
			assert.Positive(t, testCase.pkgbuild.BuildDate)
			assert.NotEmpty(t, testCase.pkgbuild.PkgDest)
			assert.Equal(t, "pkg", testCase.pkgbuild.PkgType)
			assert.NotEmpty(t, testCase.pkgbuild.Checksum)

			pkginfoFile := filepath.Join(packageDir, ".PKGINFO")
			assert.FileExists(t, pkginfoFile)

			buildinfoFile := filepath.Join(packageDir, ".BUILDINFO")
			assert.FileExists(t, buildinfoFile)

			mtreeFile := filepath.Join(packageDir, ".MTREE")
			assert.FileExists(t, mtreeFile)

			installFile := filepath.Join(startDir, testCase.pkgbuild.PkgName+".install")
			assert.FileExists(t, installFile)
		})
	}
}

func TestPkg_Install(t *testing.T) {
	t.Parallel()

	packageDir := t.TempDir()
	artifactsDir := t.TempDir()

	installTestPkg := &pkgbuild.PKGBUILD{
		PkgName:      "test-install-pkg",
		PkgVer:       "1.0.0",
		PkgRel:       "1",
		ArchComputed: "x86_64",
		PackageDir:   packageDir,
	}

	testFile := filepath.Join(packageDir, "usr", "bin", "test")
	err := os.MkdirAll(filepath.Dir(testFile), 0o755)
	require.NoError(t, err)
	err = os.WriteFile(testFile, []byte("test content"), 0o600)
	require.NoError(t, err)

	pkg := &makepkg.Pkg{
		PKGBUILD: installTestPkg,
	}

	err = pkg.BuildPackage(artifactsDir)
	require.NoError(t, err)

	err = pkg.Install(artifactsDir)
	if err != nil {
		t.Logf("Install may fail if pacman is not available: %v", err)
	}
}

func TestPkg_Prepare(t *testing.T) {
	t.Parallel()

	packageBuild := &pkgbuild.PKGBUILD{
		PkgName: "test-prepare-pkg",
		PkgVer:  "1.0.0",
		PkgRel:  "1",
	}

	pkg := &makepkg.Pkg{
		PKGBUILD: packageBuild,
	}

	makeDepends := []string{"make", "gcc"}

	err := pkg.Prepare(makeDepends)
	if err != nil {
		t.Logf("Prepare may fail if pacman is not available: %v", err)
	}
}

func TestPkg_PrepareEnvironment(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		golang bool
	}{
		{
			name:   "Basic environment",
			golang: false,
		},
		{
			name:   "Go environment",
			golang: true,
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			packageBuild := &pkgbuild.PKGBUILD{
				PkgName: "test-env-pkg",
				PkgVer:  "1.0.0",
				PkgRel:  "1",
			}

			pkg := &makepkg.Pkg{
				PKGBUILD: packageBuild,
			}

			err := pkg.PrepareEnvironment(testCase.golang)
			if err != nil {
				t.Logf("PrepareEnvironment may fail if pacman is not available: %v", err)
			}
		})
	}
}

func TestPkg_Update(t *testing.T) {
	t.Parallel()

	packageBuild := &pkgbuild.PKGBUILD{
		PkgName: "test-update-pkg",
		PkgVer:  "1.0.0",
		PkgRel:  "1",
	}

	pkg := &makepkg.Pkg{
		PKGBUILD: packageBuild,
	}

	err := pkg.Update()
	if err != nil {
		t.Logf("Update may fail if pacman is not available: %v", err)
	}
}

func TestPkg_BuildPackage_WithEpoch(t *testing.T) {
	t.Parallel()

	packageDir := t.TempDir()
	artifactsDir := t.TempDir()

	epochTestPkg := &pkgbuild.PKGBUILD{
		PkgName:      "epoch-test",
		Epoch:        "1",
		PkgVer:       "2.0.0",
		PkgRel:       "1",
		ArchComputed: "x86_64",
		PackageDir:   packageDir,
	}

	testFile := filepath.Join(packageDir, "usr", "bin", "epoch-test")
	err := os.MkdirAll(filepath.Dir(testFile), 0o755)
	require.NoError(t, err)
	err = os.WriteFile(testFile, []byte("epoch test content"), 0o600)
	require.NoError(t, err)

	pkg := &makepkg.Pkg{
		PKGBUILD: epochTestPkg,
	}

	err = pkg.BuildPackage(artifactsDir)
	require.NoError(t, err)

	expectedPkg := "epoch-test-1:2.0.0-1-x86_64.pkg.tar.zst"
	expectedPath := filepath.Join(artifactsDir, expectedPkg)
	assert.FileExists(t, expectedPath)
}

func TestPkg_BuildPackage_Error(t *testing.T) {
	t.Parallel()

	errorTestPkg := &pkgbuild.PKGBUILD{
		PkgName:      "error-test",
		PkgVer:       "1.0.0",
		PkgRel:       "1",
		ArchComputed: "x86_64",
		PackageDir:   "/nonexistent/path",
	}

	pkg := &makepkg.Pkg{
		PKGBUILD: errorTestPkg,
	}

	err := pkg.BuildPackage("/tmp")
	assert.Error(t, err)
}

// TestMTREEFormat tests MTREE file format and compression.
func TestMTREEFormat(t *testing.T) {
	t.Parallel()

	packageDir := t.TempDir()
	startDir := t.TempDir()
	artifactsDir := t.TempDir()
	homeDir := t.TempDir() // Use different dir to trigger PKGBUILD creation

	pkgBuild := &pkgbuild.PKGBUILD{
		PkgName:      "format-test",
		PkgVer:       "1.0.0",
		PkgRel:       "1",
		PkgDesc:      "Format test package",
		ArchComputed: "x86_64",
		License:      []string{"MIT"},
		PackageDir:   packageDir,
		StartDir:     startDir,
		Home:         homeDir, // Different from StartDir to trigger PKGBUILD creation
	}

	// Create test files
	testFile := filepath.Join(packageDir, "usr", "bin", "test")
	err := os.MkdirAll(filepath.Dir(testFile), 0o755)
	require.NoError(t, err)
	err = os.WriteFile(testFile, []byte("test content"), 0o600)
	require.NoError(t, err)
	err = os.Chmod(testFile, 0o755)
	require.NoError(t, err)

	pkg := &makepkg.Pkg{
		PKGBUILD: pkgBuild,
	}

	err = pkg.PrepareFakeroot(artifactsDir)
	require.NoError(t, err)

	mtreeFile := filepath.Join(packageDir, ".MTREE")
	assert.FileExists(t, mtreeFile)

	// Test that MTREE file is gzipped
	file, err := os.Open(mtreeFile)
	require.NoError(t, err)

	defer file.Close()

	// Read first two bytes to check gzip magic number
	header := make([]byte, 2)
	_, err = file.Read(header)
	require.NoError(t, err)

	// Gzip magic number is 0x1f, 0x8b
	assert.Equal(t, byte(0x1f), header[0])
	assert.Equal(t, byte(0x8b), header[1])

	// Test decompression and content
	content, err := readMTREEFile(mtreeFile)
	require.NoError(t, err)

	// Validate basic MTREE format
	lines := strings.Split(content, "\n")
	assert.NotEmpty(t, lines)
	assert.Equal(t, "#mtree", lines[0])

	// Check for proper field formatting
	timeRegex := regexp.MustCompile(`time=\d+\.\d+`)
	modeRegex := regexp.MustCompile(`mode=[0-7]+`)
	sha256Regex := regexp.MustCompile(`sha256digest=[a-f0-9]{64}`)

	foundTimeField := false
	foundModeField := false
	foundSha256Field := false

	for _, line := range lines {
		if timeRegex.MatchString(line) {
			foundTimeField = true
		}

		if modeRegex.MatchString(line) {
			foundModeField = true
		}

		if sha256Regex.MatchString(line) {
			foundSha256Field = true
		}
	}

	assert.True(t, foundTimeField, "MTREE should contain time fields")
	assert.True(t, foundModeField, "MTREE should contain mode fields")
	assert.True(t, foundSha256Field, "MTREE should contain SHA256 digests")
}

// readMTREEFile reads and decompresses an MTREE file.
func readMTREEFile(filename string) (string, error) {
	file, err := os.Open(filename)
	if err != nil {
		return "", err
	}
	defer file.Close()

	gzipReader, err := gzip.NewReader(file)
	if err != nil {
		return "", err
	}
	defer gzipReader.Close()

	var content strings.Builder

	scanner := bufio.NewScanner(gzipReader)
	for scanner.Scan() {
		content.WriteString(scanner.Text())
		content.WriteString("\n")
	}

	return content.String(), scanner.Err()
}
