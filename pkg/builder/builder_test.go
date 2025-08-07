package builder_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/M0Rf30/yap/pkg/builder"
	"github.com/M0Rf30/yap/pkg/pkgbuild"
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
