// Package buffers provides efficient memory management utilities for buffer pooling.
package buffers

import (
	"sync"

	"github.com/M0Rf30/yap/v2/pkg/constants"
)

// Buffer pools for different use cases to reduce garbage collection pressure.
var (
	// DefaultBufferPool provides buffers for general file operations (32KB).
	DefaultBufferPool = sync.Pool{
		New: func() interface{} {
			return make([]byte, constants.DefaultBufferSize)
		},
	}

	// SmallBufferPool provides smaller buffers for line-based operations (1KB).
	SmallBufferPool = sync.Pool{
		New: func() interface{} {
			return make([]byte, constants.SmallBufferSize)
		},
	}
)

// GetDefaultBuffer returns a buffer from the default pool.
func GetDefaultBuffer() []byte {
	return DefaultBufferPool.Get().([]byte)
}

// PutDefaultBuffer returns a buffer to the default pool.
func PutDefaultBuffer(buf []byte) {
	if len(buf) == constants.DefaultBufferSize {
		DefaultBufferPool.Put(&buf)
	}
}

// GetSmallBuffer returns a buffer from the small buffer pool.
func GetSmallBuffer() []byte {
	return SmallBufferPool.Get().([]byte)
}

// PutSmallBuffer returns a buffer to the small buffer pool.
func PutSmallBuffer(buf []byte) {
	if len(buf) == constants.SmallBufferSize {
		SmallBufferPool.Put(&buf)
	}
}
