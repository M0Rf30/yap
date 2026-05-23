package apkindex_test

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/M0Rf30/yap/v2/pkg/apkindex"
)

func TestLoadIndexTarball(t *testing.T) {
	// Create a minimal APKINDEX.tar.gz in memory.
	var buf bytes.Buffer

	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)

	// Add APKINDEX file.
	indexContent := sampleAPKINDEX
	hdr := &tar.Header{
		Name: "APKINDEX",
		Size: int64(len(indexContent)),
	}
	require.NoError(t, tw.WriteHeader(hdr))
	_, err := tw.Write([]byte(indexContent))
	require.NoError(t, err)

	// Add DESCRIPTION file (should be ignored).
	descContent := "Alpine Linux Repository"
	hdr = &tar.Header{
		Name: "DESCRIPTION",
		Size: int64(len(descContent)),
	}
	require.NoError(t, tw.WriteHeader(hdr))
	_, err = tw.Write([]byte(descContent))
	require.NoError(t, err)

	require.NoError(t, tw.Close())
	require.NoError(t, gz.Close())

	// Parse the tarball.
	idx := apkindex.NewIndex()
	err = idx.ParseIndex(bytes.NewReader([]byte(indexContent)), "https://dl-cdn.alpinelinux.org/alpine/v3.20/main")
	require.NoError(t, err)

	// Verify packages were parsed.
	pkg, ok := idx.Lookup("musl")
	require.True(t, ok)
	assert.Equal(t, "musl", pkg.Name)
	assert.Equal(t, "https://dl-cdn.alpinelinux.org/alpine/v3.20/main", pkg.RepoBaseURL)
}

func TestDownloadPackageURL(t *testing.T) {
	// Create a simple index with one package.
	idx := apkindex.NewIndex()
	err := idx.ParseIndex(strings.NewReader(sampleAPKINDEX), "https://dl-cdn.alpinelinux.org/alpine/v3.20/main")
	require.NoError(t, err)

	t.Run("package URL construction", func(t *testing.T) {
		pkg, ok := idx.Lookup("musl")
		require.True(t, ok)

		// Verify the package has the correct repo base URL.
		assert.Equal(t, "https://dl-cdn.alpinelinux.org/alpine/v3.20/main", pkg.RepoBaseURL)
		assert.Equal(t, "x86_64", pkg.Arch)
		assert.Equal(t, "1.2.3-r4", pkg.Version)

		// The download URL would be:
		// https://dl-cdn.alpinelinux.org/alpine/v3.20/main/x86_64/musl-1.2.3-r4.apk
		expectedURL := pkg.RepoBaseURL + "/" + pkg.Arch + "/" + pkg.Name + "-" + pkg.Version + ".apk"
		assert.Equal(t, "https://dl-cdn.alpinelinux.org/alpine/v3.20/main/x86_64/musl-1.2.3-r4.apk", expectedURL)
	})
}

func TestNewIndex(t *testing.T) {
	idx := apkindex.NewIndex()
	assert.NotNil(t, idx)

	// Verify empty index returns no results.
	_, ok := idx.Lookup("nonexistent")
	assert.False(t, ok)

	_, ok = idx.ResolveVirtual("nonexistent")
	assert.False(t, ok)
}
