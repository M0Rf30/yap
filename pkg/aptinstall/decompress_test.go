package aptinstall_test

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"io"
	"testing"

	"github.com/klauspost/compress/zstd"
	"github.com/stretchr/testify/require"
	"github.com/ulikunitz/xz"

	"github.com/M0Rf30/yap/v2/pkg/aptinstall"
)

// makeTarball produces a minimal valid tar archive containing one file at
// "usr/bin/marker". Used as the inner payload for every compression test.
func makeTarball(t *testing.T) []byte {
	t.Helper()

	var buf bytes.Buffer

	tw := tar.NewWriter(&buf)

	body := []byte("hello\n")

	require.NoError(t, tw.WriteHeader(&tar.Header{
		Name:     "usr/bin/marker",
		Mode:     0o755,
		Size:     int64(len(body)),
		Typeflag: tar.TypeReg,
	}))
	_, err := tw.Write(body)
	require.NoError(t, err)
	require.NoError(t, tw.Close())

	return buf.Bytes()
}

func gzipCompress(t *testing.T, raw []byte) []byte {
	t.Helper()

	var buf bytes.Buffer

	w := gzip.NewWriter(&buf)
	_, err := w.Write(raw)
	require.NoError(t, err)
	require.NoError(t, w.Close())

	return buf.Bytes()
}

func xzCompress(t *testing.T, raw []byte) []byte {
	t.Helper()

	var buf bytes.Buffer

	w, err := xz.NewWriter(&buf)
	require.NoError(t, err)
	_, err = w.Write(raw)
	require.NoError(t, err)
	require.NoError(t, w.Close())

	return buf.Bytes()
}

func zstdCompress(t *testing.T, raw []byte) []byte {
	t.Helper()

	var buf bytes.Buffer

	w, err := zstd.NewWriter(&buf)
	require.NoError(t, err)
	_, err = w.Write(raw)
	require.NoError(t, err)
	require.NoError(t, w.Close())

	return buf.Bytes()
}

// TestDecompressStreamSniffsMagic is the regression test for the bug
// where extractDataTar passes the os.CreateTemp("data.tar.*") name into
// decompressStream — that name has the random pattern substituted (e.g.
// "data.tar.123456789"), so any suffix-based dispatch falls through to
// the "uncompressed" default and feeds raw zstd/xz/gzip bytes into
// archive/tar. The fix is to sniff the magic bytes; this test asserts
// that sniffing wins even when the caller hands in a name that lies.
//
// Concretely: perl-modules-5.38_*.deb on Ubuntu Noble ships
// data.tar.zst. Before the fix, every install attempt died on
// "archive/tar: invalid tar header" before the user could even see one
// .deb extract successfully.
func TestDecompressStreamSniffsMagic(t *testing.T) {
	t.Parallel()

	raw := makeTarball(t)

	tests := []struct {
		name       string
		compressed []byte
	}{
		{"gzip", gzipCompress(t, raw)},
		{"xz", xzCompress(t, raw)},
		{"zstd", zstdCompress(t, raw)},
		{"uncompressed", raw},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// Pass a misleading filename — proves magic-byte sniffing
			// dominates over any suffix-based heuristic.
			dec, err := aptinstall.DecompressStreamForTesting(
				bytes.NewReader(tc.compressed),
				"/tmp/data.tar.bogus-suffix-1234567890",
			)
			require.NoError(t, err)

			defer func() { _ = dec.Close() }()

			tr := tar.NewReader(dec)

			hdr, err := tr.Next()
			require.NoError(t, err, "tar.Next on %s payload", tc.name)
			require.Equal(t, "usr/bin/marker", hdr.Name)

			body, err := io.ReadAll(tr)
			require.NoError(t, err)
			require.Equal(t, "hello\n", string(body))
		})
	}
}
