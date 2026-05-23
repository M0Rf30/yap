package common

import (
	"os"
	"strings"
	"testing"

	"github.com/M0Rf30/yap/v2/pkg/constants"
	"github.com/M0Rf30/yap/v2/pkg/pkgbuild"
)

func TestGetGNUTriplet(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		arch           string
		expectedTriple string
	}{
		{
			name:           "aarch64 architecture",
			arch:           "aarch64",
			expectedTriple: "aarch64-linux-gnu",
		},
		{
			name:           "armv7 architecture",
			arch:           "armv7",
			expectedTriple: "arm-linux-gnueabihf",
		},
		{
			name:           "armv6 architecture",
			arch:           "armv6",
			expectedTriple: "arm-linux-gnueabihf",
		},
		{
			name:           "i686 architecture",
			arch:           "i686",
			expectedTriple: "i686-linux-gnu",
		},
		{
			name:           "x86_64 architecture",
			arch:           "x86_64",
			expectedTriple: "x86_64-linux-gnu",
		},
		{
			name:           "ppc64le architecture",
			arch:           "ppc64le",
			expectedTriple: "powerpc64le-linux-gnu",
		},
		{
			name:           "s390x architecture",
			arch:           "s390x",
			expectedTriple: "s390x-linux-gnu",
		},
		{
			name:           "riscv64 architecture",
			arch:           "riscv64",
			expectedTriple: "riscv64-linux-gnu",
		},
		{
			name:           "unknown architecture returns empty",
			arch:           "unknown",
			expectedTriple: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			bb := &BaseBuilder{
				PKGBUILD: &pkgbuild.PKGBUILD{},
				Format:   constants.FormatDEB,
			}

			triplet := bb.getGNUTriplet(tt.arch)
			if triplet != tt.expectedTriple {
				t.Errorf("getGNUTriplet(%s) = %s; want %s", tt.arch, triplet, tt.expectedTriple)
			}
		})
	}
}

func TestBuildCrossEnvSlice_AutoconfVariables(t *testing.T) {
	tests := []struct {
		name              string
		targetArch        string
		buildArch         string
		format            string
		expectedHostTrip  string
		expectedBuildTrip string
	}{
		{
			name:              "aarch64 cross-compilation on x86_64",
			targetArch:        "aarch64",
			buildArch:         "x86_64",
			format:            constants.FormatDEB,
			expectedHostTrip:  "aarch64-linux-gnu",
			expectedBuildTrip: "x86_64-linux-gnu",
		},
		{
			name:              "armv7 cross-compilation on x86_64",
			targetArch:        "armv7",
			buildArch:         "x86_64",
			format:            constants.FormatDEB,
			expectedHostTrip:  "arm-linux-gnueabihf",
			expectedBuildTrip: "x86_64-linux-gnu",
		},
		{
			name:              "i686 cross-compilation on x86_64",
			targetArch:        "i686",
			buildArch:         "x86_64",
			format:            constants.FormatDEB,
			expectedHostTrip:  "i686-linux-gnu",
			expectedBuildTrip: "x86_64-linux-gnu",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			bb := &BaseBuilder{
				PKGBUILD: &pkgbuild.PKGBUILD{
					ArchComputed: tt.buildArch,
					TargetArch:   tt.targetArch,
				},
				Format: tt.format,
			}

			envSlice, err := bb.BuildCrossEnvSlice(tt.targetArch)
			if err != nil {
				t.Fatalf("BuildCrossEnvSlice() error = %v", err)
			}

			if envSlice == nil {
				t.Logf("BuildCrossEnvSlice returned nil (toolchain not available)")
				return
			}

			// ac_cv_host / ac_cv_build must NOT be set in the slice.
			// Setting them poisons sub-configures (e.g. ICU's icu-native/)
			// that need the host triplet, not the cross triplet.
			// Autoconf derives these from --host/--build flags instead.
			for _, envVar := range envSlice {
				if strings.HasPrefix(envVar, "ac_cv_host=") {
					t.Errorf("ac_cv_host should not be set in slice (got %q); use --host flag instead", envVar)
				}

				if strings.HasPrefix(envVar, "ac_cv_build=") {
					t.Errorf("ac_cv_build should not be set in slice (got %q); use --build flag instead", envVar)
				}
			}

			// Verify C/C++ compiler is set in the slice
			var cc, cxx string

			for _, envVar := range envSlice {
				if after, ok := strings.CutPrefix(envVar, "CC="); ok {
					cc = after
				}

				if after, ok := strings.CutPrefix(envVar, "CXX="); ok {
					cxx = after
				}
			}

			if cc == "" {
				t.Error("CC environment variable should be set in slice for cross-compilation")
			}

			if cxx == "" {
				t.Error("CXX environment variable should be set in slice for cross-compilation")
			}
		})
	}
}

func TestSetupCrossCompilationEnvironment_NoAutoconfForNoCrossCompilation(t *testing.T) {
	// Note: Not using t.Parallel() since we're testing global environment variables

	// Save original environment
	origHost := os.Getenv("ac_cv_host")
	origBuild := os.Getenv("ac_cv_build")

	defer func() {
		_ = os.Setenv("ac_cv_host", origHost)
		_ = os.Setenv("ac_cv_build", origBuild)
	}()

	// Clear environment
	_ = os.Unsetenv("ac_cv_host")
	_ = os.Unsetenv("ac_cv_build")

	bb := &BaseBuilder{
		PKGBUILD: &pkgbuild.PKGBUILD{
			ArchComputed: "x86_64",
			TargetArch:   "x86_64", // Same as build arch - no cross-compilation
		},
		Format: constants.FormatDEB,
	}

	envSlice, err := bb.BuildCrossEnvSlice("x86_64")
	if err != nil {
		t.Fatalf("BuildCrossEnvSlice() error = %v", err)
	}

	// When not cross-compiling, should return nil
	if envSlice != nil {
		t.Errorf("BuildCrossEnvSlice should return nil when not cross-compiling, got: %v", envSlice)
	}

	// Verify no autoconf variables are in the slice
	for _, envVar := range envSlice {
		if strings.HasPrefix(envVar, "ac_cv_") {
			t.Errorf("Autoconf variables should not be set when not cross-compiling: %s", envVar)
		}
	}
}
