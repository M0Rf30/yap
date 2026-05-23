package command

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"

	yapErrors "github.com/M0Rf30/yap/v2/pkg/errors"
	"github.com/M0Rf30/yap/v2/pkg/project"
	"github.com/M0Rf30/yap/v2/pkg/signing"
)

func TestLogStructuredError(t *testing.T) {
	tests := []struct {
		name string
		err  *yapErrors.YapError
	}{
		{
			name: "basic error",
			err:  yapErrors.New(yapErrors.ErrTypeBuild, "test error"),
		},
		{
			name: "error with operation",
			err:  yapErrors.New(yapErrors.ErrTypeBuild, "test error").WithOperation("test_op"),
		},
		{
			name: "error with context",
			err: yapErrors.New(yapErrors.ErrTypeBuild, "test error").
				WithContext("key1", "value1").
				WithContext("key2", "value2"),
		},
		{
			name: "error with operation and context",
			err: yapErrors.New(yapErrors.ErrTypeBuild, "test error").
				WithOperation("complex_op").
				WithContext("project", "test-project").
				WithContext("distro", "ubuntu-jammy"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Skip this test since logStructuredError calls Fatal which exits the process
			t.Skip("logStructuredError calls Fatal which exits process, cannot be tested")
		})
	}
}

func TestBuildCommandDefinition(t *testing.T) {
	// Initialize localized descriptions for testing
	InitializeBuildDescriptions()

	if buildCmd.Use != "build [distro] <path>" {
		t.Errorf("Expected build command use to be 'build [distro] <path>', got %q", buildCmd.Use)
	}

	if buildCmd.Short == "" {
		t.Error("Build command should have a short description")
	}

	if buildCmd.Long == "" {
		t.Error("Build command should have a long description")
	}

	if buildCmd.RunE == nil {
		t.Error("Build command should have a RunE function")
	}
}

func TestValidateCompression(t *testing.T) {
	tests := []struct {
		name        string
		compression string
		shouldError bool
	}{
		{
			name:        "valid zstd",
			compression: "zstd",
			shouldError: false,
		},
		{
			name:        "valid gzip",
			compression: "gzip",
			shouldError: false,
		},
		{
			name:        "valid xz",
			compression: "xz",
			shouldError: false,
		},
		{
			name:        "invalid compression",
			compression: "invalid",
			shouldError: true,
		},
		{
			name:        "empty compression",
			compression: "",
			shouldError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateCompression(tt.compression)
			if (err != nil) != tt.shouldError {
				t.Errorf("validateCompression(%q) error = %v, shouldError = %v",
					tt.compression, err, tt.shouldError)
			}

			if tt.shouldError && err == nil {
				t.Errorf("validateCompression(%q) should have returned an error",
					tt.compression)
			}
		})
	}
}

func TestResolveSigning_NilSigningConfig(t *testing.T) {
	// Save and restore global state
	origSignKey := signKey
	origSignPassphrase := signPassphrase
	origSignKeyName := signKeyName

	defer func() {
		signKey = origSignKey
		signPassphrase = origSignPassphrase
		signKeyName = origSignKeyName
	}()

	signKey = ""
	signPassphrase = ""
	signKeyName = ""

	mpc := &project.MultipleProject{
		Signing: nil,
	}

	cfg, err := resolveSigning(mpc)
	assert.NoError(t, err)
	assert.NotNil(t, cfg)
	// With no key, signing should be disabled and key/pass empty
	assert.Empty(t, cfg.KeyPath)
	assert.Empty(t, cfg.Passphrase)
}

func TestResolveSigning_WithSigningConfig(t *testing.T) {
	// Save and restore global state
	origSignKey := signKey
	origSignPassphrase := signPassphrase
	origSignKeyName := signKeyName

	defer func() {
		signKey = origSignKey
		signPassphrase = origSignPassphrase
		signKeyName = origSignKeyName
	}()

	// Use CLI flags to override
	signKey = ""
	signPassphrase = ""
	signKeyName = "mykey"

	// Create a temp key file so ResolveGeneric doesn't fail on missing file
	tmpDir := t.TempDir()

	keyFile := filepath.Join(tmpDir, "test.gpg")
	if err := os.WriteFile(keyFile, []byte("fake-key"), 0o600); err != nil {
		t.Fatal(err)
	}

	mpc := &project.MultipleProject{
		Signing: &signing.Config{
			KeyPath:    keyFile,
			Passphrase: "secret",
			KeyName:    "original-key-name",
		},
	}

	cfg, err := resolveSigning(mpc)
	assert.NoError(t, err)
	assert.NotNil(t, cfg)
	assert.Equal(t, keyFile, cfg.KeyPath)
	// CLI signKeyName overrides project config
	assert.Equal(t, "mykey", cfg.KeyName)
}

func TestLogStructuredError_WithPackageContext(t *testing.T) {
	// logStructuredError calls logger.Fatal which calls os.Exit — we can only
	// verify it doesn't panic before the fatal call by checking the function
	// compiles and the error struct is valid.
	err := yapErrors.New(yapErrors.ErrTypeBuild, "build failed").
		WithContext("package", "mypkg").
		WithContext("version", "1.0.0").
		WithContext("release", "1").
		WithContext("stage", "compile")

	// We can't call logStructuredError directly (it calls Fatal/os.Exit).
	// Verify the error context is accessible as expected by the function.
	assert.Equal(t, "mypkg", err.Context["package"])
	assert.Equal(t, "1.0.0", err.Context["version"])
	assert.Equal(t, "1", err.Context["release"])
	assert.Equal(t, "compile", err.Context["stage"])
}
