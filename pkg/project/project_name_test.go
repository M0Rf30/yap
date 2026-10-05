// SPDX-FileCopyrightText: 2021-2026 Gianluca Boiano
// SPDX-License-Identifier: GPL-3.0-only

package project

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResolveProjectDirsContainment(t *testing.T) {
	root := t.TempDir()
	build := filepath.Join(root, "build")
	src := filepath.Join(root, "src")

	mpc := &MultipleProject{BuildDir: build}

	for _, bad := range []string{"x/../..", "../evil", "a/../../b", ".", "a/.."} {
		_, _, err := mpc.resolveProjectDirs(src, bad)
		assert.Error(t, err, bad)
	}

	startDir, home, err := mpc.resolveProjectDirs(src, "good/sub")
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(build, "good", "sub"), startDir)
	assert.Equal(t, filepath.Join(src, "good", "sub"), home)
}

func TestResolveProjectDirsSingleProject(t *testing.T) {
	dir := t.TempDir()
	mpc := &MultipleProject{BuildDir: dir, singleProject: true}

	startDir, home, err := mpc.resolveProjectDirs(dir, "")
	require.NoError(t, err)
	assert.Equal(t, dir, startDir)
	assert.Equal(t, dir, home)
}

func TestPopulateProjectsRejectsEscapingName(t *testing.T) {
	root := t.TempDir()
	mpc := &MultipleProject{
		BuildDir: filepath.Join(root, "build"),
		Projects: []*Project{{Name: "x/../.."}},
	}

	err := mpc.populateProjects("ubuntu", "", root)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid project name")
}
