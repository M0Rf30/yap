//nolint:testpackage // exercises the unexported readProject/shouldSkipFile paths
package project

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestReadProjectNfpmOnlySetsSingleProject proves readProject recognises a
// directory containing only an nfpm spec exactly like a PKGBUILD-only
// directory: single-project mode is entered and BuildDir/Output/Projects are
// populated from the directory itself.
func TestReadProjectNfpmOnlySetsSingleProject(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "nfpm.yaml"),
		[]byte("name: demo\nversion: \"1.0.0\"\n"), 0o644))

	mpc := &MultipleProject{}
	err := mpc.readProject(dir)
	require.NoError(t, err)

	assert.True(t, mpc.singleProject, "expected single-project mode for an nfpm-only directory")
	assert.Equal(t, filepath.Clean(dir), mpc.BuildDir)
	assert.Equal(t, filepath.Clean(dir), mpc.Output)
	require.Len(t, mpc.Projects, 1)
}

// TestReadProjectNfpmDotfileSpecSetsSingleProject proves the dotfile alias
// (.nfpm.yaml) is recognised too, mirroring nfpm's own spec discovery rules.
func TestReadProjectNfpmDotfileSpecSetsSingleProject(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".nfpm.yaml"),
		[]byte("name: demo\nversion: \"1.0.0\"\n"), 0o644))

	mpc := &MultipleProject{}
	err := mpc.readProject(dir)
	require.NoError(t, err)

	assert.True(t, mpc.singleProject, "expected single-project mode for a dotfile nfpm spec")
}

// TestReadProjectPKGBUILDWinsOverNfpm proves that when both a PKGBUILD and an
// nfpm spec exist in the same directory, the PKGBUILD is still what drives
// single-project mode (existing precedence, unchanged by nfpm support).
func TestReadProjectPKGBUILDWinsOverNfpm(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "PKGBUILD"),
		[]byte("pkgname=demo\npkgver=1\npkgrel=1\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "nfpm.yaml"),
		[]byte("name: demo\nversion: \"1.0.0\"\n"), 0o644))

	mpc := &MultipleProject{}
	err := mpc.readProject(dir)
	require.NoError(t, err)

	assert.True(t, mpc.singleProject)
}

// TestShouldSkipFileNfpmSpecNotSkipped proves nfpm spec files (including the
// dotfile aliases) and their payload files survive copyProjects' skip
// filter, exactly like PKGBUILD does, while build artifacts and unrelated
// dotfiles are still skipped.
func TestShouldSkipFileNfpmSpecNotSkipped(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()

	cases := []struct {
		name     string
		filename string
		wantSkip bool
	}{
		{"nfpm.yaml is kept", "nfpm.yaml", false},
		{"dotfile nfpm.yaml alias is kept", ".nfpm.yaml", false},
		{"payload file is kept", "payload.txt", false},
		{"unrelated dotfile is skipped", ".hidden", true},
		{"built deb artifact is skipped", "out.deb", true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			src := filepath.Join(dir, tc.filename)
			require.NoError(t, os.WriteFile(src, []byte("data"), 0o644))

			info, err := os.Stat(src)
			require.NoError(t, err)

			dest := filepath.Join(t.TempDir(), tc.filename)

			skip, err := shouldSkipFile(info, src, dest)
			require.NoError(t, err)
			assert.Equal(t, tc.wantSkip, skip)
		})
	}
}
