package common

import (
	"strings"
	"testing"

	"github.com/M0Rf30/yap/v2/pkg/i18n"
)

func TestValidateToolchainI18nErrorMessages(t *testing.T) {
	tests := []struct {
		name          string
		targetArch    string
		format        string
		expectError   bool
		errorContains []string
	}{
		{
			name:        "Valid toolchain - aarch64/deb",
			targetArch:  "aarch64",
			format:      "deb",
			expectError: true, // Most dev systems won't have cross-compilers installed
			errorContains: []string{
				"aarch64",
				"deb",
			},
		},
		{
			name:        "Valid toolchain - armv7/rpm",
			targetArch:  "armv7",
			format:      "rpm",
			expectError: true,
			errorContains: []string{
				"armv7",
				"rpm",
			},
		},
		{
			name:        "Valid toolchain - i686/apk",
			targetArch:  "i686",
			format:      "apk",
			expectError: true,
			errorContains: []string{
				"i686",
				"apk",
			},
		},
		{
			name:        "Valid toolchain - x86_64/pacman",
			targetArch:  "x86_64",
			format:      "pacman",
			expectError: true,
			errorContains: []string{
				"x86_64",
				"pacman",
			},
		},
		{
			name:        "Unsupported package format",
			targetArch:  "aarch64",
			format:      "invalid-format",
			expectError: true,
			errorContains: []string{
				"invalid-format",
			},
		},
		{
			name:        "Unsupported architecture",
			targetArch:  "invalid-arch",
			format:      "deb",
			expectError: true,
			errorContains: []string{
				"invalid-arch",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateToolchain(tt.targetArch, tt.format)

			if tt.expectError {
				if err == nil {
					t.Error("Expected error but got none")
					return
				}

				errMsg := err.Error()
				for _, expected := range tt.errorContains {
					if !strings.Contains(errMsg, expected) {
						t.Errorf("Error message should contain '%s', got: %s", expected, errMsg)
					}
				}
			} else if err != nil {
				t.Errorf("Expected no error but got: %v", err)
			}
		})
	}
}

func TestValidateToolchainI18nStructure(t *testing.T) {
	// Test that error messages contain all expected sections
	err := ValidateToolchain("aarch64", "deb")
	if err == nil {
		t.Skip("Toolchain is installed on this system, skipping structure test")
	}

	errMsg := err.Error()

	// Verify error message structure
	sections := []string{
		i18n.T("errors.cross_compilation.missing_executables"),
		i18n.T("errors.cross_compilation.required_packages"),
		i18n.T("errors.cross_compilation.installation_command"),
		i18n.T("errors.cross_compilation.path_note"),
		i18n.T("errors.cross_compilation.skip_validation_tip"),
	}

	for _, section := range sections {
		if !strings.Contains(errMsg, section) {
			t.Errorf("Error message should contain section '%s', got: %s", section, errMsg)
		}
	}
}

func TestValidateToolchainI18nDistroSpecificNotes(t *testing.T) {
	tests := []struct {
		name           string
		targetArch     string
		format         string
		expectedNotes  []string
		unexpectedNote string
	}{
		{
			name:       "Alpine APK - should contain Alpine note",
			targetArch: "aarch64",
			format:     "apk",
			expectedNotes: []string{
				i18n.T("errors.cross_compilation.alpine_note"),
			},
			unexpectedNote: i18n.T("errors.cross_compilation.arch_multilib_note"),
		},
		{
			name:       "Arch i686 - should contain multilib note",
			targetArch: "i686",
			format:     "pacman",
			expectedNotes: []string{
				i18n.T("errors.cross_compilation.arch_multilib_note"),
			},
			unexpectedNote: i18n.T("errors.cross_compilation.alpine_note"),
		},
		{
			name:       "Arch aarch64 - should contain prefix note",
			targetArch: "aarch64",
			format:     "pacman",
			expectedNotes: []string{
				i18n.T("errors.cross_compilation.arch_prefix_note"),
			},
			unexpectedNote: i18n.T("errors.cross_compilation.alpine_note"),
		},
		{
			name:           "Debian - should not contain Arch/Alpine notes",
			targetArch:     "armv7",
			format:         "deb",
			expectedNotes:  []string{},
			unexpectedNote: i18n.T("errors.cross_compilation.alpine_note"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateToolchain(tt.targetArch, tt.format)
			if err == nil {
				t.Skip("Toolchain is installed on this system, skipping note test")
			}

			errMsg := err.Error()

			// Check expected notes
			for _, expected := range tt.expectedNotes {
				if !strings.Contains(errMsg, expected) {
					t.Errorf("Error message should contain note '%s'", expected)
				}
			}

			// Check that unexpected notes are not present
			if tt.unexpectedNote != "" {
				if strings.Contains(errMsg, tt.unexpectedNote) {
					t.Errorf("Error message should NOT contain note '%s'", tt.unexpectedNote)
				}
			}
		})
	}
}

func TestValidateToolchainI18nInstallationCommands(t *testing.T) {
	tests := []struct {
		name       string
		targetArch string
		format     string
	}{
		{name: "DEB format", targetArch: "aarch64", format: "deb"},
		{name: "RPM format", targetArch: "armv7", format: "rpm"},
		{name: "APK format", targetArch: "x86_64", format: "apk"},
		{name: "Pacman format", targetArch: "i686", format: "pacman"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateToolchain(tt.targetArch, tt.format)
			if err == nil {
				t.Skip("Toolchain is installed on this system, skipping command test")
			}

			errMsg := err.Error()

			// Verify installation command section exists
			installHeader := i18n.T("errors.cross_compilation.installation_command")
			if !strings.Contains(errMsg, installHeader) {
				t.Errorf("Error should contain installation command header '%s'", installHeader)
			}

			// Verify the command contains distribution-specific package manager
			formatToDistro := map[string]string{
				"deb":    "ubuntu",
				"rpm":    "fedora",
				"apk":    "alpine",
				"pacman": "arch",
			}

			distro := formatToDistro[tt.format]

			toolchain, err := GetCrossToolchain(tt.targetArch, distro)
			if err != nil {
				t.Fatalf("Failed to get toolchain: %v", err)
			}

			if installCmd, exists := toolchain.InstallCommands[distro]; exists {
				// Check that parts of the install command are present
				if !strings.Contains(errMsg, installCmd) {
					t.Errorf("Error should contain install command parts")
				}
			}
		})
	}
}

func TestValidateToolchainI18nPackageList(t *testing.T) {
	targetArch := "aarch64"
	format := "deb"

	err := ValidateToolchain(targetArch, format)
	if err == nil {
		t.Skip("Toolchain is installed on this system, skipping package list test")
	}

	errMsg := err.Error()

	// Get expected packages
	toolchain, err := GetCrossToolchain(targetArch, "ubuntu")
	if err != nil {
		t.Fatalf("Failed to get toolchain: %v", err)
	}

	packages := toolchain.GetAllPackages()
	if len(packages) == 0 {
		t.Fatal("Expected packages list to be non-empty")
	}

	// Verify required packages header
	packagesHeader := i18n.T("errors.cross_compilation.required_packages")
	if !strings.Contains(errMsg, packagesHeader) {
		t.Errorf("Error should contain packages header '%s'", packagesHeader)
	}

	// Verify some package names are listed
	foundPackages := 0

	for _, pkg := range packages {
		if strings.Contains(errMsg, pkg) {
			foundPackages++
		}
	}

	if foundPackages == 0 {
		t.Error("Error should list at least some required packages")
	}
}

func TestValidateToolchainI18nUnsupportedFormat(t *testing.T) {
	err := ValidateToolchain("aarch64", "unsupported-format")
	if err == nil {
		t.Fatal("Expected error for unsupported format")
	}

	// Check for i18n error message
	if !strings.Contains(err.Error(), "unsupported-format") {
		t.Errorf("Error should mention the unsupported format")
	}
}

func TestValidateToolchainI18nUnsupportedArchitecture(t *testing.T) {
	err := ValidateToolchain("unsupported-arch", "deb")
	if err == nil {
		t.Fatal("Expected error for unsupported architecture")
	}

	// Check for i18n error message
	if !strings.Contains(err.Error(), "unsupported-arch") {
		t.Errorf("Error should mention the unsupported architecture")
	}
}
