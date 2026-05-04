package command

import (
	"testing"

	yapErrors "github.com/M0Rf30/yap/v2/pkg/errors"
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
