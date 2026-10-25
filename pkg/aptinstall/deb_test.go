// SPDX-FileCopyrightText: 2021-2026 Gianluca Boiano
// SPDX-License-Identifier: GPL-3.0-only

package aptinstall_test

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/M0Rf30/yap/v2/pkg/aptinstall"
)

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
