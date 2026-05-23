package archive

import (
	"archive/zip"
	"io"
	"os"
)

// zipIterator wraps a zip.Reader and exposes entries through the entryIterator interface.
type zipIterator struct {
	files []*zip.File
	index int
}

// newZipIterator creates a new zip iterator from a zip.Reader.
func newZipIterator(zr *zip.Reader) *zipIterator {
	return &zipIterator{
		files: zr.File,
		index: 0,
	}
}

// Next returns the next entry from the zip archive.
func (zi *zipIterator) Next() (archiveEntry, error) {
	if zi.index >= len(zi.files) {
		return archiveEntry{}, io.EOF
	}

	zf := zi.files[zi.index]
	zi.index++

	mode := zf.Mode()

	entry := archiveEntry{
		Name:    zf.Name,
		Mode:    mode,
		IsDir:   mode.IsDir(),
		Size:    int64(zf.UncompressedSize),
		ModTime: zf.Modified,
	}

	// Handle symlinks: zip stores symlink targets in the file body
	if mode&os.ModeSymlink != 0 {
		entry.IsSymlink = true
		entry.Open = zf.Open
	} else {
		// Regular files: Open returns a reader for the zip entry
		entry.Open = zf.Open
	}

	return entry, nil
}

// Close closes the zip iterator. For zip, there's nothing to close.
func (zi *zipIterator) Close() error {
	return nil
}
