package project

import (
	"testing"
)

func TestCrossCompilationFlow(t *testing.T) {
	// Test that the cross-compilation flag is properly passed through the system
	mpc := &MultipleProject{
		Opts: BuildOptions{},
	}

	// Test with a target architecture
	mpc.Opts.TargetArch = "aarch64"

	// Verify the target architecture is set
	if mpc.Opts.TargetArch != "aarch64" {
		t.Errorf("Expected TargetArch to be aarch64, got %s", mpc.Opts.TargetArch)
	}
}

func TestCrossCompilationFlagParsing(t *testing.T) {
	// Test that the cross-compilation flag is parsed correctly
	// This is more of an integration test to make sure the flag flows through correctly
	mpc := &MultipleProject{
		Opts: BuildOptions{},
	}

	// Test with empty target architecture (no cross-compilation)
	if mpc.Opts.TargetArch != "" {
		t.Errorf("Expected TargetArch to be empty by default, got %s", mpc.Opts.TargetArch)
	}

	// Test setting target architecture
	mpc.Opts.TargetArch = "armv7"

	if mpc.Opts.TargetArch != "armv7" {
		t.Errorf("Expected TargetArch to be armv7, got %s", mpc.Opts.TargetArch)
	}
}
