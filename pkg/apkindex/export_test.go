package apkindex

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
