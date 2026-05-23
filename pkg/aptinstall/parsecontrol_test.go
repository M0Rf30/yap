package aptinstall_test

import (
	"bytes"
	"compress/gzip"
	"io"
	"testing"

	"github.com/klauspost/compress/zstd"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/ulikunitz/xz"

	"github.com/M0Rf30/yap/v2/pkg/aptinstall"
)

// TestParseControlSimpleSingleLineFields tests parsing of simple single-line fields.
func TestParseControlSimpleSingleLineFields(t *testing.T) {
	t.Parallel()

	control := `Package: gcc
Version: 12.3.0-1
Architecture: amd64
`

	fields := aptinstall.ParseControlForTesting(control)

	assert.Equal(t, "gcc", fields["Package"])
	assert.Equal(t, "12.3.0-1", fields["Version"])
	assert.Equal(t, "amd64", fields["Architecture"])
	assert.Len(t, fields, 3)
}

// TestParseControlEmptyControl tests parsing an empty control string.
func TestParseControlEmptyControl(t *testing.T) {
	t.Parallel()

	control := ""
	fields := aptinstall.ParseControlForTesting(control)

	assert.Empty(t, fields)
}

// TestParseControlFieldWithEmptyValue tests parsing a field with an empty value.
func TestParseControlFieldWithEmptyValue(t *testing.T) {
	t.Parallel()

	control := `Package: test
Conffiles:
Version: 1.0
`

	fields := aptinstall.ParseControlForTesting(control)

	assert.Equal(t, "test", fields["Package"])
	assert.Equal(t, "", fields["Conffiles"])
	assert.Equal(t, "1.0", fields["Version"])
}

// TestParseControlMultilineWithSpaceContinuation tests multi-line fields
// with space-prefixed continuation lines.
func TestParseControlMultilineWithSpaceContinuation(t *testing.T) {
	t.Parallel()

	control := `Package: gcc
Description: GNU C compiler
 This is the GNU C compiler, a fairly portable optimizing compiler for C.
 It can compile C as well as C++.
Version: 12.3.0-1
`

	fields := aptinstall.ParseControlForTesting(control)

	assert.Equal(t, "gcc", fields["Package"])
	assert.Equal(t, "12.3.0-1", fields["Version"])
	// Multi-line value should have newlines preserved (leading space stripped).
	assert.Contains(t, fields["Description"], "GNU C compiler")
	assert.Contains(t, fields["Description"], "This is the GNU C compiler")
	assert.Contains(t, fields["Description"], "It can compile C as well as C++")
}

// TestParseControlMultilineWithTabContinuation tests multi-line fields
// with tab-prefixed continuation lines.
func TestParseControlMultilineWithTabContinuation(t *testing.T) {
	t.Parallel()

	control := `Package: test
Depends: libfoo,
	libbar,
	libbaz
Version: 1.0
`

	fields := aptinstall.ParseControlForTesting(control)

	assert.Equal(t, "test", fields["Package"])
	assert.Equal(t, "1.0", fields["Version"])
	// Tab continuation should be treated like space continuation.
	assert.Contains(t, fields["Depends"], "libfoo")
	assert.Contains(t, fields["Depends"], "libbar")
	assert.Contains(t, fields["Depends"], "libbaz")
}

// TestParseControlLastFieldFlushed tests that the last field in the control
// string is properly flushed (no trailing newline required).
func TestParseControlLastFieldFlushed(t *testing.T) {
	t.Parallel()

	control := `Package: test
Version: 1.0
Description: Last field without trailing newline`

	fields := aptinstall.ParseControlForTesting(control)

	assert.Equal(t, "test", fields["Package"])
	assert.Equal(t, "1.0", fields["Version"])
	assert.Equal(t, "Last field without trailing newline", fields["Description"])
}

// TestParseControlMultipleFields tests parsing multiple fields in sequence.
func TestParseControlMultipleFields(t *testing.T) {
	t.Parallel()

	control := `Package: curl
Version: 7.85.0-1
Architecture: amd64
Maintainer: Debian Curl Maintainers <pkg-curl-maintainers@lists.alioth.debian.org>
Installed-Size: 1234
Depends: libc6 (>= 2.34), libcurl4 (= 7.85.0-1)
Conflicts: curl-ssl
Provides: curl-replacement
Description: command line tool for transferring data with URLs
 curl is a tool to transfer data from or to a server, using URLs as
 the locator. It supports a range of protocols.
`

	fields := aptinstall.ParseControlForTesting(control)

	assert.Equal(t, "curl", fields["Package"])
	assert.Equal(t, "7.85.0-1", fields["Version"])
	assert.Equal(t, "amd64", fields["Architecture"])
	assert.Equal(t, "Debian Curl Maintainers <pkg-curl-maintainers@lists.alioth.debian.org>", fields["Maintainer"])
	assert.Equal(t, "1234", fields["Installed-Size"])
	assert.Contains(t, fields["Depends"], "libc6")
	assert.Equal(t, "curl-ssl", fields["Conflicts"])
	assert.Equal(t, "curl-replacement", fields["Provides"])
	assert.Contains(t, fields["Description"], "command line tool")
	assert.Len(t, fields, 9)
}

// TestParseControlWhitespaceHandling tests that leading/trailing whitespace
// in field values is properly trimmed.
func TestParseControlWhitespaceHandling(t *testing.T) {
	t.Parallel()

	control := `Package:   test   
Version:  1.0  
Architecture:amd64
`

	fields := aptinstall.ParseControlForTesting(control)

	// Leading/trailing spaces in values should be trimmed.
	assert.Equal(t, "test", fields["Package"])
	assert.Equal(t, "1.0", fields["Version"])
	assert.Equal(t, "amd64", fields["Architecture"])
}

// TestParseControlComplexMultilineDescription tests a complex multi-line
// Description field with dots and special formatting (as seen in real dpkg status).
func TestParseControlComplexMultilineDescription(t *testing.T) {
	t.Parallel()

	control := `Package: python3-minimal
Version: 3.11.2-1+0~20230227.5+debian~11.1+1
Description: minimal Python 3 runtime
 This package contains the minimal set of modules needed to run Python 3.
 .
 These include:
  * apt-get for retrieval of packages
  * apt-cache for searching packages
`

	fields := aptinstall.ParseControlForTesting(control)

	assert.Equal(t, "python3-minimal", fields["Package"])
	desc := fields["Description"]
	assert.Contains(t, desc, "minimal Python 3 runtime")
	assert.Contains(t, desc, "This package contains the minimal set")
	assert.Contains(t, desc, "These include:")
	assert.Contains(t, desc, "apt-get for retrieval")
}

// TestParseControlFieldNameWithHyphens tests field names containing hyphens.
func TestParseControlFieldNameWithHyphens(t *testing.T) {
	t.Parallel()

	control := `Package: test
Installed-Size: 1024
Pre-Depends: libc6
Build-Depends: gcc, make
`

	fields := aptinstall.ParseControlForTesting(control)

	assert.Equal(t, "test", fields["Package"])
	assert.Equal(t, "1024", fields["Installed-Size"])
	assert.Equal(t, "libc6", fields["Pre-Depends"])
	assert.Equal(t, "gcc, make", fields["Build-Depends"])
}

// TestParseControlNoColonLine tests that lines without colons are skipped
// (not treated as field headers).
func TestParseControlNoColonLine(t *testing.T) {
	t.Parallel()

	control := `Package: test
This line has no colon
Version: 1.0
`

	fields := aptinstall.ParseControlForTesting(control)

	// The line without a colon should be ignored.
	assert.Equal(t, "test", fields["Package"])
	assert.Equal(t, "1.0", fields["Version"])
	assert.Len(t, fields, 2)
}

// TestParseControlConsecutiveNewlines tests handling of consecutive newlines.
func TestParseControlConsecutiveNewlines(t *testing.T) {
	t.Parallel()

	control := `Package: test

Version: 1.0

Architecture: amd64
`

	fields := aptinstall.ParseControlForTesting(control)

	assert.Equal(t, "test", fields["Package"])
	assert.Equal(t, "1.0", fields["Version"])
	assert.Equal(t, "amd64", fields["Architecture"])
}

// TestDecompressStreamGzip tests decompression of gzip-compressed data.
func TestDecompressStreamGzip(t *testing.T) {
	t.Parallel()

	// Create gzip-compressed data.
	originalData := []byte("Hello, World!")

	var buf bytes.Buffer

	gz := gzip.NewWriter(&buf)
	_, err := gz.Write(originalData)
	require.NoError(t, err)
	require.NoError(t, gz.Close())

	// Decompress using decompressStream.
	reader, err := aptinstall.DecompressStreamForTesting(bytes.NewReader(buf.Bytes()), "test.gz")
	require.NoError(t, err)

	defer func() { _ = reader.Close() }()

	// Verify decompressed content.
	decompressed, err := io.ReadAll(reader)
	require.NoError(t, err)
	assert.Equal(t, originalData, decompressed)
}

// TestDecompressStreamXz tests decompression of xz-compressed data.
func TestDecompressStreamXz(t *testing.T) {
	t.Parallel()

	// Create xz-compressed data.
	originalData := []byte("Hello, World!")

	var buf bytes.Buffer

	xzw, err := xz.NewWriter(&buf)
	require.NoError(t, err)
	_, err = xzw.Write(originalData)
	require.NoError(t, err)
	require.NoError(t, xzw.Close())

	// Decompress using decompressStream.
	reader, err := aptinstall.DecompressStreamForTesting(bytes.NewReader(buf.Bytes()), "test.xz")
	require.NoError(t, err)

	defer func() { _ = reader.Close() }()

	// Verify decompressed content.
	decompressed, err := io.ReadAll(reader)
	require.NoError(t, err)
	assert.Equal(t, originalData, decompressed)
}

// TestDecompressStreamZstd tests decompression of zstd-compressed data.
func TestDecompressStreamZstd(t *testing.T) {
	t.Parallel()

	// Create zstd-compressed data.
	originalData := []byte("Hello, World!")

	var buf bytes.Buffer

	zw, err := zstd.NewWriter(&buf)
	require.NoError(t, err)
	_, err = zw.Write(originalData)
	require.NoError(t, err)
	require.NoError(t, zw.Close())

	// Decompress using decompressStream.
	reader, err := aptinstall.DecompressStreamForTesting(bytes.NewReader(buf.Bytes()), "test.zst")
	require.NoError(t, err)

	defer func() { _ = reader.Close() }()

	// Verify decompressed content.
	decompressed, err := io.ReadAll(reader)
	require.NoError(t, err)
	assert.Equal(t, originalData, decompressed)
}

// TestDecompressStreamUncompressed tests handling of uncompressed data.
func TestDecompressStreamUncompressed(t *testing.T) {
	t.Parallel()

	// Plain uncompressed data.
	originalData := []byte("Hello, World!")

	// Decompress using decompressStream (should return NopCloser).
	reader, err := aptinstall.DecompressStreamForTesting(bytes.NewReader(originalData), "test.tar")
	require.NoError(t, err)

	defer func() { _ = reader.Close() }()

	// Verify content is unchanged.
	decompressed, err := io.ReadAll(reader)
	require.NoError(t, err)
	assert.Equal(t, originalData, decompressed)
}

// TestDecompressStreamEmptyData tests decompression of empty data.
func TestDecompressStreamEmptyData(t *testing.T) {
	t.Parallel()

	// Empty uncompressed data.
	originalData := []byte("")

	// Decompress using decompressStream.
	reader, err := aptinstall.DecompressStreamForTesting(bytes.NewReader(originalData), "test.tar")
	require.NoError(t, err)

	defer func() { _ = reader.Close() }()

	// Verify content is empty.
	decompressed, err := io.ReadAll(reader)
	require.NoError(t, err)
	assert.Empty(t, decompressed)
}

// TestDecompressStreamGzipMagicBytes tests that gzip magic bytes are correctly identified.
func TestDecompressStreamGzipMagicBytes(t *testing.T) {
	t.Parallel()

	// Create gzip-compressed data.
	originalData := []byte("test data")

	var buf bytes.Buffer

	gz := gzip.NewWriter(&buf)
	_, err := gz.Write(originalData)
	require.NoError(t, err)
	require.NoError(t, gz.Close())

	compressed := buf.Bytes()

	// Verify gzip magic bytes are present.
	assert.GreaterOrEqual(t, len(compressed), 2)
	assert.Equal(t, byte(0x1F), compressed[0])
	assert.Equal(t, byte(0x8B), compressed[1])

	// Decompress and verify.
	reader, err := aptinstall.DecompressStreamForTesting(bytes.NewReader(compressed), "misleading.tar")
	require.NoError(t, err)

	defer func() { _ = reader.Close() }()

	decompressed, err := io.ReadAll(reader)
	require.NoError(t, err)
	assert.Equal(t, originalData, decompressed)
}

// TestDecompressStreamXzMagicBytes tests that xz magic bytes are correctly identified.
func TestDecompressStreamXzMagicBytes(t *testing.T) {
	t.Parallel()

	// Create xz-compressed data.
	originalData := []byte("test data")

	var buf bytes.Buffer

	xzw, err := xz.NewWriter(&buf)
	require.NoError(t, err)
	_, err = xzw.Write(originalData)
	require.NoError(t, err)
	require.NoError(t, xzw.Close())

	compressed := buf.Bytes()

	// Verify xz magic bytes are present.
	assert.GreaterOrEqual(t, len(compressed), 6)
	assert.Equal(t, byte(0xFD), compressed[0])
	assert.Equal(t, byte('7'), compressed[1])
	assert.Equal(t, byte('z'), compressed[2])
	assert.Equal(t, byte('X'), compressed[3])
	assert.Equal(t, byte('Z'), compressed[4])
	assert.Equal(t, byte(0x00), compressed[5])

	// Decompress and verify.
	reader, err := aptinstall.DecompressStreamForTesting(bytes.NewReader(compressed), "misleading.tar")
	require.NoError(t, err)

	defer func() { _ = reader.Close() }()

	decompressed, err := io.ReadAll(reader)
	require.NoError(t, err)
	assert.Equal(t, originalData, decompressed)
}

// TestDecompressStreamZstdMagicBytes tests that zstd magic bytes are correctly identified.
func TestDecompressStreamZstdMagicBytes(t *testing.T) {
	t.Parallel()

	// Create zstd-compressed data.
	originalData := []byte("test data")

	var buf bytes.Buffer

	zw, err := zstd.NewWriter(&buf)
	require.NoError(t, err)
	_, err = zw.Write(originalData)
	require.NoError(t, err)
	require.NoError(t, zw.Close())

	compressed := buf.Bytes()

	// Verify zstd magic bytes are present.
	assert.GreaterOrEqual(t, len(compressed), 4)
	assert.Equal(t, byte(0x28), compressed[0])
	assert.Equal(t, byte(0xB5), compressed[1])
	assert.Equal(t, byte(0x2F), compressed[2])
	assert.Equal(t, byte(0xFD), compressed[3])

	// Decompress and verify.
	reader, err := aptinstall.DecompressStreamForTesting(bytes.NewReader(compressed), "misleading.tar")
	require.NoError(t, err)

	defer func() { _ = reader.Close() }()

	decompressed, err := io.ReadAll(reader)
	require.NoError(t, err)
	assert.Equal(t, originalData, decompressed)
}

// TestDecompressStreamLargeData tests decompression of larger data payloads.
func TestDecompressStreamLargeData(t *testing.T) {
	t.Parallel()

	// Create a large data payload.
	originalData := make([]byte, 1024*1024) // 1 MB
	for i := range originalData {
		originalData[i] = byte(i % 256)
	}

	// Compress with gzip.
	var buf bytes.Buffer

	gz := gzip.NewWriter(&buf)
	_, err := gz.Write(originalData)
	require.NoError(t, err)
	require.NoError(t, gz.Close())

	// Decompress using decompressStream.
	reader, err := aptinstall.DecompressStreamForTesting(bytes.NewReader(buf.Bytes()), "test.gz")
	require.NoError(t, err)

	defer func() { _ = reader.Close() }()

	// Verify decompressed content.
	decompressed, err := io.ReadAll(reader)
	require.NoError(t, err)
	assert.Equal(t, originalData, decompressed)
}

// TestDecompressStreamPartialRead tests that decompressStream can be read
// in chunks (not all at once).
func TestDecompressStreamPartialRead(t *testing.T) {
	t.Parallel()

	// Create gzip-compressed data.
	originalData := []byte("Hello, World! This is a test.")

	var buf bytes.Buffer

	gz := gzip.NewWriter(&buf)
	_, err := gz.Write(originalData)
	require.NoError(t, err)
	require.NoError(t, gz.Close())

	// Decompress using decompressStream.
	reader, err := aptinstall.DecompressStreamForTesting(bytes.NewReader(buf.Bytes()), "test.gz")
	require.NoError(t, err)

	defer func() { _ = reader.Close() }()

	// Read in small chunks.
	var result []byte

	chunk := make([]byte, 5)
	for {
		n, err := reader.Read(chunk)
		if err != nil && err != io.EOF {
			require.NoError(t, err)
		}

		if n == 0 {
			break
		}

		result = append(result, chunk[:n]...)
	}

	assert.Equal(t, originalData, result)
}
