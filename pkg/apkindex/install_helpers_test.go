package apkindex //nolint:testpackage

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestReadInstalledDBEmpty tests readInstalledDB with a non-existent file.
func TestReadInstalledDBEmpty(t *testing.T) {
	result := ExportReadInstalledDB()
	assert.NotNil(t, result)
}

// TestReadInstalledStanzasEmpty tests readInstalledStanzas with no file.
func TestReadInstalledStanzasEmpty(t *testing.T) {
	result := ExportReadInstalledStanzas()
	assert.NotNil(t, result)
	assert.Empty(t, result)
}

// TestWriteInstalledStanzas tests writing stanzas to a temp file.
func TestWriteInstalledStanzas(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "installed")

	stanzas := map[string]string{
		"musl": "P:musl\nV:1.2.3-r4\nA:x86_64\nI:234567\n",
		"gcc":  "P:gcc\nV:12.2.1-r1\nA:x86_64\nI:456789\n",
	}

	err := ExportWriteInstalledStanzas(dbPath, stanzas)
	require.NoError(t, err)

	assert.FileExists(t, dbPath)

	content, err := os.ReadFile(dbPath)
	require.NoError(t, err)

	contentStr := string(content)
	assert.Contains(t, contentStr, "P:musl")
	assert.Contains(t, contentStr, "P:gcc")
}

// TestWriteInstalledStanzasOrdering tests that stanzas are written in sorted order.
func TestWriteInstalledStanzasOrdering(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "installed")

	stanzas := map[string]string{
		"zulu":  "P:zulu\nV:1.0\n",
		"alpha": "P:alpha\nV:1.0\n",
		"beta":  "P:beta\nV:1.0\n",
	}

	err := ExportWriteInstalledStanzas(dbPath, stanzas)
	require.NoError(t, err)

	content, err := os.ReadFile(dbPath)
	require.NoError(t, err)

	contentStr := string(content)
	alphaIdx := strings.Index(contentStr, "P:alpha")
	betaIdx := strings.Index(contentStr, "P:beta")
	zuluIdx := strings.Index(contentStr, "P:zulu")

	assert.True(t, alphaIdx < betaIdx && betaIdx < zuluIdx, "stanzas not in sorted order")
}

// TestRegisterInstalledBasic tests registering a package in the installed DB.
func TestRegisterInstalledBasic(t *testing.T) {
	tmpDir := t.TempDir()

	pkg := &Package{
		Name:     "test-pkg",
		Version:  "1.0-r0",
		Arch:     "x86_64",
		InstSize: 1024,
	}

	err := ExportRegisterInstalled(tmpDir, pkg, "")
	require.NoError(t, err)

	dbPath := filepath.Join(tmpDir, "lib", "apk", "db", "installed")
	assert.FileExists(t, dbPath)

	content, err := os.ReadFile(dbPath)
	require.NoError(t, err)
	assert.Contains(t, string(content), "P:test-pkg")
}

// TestRegisterInstalledWithoutPkgInfo tests registering without .PKGINFO.
func TestRegisterInstalledWithoutPkgInfo(t *testing.T) {
	tmpDir := t.TempDir()

	pkg := &Package{
		Name:     "test-pkg",
		Version:  "1.0-r0",
		Arch:     "x86_64",
		InstSize: 2048,
	}

	err := ExportRegisterInstalled(tmpDir, pkg, "")
	require.NoError(t, err)

	dbPath := filepath.Join(tmpDir, "lib", "apk", "db", "installed")
	assert.FileExists(t, dbPath)

	content, err := os.ReadFile(dbPath)
	require.NoError(t, err)
	assert.Contains(t, string(content), "P:test-pkg")
	assert.Contains(t, string(content), "V:1.0-r0")
}

// TestRegisterInstalledUpdate tests updating an existing package.
func TestRegisterInstalledUpdate(t *testing.T) {
	tmpDir := t.TempDir()

	pkg1 := &Package{
		Name:     "test-pkg",
		Version:  "1.0-r0",
		Arch:     "x86_64",
		InstSize: 1024,
	}
	err := ExportRegisterInstalled(tmpDir, pkg1, "")
	require.NoError(t, err)

	pkg2 := &Package{
		Name:     "test-pkg",
		Version:  "2.0-r0",
		Arch:     "x86_64",
		InstSize: 2048,
	}
	err = ExportRegisterInstalled(tmpDir, pkg2, "")
	require.NoError(t, err)

	dbPath := filepath.Join(tmpDir, "lib", "apk", "db", "installed")
	content, err := os.ReadFile(dbPath)
	require.NoError(t, err)

	contentStr := string(content)
	count := strings.Count(contentStr, "P:test-pkg")
	assert.Equal(t, 1, count, "should have exactly one entry for test-pkg")

	assert.Contains(t, contentStr, "V:2.0-r0")
}
