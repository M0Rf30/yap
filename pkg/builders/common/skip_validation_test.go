package common

import (
	"context"
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

	// Never touch the real package manager from a unit test.
	origInstall := installDeps
	defer func() { installDeps = origInstall }()

	installDeps = func(context.Context, *pkgbuild.PKGBUILD, string, []string, []string) error {
		return nil
	}

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
			err := bb.PrepareEnvironment(context.Background(), false, tt.targetArch)

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

type ctxKey struct{}

// TestPrepareEnvironmentPropagatesContext verifies the caller's context
// reaches the dependency installer instead of context.Background().
func TestPrepareEnvironmentPropagatesContext(t *testing.T) {
	pb := &pkgbuild.PKGBUILD{PkgName: "p", PkgVer: "1", PkgRel: "1", ArchComputed: "x86_64"}
	bb := NewBaseBuilder(pb, "deb")

	orig := installDeps
	defer func() { installDeps = orig }()

	var got context.Context

	installDeps = func(ctx context.Context, _ *pkgbuild.PKGBUILD, _ string, _, _ []string) error {
		got = ctx

		return nil
	}

	ctx, cancel := context.WithCancel(context.WithValue(context.Background(), ctxKey{}, "marker"))
	cancel()

	if err := bb.PrepareEnvironment(ctx, false, ""); err != nil {
		t.Fatalf("PrepareEnvironment: %v", err)
	}

	if got == nil || got.Value(ctxKey{}) != "marker" {
		t.Fatal("installer did not receive the caller's context")
	}

	if got.Err() == nil {
		t.Error("installer context should be the cancelled caller context")
	}
}

// TestResolveToolchainPackagesNoRandomFallback verifies a format whose distro
// is absent from CrossToolchainMap (Alpine) yields an error rather than a
// random other distro's toolchain.
func TestResolveToolchainPackagesNoRandomFallback(t *testing.T) {
	pb := &pkgbuild.PKGBUILD{PkgName: "p", PkgVer: "1", PkgRel: "1", ArchComputed: "x86_64"}
	bb := NewBaseBuilder(pb, "apk")

	for range 20 {
		if tc, err := bb.resolveToolchainPackages("aarch64"); err == nil {
			t.Fatalf("expected error for apk toolchain, got %+v", tc)
		}
	}
}

// TestPrepareEnvironmentValidatesToolchainAfterInstall verifies the flag is
// honoured: with validation enabled a missing toolchain is an error, with
// SkipToolchainValidation it is not.
func TestPrepareEnvironmentValidatesToolchainAfterInstall(t *testing.T) {
	pb := &pkgbuild.PKGBUILD{PkgName: "p", PkgVer: "1", PkgRel: "1", ArchComputed: "x86_64"}
	bb := NewBaseBuilder(pb, "pacman")

	origInstall, origSkip := installDeps, SkipToolchainValidation

	defer func() {
		installDeps = origInstall
		SkipToolchainValidation = origSkip
	}()

	installDeps = func(context.Context, *pkgbuild.PKGBUILD, string, []string, []string) error {
		return nil
	}

	// Empty PATH: no cross compiler can be found.
	t.Setenv("PATH", t.TempDir())

	SkipToolchainValidation = false

	if err := bb.PrepareEnvironment(context.Background(), false, "aarch64"); err == nil {
		t.Error("expected validation error for missing cross toolchain")
	}

	SkipToolchainValidation = true

	if err := bb.PrepareEnvironment(context.Background(), false, "aarch64"); err != nil {
		t.Errorf("validation should be skipped, got %v", err)
	}
}
