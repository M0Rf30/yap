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
