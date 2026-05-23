package apkindex

import (
	"archive/tar"
	"io"
)

// This file exports internal functions and variables for testing purposes.

// NewIndexForTesting creates a new Index for testing.
func NewIndexForTesting() *Index {
	return NewIndex()
}

// ParseReposContent exposes the internal parseReposContent function for testing.
var ParseReposContent = parseReposContent

// SetGlobalIndex sets the global index cache for testing purposes.
// Call with nil to reset the cache.
func SetGlobalIndex(idx *Index) {
	globalIndex.Store(idx)
}

// ExportSafeAPKPath exposes safeAPKPath for testing.
func ExportSafeAPKPath(entryName string) (string, bool) {
	return safeAPKPath(entryName)
}

// ExportSafeAPKSymlinkTarget exposes safeAPKSymlinkTarget for testing.
func ExportSafeAPKSymlinkTarget(linkPath, target string) error {
	return safeAPKSymlinkTarget(linkPath, target)
}

// ExportExtractAPKEntry exposes extractAPKEntry for testing.
func ExportExtractAPKEntry(tr *tar.Reader, hdr *tar.Header) error {
	return extractAPKEntry(tr, hdr)
}

// ExportExtractAPKData exposes extractAPKData for testing.
func ExportExtractAPKData(r io.Reader) error {
	return extractAPKData(r)
}
