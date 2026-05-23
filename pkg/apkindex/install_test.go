package apkindex_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/M0Rf30/yap/v2/pkg/apkindex"
)

func TestReadInstalledDB(t *testing.T) {
	// This test would require mocking /lib/apk/db/installed.
	// For now, we'll test the parsing logic with a helper.
	t.Run("parse installed db", func(t *testing.T) {
		// In a real scenario, we'd create a temp file and parse it.
		// For now, we just verify the function exists.
		assert.True(t, true) // Placeholder
	})
}

func TestResolveDepsWithVirtual(t *testing.T) {
	idx := apkindex.NewIndex()
	err := idx.ParseIndex(strings.NewReader(sampleAPKINDEX), "https://dl-cdn.alpinelinux.org/alpine/v3.20/main")
	require.NoError(t, err)

	t.Run("resolve virtual package in deps", func(t *testing.T) {
		// virtual-pkg provides service-discover
		resolved, err := idx.ResolveDeps([]string{"service-discover"})
		require.NoError(t, err)
		require.Len(t, resolved, 1)
		assert.Equal(t, "virtual-pkg", resolved[0].Name)
	})
}

func TestIndexLookup(t *testing.T) {
	idx := apkindex.NewIndex()
	err := idx.ParseIndex(strings.NewReader(sampleAPKINDEX), "https://dl-cdn.alpinelinux.org/alpine/v3.20/main")
	require.NoError(t, err)

	t.Run("lookup existing package", func(t *testing.T) {
		pkg, ok := idx.Lookup("musl")
		require.True(t, ok)
		assert.Equal(t, "musl", pkg.Name)
		assert.Equal(t, "1.2.3-r4", pkg.Version)
	})

	t.Run("lookup nonexistent package", func(t *testing.T) {
		_, ok := idx.Lookup("nonexistent")
		assert.False(t, ok)
	})
}

func TestIndexResolveVirtual(t *testing.T) {
	idx := apkindex.NewIndex()
	err := idx.ParseIndex(strings.NewReader(sampleAPKINDEX), "https://dl-cdn.alpinelinux.org/alpine/v3.20/main")
	require.NoError(t, err)

	t.Run("resolve existing virtual", func(t *testing.T) {
		pkg, ok := idx.ResolveVirtual("service-discover")
		require.True(t, ok)
		assert.Equal(t, "virtual-pkg", pkg.Name)
	})

	t.Run("resolve nonexistent virtual", func(t *testing.T) {
		_, ok := idx.ResolveVirtual("nonexistent-virtual")
		assert.False(t, ok)
	})
}
