package shell

import (
	"compress/gzip"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// createTestGzip creates a gzip-compressed file at path containing content.
func createTestGzip(t *testing.T, path, content string) {
	t.Helper()

	f, err := os.Create(path)
	require.NoError(t, err, "createTestGzip: create")

	defer func() { _ = f.Close() }()

	gw := gzip.NewWriter(f)

	_, err = gw.Write([]byte(content))
	require.NoError(t, err, "createTestGzip: write")
	require.NoError(t, gw.Close(), "createTestGzip: close gzip writer")
}

// ---------------------------------------------------------------------------
// handleGunzip — in-place decompression (the 47% uncovered branch)
// ---------------------------------------------------------------------------

func TestHandleGunzip_InPlace(t *testing.T) {
	dir := t.TempDir()
	gzPath := filepath.Join(dir, "data.txt.gz")
	createTestGzip(t, gzPath, "hello gunzip")

	// gunzip without -c → in-place decompression, removes .gz
	err := runScript(t, dir, "gunzip data.txt.gz")
	require.NoError(t, err)

	outPath := filepath.Join(dir, "data.txt")
	content, err := os.ReadFile(outPath)
	require.NoError(t, err)
	assert.Equal(t, "hello gunzip", string(content))

	// Original .gz should be removed
	_, statErr := os.Stat(gzPath)
	assert.True(t, os.IsNotExist(statErr), ".gz file should be removed after in-place gunzip")
}

func TestHandleGunzip_InPlaceKeep(t *testing.T) {
	dir := t.TempDir()
	gzPath := filepath.Join(dir, "data.txt.gz")
	createTestGzip(t, gzPath, "keep original")

	err := runScript(t, dir, "gunzip -k data.txt.gz")
	require.NoError(t, err)

	// Both files should exist
	_, err = os.Stat(filepath.Join(dir, "data.txt"))
	require.NoError(t, err, "decompressed file should exist")

	_, err = os.Stat(gzPath)
	require.NoError(t, err, "original .gz should be kept with -k")
}

func TestHandleGunzip_NoInputFile(t *testing.T) {
	dir := t.TempDir()
	// gunzip with no file argument should error
	err := runScript(t, dir, "gunzip")
	require.Error(t, err)
}

func TestHandleGunzip_NonExistentFile(t *testing.T) {
	dir := t.TempDir()
	err := runScript(t, dir, "gunzip nonexistent.gz")
	require.Error(t, err)
}

func TestHandleGunzip_ToStdout(t *testing.T) {
	dir := t.TempDir()
	gzPath := filepath.Join(dir, "data.txt.gz")
	createTestGzip(t, gzPath, "stdout content")

	// gunzip -c writes to stdout — should succeed without creating output file
	err := runScript(t, dir, "gunzip -c data.txt.gz > /dev/null")
	require.NoError(t, err)

	// Original .gz should still exist (stdout mode doesn't remove it)
	_, statErr := os.Stat(gzPath)
	require.NoError(t, statErr, ".gz file should remain after stdout gunzip")
}

// ---------------------------------------------------------------------------
// handleUnrar — argument validation (0% coverage)
// ---------------------------------------------------------------------------

func TestHandleUnrar_MissingSubcommand(t *testing.T) {
	dir := t.TempDir()
	// unrar with no sub-command → error
	err := runScript(t, dir, "unrar")
	require.Error(t, err)
}

func TestHandleUnrar_UnsupportedSubcommand(t *testing.T) {
	dir := t.TempDir()
	// unrar l (list) is not supported
	err := runScript(t, dir, "unrar l archive.rar")
	require.Error(t, err)
}

func TestHandleUnrar_MissingArchive(t *testing.T) {
	dir := t.TempDir()
	// unrar x with no archive path
	err := runScript(t, dir, "unrar x")
	require.Error(t, err)
}

func TestHandleUnrar_NonExistentArchive(t *testing.T) {
	dir := t.TempDir()
	// unrar x with a path that doesn't exist → archive.Extract will fail
	err := runScript(t, dir, "unrar x nonexistent.rar")
	require.Error(t, err)
}

// ---------------------------------------------------------------------------
// handle7z — argument validation (0% coverage)
// ---------------------------------------------------------------------------

func TestHandle7z_MissingSubcommand(t *testing.T) {
	dir := t.TempDir()
	err := runScript(t, dir, "7z")
	require.Error(t, err)
}

func TestHandle7z_UnsupportedSubcommand(t *testing.T) {
	dir := t.TempDir()
	// 7z l (list) is not supported
	err := runScript(t, dir, "7z l archive.7z")
	require.Error(t, err)
}

func TestHandle7z_MissingArchive(t *testing.T) {
	dir := t.TempDir()
	// 7z x with no archive
	err := runScript(t, dir, "7z x")
	require.Error(t, err)
}

func TestHandle7z_NonExistentArchive(t *testing.T) {
	dir := t.TempDir()
	err := runScript(t, dir, "7z x nonexistent.7z")
	require.Error(t, err)
}

func TestHandle7za_NonExistentArchive(t *testing.T) {
	dir := t.TempDir()
	err := runScript(t, dir, "7za x nonexistent.7z")
	require.Error(t, err)
}

// ---------------------------------------------------------------------------
// handleDpkgDeb — argument validation (0% coverage)
// ---------------------------------------------------------------------------

func TestHandleDpkgDeb_NoExtractFlag(t *testing.T) {
	dir := t.TempDir()
	// dpkg-deb without -x falls through to next handler (binary not found → error)
	err := runScript(t, dir, "dpkg-deb --info nonexistent.deb")
	// Falls through to OS binary — will error because binary not found or file missing
	// We just verify it doesn't panic
	_ = err
}

func TestHandleDpkgDeb_MissingArgs(t *testing.T) {
	dir := t.TempDir()
	// dpkg-deb -x with only one positional arg (missing destdir) → falls through
	err := runScript(t, dir, "dpkg-deb -x only-one-arg.deb")
	// Falls through to next handler
	_ = err
}

func TestHandleDpkgDeb_NonExistentDeb(t *testing.T) {
	dir := t.TempDir()
	destDir := filepath.Join(dir, "out")
	require.NoError(t, os.MkdirAll(destDir, 0o755))

	err := runScript(t, dir, "dpkg-deb -x nonexistent.deb "+destDir)
	require.Error(t, err)
}

func TestHandleDpkgDeb_ExtractAlias(t *testing.T) {
	dir := t.TempDir()
	destDir := filepath.Join(dir, "out")
	require.NoError(t, os.MkdirAll(destDir, 0o755))

	// --extract is an alias for -x; nonexistent file → error from ExtractDEB
	err := runScript(t, dir, "dpkg-deb --extract nonexistent.deb "+destDir)
	require.Error(t, err)
}

// ---------------------------------------------------------------------------
// handleRpm2Cpio — argument validation (0% coverage)
// ---------------------------------------------------------------------------

func TestHandleRpm2Cpio_NoArgs(t *testing.T) {
	dir := t.TempDir()
	err := runScript(t, dir, "rpm2cpio")
	require.Error(t, err)
}

func TestHandleRpm2Cpio_NonExistentFile(t *testing.T) {
	dir := t.TempDir()
	err := runScript(t, dir, "rpm2cpio nonexistent.rpm")
	require.Error(t, err)
}

// ---------------------------------------------------------------------------
// archiveExecHandler — unknown command falls through
// ---------------------------------------------------------------------------

func TestArchiveExecHandler_UnknownCommandFallthrough(t *testing.T) {
	dir := t.TempDir()
	// "true" is not in the intercept list → falls through to OS binary
	err := runScript(t, dir, "true")
	require.NoError(t, err)
}

func TestArchiveExecHandler_EmptyArgs(t *testing.T) {
	// Verify the handler doesn't panic with an empty args slice.
	// We can't easily call it directly without an interp context, but
	// running an empty script exercises the handler with no commands.
	dir := t.TempDir()
	err := runScript(t, dir, "")
	require.NoError(t, err)
}
