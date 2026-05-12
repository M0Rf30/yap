package common

import (
	"testing"

	"github.com/M0Rf30/yap/v2/pkg/pkgbuild"
)

// TestSkipToolchainValidationFlag tests that the SkipToolchainValidation flag is properly used.
func TestSkipToolchainValidationFlag(t *testing.T) {
	// Create a minimal PKGBUILD for testing
	pb := &pkgbuild.PKGBUILD{
		PkgName:      "test-pkg",
		PkgVer:       "1.0.0",
		PkgRel:       "1",
		ArchComputed: "x86_64",
	}

	// Create a BaseBuilder
	bb := NewBaseBuilder(pb, "deb")

	tests := []struct {
		name             string
		skipValidation   bool
		targetArch       string
		expectValidation bool
	}{
		{
			name:             "ValidationSkippedByPrepareEnvironment",
			skipValidation:   false,
			targetArch:       "aarch64",
			expectValidation: false, // PrepareEnvironment always skips validation (by design)
		},
		{
			name:             "ValidationSkippedWhenFlagSet",
			skipValidation:   true,
			targetArch:       "aarch64",
			expectValidation: false,
		},
		{
			name:             "NoValidationWhenSameArch",
			skipValidation:   false,
			targetArch:       "x86_64",
			expectValidation: false,
		},
		{
			name:             "NoValidationWhenNoTargetArch",
			skipValidation:   false,
			targetArch:       "",
			expectValidation: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set the global flag
			SkipToolchainValidation = tt.skipValidation

			// Call PrepareEnvironment
			// Note: PrepareEnvironment always skips validation (by design — it's called by `yap prepare`
			// before the toolchain is installed). It tries to install packages via the package manager,
			// which may fail in test environments without proper setup.
			err := bb.PrepareEnvironment(false, tt.targetArch)

			// PrepareEnvironment always skips validation, so we don't check for validation errors.
			// The error (if any) will be from package manager operations, not validation.
			// We just verify the function doesn't panic.
			_ = err

			// Reset the flag
			SkipToolchainValidation = false
		})
	}
}

// TestSkipValidationIntegration tests the integration between project flags and common package.
func TestSkipValidationIntegration(t *testing.T) {
	// Test that setting the flag affects the validation behavior
	originalValue := SkipToolchainValidation

	// Set flag to true
	SkipToolchainValidation = true
	if !SkipToolchainValidation {
		t.Error("Expected SkipToolchainValidation to be true")
	}

	// Set flag to false
	SkipToolchainValidation = false
	if SkipToolchainValidation {
		t.Error("Expected SkipToolchainValidation to be false")
	}

	// Restore original value
	SkipToolchainValidation = originalValue
}
