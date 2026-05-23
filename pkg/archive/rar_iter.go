package archive

import (
	"io"
	"os"

	"github.com/nwaples/rardecode/v2"
)

// rarIterator wraps a rardecode.Reader and exposes entries through the entryIterator interface.
type rarIterator struct {
	rr *rardecode.Reader
}

// newRarIterator creates a new rar iterator from a rardecode.Reader.
func newRarIterator(rr *rardecode.Reader) *rarIterator {
	return &rarIterator{rr: rr}
}

// Next returns the next entry from the rar archive.
func (ri *rarIterator) Next() (archiveEntry, error) {
	hdr, err := ri.rr.Next()
	if err != nil {
		return archiveEntry{}, err
	}

	mode := hdr.Mode()

	entry := archiveEntry{
		Name:    hdr.Name,
		Mode:    mode,
		IsDir:   hdr.IsDir,
		Size:    hdr.UnPackedSize,
		ModTime: hdr.ModificationTime,
	}

	// Handle symlinks: rar stores symlink targets in the file body
	if mode&os.ModeSymlink != 0 {
		entry.IsSymlink = true
		entry.Open = func() (io.ReadCloser, error) {
			// The rar reader is already positioned at the file content.
			// We return a wrapper that reads from the rar reader directly.
			return io.NopCloser(ri.rr), nil
		}
	} else {
		// Regular files: Open returns a reader for the rar entry
		entry.Open = func() (io.ReadCloser, error) {
			// The rar reader is already positioned at the file content.
			// We return a wrapper that reads from the rar reader directly.
			return io.NopCloser(ri.rr), nil
		}
	}

	return entry, nil
}

// Close closes the rar iterator. For rar, there's nothing to close.
func (ri *rarIterator) Close() error {
	return nil
}
