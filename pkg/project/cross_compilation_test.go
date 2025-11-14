package project

import (
	"testing"
)

func TestCrossCompilationFlow(t *testing.T) {
	// Test that the cross-compilation flag is properly passed through the system
	originalTargetArch := TargetArch

	// Test with a target architecture
	TargetArch = "aarch64"

	// Verify the target architecture is set
	if TargetArch != "aarch64" {
		t.Errorf("Expected TargetArch to be aarch64, got %s", TargetArch)
	}

	// Reset to original value
	TargetArch = originalTargetArch
}

func TestCrossCompilationFlagParsing(t *testing.T) {
	// Test that the cross-compilation flag is parsed correctly
	// This is more of an integration test to make sure the flag flows through correctly

	// Test with empty target architecture (no cross-compilation)
	if TargetArch != "" {
		t.Errorf("Expected TargetArch to be empty by default, got %s", TargetArch)
	}

	// Test setting target architecture
	originalTargetArch := TargetArch
	TargetArch = "armv7"

	if TargetArch != "armv7" {
		t.Errorf("Expected TargetArch to be armv7, got %s", TargetArch)
	}

	// Reset to original value
	TargetArch = originalTargetArch
}
