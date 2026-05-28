package project

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/M0Rf30/yap/v2/pkg/builder"
	"github.com/M0Rf30/yap/v2/pkg/pkgbuild"
)

// TestGetProjectsInRange tests the getProjectsInRange method with various flag combinations.
func TestGetProjectsInRange(t *testing.T) {
	tests := []struct {
		name          string
		projects      []*Project
		fromPkgName   string
		toPkgName     string
		expectedCount int
		expectedNames []string
	}{
		{
			name: "no filtering - return all projects",
			projects: []*Project{
				{Builder: &builder.Builder{PKGBUILD: &pkgbuild.PKGBUILD{PkgName: "pkg-a"}}},
				{Builder: &builder.Builder{PKGBUILD: &pkgbuild.PKGBUILD{PkgName: "pkg-b"}}},
				{Builder: &builder.Builder{PKGBUILD: &pkgbuild.PKGBUILD{PkgName: "pkg-c"}}},
			},
			fromPkgName:   "",
			toPkgName:     "",
			expectedCount: 3,
			expectedNames: []string{"pkg-a", "pkg-b", "pkg-c"},
		},
		{
			name: "from package only",
			projects: []*Project{
				{Builder: &builder.Builder{PKGBUILD: &pkgbuild.PKGBUILD{PkgName: "pkg-a"}}},
				{Builder: &builder.Builder{PKGBUILD: &pkgbuild.PKGBUILD{PkgName: "pkg-b"}}},
				{Builder: &builder.Builder{PKGBUILD: &pkgbuild.PKGBUILD{PkgName: "pkg-c"}}},
			},
			fromPkgName:   "pkg-b",
			toPkgName:     "",
			expectedCount: 2,
			expectedNames: []string{"pkg-b", "pkg-c"},
		},
		{
			name: "to package only",
			projects: []*Project{
				{Builder: &builder.Builder{PKGBUILD: &pkgbuild.PKGBUILD{PkgName: "pkg-a"}}},
				{Builder: &builder.Builder{PKGBUILD: &pkgbuild.PKGBUILD{PkgName: "pkg-b"}}},
				{Builder: &builder.Builder{PKGBUILD: &pkgbuild.PKGBUILD{PkgName: "pkg-c"}}},
			},
			fromPkgName:   "",
			toPkgName:     "pkg-b",
			expectedCount: 2,
			expectedNames: []string{"pkg-a", "pkg-b"},
		},
		{
			name: "from and to package",
			projects: []*Project{
				{Builder: &builder.Builder{PKGBUILD: &pkgbuild.PKGBUILD{PkgName: "pkg-a"}}},
				{Builder: &builder.Builder{PKGBUILD: &pkgbuild.PKGBUILD{PkgName: "pkg-b"}}},
				{Builder: &builder.Builder{PKGBUILD: &pkgbuild.PKGBUILD{PkgName: "pkg-c"}}},
				{Builder: &builder.Builder{PKGBUILD: &pkgbuild.PKGBUILD{PkgName: "pkg-d"}}},
			},
			fromPkgName:   "pkg-b",
			toPkgName:     "pkg-c",
			expectedCount: 2,
			expectedNames: []string{"pkg-b", "pkg-c"},
		},
		{
			name: "single package range",
			projects: []*Project{
				{Builder: &builder.Builder{PKGBUILD: &pkgbuild.PKGBUILD{PkgName: "pkg-a"}}},
				{Builder: &builder.Builder{PKGBUILD: &pkgbuild.PKGBUILD{PkgName: "pkg-b"}}},
			},
			fromPkgName:   "pkg-a",
			toPkgName:     "pkg-a",
			expectedCount: 1,
			expectedNames: []string{"pkg-a"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mpc := &MultipleProject{
				Projects: tt.projects,
				Opts: BuildOptions{
					FromPkgName: tt.fromPkgName,
					ToPkgName:   tt.toPkgName,
				},
			}
			result := mpc.getProjectsInRange()

			assert.Equal(t, tt.expectedCount, len(result), "expected count mismatch")

			for i, proj := range result {
				assert.Equal(t, tt.expectedNames[i], proj.Builder.PKGBUILD.PkgName)
			}
		})
	}
}

// TestCheckPkgsRange tests the checkPkgsRange method for validation.
func TestCheckPkgsRange(t *testing.T) {
	tests := []struct {
		name        string
		projects    []*Project
		fromPkgName string
		toPkgName   string
		expectError bool
		errorMsg    string
	}{
		{
			name: "valid range",
			projects: []*Project{
				{Builder: &builder.Builder{PKGBUILD: &pkgbuild.PKGBUILD{PkgName: "pkg-a"}}},
				{Builder: &builder.Builder{PKGBUILD: &pkgbuild.PKGBUILD{PkgName: "pkg-b"}}},
				{Builder: &builder.Builder{PKGBUILD: &pkgbuild.PKGBUILD{PkgName: "pkg-c"}}},
			},
			fromPkgName: "pkg-a",
			toPkgName:   "pkg-c",
			expectError: false,
		},
		{
			name: "invalid range - from after to",
			projects: []*Project{
				{Builder: &builder.Builder{PKGBUILD: &pkgbuild.PKGBUILD{PkgName: "pkg-a"}}},
				{Builder: &builder.Builder{PKGBUILD: &pkgbuild.PKGBUILD{PkgName: "pkg-b"}}},
				{Builder: &builder.Builder{PKGBUILD: &pkgbuild.PKGBUILD{PkgName: "pkg-c"}}},
			},
			fromPkgName: "pkg-c",
			toPkgName:   "pkg-a",
			expectError: true,
		},
		{
			name: "from package not found",
			projects: []*Project{
				{Builder: &builder.Builder{PKGBUILD: &pkgbuild.PKGBUILD{PkgName: "pkg-a"}}},
			},
			fromPkgName: "pkg-nonexistent",
			toPkgName:   "",
			expectError: true,
		},
		{
			name: "to package not found",
			projects: []*Project{
				{Builder: &builder.Builder{PKGBUILD: &pkgbuild.PKGBUILD{PkgName: "pkg-a"}}},
			},
			fromPkgName: "",
			toPkgName:   "pkg-nonexistent",
			expectError: true,
		},
		{
			name: "no range specified",
			projects: []*Project{
				{Builder: &builder.Builder{PKGBUILD: &pkgbuild.PKGBUILD{PkgName: "pkg-a"}}},
			},
			fromPkgName: "",
			toPkgName:   "",
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mpc := &MultipleProject{
				Projects: tt.projects,
			}

			err := mpc.checkPkgsRange(tt.fromPkgName, tt.toPkgName)

			if tt.expectError {
				assert.Error(t, err, "expected error but got none")
			} else {
				assert.NoError(t, err, "expected no error but got: %v", err)
			}
		})
	}
}

// TestFindPackageInProjects tests the findPackageInProjects method.
func TestFindPackageInProjects(t *testing.T) {
	tests := []struct {
		name        string
		projects    []*Project
		pkgName     string
		expectedIdx int
		expectError bool
	}{
		{
			name: "find first package",
			projects: []*Project{
				{Builder: &builder.Builder{PKGBUILD: &pkgbuild.PKGBUILD{PkgName: "pkg-a"}}},
				{Builder: &builder.Builder{PKGBUILD: &pkgbuild.PKGBUILD{PkgName: "pkg-b"}}},
			},
			pkgName:     "pkg-a",
			expectedIdx: 0,
			expectError: false,
		},
		{
			name: "find last package",
			projects: []*Project{
				{Builder: &builder.Builder{PKGBUILD: &pkgbuild.PKGBUILD{PkgName: "pkg-a"}}},
				{Builder: &builder.Builder{PKGBUILD: &pkgbuild.PKGBUILD{PkgName: "pkg-b"}}},
				{Builder: &builder.Builder{PKGBUILD: &pkgbuild.PKGBUILD{PkgName: "pkg-c"}}},
			},
			pkgName:     "pkg-c",
			expectedIdx: 2,
			expectError: false,
		},
		{
			name: "package not found",
			projects: []*Project{
				{Builder: &builder.Builder{PKGBUILD: &pkgbuild.PKGBUILD{PkgName: "pkg-a"}}},
			},
			pkgName:     "pkg-nonexistent",
			expectedIdx: -1,
			expectError: true,
		},
		{
			name:        "empty projects list",
			projects:    []*Project{},
			pkgName:     "pkg-a",
			expectedIdx: -1,
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mpc := &MultipleProject{
				Projects: tt.projects,
			}

			idx, err := mpc.findPackageInProjects(tt.pkgName)

			if tt.expectError {
				assert.Error(t, err, "expected error but got none")
				assert.Equal(t, -1, idx, "expected index -1 on error")
			} else {
				assert.NoError(t, err, "expected no error but got: %v", err)
				assert.Equal(t, tt.expectedIdx, idx, "index mismatch")
			}
		})
	}
}

// TestShouldSkipFile tests the shouldSkipFile function.
func TestShouldSkipFile(t *testing.T) {
	tmpDir := t.TempDir()

	tests := []struct {
		name       string
		filename   string
		shouldSkip bool
	}{
		{
			name:       "skip .deb file",
			filename:   "package.deb",
			shouldSkip: true,
		},
		{
			name:       "skip .rpm file",
			filename:   "package.rpm",
			shouldSkip: true,
		},
		{
			name:       "skip .apk file",
			filename:   "package.apk",
			shouldSkip: true,
		},
		{
			name:       "skip .pkg.tar.zst file",
			filename:   "package.pkg.tar.zst",
			shouldSkip: true,
		},
		{
			name:       "skip hidden file",
			filename:   ".hidden",
			shouldSkip: true,
		},
		{
			name:       "skip temp file",
			filename:   "file.tmp",
			shouldSkip: true,
		},
		{
			name:       "skip backup file",
			filename:   "file~",
			shouldSkip: true,
		},
		{
			name:       "skip Thumbs.db",
			filename:   "Thumbs.db",
			shouldSkip: true,
		},
		{
			name:       "skip .DS_Store",
			filename:   ".DS_Store",
			shouldSkip: true,
		},
		{
			name:       "keep regular file",
			filename:   "PKGBUILD",
			shouldSkip: false,
		},
		{
			name:       "keep source archive",
			filename:   "source.tar.gz",
			shouldSkip: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a temporary file
			filePath := filepath.Join(tmpDir, tt.filename)
			err := os.WriteFile(filePath, []byte("test"), 0o644)
			require.NoError(t, err)

			// Get file info
			info, err := os.Stat(filePath)
			require.NoError(t, err)

			// Test shouldSkipFile
			skip, err := shouldSkipFile(info, filePath, filepath.Join(tmpDir, "dest", tt.filename))
			assert.NoError(t, err)
			assert.Equal(t, tt.shouldSkip, skip, "skip mismatch for %s", tt.filename)
		})
	}
}

// TestShouldSkipFileWithExistingDest tests shouldSkipFile when destination exists.
func TestShouldSkipFileWithExistingDest(t *testing.T) {
	tmpDir := t.TempDir()
	srcDir := filepath.Join(tmpDir, "src")
	destDir := filepath.Join(tmpDir, "dest")

	require.NoError(t, os.MkdirAll(srcDir, 0o755))
	require.NoError(t, os.MkdirAll(destDir, 0o755))

	// Create source file
	srcFile := filepath.Join(srcDir, "test.txt")
	require.NoError(t, os.WriteFile(srcFile, []byte("content"), 0o644))

	srcInfo, err := os.Stat(srcFile)
	require.NoError(t, err)

	// Create identical destination file
	destFile := filepath.Join(destDir, "test.txt")
	require.NoError(t, os.WriteFile(destFile, []byte("content"), 0o644))

	// Set destination to have same modification time
	require.NoError(t, os.Chtimes(destFile, srcInfo.ModTime(), srcInfo.ModTime()))

	// Test shouldSkipFile - should skip because dest exists with same size and mtime
	skip, err := shouldSkipFile(srcInfo, srcFile, destFile)
	assert.NoError(t, err)
	assert.True(t, skip, "should skip file when destination exists with same size and mtime")
}

// TestLocalArtifactExists tests the localArtifactExists method.
func TestLocalArtifactExists(t *testing.T) {
	tmpDir := t.TempDir()

	tests := []struct {
		name        string
		pkgName     string
		artifacts   []string
		shouldExist bool
	}{
		{
			name:        "deb artifact exists",
			pkgName:     "mypackage",
			artifacts:   []string{"mypackage-1.0.0-1.x86_64.deb"},
			shouldExist: true,
		},
		{
			name:        "rpm artifact exists",
			pkgName:     "mypackage",
			artifacts:   []string{"mypackage-1.0.0-1.x86_64.rpm"},
			shouldExist: true,
		},
		{
			name:        "apk artifact exists",
			pkgName:     "mypackage",
			artifacts:   []string{"mypackage-1.0.0-r1.apk"},
			shouldExist: true,
		},
		{
			name:        "pacman artifact exists (zst)",
			pkgName:     "mypackage",
			artifacts:   []string{"mypackage-1.0.0-1-x86_64.pkg.tar.zst"},
			shouldExist: true,
		},
		{
			name:        "pacman artifact exists (xz)",
			pkgName:     "mypackage",
			artifacts:   []string{"mypackage-1.0.0-1-x86_64.pkg.tar.xz"},
			shouldExist: true,
		},
		{
			name:        "pacman artifact exists (gz)",
			pkgName:     "mypackage",
			artifacts:   []string{"mypackage-1.0.0-1-x86_64.pkg.tar.gz"},
			shouldExist: true,
		},
		{
			name:        "underscore separator",
			pkgName:     "mypackage",
			artifacts:   []string{"mypackage_1.0.0-1.x86_64.deb"},
			shouldExist: true,
		},
		{
			name:        "no artifact",
			pkgName:     "mypackage",
			artifacts:   []string{},
			shouldExist: false,
		},
		{
			name:        "prefix collision - should not match",
			pkgName:     "foo-curl",
			artifacts:   []string{"foo-curl-dev-1.0.0-1.x86_64.deb"},
			shouldExist: false,
		},
		{
			name:        "prefix collision - exact match",
			pkgName:     "foo-curl-dev",
			artifacts:   []string{"foo-curl-dev-1.0.0-1.x86_64.deb"},
			shouldExist: true,
		},
		{
			name:        "empty package name",
			pkgName:     "",
			artifacts:   []string{"mypackage-1.0.0-1.x86_64.deb"},
			shouldExist: false,
		},
		{
			name:        "case insensitive extension",
			pkgName:     "mypackage",
			artifacts:   []string{"mypackage-1.0.0-1.x86_64.DEB"},
			shouldExist: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create artifacts
			for _, artifact := range tt.artifacts {
				filePath := filepath.Join(tmpDir, artifact)
				require.NoError(t, os.WriteFile(filePath, []byte("x"), 0o644))
			}

			mpc := &MultipleProject{
				Output: tmpDir,
			}

			exists := mpc.localArtifactExists(tt.pkgName)
			assert.Equal(t, tt.shouldExist, exists, "localArtifactExists mismatch for %s", tt.pkgName)

			// Clean up for next iteration
			for _, artifact := range tt.artifacts {
				filePath := filepath.Join(tmpDir, artifact)
				_ = os.Remove(filePath)
			}
		})
	}
}

// TestLocalArtifactExistsEmptyOutput tests localArtifactExists with empty output directory.
func TestLocalArtifactExistsEmptyOutput(t *testing.T) {
	mpc := &MultipleProject{
		Output: "",
	}

	exists := mpc.localArtifactExists("mypackage")
	assert.False(t, exists, "should return false for empty output directory")
}

// TestTopologicalSort tests the topologicalSort method with various dependency scenarios.
func TestTopologicalSort(t *testing.T) {
	tests := []struct {
		name            string
		projects        map[string]*Project
		dependsOn       map[string][]string
		dependedBy      map[string][]string
		expectedBatches int
		expectError     bool
	}{
		{
			name: "no dependencies",
			projects: map[string]*Project{
				"pkg-a": {Builder: &builder.Builder{PKGBUILD: &pkgbuild.PKGBUILD{PkgName: "pkg-a"}}},
				"pkg-b": {Builder: &builder.Builder{PKGBUILD: &pkgbuild.PKGBUILD{PkgName: "pkg-b"}}},
			},
			dependsOn: map[string][]string{
				"pkg-a": {},
				"pkg-b": {},
			},
			dependedBy: map[string][]string{
				"pkg-a": {},
				"pkg-b": {},
			},
			expectedBatches: 1,
			expectError:     false,
		},
		{
			name: "linear dependency chain",
			projects: map[string]*Project{
				"pkg-a": {Builder: &builder.Builder{PKGBUILD: &pkgbuild.PKGBUILD{PkgName: "pkg-a"}}},
				"pkg-b": {Builder: &builder.Builder{PKGBUILD: &pkgbuild.PKGBUILD{PkgName: "pkg-b"}}},
				"pkg-c": {Builder: &builder.Builder{PKGBUILD: &pkgbuild.PKGBUILD{PkgName: "pkg-c"}}},
			},
			dependsOn: map[string][]string{
				"pkg-a": {},
				"pkg-b": {"pkg-a"},
				"pkg-c": {"pkg-b"},
			},
			dependedBy: map[string][]string{
				"pkg-a": {"pkg-b"},
				"pkg-b": {"pkg-c"},
				"pkg-c": {},
			},
			expectedBatches: 3,
			expectError:     false,
		},
		{
			name: "diamond dependency",
			projects: map[string]*Project{
				"pkg-a": {Builder: &builder.Builder{PKGBUILD: &pkgbuild.PKGBUILD{PkgName: "pkg-a"}}},
				"pkg-b": {Builder: &builder.Builder{PKGBUILD: &pkgbuild.PKGBUILD{PkgName: "pkg-b"}}},
				"pkg-c": {Builder: &builder.Builder{PKGBUILD: &pkgbuild.PKGBUILD{PkgName: "pkg-c"}}},
				"pkg-d": {Builder: &builder.Builder{PKGBUILD: &pkgbuild.PKGBUILD{PkgName: "pkg-d"}}},
			},
			dependsOn: map[string][]string{
				"pkg-a": {},
				"pkg-b": {"pkg-a"},
				"pkg-c": {"pkg-a"},
				"pkg-d": {"pkg-b", "pkg-c"},
			},
			dependedBy: map[string][]string{
				"pkg-a": {"pkg-b", "pkg-c"},
				"pkg-b": {"pkg-d"},
				"pkg-c": {"pkg-d"},
				"pkg-d": {},
			},
			expectedBatches: 3,
			expectError:     false,
		},
		{
			name: "circular dependency",
			projects: map[string]*Project{
				"pkg-a": {Builder: &builder.Builder{PKGBUILD: &pkgbuild.PKGBUILD{PkgName: "pkg-a"}}},
				"pkg-b": {Builder: &builder.Builder{PKGBUILD: &pkgbuild.PKGBUILD{PkgName: "pkg-b"}}},
			},
			dependsOn: map[string][]string{
				"pkg-a": {"pkg-b"},
				"pkg-b": {"pkg-a"},
			},
			dependedBy: map[string][]string{
				"pkg-a": {"pkg-b"},
				"pkg-b": {"pkg-a"},
			},
			expectedBatches: 0,
			expectError:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mpc := &MultipleProject{
				Projects: make([]*Project, 0, len(tt.projects)),
			}

			// Populate projects slice
			for _, proj := range tt.projects {
				mpc.Projects = append(mpc.Projects, proj)
			}

			batches, err := mpc.topologicalSort(tt.projects, tt.dependsOn, tt.dependedBy)

			if tt.expectError {
				assert.Error(t, err, "expected error but got none")
			} else {
				assert.NoError(t, err, "expected no error but got: %v", err)
				assert.Equal(t, tt.expectedBatches, len(batches), "batch count mismatch")
			}
		})
	}
}

// TestResolveDependencies tests the resolveDependencies method.
func TestResolveDependencies(t *testing.T) {
	tests := []struct {
		name            string
		projects        []*Project
		expectedBatches int
		expectError     bool
	}{
		{
			name: "single project no dependencies",
			projects: []*Project{
				{
					Builder: &builder.Builder{
						PKGBUILD: &pkgbuild.PKGBUILD{
							PkgName:     "pkg-a",
							Depends:     []string{},
							MakeDepends: []string{},
						},
					},
				},
			},
			expectedBatches: 1,
			expectError:     false,
		},
		{
			name: "multiple projects with internal dependencies",
			projects: []*Project{
				{
					Builder: &builder.Builder{
						PKGBUILD: &pkgbuild.PKGBUILD{
							PkgName:     "pkg-a",
							Depends:     []string{},
							MakeDepends: []string{},
						},
					},
				},
				{
					Builder: &builder.Builder{
						PKGBUILD: &pkgbuild.PKGBUILD{
							PkgName:     "pkg-b",
							Depends:     []string{"pkg-a"},
							MakeDepends: []string{},
						},
					},
				},
			},
			expectedBatches: 2,
			expectError:     false,
		},
		{
			name: "multiple projects with external dependencies",
			projects: []*Project{
				{
					Builder: &builder.Builder{
						PKGBUILD: &pkgbuild.PKGBUILD{
							PkgName:     "pkg-a",
							Depends:     []string{"libc"},
							MakeDepends: []string{"gcc"},
						},
					},
				},
			},
			expectedBatches: 1,
			expectError:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mpc := &MultipleProject{
				Projects: tt.projects,
			}

			batches, err := mpc.resolveDependencies(tt.projects)

			if tt.expectError {
				assert.Error(t, err, "expected error but got none")
			} else {
				assert.NoError(t, err, "expected no error but got: %v", err)
				assert.Equal(t, tt.expectedBatches, len(batches), "batch count mismatch")
			}
		})
	}
}

// TestCalculateDependencyPopularity tests the calculateDependencyPopularity method.
func TestCalculateDependencyPopularity(t *testing.T) {
	tests := []struct {
		name     string
		projects []*Project
		expected map[string]int
	}{
		{
			name: "no dependencies",
			projects: []*Project{
				{Builder: &builder.Builder{PKGBUILD: &pkgbuild.PKGBUILD{PkgName: "pkg-a", Depends: []string{}, MakeDepends: []string{}}}},
				{Builder: &builder.Builder{PKGBUILD: &pkgbuild.PKGBUILD{PkgName: "pkg-b", Depends: []string{}, MakeDepends: []string{}}}},
			},
			expected: map[string]int{
				"pkg-a": 0,
				"pkg-b": 0,
			},
		},
		{
			name: "single dependency",
			projects: []*Project{
				{Builder: &builder.Builder{PKGBUILD: &pkgbuild.PKGBUILD{PkgName: "pkg-a", Depends: []string{}, MakeDepends: []string{}}}},
				{Builder: &builder.Builder{PKGBUILD: &pkgbuild.PKGBUILD{PkgName: "pkg-b", Depends: []string{"pkg-a"}, MakeDepends: []string{}}}},
			},
			expected: map[string]int{
				"pkg-a": 1,
				"pkg-b": 0,
			},
		},
		{
			name: "multiple dependents",
			projects: []*Project{
				{Builder: &builder.Builder{PKGBUILD: &pkgbuild.PKGBUILD{PkgName: "pkg-a", Depends: []string{}, MakeDepends: []string{}}}},
				{Builder: &builder.Builder{PKGBUILD: &pkgbuild.PKGBUILD{PkgName: "pkg-b", Depends: []string{"pkg-a"}, MakeDepends: []string{}}}},
				{Builder: &builder.Builder{PKGBUILD: &pkgbuild.PKGBUILD{PkgName: "pkg-c", Depends: []string{"pkg-a"}, MakeDepends: []string{}}}},
			},
			expected: map[string]int{
				"pkg-a": 2,
				"pkg-b": 0,
				"pkg-c": 0,
			},
		},
		{
			name: "make dependencies",
			projects: []*Project{
				{Builder: &builder.Builder{PKGBUILD: &pkgbuild.PKGBUILD{PkgName: "pkg-a", Depends: []string{}, MakeDepends: []string{}}}},
				{Builder: &builder.Builder{PKGBUILD: &pkgbuild.PKGBUILD{PkgName: "pkg-b", Depends: []string{}, MakeDepends: []string{"pkg-a"}}}},
			},
			expected: map[string]int{
				"pkg-a": 1,
				"pkg-b": 0,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mpc := &MultipleProject{
				Projects: tt.projects,
			}

			popularity := mpc.calculateDependencyPopularity()

			for pkgName, expectedCount := range tt.expected {
				assert.Equal(t, expectedCount, popularity[pkgName], "popularity mismatch for %s", pkgName)
			}
		})
	}
}

// TestSetupCopyOptions tests the setupCopyOptions function.
func TestSetupCopyOptions(t *testing.T) {
	opts := setupCopyOptions()

	// Verify that the options are set correctly
	assert.NotNil(t, opts.OnSymlink, "OnSymlink should not be nil")
	assert.NotNil(t, opts.OnDirExists, "OnDirExists should not be nil")
	assert.NotNil(t, opts.Skip, "Skip should not be nil")
	assert.False(t, opts.Sync, "Sync should be false")
	assert.False(t, opts.PreserveTimes, "PreserveTimes should be false")
	assert.False(t, opts.PreserveOwner, "PreserveOwner should be false")
}
