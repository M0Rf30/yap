package aptinstall_test

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"os"
	"path/filepath"
	"testing"

	"github.com/m0rf30/ar"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/M0Rf30/yap/v2/pkg/aptinstall"
)

// createTestDEB creates a minimal but valid .deb file for testing.
// Returns the path to the created .deb file.
// Currently unused but kept for future integration tests.
// nolint:unused
func createTestDEB(t *testing.T, tmpDir, pkgName, version string) string {
	t.Helper()

	// Create control file content.
	controlContent := `Package: ` + pkgName + `
Version: ` + version + `
Architecture: amd64
Maintainer: Test <test@example.com>
Description: Test package
`

	// Create control.tar.gz.
	controlBuf := new(bytes.Buffer)
	controlGz := gzip.NewWriter(controlBuf)
	controlTar := tar.NewWriter(controlGz)

	// Write control file.
	controlHeader := &tar.Header{
		Name: "control",
		Mode: 0o644,
		Size: int64(len(controlContent)),
	}

	require.NoError(t, controlTar.WriteHeader(controlHeader))
	_, err := controlTar.Write([]byte(controlContent))
	require.NoError(t, err)

	require.NoError(t, controlTar.Close())
	require.NoError(t, controlGz.Close())

	// Create data.tar.gz (empty for now).
	dataBuf := new(bytes.Buffer)
	dataGz := gzip.NewWriter(dataBuf)
	dataTar := tar.NewWriter(dataGz)

	// Write a dummy file.
	fileContent := "test content"
	fileHeader := &tar.Header{
		Name: "usr/bin/test",
		Mode: 0o755,
		Size: int64(len(fileContent)),
	}

	require.NoError(t, dataTar.WriteHeader(fileHeader))
	_, err = dataTar.Write([]byte(fileContent))
	require.NoError(t, err)

	require.NoError(t, dataTar.Close())
	require.NoError(t, dataGz.Close())

	// Create AR archive.
	debPath := filepath.Join(tmpDir, pkgName+"_"+version+"_amd64.deb")
	debFile, err := os.Create(debPath)
	require.NoError(t, err)

	defer func() { _ = debFile.Close() }()

	arWriter := ar.NewWriter(debFile)

	// Write debian-binary.
	debianBinary := "2.0\n"
	debianHeader := ar.Header{
		Name: "debian-binary",
		Size: int64(len(debianBinary)),
		Mode: 0o644,
	}

	require.NoError(t, arWriter.WriteHeader(&debianHeader))
	_, err = arWriter.Write([]byte(debianBinary))
	require.NoError(t, err)

	// Write control.tar.gz.
	controlArHeader := ar.Header{
		Name: "control.tar.gz",
		Size: int64(controlBuf.Len()),
		Mode: 0o644,
	}

	require.NoError(t, arWriter.WriteHeader(&controlArHeader))
	_, err = arWriter.Write(controlBuf.Bytes())
	require.NoError(t, err)

	// Write data.tar.gz.
	dataArHeader := ar.Header{
		Name: "data.tar.gz",
		Size: int64(dataBuf.Len()),
		Mode: 0o644,
	}

	require.NoError(t, arWriter.WriteHeader(&dataArHeader))
	_, err = arWriter.Write(dataBuf.Bytes())
	require.NoError(t, err)

	return debPath
}

// TestParseControl tests the parseControl function.
func TestParseControl(t *testing.T) {
	control := `Package: gcc
Version: 12.3.0-1
Architecture: amd64
Maintainer: Debian GCC Maintainers <debian-gcc@lists.debian.org>
Description: GNU C compiler
 This is the GNU C compiler, a fairly portable optimizing compiler for C.
`

	fields := aptinstall.ParseControlForTesting(control)

	assert.Equal(t, "gcc", fields["Package"])
	assert.Equal(t, "12.3.0-1", fields["Version"])
	assert.Equal(t, "amd64", fields["Architecture"])
	assert.Contains(t, fields["Description"], "GNU C compiler")
}

// TestParseDataTar tests the parseDataTar function.
func TestParseDataTar(t *testing.T) {
	// Create a simple tar.gz with a file.
	dataBuf := new(bytes.Buffer)
	dataGz := gzip.NewWriter(dataBuf)
	dataTar := tar.NewWriter(dataGz)

	fileContent := "test content"
	fileHeader := &tar.Header{
		Name: "usr/bin/test",
		Mode: 0o755,
		Size: int64(len(fileContent)),
	}

	require.NoError(t, dataTar.WriteHeader(fileHeader))
	_, err := dataTar.Write([]byte(fileContent))
	require.NoError(t, err)

	require.NoError(t, dataTar.Close())
	require.NoError(t, dataGz.Close())

	// Write to a temp file and parse it.
	tmpFile, err := os.CreateTemp("", "data.tar.gz")
	require.NoError(t, err)

	defer func() { _ = os.Remove(tmpFile.Name()) }()

	_, err = tmpFile.Write(dataBuf.Bytes())
	require.NoError(t, err)
	require.NoError(t, tmpFile.Close())

	// Parse using the decompressStream and tar reader logic.
	f, err := os.Open(tmpFile.Name())
	require.NoError(t, err)

	defer func() { _ = f.Close() }()

	decompressed, err := aptinstall.DecompressStreamForTesting(f, "data.tar.gz")
	require.NoError(t, err)

	defer func() { _ = decompressed.Close() }()

	tr := tar.NewReader(decompressed)

	hdr, err := tr.Next()
	require.NoError(t, err)
	assert.Equal(t, "usr/bin/test", hdr.Name)
}
