package dnfinstall

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestResolveRootDirLive tests resolveRootDir with "/" as the target.
func TestResolveRootDirLive(t *testing.T) {
	tests := []struct {
		name              string
		opts              Options
		wantErr           bool
		wantErrSubstring  string
	}{
		{
			name: "root without AllowRootInstall rejected",
			opts: Options{
				RootDir:          "/",
				AllowRootInstall: false,
			},
			wantErr:          true,
			wantErrSubstring: "refusing to install to /",
		},
		{
			name: "root with AllowRootInstall allowed",
			opts: Options{
				RootDir:          "/",
				AllowRootInstall: true,
			},
			wantErr: false,
		},
		{
			name: "empty RootDir defaults to /",
			opts: Options{
				RootDir:          "",
				AllowRootInstall: true,
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir, err := resolveRootDir(tt.opts)
			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErrSubstring)
			} else {
				require.NoError(t, err)
				assert.Equal(t, "/", dir)
			}
		})
	}
}

// TestResolveRootDirFakeroot tests resolveRootDir with a fakeroot directory.
func TestResolveRootDirFakeroot(t *testing.T) {
	tmpDir := t.TempDir()

	tests := []struct {
		name             string
		rootDir          string
		wantErr          bool
		wantErrSubstring string
	}{
		{
			name:    "valid fakeroot directory",
			rootDir: tmpDir,
			wantErr: false,
		},
		{
			name:             "nonexistent fakeroot directory",
			rootDir:          filepath.Join(tmpDir, "nonexistent"),
			wantErr:          true,
			wantErrSubstring: "does not exist",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir, err := resolveRootDir(Options{RootDir: tt.rootDir})
			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErrSubstring)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.rootDir, dir)
			}
		})
	}
}

// TestInstallEmptyList tests Install with an empty package list.
func TestInstallEmptyList(t *testing.T) {
	ctx := context.Background()
	err := Install(ctx, []string{})
	assert.NoError(t, err)
}

// TestInstallWithOptionsEmptyList tests InstallWithOptions with an empty list.
func TestInstallWithOptionsEmptyList(t *testing.T) {
	ctx := context.Background()
	opts := Options{
		RootDir:          "/",
		AllowRootInstall: true,
	}
	err := InstallWithOptions(ctx, []string{}, opts)
	assert.NoError(t, err)
}

// TestInstallFileNonexistent tests InstallFile with a nonexistent RPM.
func TestInstallFileNonexistent(t *testing.T) {
	ctx := context.Background()
	opts := Options{
		RootDir:          "/",
		AllowRootInstall: true,
	}
	err := InstallFile(ctx, "/nonexistent/package.rpm", opts)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

// TestInstallFileValidation tests InstallFile with a valid but empty file.
func TestInstallFileValidation(t *testing.T) {
	tmpDir := t.TempDir()
	rpmPath := filepath.Join(tmpDir, "test.rpm")

	// Create an empty file (not a valid RPM).
	f, err := os.Create(rpmPath)
	require.NoError(t, err)
	require.NoError(t, f.Close())

	ctx := context.Background()
	opts := Options{
		RootDir:          tmpDir,
		AllowRootInstall: false,
	}

	err = InstallFile(ctx, rpmPath, opts)
	require.Error(t, err)
	// Should fail during signature verification or RPM parsing
	// (signature verification happens first now in Phase 5)
	assert.True(t, 
		assert.Contains(t, err.Error(), "signature verification") ||
			assert.Contains(t, err.Error(), "failed to parse RPM"),
		"error should mention signature verification or RPM parsing")
}
