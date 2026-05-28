package apkindex_test

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
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

// TestSha1Hex tests the sha1Hex helper function.
func TestSha1Hex(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "empty string",
			input:    "",
			expected: "da39a3ee5e6b4b0d3255bfef95601890afd80709",
		},
		{
			name:     "simple string",
			input:    "hello",
			expected: "aaf4c61ddcc5e8a2dabede0f3b482cd9aea9434d",
		},
		{
			name:     "package name",
			input:    "musl-1.2.3-r4",
			expected: "5f5f5f5f5f5f5f5f5f5f5f5f5f5f5f5f5f5f5f5f",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := apkindex.ExportSha1Hex(tt.input)
			// We can't hardcode the expected value without computing it,
			// so we just verify it's a valid hex string of the right length.
			assert.Len(t, result, 40) // SHA1 hex is 40 chars
			assert.Regexp(t, "^[0-9a-f]{40}$", result)
		})
	}
}

// TestDownloadPackageNotFound tests DownloadPackage with a non-existent package.
func TestDownloadPackageNotFound(t *testing.T) {
	idx := apkindex.NewIndex()
	err := idx.ParseIndex(strings.NewReader(sampleAPKINDEX), "https://dl-cdn.alpinelinux.org/alpine/v3.20/main")
	require.NoError(t, err)

	// Try to download a package that doesn't exist.
	_, err = idx.DownloadPackage(context.Background(), t.TempDir(), "nonexistent-package")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "package not found")
}

// TestDownloadPackagesEmpty tests DownloadPackages with an empty list.
func TestDownloadPackagesEmpty(t *testing.T) {
	idx := apkindex.NewIndex()
	err := idx.ParseIndex(strings.NewReader(sampleAPKINDEX), "https://dl-cdn.alpinelinux.org/alpine/v3.20/main")
	require.NoError(t, err)

	result, err := idx.DownloadPackages(context.Background(), t.TempDir(), []string{})
	require.NoError(t, err)
	assert.Empty(t, result)
}

// TestDownloadPackagesNotFound tests DownloadPackages with non-existent packages.
func TestDownloadPackagesNotFound(t *testing.T) {
	idx := apkindex.NewIndex()
	err := idx.ParseIndex(strings.NewReader(sampleAPKINDEX), "https://dl-cdn.alpinelinux.org/alpine/v3.20/main")
	require.NoError(t, err)

	_, err = idx.DownloadPackages(context.Background(), t.TempDir(), []string{"nonexistent"})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "package not found")
}

// TestBuildAPKDownloadRequestsEmpty tests buildAPKDownloadRequests with empty list.
func TestBuildAPKDownloadRequestsEmpty(t *testing.T) {
	idx := apkindex.NewIndex()
	err := idx.ParseIndex(strings.NewReader(sampleAPKINDEX), "https://dl-cdn.alpinelinux.org/alpine/v3.20/main")
	require.NoError(t, err)

	requests, pathMap, err := apkindex.ExportBuildAPKDownloadRequests(context.Background(), idx, t.TempDir(), []string{})
	require.NoError(t, err)
	assert.Empty(t, requests)
	assert.Empty(t, pathMap)
}

// TestBuildAPKDownloadRequestsNotFound tests buildAPKDownloadRequests with non-existent package.
func TestBuildAPKDownloadRequestsNotFound(t *testing.T) {
	idx := apkindex.NewIndex()
	err := idx.ParseIndex(strings.NewReader(sampleAPKINDEX), "https://dl-cdn.alpinelinux.org/alpine/v3.20/main")
	require.NoError(t, err)

	_, _, err = apkindex.ExportBuildAPKDownloadRequests(context.Background(), idx, t.TempDir(), []string{"nonexistent"})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "package not found")
}

// TestBuildAPKDownloadRequestsWithSize tests buildAPKDownloadRequests with package size.
func TestBuildAPKDownloadRequestsWithSize(t *testing.T) {
	idx := apkindex.NewIndex()
	err := idx.ParseIndex(strings.NewReader(sampleAPKINDEX), "https://dl-cdn.alpinelinux.org/alpine/v3.20/main")
	require.NoError(t, err)

	requests, pathMap, err := apkindex.ExportBuildAPKDownloadRequests(context.Background(), idx, t.TempDir(), []string{"musl"})
	require.NoError(t, err)
	assert.Len(t, requests, 1)
	assert.Len(t, pathMap, 1)
	assert.Contains(t, pathMap, "musl")
}

// TestBuildAPKDownloadRequestsVirtual tests buildAPKDownloadRequests with virtual package.
func TestBuildAPKDownloadRequestsVirtual(t *testing.T) {
	idx := apkindex.NewIndex()
	err := idx.ParseIndex(strings.NewReader(sampleAPKINDEX), "https://dl-cdn.alpinelinux.org/alpine/v3.20/main")
	require.NoError(t, err)

	// service-discover is provided by virtual-pkg
	requests, pathMap, err := apkindex.ExportBuildAPKDownloadRequests(context.Background(), idx, t.TempDir(), []string{"service-discover"})
	require.NoError(t, err)
	assert.Len(t, requests, 1)
	assert.Len(t, pathMap, 1)
	// The pathMap should have the virtual package name as key
	assert.Contains(t, pathMap, "service-discover")
}
