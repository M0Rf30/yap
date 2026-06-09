package dnfcache

import (
	"bufio"
	"io"
	"os"
	"strings"

	"github.com/klauspost/compress/gzip"
	"github.com/klauspost/compress/zstd"
	"github.com/ulikunitz/xz"
)

// openCompressed opens path and returns a reader that transparently
// decompresses .gz/.xz/.zst content; any other extension passes through
// as-is. The returned closer releases the decompressor and the underlying
// file and must always be called (also on parse errors).
//
// All decompressors read through bufio.NewReader: ulikunitz/xz in
// particular reads in small chunks, and without buffering this causes
// excessive syscall overhead — 5-8x slower than buffered I/O.
func openCompressed(path string) (io.Reader, func(), error) {
	f, err := os.Open(path) //nolint:gosec
	if err != nil {
		return nil, nil, err
	}

	br := bufio.NewReader(f)

	switch {
	case strings.HasSuffix(path, ".gz"):
		gz, err := gzip.NewReader(br)
		if err != nil {
			_ = f.Close()
			return nil, nil, err
		}

		return gz, func() { _ = gz.Close(); _ = f.Close() }, nil

	case strings.HasSuffix(path, ".xz"):
		xzr, err := xz.NewReader(br)
		if err != nil {
			_ = f.Close()
			return nil, nil, err
		}

		return xzr, func() { _ = f.Close() }, nil

	case strings.HasSuffix(path, ".zst"):
		zr, err := zstd.NewReader(br)
		if err != nil {
			_ = f.Close()
			return nil, nil, err
		}

		return zr, func() { zr.Close(); _ = f.Close() }, nil
	}

	return br, func() { _ = f.Close() }, nil
}
