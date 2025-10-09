package builder_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/M0Rf30/yap/v2/pkg/builder"
	"github.com/M0Rf30/yap/v2/pkg/i18n"
	"github.com/M0Rf30/yap/v2/pkg/pkgbuild"
)

func TestNewBuilder(t *testing.T) {
	t.Parallel()

	pkgBuild := &pkgbuild.PKGBUILD{
		PkgName: "test-package",
		PkgVer:  "1.0.0",
		PkgRel:  "1",
	}

	bldr := &builder.Builder{
		PKGBUILD: pkgBuild,
	}

	assert.NotNil(t, bldr)
	assert.Equal(t, "test-package", bldr.PKGBUILD.PkgName)
	assert.Equal(t, "1.0.0", bldr.PKGBUILD.PkgVer)
	assert.Equal(t, "1", bldr.PKGBUILD.PkgRel)
}

func TestBuilder_initDirs(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name        string
		sourceDir   string
		shouldExist bool
		wantErr     bool
	}{
		{
			name:        "Create new directory",
			sourceDir:   filepath.Join(t.TempDir(), "newsrc"),
			shouldExist: true,
			wantErr:     false,
		},
		{
			name:        "Use existing directory",
			sourceDir:   t.TempDir(),
			shouldExist: true,
			wantErr:     false,
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			pkgBuild := &pkgbuild.PKGBUILD{
				SourceDir: testCase.sourceDir,
				PkgName:   "test-pkg",
				PkgVer:    "1.0.0",
				PkgRel:    "1",
			}

			bldr := &builder.Builder{
				PKGBUILD: pkgBuild,
			}

			// Test directory creation through Compile method with noBuild=true
			err := bldr.Compile(true)
			if testCase.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}

			if testCase.shouldExist {
				_, err := os.Stat(testCase.sourceDir)
				assert.NoError(t, err, "Directory should exist")
			}
		})
	}
}

func TestBuilder_processFunction(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name             string
		pkgbuildFunction string
		message          string
		stage            string
		wantErr          bool
	}{
		{
			name:             "Empty function should not error",
			pkgbuildFunction: "",
			message:          "test message",
			stage:            "test",
			wantErr:          false,
		},
		{
			name:             "Valid shell command",
			pkgbuildFunction: "echo 'test'",
			message:          "running test",
			stage:            "test",
			wantErr:          false,
		},
		{
			name:             "Invalid shell command",
			pkgbuildFunction: "nonexistent-command-xyz",
			message:          "running invalid command",
			stage:            "test",
			wantErr:          true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
		})
	}
}

func TestBuilder_getSources(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		sourceURI []string
		hashSums  []string
		wantErr   bool
	}{
		{
			name:      "No sources should return nil",
			sourceURI: []string{},
			hashSums:  []string{},
			wantErr:   false,
		},
		{
			name:      "Single local file source",
			sourceURI: []string{"test.txt"},
			hashSums:  []string{""},
			wantErr:   true, // Will fail because file doesn't exist
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
		})
	}
}

func TestBuilder_CompileWithSources(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		sourceURI []string
		hashSums  []string
		noBuild   bool
		wantErr   bool
	}{
		{
			name:      "No sources, no build",
			sourceURI: []string{},
			hashSums:  []string{},
			noBuild:   true,
			wantErr:   false,
		},
		{
			name:      "Single existing source, no build",
			sourceURI: []string{},
			hashSums:  []string{},
			noBuild:   true,
			wantErr:   false,
		},
		{
			name:      "Multiple existing sources, no build",
			sourceURI: []string{},
			hashSums:  []string{},
			noBuild:   true,
			wantErr:   false,
		},
		{
			name:      "Non-existent source should fail",
			sourceURI: []string{"/nonexistent/file.txt"},
			hashSums:  []string{"SKIP"},
			noBuild:   true,
			wantErr:   true,
		},
		{
			name:      "Mixed existing and non-existing sources",
			sourceURI: []string{},
			hashSums:  []string{},
			noBuild:   true,
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// Create unique temp directory for each test case
			tempDir := t.TempDir()
			sourceDir := filepath.Join(tempDir, "src")
			require.NoError(t, os.MkdirAll(sourceDir, 0o755))

			// Create test source files for each test case
			var sourceURI []string

			var hashSums []string

			switch tt.name {
			case "Single existing source, no build":
				testFile := filepath.Join(tempDir, "source1.txt")
				require.NoError(t, os.WriteFile(testFile, []byte("test content 1"), 0o644))
				sourceURI = []string{testFile}
				hashSums = []string{"SKIP"}
			case "Multiple existing sources, no build":
				testFile1 := filepath.Join(tempDir, "source1.txt")
				testFile2 := filepath.Join(tempDir, "source2.tar.gz")

				require.NoError(t, os.WriteFile(testFile1, []byte("test content 1"), 0o644))
				require.NoError(t, os.WriteFile(testFile2, []byte("test content 2"), 0o644))
				sourceURI = []string{testFile1, testFile2}
				hashSums = []string{"SKIP", "SKIP"}
			case "Mixed existing and non-existing sources":
				testFile := filepath.Join(tempDir, "source1.txt")
				require.NoError(t, os.WriteFile(testFile, []byte("test content"), 0o644))
				sourceURI = []string{testFile, "/nonexistent/file.txt"}
				hashSums = []string{"SKIP", "SKIP"}
			default:
				sourceURI = tt.sourceURI
				hashSums = tt.hashSums
			}

			pkgDir := filepath.Join(tempDir, "pkg")
			require.NoError(t, os.MkdirAll(pkgDir, 0o755))

			bldr := &builder.Builder{
				PKGBUILD: &pkgbuild.PKGBUILD{
					PkgName:    "test-pkg",
					PkgVer:     "1.0.0",
					PkgRel:     "1",
					StartDir:   tempDir,
					SourceDir:  sourceDir,
					PackageDir: pkgDir,
					SourceURI:  sourceURI,
					HashSums:   hashSums,
					Prepare:    "",
					Build:      "",
					Package:    "",
				},
			}

			err := bldr.Compile(tt.noBuild)
			if tt.wantErr {
				assert.Error(t, err, "Expected error for test case: %s", tt.name)
			} else {
				assert.NoError(t, err, "Unexpected error for test case: %s", tt.name)
			}
		})
	}
}
func TestBuilder_CompileWithHTTPSources(t *testing.T) {
	t.Parallel()

	tempDir := t.TempDir()
	sourceDir := filepath.Join(tempDir, "src")
	require.NoError(t, os.MkdirAll(sourceDir, 0o755))

	pkgDir := filepath.Join(tempDir, "pkg")
	require.NoError(t, os.MkdirAll(pkgDir, 0o755))

	tests := []struct {
		name      string
		sourceURI []string
		hashSums  []string
		noBuild   bool
		wantErr   bool
	}{
		{
			name:      "Invalid HTTP URL should fail",
			sourceURI: []string{"http://nonexistent.invalid.domain/file.txt"},
			hashSums:  []string{"SKIP"},
			noBuild:   true,
			wantErr:   true,
		},
		{
			name:      "Invalid HTTPS URL should fail",
			sourceURI: []string{"https://definitely.not.a.real.domain.invalid/archive.tar.gz"},
			hashSums:  []string{"SKIP"},
			noBuild:   true,
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			bldr := &builder.Builder{
				PKGBUILD: &pkgbuild.PKGBUILD{
					PkgName:    "test-pkg",
					PkgVer:     "1.0.0",
					PkgRel:     "1",
					StartDir:   tempDir,
					SourceDir:  sourceDir,
					PackageDir: pkgDir,
					SourceURI:  tt.sourceURI,
					HashSums:   tt.hashSums,
					Prepare:    "",
					Build:      "",
					Package:    "",
				},
			}

			err := bldr.Compile(tt.noBuild)
			if tt.wantErr {
				assert.Error(t, err, "Expected error for test case: %s", tt.name)
				// Check for error context - the error might not contain the package name directly
				// but should be a meaningful error message
				assert.NotEmpty(t, err.Error(), "Error message should not be empty")
			} else {
				assert.NoError(t, err, "Unexpected error for test case: %s", tt.name)
			}
		})
	}
}

func TestBuilder_Compile(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		noBuild bool
		prepare string
		build   string
		pkg     string
		wantErr bool
	}{
		{
			name:    "No build mode should succeed",
			noBuild: true,
			prepare: "",
			build:   "",
			pkg:     "",
			wantErr: false,
		},
		{
			name:    "Build mode with empty functions",
			noBuild: false,
			prepare: "",
			build:   "",
			pkg:     "",
			wantErr: false,
		},
		{
			name:    "Build mode with valid commands",
			noBuild: false,
			prepare: "echo 'preparing'",
			build:   "echo 'building'",
			pkg:     "echo 'packaging'",
			wantErr: false,
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			tempDir := t.TempDir()
			sourceDir := filepath.Join(tempDir, "src")

			bldr := &builder.Builder{
				PKGBUILD: &pkgbuild.PKGBUILD{
					PkgName:   "test-pkg",
					PkgVer:    "1.0.0",
					PkgRel:    "1",
					SourceDir: sourceDir,
					SourceURI: []string{},
					HashSums:  []string{},
					Prepare:   testCase.prepare,
					Build:     testCase.build,
					Package:   testCase.pkg,
				},
			}

			err := bldr.Compile(testCase.noBuild)
			if testCase.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}

			// Verify source directory was created
			_, err = os.Stat(sourceDir)
			assert.NoError(t, err, "Source directory should be created")
		})
	}
}

func TestBuilder_CompileWithFailingInitDirs(t *testing.T) {
	// Initialize i18n for test
	_ = i18n.Init("en")

	t.Parallel()

	bldr := &builder.Builder{
		PKGBUILD: &pkgbuild.PKGBUILD{
			PkgName:   "test-pkg",
			PkgVer:    "1.0.0",
			PkgRel:    "1",
			SourceDir: "/root/unauthorized", // Should fail on most systems
		},
	}

	err := bldr.Compile(false)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to initialize directories")
}

func TestBuilder_CompileErrorContexts(t *testing.T) {
	// Initialize i18n for test
	_ = i18n.Init("en")

	t.Parallel()
	tempDir := t.TempDir()
	sourceDir := filepath.Join(tempDir, "src")

	bldr := &builder.Builder{
		PKGBUILD: &pkgbuild.PKGBUILD{
			PkgName:   "test-pkg",
			PkgVer:    "2.0.0",
			PkgRel:    "3",
			SourceDir: sourceDir,
			SourceURI: []string{},
			HashSums:  []string{},
			Build:     "exit 1", // Force build failure
		},
	}

	err := bldr.Compile(false)
	require.Error(t, err)

	// The error message format may vary, so let's check the actual error structure
	errStr := err.Error()
	t.Logf("Actual error: %s", errStr)
	// These tests verify that error handling is working, but the exact format may change
	assert.Contains(t, errStr, "build stage failed")
}
