package archive

import (
	"archive/tar"
	"io"
)

// tarIterator wraps a tar.Reader and exposes entries through the entryIterator interface.
type tarIterator struct {
	tr *tar.Reader
}

// newTarIterator creates a new tar iterator from a tar.Reader.
func newTarIterator(tr *tar.Reader) *tarIterator {
	return &tarIterator{tr: tr}
}

// Next returns the next entry from the tar archive.
func (ti *tarIterator) Next() (archiveEntry, error) {
	hdr, err := ti.tr.Next()
	if err != nil {
		return archiveEntry{}, err
	}

	entry := archiveEntry{
		Name:    hdr.Name,
		Mode:    hdr.FileInfo().Mode(),
		IsDir:   hdr.Typeflag == tar.TypeDir,
		Size:    hdr.Size,
		ModTime: hdr.ModTime,
	}

	// Handle symlinks
	switch hdr.Typeflag {
	case tar.TypeSymlink:
		entry.IsSymlink = true
		entry.LinkTarget = hdr.Linkname
		entry.Open = nil
	case tar.TypeReg, tar.TypeRegA: //nolint:staticcheck // TypeRegA kept for legacy tar compatibility
		// Regular files: Open returns a reader for the tar stream
		entry.Open = func() (io.ReadCloser, error) {
			// The tar reader is already positioned at the file content.
			// We return a wrapper that reads from the tar reader directly.
			return io.NopCloser(ti.tr), nil
		}
	}

	return entry, nil
}

// Close closes the tar iterator. For tar, there's nothing to close.
func (ti *tarIterator) Close() error {
	return nil
}
