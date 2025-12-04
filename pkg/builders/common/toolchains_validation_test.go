package common

import (
	"slices"
	"strings"
	"testing"
)

func TestCrossToolchainValidate(t *testing.T) {
	// Test with toolchain from the map (may not be installed on system)
	toolchain := CrossToolchainMap["x86_64"]["ubuntu"]

	missing, err := toolchain.Validate()
	// We expect the toolchain to be missing on most dev systems
	// The test verifies that Validate() returns proper structure
	if err != nil {
		// Error expected if toolchain not installed
		if len(missing) == 0 {
			t.Error("If error is returned, missing list should not be empty")
		}

		if !strings.Contains(err.Error(), "missing required toolchain executables") {
			t.Errorf("Error message should mention missing executables, got: %v", err)
		}
	} else if len(missing) != 0 {
		// If no error, toolchain is installed
		t.Errorf("If no error, missing list should be empty, got: %v", missing)
	}
}

func TestCrossToolchainValidateMissingGCC(t *testing.T) {
	// Test with missing GCC (nonexistent executable)
	toolchain := CrossToolchainMap["x86_64"]["ubuntu"]
	toolchain.GCCPackage = "nonexistent-gcc-totally-fake"

	missing, err := toolchain.Validate()
	if err == nil {
		t.Error("Missing GCC should return error")
	}

	// Should include the missing GCC executable
	if !slices.Contains(missing, "nonexistent-gcc-totally-fake") {
		t.Errorf("Missing list should include nonexistent-gcc-totally-fake, got: %v", missing)
	}

	if !strings.Contains(err.Error(), "missing required toolchain executables") {
		t.Errorf("Error should mention missing toolchain executables, got: %v", err)
	}
}

func TestCrossToolchainValidateMissingGPlus(t *testing.T) {
	// Test with missing G++ (nonexistent executable)
	toolchain := CrossToolchainMap["x86_64"]["ubuntu"]
	toolchain.GPlusPlusPackage = "nonexistent-gpp-totally-fake"

	missing, err := toolchain.Validate()
	if err == nil {
		t.Error("Missing G++ should return error")
	}

	// Should include the missing G++ executable
	if !slices.Contains(missing, "nonexistent-gpp-totally-fake") {
		t.Errorf("Missing list should include nonexistent-gpp-totally-fake, got: %v", missing)
	}

	if !strings.Contains(err.Error(), "missing required toolchain executables") {
		t.Errorf("Error should mention missing toolchain executables, got: %v", err)
	}
}

func TestCrossToolchainValidateMissingBinutils(t *testing.T) {
	// Test with missing binutils (nonexistent executable)
	toolchain := CrossToolchainMap["x86_64"]["ubuntu"]
	toolchain.BinutilsPackage = "nonexistent-binutils-totally-fake"

	missing, err := toolchain.Validate()
	if err == nil {
		t.Error("Missing binutils should return error")
	}

	// Should include the missing binutils executable
	if !slices.Contains(missing, "nonexistent-binutils-totally-fake") {
		t.Errorf("Missing list should include nonexistent-binutils-totally-fake, got: %v", missing)
	}

	if !strings.Contains(err.Error(), "missing required toolchain executables") {
		t.Errorf("Error should mention missing toolchain executables, got: %v", err)
	}
}

func TestCrossToolchainValidateMultipleMissing(t *testing.T) {
	// Test with multiple missing packages (all fake executables)
	toolchain := CrossToolchainMap["x86_64"]["ubuntu"]
	toolchain.GCCPackage = "missing-gcc-fake"
	toolchain.GPlusPlusPackage = "missing-gpp-fake"
	toolchain.BinutilsPackage = "missing-binutils-fake"

	missing, err := toolchain.Validate()
	if err == nil {
		t.Error("Multiple missing packages should return error")
	}

	// Should include all three fake executables
	expectedMissing := []string{"missing-gcc-fake", "missing-gpp-fake", "missing-binutils-fake"}
	for _, expected := range expectedMissing {
		if !slices.Contains(missing, expected) {
			t.Errorf("Missing list should include %s, got: %v", expected, missing)
		}
	}
}

func TestCrossToolchainValidateEmptyPackages(t *testing.T) {
	// Test with empty package names
	toolchain := CrossToolchainMap["x86_64"]["ubuntu"]
	toolchain.GCCPackage = ""
	toolchain.GPlusPlusPackage = ""
	toolchain.BinutilsPackage = ""

	missing, err := toolchain.Validate()
	if err != nil {
		t.Errorf("Empty packages should not return error, got: %v", err)
	}

	if len(missing) > 0 {
		t.Errorf("Empty packages should not return missing list, got: %v", missing)
	}
}

func TestCrossToolchainValidatePartialToolchain(t *testing.T) {
	// Test with partial toolchain (only GCC present)
	toolchain := CrossToolchainMap["x86_64"]["ubuntu"]
	toolchain.GCCPackage = "gcc"
	toolchain.GPlusPlusPackage = ""
	toolchain.BinutilsPackage = ""

	missing, err := toolchain.Validate()
	if err != nil {
		t.Errorf("Partial toolchain should not return error, got: %v", err)
	}

	if len(missing) > 0 {
		t.Errorf("Partial toolchain should not return missing list, got: %v", missing)
	}
}
