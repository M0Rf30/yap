package archive

import (
	"io"
	"io/fs"
	"time"
)

// archiveEntry represents a single entry in an archive (file, directory, or symlink).
type archiveEntry struct {
	Name       string                        // Entry name within the archive
	Mode       fs.FileMode                   // File mode (permissions + type bits)
	IsDir      bool                          // True if this is a directory
	IsSymlink  bool                          // True if this is a symlink
	LinkTarget string                        // Symlink target (only valid if IsSymlink)
	Size       int64                         // File size in bytes
	ModTime    time.Time                     // Modification time
	Open       func() (io.ReadCloser, error) // Open the entry payload (nil for dirs/symlinks)
}

// entryIterator provides a uniform interface for iterating over archive entries.
// Implementations must handle format-specific details (tar headers, zip files, etc.)
// and expose them through the archiveEntry abstraction.
type entryIterator interface {
	// Next returns the next entry in the archive. Returns io.EOF when done.
	Next() (archiveEntry, error)

	// Close closes the iterator and any underlying resources.
	Close() error
}
