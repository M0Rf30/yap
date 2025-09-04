package command

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/M0Rf30/yap/v2/pkg/i18n"
)

func TestDetectPackageType(t *testing.T) {
	tests := []struct {
		name     string
		filename string
		expected string
		wantErr  bool
	}{
		{
			name:     "DEB package",
			filename: "package.deb",
			expected: "deb",
			wantErr:  false,
		},
		{
			name:     "RPM package",
			filename: "package.rpm",
			expected: "rpm",
			wantErr:  false,
		},
		{
			name:     "APK package",
			filename: "package.apk",
			expected: "apk",
			wantErr:  false,
		},
		{
			name:     "PKG package with tar.zst",
			filename: "package.pkg.tar.zst",
			expected: "pkg",
			wantErr:  false,
		},
		{
			name:     "PKG package with tar.xz",
			filename: "package.pkg.tar.xz",
			expected: "pkg",
			wantErr:  false,
		},
		{
			name:     "PKG package with tar.gz",
			filename: "package.pkg.tar.gz",
			expected: "pkg",
			wantErr:  false,
		},
		{
			name:     "Unsupported extension",
			filename: "package.txt",
			expected: "",
			wantErr:  true,
		},
		{
			name:     "No extension",
			filename: "package",
			expected: "",
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := detectPackageType(tt.filename)

			if tt.wantErr {
				assert.Error(t, err)
				assert.Empty(t, result)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expected, result)
			}
		})
	}
}

func TestRunInstall(t *testing.T) {
	// Test with non-existent file
	t.Run("non-existent file", func(t *testing.T) {
		err := runInstall(nil, []string{"/non/existent/file.deb"})
		assert.Error(t, err)
		// Check that the error mentions the file path, regardless of locale
		assert.Contains(t, err.Error(), "/non/existent/file.deb")
	})

	// Test with unsupported file type
	t.Run("unsupported file type", func(t *testing.T) {
		// Create a temporary file with unsupported extension
		tempDir := t.TempDir()
		testFile := filepath.Join(tempDir, "test.txt")
		require.NoError(t, os.WriteFile(testFile, []byte("test content"), 0o644))

		err := runInstall(nil, []string{testFile})
		assert.Error(t, err)
	})

	// Test with no arguments - this will panic as the function doesn't check args length
	t.Run("no arguments", func(t *testing.T) {
		// The function will panic if no arguments are provided
		// This is expected behavior as the command should validate args before calling runInstall
		assert.Panics(t, func() {
			_ = runInstall(nil, []string{})
		})
	})
}

func TestInstallPackage(t *testing.T) {
	// Create a temporary file for testing
	tempDir := t.TempDir()
	testFile := filepath.Join(tempDir, "test.deb")
	require.NoError(t, os.WriteFile(testFile, []byte("fake deb content"), 0o644))

	tests := []struct {
		name        string
		packageType string
		filePath    string
		expectError bool
	}{
		{
			name:        "DEB package",
			packageType: "deb",
			filePath:    testFile,
			expectError: true, // Will fail because we don't have dpkg installed
		},
		{
			name:        "RPM package",
			packageType: "rpm",
			filePath:    testFile,
			expectError: true, // Will fail because we don't have rpm installed
		},
		{
			name:        "APK package",
			packageType: "apk",
			filePath:    testFile,
			expectError: true, // Will fail because we don't have apk installed
		},
		{
			name:        "PKG package",
			packageType: "pkg",
			filePath:    testFile,
			expectError: true, // Will fail because we don't have pacman installed
		},
		{
			name:        "Unknown package type",
			packageType: "unknown",
			filePath:    testFile,
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := installPackage(tt.packageType, tt.filePath)

			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestInstallCommandDefinition(t *testing.T) {
	// Initialize i18n and descriptions for testing
	_ = i18n.Init("en")

	InitializeInstallDescriptions()

	// Test that the install command is properly defined
	assert.NotNil(t, installCmd)
	assert.Equal(t, "install <artifact-file>", installCmd.Use)
	assert.Contains(t, installCmd.Short, "Install a package artifact")
	assert.Equal(t, "utility", installCmd.GroupID)
	assert.NotEmpty(t, installCmd.Long)
	assert.NotNil(t, installCmd.RunE)
}
