package project

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/M0Rf30/yap/v2/pkg/builder"
	"github.com/M0Rf30/yap/v2/pkg/pkgbuild"
)

func TestFindGitRoot(t *testing.T) {
	t.Run("finds .git directory at repo root", func(t *testing.T) {
		root := t.TempDir()
		if err := os.Mkdir(filepath.Join(root, ".git"), 0o755); err != nil {
			t.Fatal(err)
		}

		sub := filepath.Join(root, "packages", "mypkg")
		if err := os.MkdirAll(sub, 0o755); err != nil {
			t.Fatal(err)
		}

		got := findGitRoot(sub)
		if got != root {
			t.Errorf("findGitRoot(%q) = %q, want %q", sub, got, root)
		}
	})

	t.Run("skips .git file (submodule marker), finds parent .git dir", func(t *testing.T) {
		root := t.TempDir()
		if err := os.Mkdir(filepath.Join(root, ".git"), 0o755); err != nil {
			t.Fatal(err)
		}

		// Submodule: packages/ has a .git file, not a directory.
		packages := filepath.Join(root, "packages")
		if err := os.Mkdir(packages, 0o755); err != nil {
			t.Fatal(err)
		}

		if err := os.WriteFile(filepath.Join(packages, ".git"), []byte("gitdir: ../.git/modules/packages\n"), 0o644); err != nil {
			t.Fatal(err)
		}

		sub := filepath.Join(packages, "mypkg")
		if err := os.Mkdir(sub, 0o755); err != nil {
			t.Fatal(err)
		}

		got := findGitRoot(sub)
		if got != root {
			t.Errorf("findGitRoot(%q) = %q, want %q", sub, got, root)
		}
	})

	t.Run("falls back to parent of dir when no .git directory found", func(t *testing.T) {
		// Simulate a CI staging layout: staging/packages/ with no .git anywhere.
		// findGitRoot should return staging/ (parent of packages/).
		isolated := t.TempDir()
		packages := filepath.Join(isolated, "staging", "packages")

		if err := os.MkdirAll(packages, 0o755); err != nil {
			t.Fatal(err)
		}

		got := findGitRoot(packages)

		// If an ancestor of isolated has a real .git dir, accept that result.
		// Otherwise expect the parent of packages: isolated/staging.
		want := filepath.Join(isolated, "staging")
		if got != want {
			info, err := os.Stat(filepath.Join(got, ".git"))
			if err != nil || !info.IsDir() {
				t.Errorf("findGitRoot(%q) = %q, want %q", packages, got, want)
			}
		}
	})
}

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
			expected: "gcc",
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

	// Get make dependencies
	makeDepends := mpc.getMakeDeps()

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

	// Get runtime dependencies
	runtimeDepends := mpc.getRuntimeDeps()

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

func TestGetRuntimeDepsWithSkipFilter(t *testing.T) {
	// Setup: 3 packages in yap.json — pkga, pkgb, pkgc.
	// pkgb depends on pkga (internal).
	// pkgc depends on pkgb (internal) and libexternal (external).
	t.Run("skipped package without local artifact becomes external", func(t *testing.T) {
		// Hybrid rule: when --skip removes pkga and no built artifact for pkga
		// exists in Output, pkga is treated as external so the package manager
		// can fetch it. Was previously the inverse; see getRuntimeDeps doc.
		mpc := &MultipleProject{
			Projects: []*Project{
				{
					Builder: &builder.Builder{
						PKGBUILD: &pkgbuild.PKGBUILD{
							PkgName: "pkga",
							Depends: []string{},
						},
					},
				},
				{
					Builder: &builder.Builder{
						PKGBUILD: &pkgbuild.PKGBUILD{
							PkgName: "pkgb",
							Depends: []string{"pkga"},
						},
					},
				},
				{
					Builder: &builder.Builder{
						PKGBUILD: &pkgbuild.PKGBUILD{
							PkgName: "pkgc",
							Depends: []string{"pkgb", "libexternal"},
						},
					},
				},
			},
		}

		// Snapshot the full project list before filtering (simulating populateProjects behavior)
		mpc.allProjects = append([]*Project(nil), mpc.Projects...)

		// Simulate --skip pkga: remove pkga from Projects but keep it in allProjects
		mpc.Projects = mpc.Projects[1:] // Remove pkga, keep pkgb and pkgc

		// Get runtime dependencies
		runtimeDeps := mpc.getRuntimeDeps()

		// New hybrid behaviour: pkga was filtered out and has no local artifact
		// in Output, so it MUST appear in runtimeDepends so apt can fetch it.
		foundPkga := false

		for _, dep := range runtimeDeps {
			if extractPackageName(dep) == "pkga" {
				foundPkga = true
				break
			}
		}

		if !foundPkga {
			t.Errorf("skipped package pkga without local artifact should be in runtimeDepends")
		}

		// libexternal SHOULD be in runtimeDepends
		found := false

		for _, dep := range runtimeDeps {
			if extractPackageName(dep) == "libexternal" {
				found = true
				break
			}
		}

		if !found {
			t.Errorf("libexternal should be in runtimeDepends")
		}

		// pkgb and pkgc should NOT be in runtimeDepends (they are internal)
		for _, dep := range runtimeDeps {
			depName := extractPackageName(dep)
			if depName == "pkgb" {
				t.Errorf("internal package pkgb should not be in runtimeDepends")
			}

			if depName == "pkgc" {
				t.Errorf("internal package pkgc should not be in runtimeDepends")
			}
		}
	})

	t.Run("only selected packages, deps of unselected still excluded", func(t *testing.T) {
		// When --only selects only pkgc, pkgb (which pkgc depends on) should still
		// be treated as internal and NOT included in the runtime deps to download.
		mpc := &MultipleProject{
			Projects: []*Project{
				{
					Builder: &builder.Builder{
						PKGBUILD: &pkgbuild.PKGBUILD{
							PkgName: "pkga",
							Depends: []string{},
						},
					},
				},
				{
					Builder: &builder.Builder{
						PKGBUILD: &pkgbuild.PKGBUILD{
							PkgName: "pkgb",
							Depends: []string{"pkga"},
						},
					},
				},
				{
					Builder: &builder.Builder{
						PKGBUILD: &pkgbuild.PKGBUILD{
							PkgName: "pkgc",
							Depends: []string{"pkgb", "libexternal"},
						},
					},
				},
			},
		}

		// Snapshot the full project list before filtering (simulating populateProjects behavior)
		mpc.allProjects = append([]*Project(nil), mpc.Projects...)

		// Simulate --only pkgc: keep only pkgc in Projects
		mpc.Projects = mpc.Projects[2:] // Keep only pkgc

		// Get runtime dependencies
		runtimeDeps := mpc.getRuntimeDeps()

		// New hybrid behaviour: pkgb is filtered out and has no local artifact,
		// so it must be downloaded externally.
		foundPkgb := false

		for _, dep := range runtimeDeps {
			if extractPackageName(dep) == "pkgb" {
				foundPkgb = true
				break
			}
		}

		if !foundPkgb {
			t.Errorf("filtered-out pkgb without local artifact should be in runtimeDepends")
		}

		// libexternal SHOULD be in runtimeDepends
		found := false

		for _, dep := range runtimeDeps {
			if extractPackageName(dep) == "libexternal" {
				found = true
				break
			}
		}

		if !found {
			t.Errorf("libexternal should be in runtimeDepends")
		}

		// pkga and pkgc should NOT be in runtimeDepends (they are internal)
		for _, dep := range runtimeDeps {
			depName := extractPackageName(dep)
			if depName == "pkga" {
				t.Errorf("internal package pkga should not be in runtimeDepends")
			}

			if depName == "pkgc" {
				t.Errorf("internal package pkgc should not be in runtimeDepends")
			}
		}
	})
}

func TestGetRuntimeDepsLocalArtifactKeepsInternal(t *testing.T) {
	// When a filtered-out package has a built artifact in Output, it must be
	// treated as internal (NOT downloaded).
	outDir := t.TempDir()

	// Drop a fake artifact for pkga.
	if err := os.WriteFile(filepath.Join(outDir, "pkga-1.0.0-1.x86_64.rpm"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	mpc := &MultipleProject{
		Output: outDir,
		Projects: []*Project{
			{Builder: &builder.Builder{PKGBUILD: &pkgbuild.PKGBUILD{PkgName: "pkga"}}},
			{Builder: &builder.Builder{PKGBUILD: &pkgbuild.PKGBUILD{PkgName: "pkgb", Depends: []string{"pkga", "libexternal"}}}},
		},
	}

	mpc.allProjects = append([]*Project(nil), mpc.Projects...)
	// Simulate --only pkgb
	mpc.Projects = mpc.Projects[1:]

	deps := mpc.getRuntimeDeps()
	for _, d := range deps {
		if extractPackageName(d) == "pkga" {
			t.Errorf("pkga has a local artifact and must NOT be in runtimeDepends, got %v", deps)
		}
	}

	foundExt := false

	for _, d := range deps {
		if extractPackageName(d) == "libexternal" {
			foundExt = true
		}
	}

	if !foundExt {
		t.Errorf("libexternal should still be in runtimeDepends")
	}
}

func TestLocalArtifactExistsPrefixSafety(t *testing.T) {
	// foo-curl-dev-1.0.0... must NOT match a query for foo-curl.
	outDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(outDir, "foo-curl-dev-1.0.0-1.x86_64.rpm"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	mpc := &MultipleProject{Output: outDir}
	if mpc.localArtifactExists("foo-curl") {
		t.Errorf("foo-curl should not match foo-curl-dev artifact")
	}

	if !mpc.localArtifactExists("foo-curl-dev") {
		t.Errorf("foo-curl-dev should match its own artifact")
	}
}
