package archive

import (
	"io"
	"os"

	"github.com/bodgit/sevenzip"
)

// sevenZipIterator wraps a sevenzip.Reader and exposes entries through the entryIterator interface.
type sevenZipIterator struct {
	files []*sevenzip.File
	index int
}

// newSevenZipIterator creates a new 7z iterator from a sevenzip.Reader.
func newSevenZipIterator(zr *sevenzip.Reader) *sevenZipIterator {
	return &sevenZipIterator{
		files: zr.File,
		index: 0,
	}
}

// Next returns the next entry from the 7z archive.
func (szi *sevenZipIterator) Next() (archiveEntry, error) {
	if szi.index >= len(szi.files) {
		return archiveEntry{}, io.EOF
	}

	sf := szi.files[szi.index]
	szi.index++

	mode := sf.FileInfo().Mode()

	entry := archiveEntry{
		Name:    sf.Name,
		Mode:    mode,
		IsDir:   mode.IsDir(),
		Size:    sf.FileInfo().Size(),
		ModTime: sf.FileInfo().ModTime(),
	}

	// Handle symlinks: 7z stores symlink targets in the file body
	if mode&os.ModeSymlink != 0 {
		entry.IsSymlink = true
		entry.Open = sf.Open
	} else {
		// Regular files: Open returns a reader for the 7z entry
		entry.Open = sf.Open
	}

	return entry, nil
}

// Close closes the 7z iterator. For 7z, there's nothing to close.
func (szi *sevenZipIterator) Close() error {
	return nil
}
