package project

import (
	"testing"

	"github.com/M0Rf30/yap/v2/pkg/builder"
	"github.com/M0Rf30/yap/v2/pkg/pkgbuild"
)

func TestExtractPackageName(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "simple package name",
			input:    "gcc",
			expected: "gcc",
		},
		{
			name:     "package with version constraint without space (>=)",
			input:    "gcc>=11.0",
			expected: "gcc>=11.0",
		},
		{
			name:     "package with space and version",
			input:    "python3 >=3.9",
			expected: "python3",
		},
		{
			name:     "package with multiple spaces",
			input:    "libssl-dev   >=1.1.1",
			expected: "libssl-dev",
		},
		{
			name:     "empty string",
			input:    "",
			expected: "",
		},
		{
			name:     "only whitespace",
			input:    "   ",
			expected: "",
		},
		{
			name:     "package with complex version constraint",
			input:    "nodejs >=14.0 <20.0",
			expected: "nodejs",
		},
		{
			name:     "package with equals version",
			input:    "kernel =5.15.0",
			expected: "kernel",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractPackageName(tt.input)
			if result != tt.expected {
				t.Errorf("extractPackageName(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestProcessDependencies(t *testing.T) {
	// Create test package map
	packageMap := map[string]*Project{
		"pkg-a": {
			Builder: &builder.Builder{
				PKGBUILD: &pkgbuild.PKGBUILD{
					PkgName: "pkg-a",
				},
			},
		},
		"pkg-b": {
			Builder: &builder.Builder{
				PKGBUILD: &pkgbuild.PKGBUILD{
					PkgName: "pkg-b",
				},
			},
		},
		"pkg-c": {
			Builder: &builder.Builder{
				PKGBUILD: &pkgbuild.PKGBUILD{
					PkgName: "pkg-c",
				},
			},
		},
	}

	tests := []struct {
		name     string
		deps     []string
		expected []string
	}{
		{
			name:     "all dependencies exist",
			deps:     []string{"pkg-a", "pkg-b >=1.0", "pkg-c"},
			expected: []string{"pkg-a", "pkg-b", "pkg-c"},
		},
		{
			name:     "some dependencies don't exist",
			deps:     []string{"pkg-a", "pkg-external >=2.0", "pkg-b"},
			expected: []string{"pkg-a", "pkg-b"},
		},
		{
			name:     "no dependencies exist",
			deps:     []string{"pkg-external1", "pkg-external2"},
			expected: []string{},
		},
		{
			name:     "empty dependencies",
			deps:     []string{},
			expected: []string{},
		},
		{
			name:     "dependencies with whitespace",
			deps:     []string{"  pkg-a  ", "pkg-b", "   "},
			expected: []string{"pkg-a", "pkg-b"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var processed []string

			processDependencies(tt.deps, packageMap, func(depName string) {
				processed = append(processed, depName)
			})

			if len(processed) != len(tt.expected) {
				t.Errorf("processDependencies() processed %d deps, want %d", len(processed), len(tt.expected))
				return
			}

			for i, dep := range processed {
				if dep != tt.expected[i] {
					t.Errorf("processDependencies() processed[%d] = %q, want %q", i, dep, tt.expected[i])
				}
			}
		})
	}
}

func TestGetMakeDepsDeduplication(t *testing.T) {
	// Create test projects with duplicate make dependencies
	mpc := &MultipleProject{
		Projects: []*Project{
			{
				Builder: &builder.Builder{
					PKGBUILD: &pkgbuild.PKGBUILD{
						PkgName:     "pkg-a",
						MakeDepends: []string{"gcc", "make", "perl"},
					},
				},
			},
			{
				Builder: &builder.Builder{
					PKGBUILD: &pkgbuild.PKGBUILD{
						PkgName:     "pkg-b",
						MakeDepends: []string{"perl", "gcc", "autoconf"},
					},
				},
			},
			{
				Builder: &builder.Builder{
					PKGBUILD: &pkgbuild.PKGBUILD{
						PkgName:     "pkg-c",
						MakeDepends: []string{"make", "perl", "automake"},
					},
				},
			},
		},
	}

	// Reset global makeDepends before test and assign return value
	makeDepends = []string{}
	makeDepends = mpc.getMakeDeps()

	// Check that dependencies are deduplicated
	depCount := make(map[string]int)

	for _, dep := range makeDepends {
		depName := extractPackageName(dep)
		depCount[depName]++
	}

	// Each dependency should appear only once
	for depName, count := range depCount {
		if count > 1 {
			t.Errorf("Dependency %q appears %d times, expected 1", depName, count)
		}
	}

	// Check expected dependencies are present
	expectedDeps := map[string]bool{
		"gcc":      true,
		"make":     true,
		"perl":     true,
		"autoconf": true,
		"automake": true,
	}

	for expectedDep := range expectedDeps {
		found := false

		for _, dep := range makeDepends {
			if extractPackageName(dep) == expectedDep {
				found = true
				break
			}
		}

		if !found {
			t.Errorf("Expected dependency %q not found in makeDepends", expectedDep)
		}
	}
}

func TestGetRuntimeDepsDeduplication(t *testing.T) {
	// Create test projects with duplicate runtime dependencies
	mpc := &MultipleProject{
		Projects: []*Project{
			{
				Builder: &builder.Builder{
					PKGBUILD: &pkgbuild.PKGBUILD{
						PkgName: "pkg-a",
						Depends: []string{"libc", "perl", "zlib"},
					},
				},
			},
			{
				Builder: &builder.Builder{
					PKGBUILD: &pkgbuild.PKGBUILD{
						PkgName: "pkg-b",
						Depends: []string{"perl", "libc", "openssl"},
					},
				},
			},
			{
				Builder: &builder.Builder{
					PKGBUILD: &pkgbuild.PKGBUILD{
						PkgName: "pkg-c",
						Depends: []string{"zlib", "perl", "libxml2"},
					},
				},
			},
		},
	}

	// Reset global runtimeDepends before test and assign return value
	runtimeDepends = []string{}
	runtimeDepends = mpc.getRuntimeDeps()

	// Check that dependencies are deduplicated
	depCount := make(map[string]int)

	for _, dep := range runtimeDepends {
		depName := extractPackageName(dep)
		depCount[depName]++
	}

	// Each dependency should appear only once
	for depName, count := range depCount {
		if count > 1 {
			t.Errorf("Dependency %q appears %d times, expected 1", depName, count)
		}
	}

	// Check expected external dependencies are present (internal packages should be filtered)
	expectedDeps := map[string]bool{
		"libc":    true,
		"perl":    true,
		"zlib":    true,
		"openssl": true,
		"libxml2": true,
	}

	for expectedDep := range expectedDeps {
		found := false

		for _, dep := range runtimeDepends {
			if extractPackageName(dep) == expectedDep {
				found = true
				break
			}
		}

		if !found {
			t.Errorf("Expected dependency %q not found in runtimeDepends", expectedDep)
		}
	}

	// Check that internal dependencies are NOT present
	internalDeps := []string{"pkg-a", "pkg-b", "pkg-c"}
	for _, internalDep := range internalDeps {
		for _, dep := range runtimeDepends {
			if extractPackageName(dep) == internalDep {
				t.Errorf("Internal dependency %q should not be in runtimeDepends", internalDep)
			}
		}
	}
}
