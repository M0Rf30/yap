package abuild_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/M0Rf30/yap/pkg/abuild"
	"github.com/M0Rf30/yap/pkg/pkgbuild"
)

func TestApk_BuildPackage(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		pkgbuild    *pkgbuild.PKGBUILD
		setupFiles  func(packageDir string) error
		wantErr     bool
		expectedPkg string
	}{
		{
			name: "Basic APK package creation",
			pkgbuild: &pkgbuild.PKGBUILD{
				PkgName:      "test-apk",
				PkgVer:       "1.0.0",
				PkgRel:       "1",
				PkgDesc:      "Test APK package",
				ArchComputed: "x86_64",
				URL:          "https://example.com",
				License:      []string{"MIT"},
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
			expectedPkg: "test-apk-1.0.0-r1.x86_64.apk",
		},
		{
			name: "APK package with epoch",
			pkgbuild: &pkgbuild.PKGBUILD{
				PkgName:      "epoch-apk",
				Epoch:        "2",
				PkgVer:       "1.5.0",
				PkgRel:       "3",
				PkgDesc:      "APK package with epoch",
				ArchComputed: "aarch64",
				License:      []string{"GPL-3.0"},
			},
			setupFiles: func(packageDir string) error {
				configFile := filepath.Join(packageDir, "etc", "test.conf")
				err := os.MkdirAll(filepath.Dir(configFile), 0o755)
				if err != nil {
					return err
				}

				return os.WriteFile(configFile, []byte("config=value"), 0o600)
			},
			wantErr:     false,
			expectedPkg: "epoch-apk-1.5.0-r3.aarch64.apk",
		},
		{
			name: "APK package with install scripts",
			pkgbuild: &pkgbuild.PKGBUILD{
				PkgName:      "script-apk",
				PkgVer:       "2.0.0",
				PkgRel:       "1",
				PkgDesc:      "APK with install scripts",
				ArchComputed: "x86_64",
				PreInst:      "echo 'pre-install'",
				PostInst:     "echo 'post-install'",
				PreRm:        "echo 'pre-remove'",
				PostRm:       "echo 'post-remove'",
			},
			setupFiles: func(packageDir string) error {
				testFile := filepath.Join(packageDir, "usr", "share", "test", "data.txt")
				err := os.MkdirAll(filepath.Dir(testFile), 0o755)
				if err != nil {
					return err
				}

				return os.WriteFile(testFile, []byte("data content"), 0o600)
			},
			wantErr:     false,
			expectedPkg: "script-apk-2.0.0-r1.x86_64.apk",
		},
		{
			name: "Empty APK package",
			pkgbuild: &pkgbuild.PKGBUILD{
				PkgName:      "empty-apk",
				PkgVer:       "1.0.0",
				PkgRel:       "1",
				PkgDesc:      "Empty APK package",
				ArchComputed: "x86_64",
			},
			setupFiles: func(_ string) error {
				return nil
			},
			wantErr:     false,
			expectedPkg: "empty-apk-1.0.0-r1.x86_64.apk",
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

			apk := &abuild.Apk{
				PKGBUILD: testCase.pkgbuild,
			}

			err = apk.BuildPackage(artifactsDir)

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

func TestApk_PrepareFakeroot(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		pkgbuild   *pkgbuild.PKGBUILD
		setupFiles func(packageDir string) error
		wantErr    bool
	}{
		{
			name: "Basic fakeroot preparation",
			pkgbuild: &pkgbuild.PKGBUILD{
				PkgName:      "test-apk",
				PkgVer:       "1.0.0",
				PkgRel:       "1",
				PkgDesc:      "Test APK package",
				ArchComputed: "x86_64",
				License:      []string{"MIT"},
				URL:          "https://example.com",
			},
			setupFiles: func(packageDir string) error {
				testFile := filepath.Join(packageDir, "usr", "bin", "test")
				err := os.MkdirAll(filepath.Dir(testFile), 0o755)
				if err != nil {
					return err
				}

				return os.WriteFile(testFile, []byte("#!/bin/sh\necho test"), 0o600)
			},
			wantErr: false,
		},
		{
			name: "Fakeroot with install scripts",
			pkgbuild: &pkgbuild.PKGBUILD{
				PkgName:      "script-apk",
				PkgVer:       "1.0.0",
				PkgRel:       "1",
				PkgDesc:      "APK with scripts",
				ArchComputed: "x86_64",
				PreInst:      "echo 'pre-install'",
				PostInst:     "echo 'post-install'",
			},
			setupFiles: func(packageDir string) error {
				testFile := filepath.Join(packageDir, "etc", "config")
				err := os.MkdirAll(filepath.Dir(testFile), 0o755)
				if err != nil {
					return err
				}

				return os.WriteFile(testFile, []byte("test=value"), 0o600)
			},
			wantErr: false,
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

			apk := &abuild.Apk{
				PKGBUILD: testCase.pkgbuild,
			}

			err = apk.PrepareFakeroot(artifactsDir)

			if testCase.wantErr {
				assert.Error(t, err)

				return
			}

			require.NoError(t, err)

			assert.Positive(t, testCase.pkgbuild.InstalledSize)
			assert.Positive(t, testCase.pkgbuild.BuildDate)
			assert.NotEmpty(t, testCase.pkgbuild.PkgDest)
			assert.NotEmpty(t, testCase.pkgbuild.YAPVersion)

			pkginfoFile := filepath.Join(packageDir, ".PKGINFO")
			assert.FileExists(t, pkginfoFile)

			if testCase.pkgbuild.PreInst != "" {
				preInstFile := filepath.Join(packageDir, ".pre-install")
				assert.FileExists(t, preInstFile)
			}

			if testCase.pkgbuild.PostInst != "" {
				postInstFile := filepath.Join(packageDir, ".post-install")
				assert.FileExists(t, postInstFile)
			}
		})
	}
}

func TestApk_Install(t *testing.T) {
	t.Parallel()

	packageDir := t.TempDir()
	artifactsDir := t.TempDir()

	apkPackage := &pkgbuild.PKGBUILD{
		PkgName:      "test-install-apk",
		PkgVer:       "1.0.0",
		PkgRel:       "1",
		PkgDesc:      "Test install APK",
		ArchComputed: "x86_64",
		PackageDir:   packageDir,
	}

	testFile := filepath.Join(packageDir, "usr", "bin", "test")
	err := os.MkdirAll(filepath.Dir(testFile), 0o755)
	require.NoError(t, err)
	err = os.WriteFile(testFile, []byte("test content"), 0o600)
	require.NoError(t, err)

	apk := &abuild.Apk{
		PKGBUILD: apkPackage,
	}

	err = apk.BuildPackage(artifactsDir)
	require.NoError(t, err)

	err = apk.Install(artifactsDir)
	if err != nil {
		t.Logf("Install may fail if apk is not available: %v", err)
	}
}

func TestApk_Prepare(t *testing.T) {
	t.Parallel()

	testApk := &pkgbuild.PKGBUILD{
		PkgName: "test-prepare-apk",
		PkgVer:  "1.0.0",
		PkgRel:  "1",
	}

	apk := &abuild.Apk{
		PKGBUILD: testApk,
	}

	makeDepends := []string{"gcc", "make"}

	err := apk.Prepare(makeDepends)
	if err != nil {
		t.Logf("Prepare may fail if apk is not available: %v", err)
	}
}

func TestApk_PrepareEnvironment(t *testing.T) {
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

			testApk := &pkgbuild.PKGBUILD{
				PkgName: "test-env-apk",
				PkgVer:  "1.0.0",
				PkgRel:  "1",
			}

			apk := &abuild.Apk{
				PKGBUILD: testApk,
			}

			err := apk.PrepareEnvironment(testCase.golang)
			if err != nil {
				t.Logf("PrepareEnvironment may fail if apk is not available: %v", err)
			}
		})
	}
}

func TestApk_Update(t *testing.T) {
	t.Parallel()

	testApk := &pkgbuild.PKGBUILD{
		PkgName: "test-update-apk",
		PkgVer:  "1.0.0",
		PkgRel:  "1",
	}

	apk := &abuild.Apk{
		PKGBUILD: testApk,
	}

	err := apk.Update()
	if err != nil {
		t.Logf("Update may fail if apk is not available: %v", err)
	}
}

func TestApk_BuildPackage_Error(t *testing.T) {
	t.Parallel()

	errorApk := &pkgbuild.PKGBUILD{
		PkgName:      "error-test-apk",
		PkgVer:       "1.0.0",
		PkgRel:       "1",
		ArchComputed: "x86_64",
		PackageDir:   "/nonexistent/path",
	}

	apk := &abuild.Apk{
		PKGBUILD: errorApk,
	}

	err := apk.BuildPackage("/tmp")
	assert.Error(t, err)
}

func TestApkArchMapping(t *testing.T) {
	t.Parallel()

	tests := []struct {
		input    string
		expected string
	}{
		{"x86_64", "x86_64"},
		{"i686", "x86"},
		{"aarch64", "aarch64"},
		{"armv7h", "armv7h"},
		{"armv6h", "armv6h"},
		{"any", "all"},
	}

	for _, test := range tests {
		t.Run(test.input, func(t *testing.T) {
			t.Parallel()

			mapped := abuild.APKArchs[test.input]
			assert.Equal(t, test.expected, mapped)
		})
	}
}
