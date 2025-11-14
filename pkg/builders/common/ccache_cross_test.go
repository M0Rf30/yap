package common

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/M0Rf30/yap/v2/pkg/pkgbuild"
)

// TestCrossCompilationWithCcache verifies that ccache is properly preserved
// when setting up cross-compilation environment.
func TestCrossCompilationWithCcache(t *testing.T) {
	// Save original environment
	originalCC := os.Getenv("CC")
	originalCXX := os.Getenv("CXX")
	originalPath := os.Getenv("PATH")

	defer func() {
		_ = os.Setenv("CC", originalCC)
		_ = os.Setenv("CXX", originalCXX)
		_ = os.Setenv("PATH", originalPath)
	}()

	tempDir := t.TempDir()

	pkg := &pkgbuild.PKGBUILD{
		PkgName:      "test-package",
		PkgVer:       "1.0.0",
		PkgRel:       "1",
		ArchComputed: "x86_64",
		StartDir:     tempDir,
	}

	// Create a fake ccache executable
	fakeBinDir := filepath.Join(tempDir, "bin")
	_ = os.MkdirAll(fakeBinDir, 0o755)
	ccachePath := filepath.Join(fakeBinDir, "ccache")
	_ = os.WriteFile(ccachePath, []byte("#!/bin/sh\necho 'fake ccache'\n"), 0o755)

	// Set PATH to include our fake ccache
	_ = os.Setenv("PATH", fakeBinDir+":"+originalPath)

	tests := []struct {
		name       string
		format     string
		targetArch string
	}{
		{
			name:       "Arch Linux aarch64",
			format:     "pacman",
			targetArch: "aarch64",
		},
		{
			name:       "Debian aarch64",
			format:     "deb",
			targetArch: "aarch64",
		},
		{
			name:       "Alpine armv7",
			format:     "apk",
			targetArch: "armv7",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Clear environment
			_ = os.Setenv("CC", "")
			_ = os.Setenv("CXX", "")

			bb := NewBaseBuilder(pkg, tt.format)

			// First setup ccache (simulating what happens in builder.go)
			err := bb.SetupCcache()
			if err != nil {
				t.Fatalf("SetupCcache failed: %v", err)
			}

			// Verify ccache is set
			ccBefore := os.Getenv("CC")
			cxxBefore := os.Getenv("CXX")

			if !strings.Contains(ccBefore, "ccache") {
				t.Errorf("Expected CC to contain 'ccache' after SetupCcache, got: %s", ccBefore)
			}

			if !strings.Contains(cxxBefore, "ccache") {
				t.Errorf("Expected CXX to contain 'ccache' after SetupCcache, got: %s", cxxBefore)
			}

			// Now setup cross-compilation (this is the bug - it overwrites CC/CXX)
			err = bb.SetupCrossCompilationEnvironment(tt.targetArch)
			if err != nil {
				t.Logf("SetupCrossCompilationEnvironment returned error: %v", err)
				// Don't fail - toolchain might not be available
				return
			}

			// Check if ccache is still in the environment
			ccAfter := os.Getenv("CC")
			cxxAfter := os.Getenv("CXX")

			t.Logf("Before cross-compilation: CC=%s, CXX=%s", ccBefore, cxxBefore)
			t.Logf("After cross-compilation:  CC=%s, CXX=%s", ccAfter, cxxAfter)

			// Verify ccache is preserved after cross-compilation setup
			if !strings.Contains(ccAfter, "ccache") {
				t.Errorf("CC lost 'ccache' wrapper after cross-compilation setup. Got: %s", ccAfter)
			}

			if !strings.Contains(cxxAfter, "ccache") {
				t.Errorf("CXX lost 'ccache' wrapper after cross-compilation setup. Got: %s", cxxAfter)
			}

			// Verify the cross-compiler is also present
			if !strings.Contains(ccAfter, "gcc") {
				t.Errorf("CC doesn't contain gcc after cross-compilation: %s", ccAfter)
			}

			if !strings.Contains(cxxAfter, "g++") {
				t.Errorf("CXX doesn't contain g++ after cross-compilation: %s", cxxAfter)
			}
		})
	}
}
