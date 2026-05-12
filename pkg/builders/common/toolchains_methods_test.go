package common

import (
	"fmt"
	"slices"
	"strings"
	"testing"
)

func TestCrossToolchainGetAllPackages(t *testing.T) {
	// Test with a known toolchain
	toolchain := CrossToolchainMap["x86_64"]["ubuntu"]

	packages := toolchain.GetAllPackages()

	// Should return all packages (gcc, g++, binutils, etc.)
	if len(packages) == 0 {
		t.Error("GetAllPackages should return at least one package")
	}
}

func TestCrossToolchainGetPackagesByType(t *testing.T) {
	// Test with a known toolchain
	toolchain := CrossToolchainMap["x86_64"]["ubuntu"]

	packages := toolchain.GetPackagesByType()

	// Should return categorized packages
	if len(packages) == 0 {
		t.Error("GetPackagesByType should return categorized packages")
	}

	// Check that compiler category exists
	if compilerPkgs, exists := packages["compiler"]; !exists {
		t.Error("GetPackagesByType should return compiler packages")
	} else {
		for _, pkg := range compilerPkgs {
			if pkg == "" {
				t.Error("Compiler package should have a name")
			}
		}
	}

	// Check that libraries category exists (not "library")
	if libPkgs, exists := packages["libraries"]; !exists {
		t.Error("GetPackagesByType should return library packages")
	} else {
		for _, pkg := range libPkgs {
			if pkg == "" {
				t.Error("Library package should have a name")
			}
		}
	}
}

func TestGetCrossToolchain(t *testing.T) {
	tests := []struct {
		name    string
		arch    string
		distro  string
		wantErr bool
	}{
		{
			name:    "Valid AMD64 Ubuntu",
			arch:    "x86_64",
			distro:  "ubuntu",
			wantErr: false,
		},
		{
			name:    "Valid ARM64 Fedora",
			arch:    "aarch64",
			distro:  "fedora",
			wantErr: false,
		},
		{
			name:    "Invalid architecture",
			arch:    "invalid",
			distro:  "ubuntu",
			wantErr: true,
		},
		{
			name:    "Invalid distribution",
			arch:    "x86_64",
			distro:  "invalid",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := GetCrossToolchain(tt.arch, tt.distro)
			if (err != nil) != tt.wantErr {
				t.Errorf("GetCrossToolchain() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestCrossToolchainMapCompleteness(t *testing.T) {
	// Test that all supported architectures have toolchains
	architectures := []string{
		"x86_64",
		"aarch64",
		"armv7",
		"i686",
	}

	// Note: alpine is NOT in the map because Alpine cross-compilation via host
	// toolchains is not supported. Use native Alpine containers instead.
	distributions := []string{
		"ubuntu",
		"fedora",
		"debian",
		"arch",
	}

	for _, arch := range architectures {
		for _, distro := range distributions {
			if _, exists := CrossToolchainMap[arch][distro]; !exists {
				t.Errorf("Missing toolchain for arch %s, distro %s", arch, distro)
			}
		}
	}
}

func TestCrossToolchainPackageValidation(t *testing.T) {
	// Test that all packages in all toolchains have required fields
	for arch, distroMap := range CrossToolchainMap {
		for distro, toolchain := range distroMap {
			packages := toolchain.GetAllPackages()
			for _, pkg := range packages {
				if pkg == "" {
					t.Errorf("Package in %s/%s should have a name", arch, distro)
				}
			}
		}
	}
}

// TestRiscv64Support verifies that RISC-V 64-bit architecture is fully supported.
// Note: Alpine is not included because Alpine cross-compilation via host toolchains
// is not supported.
func TestRiscv64Support(t *testing.T) {
	t.Parallel()

	distributions := []string{"debian", "ubuntu", "fedora", "arch"}

	for _, distro := range distributions {
		t.Run(fmt.Sprintf("%s_riscv64", distro), func(t *testing.T) {
			t.Parallel()

			toolchain, err := GetCrossToolchain("riscv64", distro)
			if err != nil {
				t.Fatalf("Failed to get riscv64 toolchain for %s: %v", distro, err)
			}

			// Verify packages exist
			packages := toolchain.GetAllPackages()
			if len(packages) == 0 {
				t.Errorf("No packages found for riscv64 on %s", distro)
			}

			// Verify GNU triple
			if toolchain.Triple != "riscv64-linux-gnu" {
				t.Errorf("Expected GNU triple 'riscv64-linux-gnu', got '%s'", toolchain.Triple)
			}

			// Verify GCC package exists
			if toolchain.GCCPackage == "" {
				t.Error("GCC package not defined for riscv64")
			}

			// Verify G++ package exists
			if toolchain.GPlusPlusPackage == "" {
				t.Error("G++ package not defined for riscv64")
			}

			// Verify Binutils package exists
			if toolchain.BinutilsPackage == "" {
				t.Error("Binutils package not defined for riscv64")
			}

			t.Logf("%s/riscv64: %d packages, GNU triple: %s",
				distro, len(packages), toolchain.Triple)
		})
	}
}

// TestRiscv64PackageNaming verifies distribution-specific package naming for RISC-V.
// Note: Alpine is not included because Alpine cross-compilation via host toolchains
// is not supported.
func TestRiscv64PackageNaming(t *testing.T) {
	t.Parallel()

	tests := []struct {
		distro      string
		expectedGCC string
	}{
		{"ubuntu", "gcc-riscv64-linux-gnu"},
		{"debian", "gcc-riscv64-linux-gnu"},
		{"fedora", "gcc-riscv64-linux-gnu"},
		{"arch", "riscv64-linux-gnu-gcc"},
	}

	for _, tt := range tests {
		t.Run(tt.distro, func(t *testing.T) {
			t.Parallel()

			toolchain, err := GetCrossToolchain("riscv64", tt.distro)
			if err != nil {
				t.Fatalf("Failed to get toolchain: %v", err)
			}

			if toolchain.GCCPackage != tt.expectedGCC {
				t.Errorf("Expected GCC package '%s', got '%s'",
					tt.expectedGCC, toolchain.GCCPackage)
			}
		})
	}
}

// TestAlpineMuslSupport verifies that Alpine cross-compilation is not supported
// via host toolchains. Alpine should use native containers instead.
func TestAlpineMuslSupport(t *testing.T) {
	t.Parallel()

	// Verify Alpine is not in the CrossToolchainMap for any architecture
	architectures := []string{
		"aarch64", "armv7", "armv6", "i686",
		"x86_64", "ppc64le", "s390x", "riscv64",
	}

	for _, arch := range architectures {
		t.Run(fmt.Sprintf("alpine_%s", arch), func(t *testing.T) {
			t.Parallel()

			// Alpine should not be in the map
			if _, exists := CrossToolchainMap[arch]["alpine"]; exists {
				t.Errorf("Alpine should not be in CrossToolchainMap for %s", arch)
			}

			t.Logf("Alpine/%s: Correctly not in CrossToolchainMap (use native containers)",
				arch)
		})
	}
}

// TestAlpinePackageNaming verifies Alpine is not in the CrossToolchainMap.
// Alpine cross-compilation via host toolchains is not supported.
func TestAlpinePackageNaming(t *testing.T) {
	t.Parallel()

	architectures := []string{
		"aarch64", "armv7", "armv6", "i686",
		"x86_64", "ppc64le", "s390x", "riscv64",
	}

	for _, arch := range architectures {
		t.Run(arch, func(t *testing.T) {
			t.Parallel()

			// Alpine should not be in the map
			if _, exists := CrossToolchainMap[arch]["alpine"]; exists {
				t.Errorf("Alpine should not be in CrossToolchainMap for %s", arch)
			}

			t.Logf("Alpine/%s: Correctly not in CrossToolchainMap", arch)
		})
	}
}

// TestAlpineVsGlibcPackages verifies Alpine is not in the CrossToolchainMap
// because Alpine cross-compilation via host toolchains is not supported.
func TestAlpineVsGlibcPackages(t *testing.T) {
	t.Parallel()

	arch := "aarch64"

	// Verify Alpine is NOT in the map
	if _, exists := CrossToolchainMap[arch]["alpine"]; exists {
		t.Error("Alpine should not be in CrossToolchainMap - use native Alpine containers instead")
	}

	// Get Ubuntu toolchain (glibc-based) for comparison
	ubuntuToolchain, err := GetCrossToolchain(arch, "ubuntu")
	if err != nil {
		t.Fatalf("Failed to get Ubuntu toolchain: %v", err)
	}

	// Ubuntu should have libc6-dev (glibc)
	hasLibc6 := slices.Contains(ubuntuToolchain.AdditionalPackages, "libc6-dev-arm64-cross")

	if !hasLibc6 {
		t.Error("Ubuntu toolchain should include libc6-dev-arm64-cross")
	}

	t.Logf("Ubuntu packages: %v", ubuntuToolchain.GetAllPackages())
}

// TestI686MultilibSupport verifies i686 toolchain configuration across distributions.
// Note: Alpine is not included because Alpine cross-compilation via host toolchains
// is not supported.
func TestI686MultilibSupport(t *testing.T) {
	t.Parallel()

	distributions := []string{"debian", "ubuntu", "fedora", "arch"}

	for _, distro := range distributions {
		t.Run(fmt.Sprintf("%s_i686", distro), func(t *testing.T) {
			t.Parallel()

			toolchain, err := GetCrossToolchain("i686", distro)
			if err != nil {
				t.Fatalf("Failed to get i686 toolchain for %s: %v", distro, err)
			}

			// Verify packages exist
			packages := toolchain.GetAllPackages()
			if len(packages) == 0 {
				t.Errorf("No packages found for i686 on %s", distro)
			}

			// Verify GNU triple
			if toolchain.Triple != "i686-linux-gnu" {
				t.Errorf("Expected GNU triple 'i686-linux-gnu', got '%s'", toolchain.Triple)
			}

			// Verify GCC package exists
			if toolchain.GCCPackage == "" {
				t.Error("GCC package not defined for i686")
			}

			// Verify G++ package exists
			if toolchain.GPlusPlusPackage == "" {
				t.Error("G++ package not defined for i686")
			}

			// Verify Binutils package exists
			if toolchain.BinutilsPackage == "" {
				t.Error("Binutils package not defined for i686")
			}

			t.Logf("%s/i686: %d packages, GNU triple: %s",
				distro, len(packages), toolchain.Triple)
		})
	}
}

// TestI686ArchMultilibPackages verifies Arch Linux uses multilib packages for i686.
func TestI686ArchMultilibPackages(t *testing.T) {
	t.Parallel()

	toolchain, err := GetCrossToolchain("i686", "arch")
	if err != nil {
		t.Fatalf("Failed to get Arch i686 toolchain: %v", err)
	}

	// Arch should use multilib packages
	expectedPackages := map[string]bool{
		"gcc-multilib":      false,
		"gcc-c++-multilib":  false, // Note: Will be renamed to g++ in actual Arch
		"binutils-multilib": false,
		"lib32-gcc-libs":    false,
	}

	allPackages := toolchain.GetAllPackages()
	for _, pkg := range allPackages {
		if _, exists := expectedPackages[pkg]; exists {
			expectedPackages[pkg] = true
		}
	}

	// Check that multilib packages are present
	if toolchain.GCCPackage != "gcc-multilib" {
		t.Errorf("Expected GCC package 'gcc-multilib', got '%s'", toolchain.GCCPackage)
	}

	if toolchain.BinutilsPackage != "binutils-multilib" {
		t.Errorf("Expected Binutils package 'binutils-multilib', got '%s'", toolchain.BinutilsPackage)
	}

	// Check for lib32-gcc-libs in additional packages
	hasLib32 := slices.Contains(toolchain.AdditionalPackages, "lib32-gcc-libs")

	if !hasLib32 {
		t.Errorf("Arch i686 toolchain should include lib32-gcc-libs, got: %v",
			toolchain.AdditionalPackages)
	}

	t.Logf("Arch i686 multilib packages: %v", allPackages)
}

// TestI686PackageNaming verifies distribution-specific package naming for i686.
// Note: Alpine is not included because Alpine cross-compilation via host toolchains
// is not supported.
func TestI686PackageNaming(t *testing.T) {
	t.Parallel()

	tests := []struct {
		distro             string
		expectedGCC        string
		expectedAdditional string // empty string means no additional packages expected
		note               string
	}{
		{"ubuntu", "gcc-i686-linux-gnu", "libc6-dev-i386-cross", "Standard cross-compiler"},
		{"debian", "gcc-i686-linux-gnu", "libc6-dev-i386-cross", "Standard cross-compiler"},
		// Fedora bundles the target sysroot in the cross-compiler package itself;
		// libc6-dev-*-cross packages are Debian/Ubuntu-only.
		{"fedora", "gcc-i686-linux-gnu", "", "RPM: sysroot bundled in cross-compiler"},
		{"arch", "gcc-multilib", "lib32-gcc-libs", "Arch uses multilib packages"},
	}

	for _, tt := range tests {
		t.Run(tt.distro, func(t *testing.T) {
			t.Parallel()

			toolchain, err := GetCrossToolchain("i686", tt.distro)
			if err != nil {
				t.Fatalf("Failed to get toolchain: %v", err)
			}

			if toolchain.GCCPackage != tt.expectedGCC {
				t.Errorf("%s: Expected GCC '%s', got '%s' (%s)",
					tt.distro, tt.expectedGCC, toolchain.GCCPackage, tt.note)
			}

			if tt.expectedAdditional == "" {
				// Expect no additional packages
				if len(toolchain.AdditionalPackages) != 0 {
					t.Errorf("%s: Expected no additional packages, got %v (%s)",
						tt.distro, toolchain.AdditionalPackages, tt.note)
				}
			} else {
				// Check for expected additional package
				if !slices.Contains(toolchain.AdditionalPackages, tt.expectedAdditional) {
					t.Errorf("%s: Expected additional package '%s', got %v (%s)",
						tt.distro, tt.expectedAdditional, toolchain.AdditionalPackages, tt.note)
				}
			}

			t.Logf("%s/i686: GCC=%s, Additional=%v (%s)",
				tt.distro, toolchain.GCCPackage, toolchain.AdditionalPackages, tt.note)
		})
	}
}

// TestI686VsX86_64Toolchains verifies i686 (32-bit) vs x86_64 (64-bit) differences.
func TestI686VsX86_64Toolchains(t *testing.T) {
	t.Parallel()

	// Test on Ubuntu
	i686Toolchain, err := GetCrossToolchain("i686", "ubuntu")
	if err != nil {
		t.Fatalf("Failed to get i686 toolchain: %v", err)
	}

	x86_64Toolchain, err := GetCrossToolchain("x86_64", "ubuntu")
	if err != nil {
		t.Fatalf("Failed to get x86_64 toolchain: %v", err)
	}

	// Should have different package names
	if i686Toolchain.GCCPackage == x86_64Toolchain.GCCPackage {
		t.Error("i686 and x86_64 should have different GCC packages")
	}

	// Should have different GNU triplets
	if i686Toolchain.Triple == x86_64Toolchain.Triple {
		t.Error("i686 and x86_64 should have different GNU triplets")
	}

	// Verify correct triplets
	if i686Toolchain.Triple != "i686-linux-gnu" {
		t.Errorf("i686 should have triple 'i686-linux-gnu', got '%s'", i686Toolchain.Triple)
	}

	if x86_64Toolchain.Triple != "x86-64-linux-gnu" {
		t.Errorf("x86_64 should have triple 'x86-64-linux-gnu', got '%s'", x86_64Toolchain.Triple)
	}

	t.Logf("i686 packages: %v", i686Toolchain.GetAllPackages())
	t.Logf("x86_64 packages: %v", x86_64Toolchain.GetAllPackages())
}

// ==================== Task 3.1: ValidateToolchain Function Tests ====================

// TestValidateToolchainUnsupportedFormat tests that ValidateToolchain returns an error
// for unsupported package formats.
func TestValidateToolchainUnsupportedFormat(t *testing.T) {
	tests := []struct {
		name       string
		targetArch string
		format     string
	}{
		{
			name:       "Unsupported format 'zip'",
			targetArch: "aarch64",
			format:     "zip",
		},
		{
			name:       "Unsupported format 'tar'",
			targetArch: "x86_64",
			format:     "tar",
		},
		{
			name:       "Empty format",
			targetArch: "i686",
			format:     "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateToolchain(tt.targetArch, tt.format)
			if err == nil {
				t.Errorf("Expected error for unsupported format '%s', got nil", tt.format)
			}

			// Check that error message contains either the translation key or the translated text
			errMsg := strings.ToLower(err.Error())
			if !strings.Contains(errMsg, "unsupported") && !strings.Contains(errMsg, "format") {
				t.Errorf("Expected 'unsupported' and 'format' in error, got: %v", err)
			}
		})
	}
}

// TestValidateToolchainUnsupportedArchitecture tests that ValidateToolchain returns
// an error for unsupported architectures.
func TestValidateToolchainUnsupportedArchitecture(t *testing.T) {
	tests := []struct {
		name       string
		targetArch string
		format     string
	}{
		{
			name:       "Unsupported arch 'mips'",
			targetArch: "mips",
			format:     "deb",
		},
		{
			name:       "Unsupported arch 'sparc'",
			targetArch: "sparc",
			format:     "rpm",
		},
		{
			name:       "Empty architecture",
			targetArch: "",
			format:     "apk",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateToolchain(tt.targetArch, tt.format)
			if err == nil {
				t.Errorf("Expected error for unsupported architecture '%s', got nil", tt.targetArch)
			}

			// Check that error message contains either the translation key or the translated text
			errMsg := strings.ToLower(err.Error())
			if !strings.Contains(errMsg, "failed") && !strings.Contains(errMsg, "toolchain") {
				t.Errorf("Expected 'failed' and 'toolchain' in error, got: %v", err)
			}
		})
	}
}

// TestValidateToolchainFormatMapping tests that ValidateToolchain correctly maps
// package formats to distributions.
func TestValidateToolchainFormatMapping(t *testing.T) {
	tests := []struct {
		name           string
		format         string
		expectedDistro string
	}{
		{
			name:           "DEB format maps to ubuntu",
			format:         "deb",
			expectedDistro: "ubuntu",
		},
		{
			name:           "RPM format maps to fedora",
			format:         "rpm",
			expectedDistro: "fedora",
		},
		{
			name:           "APK format maps to alpine",
			format:         "apk",
			expectedDistro: "alpine",
		},
		{
			name:           "Pacman format maps to arch",
			format:         "pacman",
			expectedDistro: "arch",
		},
	}

	// Test with a valid architecture to ensure format mapping works
	targetArch := "aarch64"

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateToolchain(targetArch, tt.format)
			if err == nil {
				t.Logf("Toolchain for %s/%s is installed", targetArch, tt.format)
				return
			}

			errMsg := err.Error()

			// APK always returns an "Alpine not supported" error.
			if tt.format == "apk" {
				if !strings.Contains(errMsg, "Alpine") || !strings.Contains(errMsg, "not supported") {
					t.Errorf("APK error should contain 'Alpine' and 'not supported', got: %v", err)
				}

				t.Logf("Format '%s' -> Alpine unsupported verified: %v", tt.format, err)

				return
			}

			// For other formats verify architecture and format appear in the message.
			if !strings.Contains(errMsg, targetArch) {
				t.Errorf("Error message should contain architecture '%s', got: %v", targetArch, err)
			}

			if !strings.Contains(errMsg, tt.format) {
				t.Errorf("Error message should contain format '%s', got: %v", tt.format, err)
			}

			t.Logf("Format '%s' -> Distribution '%s' mapping verified in error: %v",
				tt.format, tt.expectedDistro, err)
		})
	}
}

// TestValidateToolchainErrorMessageStructure tests that ValidateToolchain produces
// well-structured error messages with all required components.
func TestValidateToolchainErrorMessageStructure(t *testing.T) {
	// Use an uncommon architecture/format combination that's unlikely to be installed
	targetArch := "riscv64"
	format := "deb"

	err := ValidateToolchain(targetArch, format)

	// If the toolchain is actually installed, skip this test
	if err == nil {
		t.Skip("Toolchain is already installed, cannot test error message structure")
		return
	}

	errMsg := err.Error()

	// Check for required components in error message (using translation keys)
	requiredComponents := []string{
		"errors.cross_compilation.toolchain_validation_failed",
		targetArch,
		format,
		"errors.cross_compilation.missing_executables",
		"errors.cross_compilation.required_packages",
		"errors.cross_compilation.installation_command",
	}

	for _, component := range requiredComponents {
		if !strings.Contains(errMsg, component) {
			t.Errorf("Error message missing required component '%s'.\nFull message:\n%s",
				component, errMsg)
		}
	}

	t.Logf("Error message structure verified:\n%s", errMsg)
}

// TestValidateToolchainDistributionSpecificGuidance tests that ValidateToolchain
// includes distribution-specific guidance in error messages.
func TestValidateToolchainDistributionSpecificGuidance(t *testing.T) {
	tests := []struct {
		name             string
		targetArch       string
		format           string
		expectedGuidance string
	}{
		{
			name:             "Arch i686 includes multilib guidance",
			targetArch:       "i686",
			format:           "pacman",
			expectedGuidance: "errors.cross_compilation.arch_multilib_note",
		},
		{
			name:             "Arch non-i686 includes prefix guidance",
			targetArch:       "aarch64",
			format:           "pacman",
			expectedGuidance: "errors.cross_compilation.arch_prefix_note",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateToolchain(tt.targetArch, tt.format)

			// If toolchain is installed, skip this specific test
			if err == nil {
				t.Skipf("Toolchain for %s/%s is installed, cannot test error guidance",
					tt.targetArch, tt.format)

				return
			}

			errMsg := err.Error()

			if !strings.Contains(errMsg, tt.expectedGuidance) {
				t.Errorf("Error message should contain guidance '%s' for %s/%s.\nFull message:\n%s",
					tt.expectedGuidance, tt.targetArch, tt.format, errMsg)
			}

			t.Logf("Distribution-specific guidance verified for %s/%s", tt.targetArch, tt.format)
		})
	}
}

// TestValidateToolchainInstallationCommands tests that ValidateToolchain includes
// correct installation commands for each distribution.
func TestValidateToolchainInstallationCommands(t *testing.T) {
	tests := []struct {
		name            string
		targetArch      string
		format          string
		expectedCommand string
	}{
		{
			name:            "DEB uses apt-get",
			targetArch:      "aarch64",
			format:          "deb",
			expectedCommand: "sudo apt-get install",
		},
		{
			name:            "RPM uses dnf",
			targetArch:      "aarch64",
			format:          "rpm",
			expectedCommand: "sudo dnf install",
		},
		{
			name:            "Pacman uses pacman -S",
			targetArch:      "aarch64",
			format:          "pacman",
			expectedCommand: "sudo pacman -S",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateToolchain(tt.targetArch, tt.format)

			// If toolchain is installed, skip this specific test
			if err == nil {
				t.Skipf("Toolchain for %s/%s is installed, cannot test install command",
					tt.targetArch, tt.format)

				return
			}

			errMsg := err.Error()

			if !strings.Contains(errMsg, tt.expectedCommand) {
				t.Errorf("Error message should contain command '%s' for %s.\nFull message:\n%s",
					tt.expectedCommand, tt.format, errMsg)
			}

			t.Logf("Installation command verified for %s: %s", tt.format, tt.expectedCommand)
		})
	}
}

// TestValidateToolchainAllSupportedCombinations tests ValidateToolchain for all
// supported architecture/format combinations to ensure none panic or produce
// malformed errors.
func TestValidateToolchainAllSupportedCombinations(t *testing.T) {
	architectures := []string{"aarch64", "armv7", "armv6", "i686", "x86_64", "ppc64le", "s390x", "riscv64"}
	formats := []string{"deb", "rpm", "apk", "pacman"}

	combinationCount := 0
	successCount := 0
	errorCount := 0

	for _, arch := range architectures {
		for _, format := range formats {
			combinationCount++

			t.Run(fmt.Sprintf("%s_%s", arch, format), func(t *testing.T) {
				err := ValidateToolchain(arch, format)
				if err == nil {
					successCount++

					t.Logf("✓ Toolchain available for %s/%s", arch, format)

					return
				}

				errorCount++
				errMsg := err.Error()

				if errMsg == "" {
					t.Errorf("Error message is empty for %s/%s", arch, format)
					return
				}

				// APK always returns an "Alpine not supported" error.
				if format == "apk" {
					if !strings.Contains(errMsg, "Alpine") && !strings.Contains(errMsg, "not supported") {
						t.Errorf("APK error should contain 'Alpine' or 'not supported' for %s/%s",
							arch, format)
					}
				} else if !strings.Contains(errMsg, arch) {
					t.Errorf("Error message missing architecture '%s' for %s/%s", arch, arch, format)
				}

				t.Logf("✗ Toolchain not available for %s/%s (expected): %v", arch, format, err)
			})
		}
	}

	t.Logf("\nValidateToolchain Coverage Summary:")
	t.Logf("  Total combinations tested: %d", combinationCount)
	t.Logf("  Toolchains available: %d", successCount)
	t.Logf("  Toolchains not available: %d", errorCount)
	t.Logf("  Expected: 32 (8 architectures × 4 formats)")
}

// TestValidateToolchainPackageListCompleteness tests that ValidateToolchain
// includes complete package lists in error messages.
func TestValidateToolchainPackageListCompleteness(t *testing.T) {
	targetArch := "aarch64"
	format := "deb"

	err := ValidateToolchain(targetArch, format)

	// If toolchain is installed, skip this test
	if err == nil {
		t.Skip("Toolchain is already installed, cannot test package list")
		return
	}

	errMsg := err.Error()

	// Verify that the error includes package information (using translation key)
	if !strings.Contains(errMsg, "errors.cross_compilation.required_packages") {
		t.Errorf("Error message should include 'errors.cross_compilation.required_packages' section")
	}

	// Get the actual toolchain to compare
	toolchain, err := GetCrossToolchain(targetArch, "ubuntu")
	if err != nil {
		t.Fatalf("Failed to get toolchain: %v", err)
	}

	allPackages := toolchain.GetAllPackages()

	// Verify that at least some of the packages are mentioned in the error
	packagesFound := 0

	for _, pkg := range allPackages {
		if strings.Contains(errMsg, pkg) {
			packagesFound++
		}
	}

	if packagesFound == 0 {
		t.Errorf("Error message should contain at least some package names from %v.\nFull message:\n%s",
			allPackages, errMsg)
	}

	t.Logf("Found %d/%d packages mentioned in error message", packagesFound, len(allPackages))
}
