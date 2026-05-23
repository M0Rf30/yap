package common

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/M0Rf30/yap/v2/pkg/pkgbuild"
)

// TestCrossCompilationWithCcache verifies that ccache and cross-compilation
// environment variables are properly set in the returned slice.
func TestCrossCompilationWithCcache(t *testing.T) {
	// Save original environment
	originalPath := os.Getenv("PATH")

	defer func() {
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
			bb := NewBaseBuilder(pkg, tt.format)

			// First build ccache environment slice
			ccacheEnv := bb.BuildCcacheEnvSlice()
			if ccacheEnv == nil {
				t.Logf("BuildCcacheEnvSlice returned nil (ccache not available)")
				return
			}

			// Verify ccache is in the slice
			ccFound := false
			cxxFound := false

			for _, envVar := range ccacheEnv {
				if strings.HasPrefix(envVar, "CC=") && strings.Contains(envVar, "ccache") {
					ccFound = true
				}

				if strings.HasPrefix(envVar, "CXX=") && strings.Contains(envVar, "ccache") {
					cxxFound = true
				}
			}

			if !ccFound {
				t.Errorf("Expected CC to contain 'ccache' in ccache env slice")
			}

			if !cxxFound {
				t.Errorf("Expected CXX to contain 'ccache' in ccache env slice")
			}

			// Now build cross-compilation environment slice
			crossEnv, err := bb.BuildCrossEnvSlice(tt.targetArch)
			if err != nil {
				t.Logf("BuildCrossEnvSlice returned error: %v", err)
				// Don't fail - toolchain might not be available
				return
			}

			if crossEnv == nil {
				t.Logf("BuildCrossEnvSlice returned nil (toolchain not available)")
				return
			}

			// Verify CC/CXX in cross-compilation slice are bare cross-compilers, not ccache-wrapped
			ccCrossFound := false
			cxxCrossFound := false

			for _, envVar := range crossEnv {
				if strings.HasPrefix(envVar, "CC=") {
					if strings.Contains(envVar, "ccache") {
						t.Errorf("CC in cross-compilation should not contain 'ccache' directly. Got: %s", envVar)
					}

					if strings.Contains(envVar, "gcc") {
						ccCrossFound = true
					}
				}

				if strings.HasPrefix(envVar, "CXX=") {
					if strings.Contains(envVar, "ccache") {
						t.Errorf("CXX in cross-compilation should not contain 'ccache' directly. Got: %s", envVar)
					}

					if strings.Contains(envVar, "g++") {
						cxxCrossFound = true
					}
				}
			}

			if !ccCrossFound {
				t.Errorf("CC doesn't contain gcc in cross-compilation slice")
			}

			if !cxxCrossFound {
				t.Errorf("CXX doesn't contain g++ in cross-compilation slice")
			}

			// Verify CCACHE_PREFIX is not in the cross-compilation slice
			for _, envVar := range crossEnv {
				if strings.HasPrefix(envVar, "CCACHE_PREFIX=") {
					t.Errorf("CCACHE_PREFIX should not be set in cross-compilation slice. Got: %s", envVar)
				}
			}
		})
	}
}
