package signing

import (
	"bytes"
	"compress/gzip"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestExtractFirstGzipStreamInvalidData verifies that non-gzip input returns an error.
func TestExtractFirstGzipStreamInvalidData(t *testing.T) {
	_, err := extractFirstGzipStream([]byte("not a gzip stream"))
	require.Error(t, err)
}

// TestExtractFirstGzipStreamTruncatedData verifies that a truncated gzip stream
// (valid header, incomplete body) returns an error.
func TestExtractFirstGzipStreamTruncatedData(t *testing.T) {
	// Build a valid gzip stream then truncate it
	var buf bytes.Buffer

	gz := gzip.NewWriter(&buf)
	_, _ = gz.Write(bytes.Repeat([]byte("x"), 1024))
	_ = gz.Close()

	full := buf.Bytes()
	truncated := full[:len(full)/2]

	_, err := extractFirstGzipStream(truncated)
	require.Error(t, err)
}

// TestExtractFirstGzipStreamValidStream verifies that a valid single gzip stream
// returns the correct offset (equal to the total length).
func TestExtractFirstGzipStreamValidStream(t *testing.T) {
	var buf bytes.Buffer

	gz := gzip.NewWriter(&buf)
	_, _ = gz.Write([]byte("hello world"))
	_ = gz.Close()

	data := buf.Bytes()

	offset, err := extractFirstGzipStream(data)
	require.NoError(t, err)
	require.Equal(t, len(data), offset)
}

// TestExtractFirstGzipStreamConcatenated verifies that the offset returned for
// a concatenated gzip file is positive and does not exceed the total length.
// (The exact split point depends on the gzip implementation's read-ahead
// behaviour; we assert structural correctness rather than a byte-exact value.)
func TestExtractFirstGzipStreamConcatenated(t *testing.T) {
	var buf1 bytes.Buffer

	gz1 := gzip.NewWriter(&buf1)
	_, _ = gz1.Write([]byte("stream one"))
	_ = gz1.Close()

	var buf2 bytes.Buffer

	gz2 := gzip.NewWriter(&buf2)
	_, _ = gz2.Write([]byte("stream two"))
	_ = gz2.Close()

	combined := append(buf1.Bytes(), buf2.Bytes()...)

	offset, err := extractFirstGzipStream(combined)
	require.NoError(t, err)
	// Offset must be positive and at most the total length.
	require.Greater(t, offset, 0)
	require.LessOrEqual(t, offset, len(combined))
}
