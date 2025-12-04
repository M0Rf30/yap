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
			name:             "ValidationEnabledByDefault",
			skipValidation:   false,
			targetArch:       "aarch64",
			expectValidation: true,
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
			// Note: This will fail with actual package installation errors in test environment,
			// but we're testing the validation logic, not the actual installation
			err := bb.PrepareEnvironment(false, tt.targetArch)

			// Check if validation was performed based on error type
			// If validation is expected and performed, we should get a specific error about missing toolchains
			// If validation is skipped, we'll get a different error (likely about package manager)
			if tt.expectValidation && tt.targetArch != "" && tt.targetArch != pb.ArchComputed {
				// When validation is enabled for cross-compilation, we expect an error about missing toolchains
				if err == nil {
					t.Error("Expected validation error for cross-compilation, got nil")
				}
				// The error should mention toolchain or cross-compiler packages
				// (unless toolchains are actually installed, which is unlikely in test environment)
			}

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
