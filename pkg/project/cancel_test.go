// SPDX-FileCopyrightText: 2021-2026 Gianluca Boiano
// SPDX-License-Identifier: GPL-3.0-only

package project

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/M0Rf30/yap/v2/pkg/builder"
	"github.com/M0Rf30/yap/v2/pkg/pkgbuild"
)

func cancelledProject() *Project {
	return &Project{
		Builder: &builder.Builder{PKGBUILD: &pkgbuild.PKGBUILD{PkgName: "p"}},
	}
}

// TestCancellationPropagates verifies the already-cancelled context reaches
// every stage of the pipeline instead of being replaced by context.Background().
func TestCancellationPropagates(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	mpc := &MultipleProject{Output: t.TempDir()}

	t.Run("createPackage", func(t *testing.T) {
		err := mpc.createPackage(ctx, cancelledProject())
		require.Error(t, err)
		assert.True(t, errors.Is(err, context.Canceled))
	})

	t.Run("buildProjectsParallel", func(t *testing.T) {
		err := mpc.buildProjectsParallel(ctx, []*Project{cancelledProject()}, 1, false)
		require.Error(t, err)
		assert.True(t, errors.Is(err, context.Canceled))
	})

	t.Run("buildProjectsInOrder", func(t *testing.T) {
		order := [][]*Project{{cancelledProject()}}
		err := mpc.buildProjectsInOrder(ctx, order, 1)
		require.Error(t, err)
		assert.True(t, errors.Is(err, context.Canceled))
	})

	t.Run("buildProjectsSequential", func(t *testing.T) {
		err := mpc.buildProjectsSequential(ctx, []*Project{cancelledProject()})
		require.Error(t, err)
		assert.True(t, errors.Is(err, context.Canceled))
	})
}
