package buffers

import (
	"testing"

	"github.com/M0Rf30/yap/v2/pkg/constants"
)

func TestBufferPools(t *testing.T) {
	t.Run("DefaultBufferPool", func(t *testing.T) {
		buf := DefaultBufferPool.Get()
		if buf == nil {
			t.Fatal("DefaultBufferPool.Get() returned nil")
		}

		byteSlice, ok := buf.([]byte)
		if !ok {
			t.Fatal("DefaultBufferPool.Get() did not return []byte")
		}

		if len(byteSlice) != constants.DefaultBufferSize {
			t.Errorf("Expected buffer size %d, got %d", constants.DefaultBufferSize, len(byteSlice))
		}

		// Put it back to verify the pool works
		DefaultBufferPool.Put(byteSlice) //nolint:staticcheck // SA6002: sync.Pool expects value, not pointer
	})

	t.Run("SmallBufferPool", func(t *testing.T) {
		buf := SmallBufferPool.Get()
		if buf == nil {
			t.Fatal("SmallBufferPool.Get() returned nil")
		}

		byteSlice, ok := buf.([]byte)
		if !ok {
			t.Fatal("SmallBufferPool.Get() did not return []byte")
		}

		if len(byteSlice) != constants.SmallBufferSize {
			t.Errorf("Expected buffer size %d, got %d", constants.SmallBufferSize, len(byteSlice))
		}

		// Put it back to verify the pool works
		SmallBufferPool.Put(byteSlice) //nolint:staticcheck // SA6002: sync.Pool expects value, not pointer
	})
}

func TestGetAndPutSmallBuffer(t *testing.T) {
	// Get a buffer from the small buffer pool
	buf := GetSmallBuffer()

	// Verify the buffer has the correct size
	if len(buf) != constants.SmallBufferSize {
		t.Errorf("Expected buffer size %d, got %d", constants.SmallBufferSize, len(buf))
	}

	// Put the buffer back
	PutSmallBuffer(buf)

	// Verify that a buffer with wrong size is not put back
	wrongSizedBuf := make([]byte, constants.SmallBufferSize+1)
	PutSmallBuffer(wrongSizedBuf) // This should not put the buffer back

	// Get another buffer to make sure the pool still works
	anotherBuf := GetSmallBuffer()
	if len(anotherBuf) != constants.SmallBufferSize {
		t.Errorf("Expected buffer size %d after put with wrong size, got %d", constants.SmallBufferSize, len(anotherBuf))
	}

	// Put it back
	PutSmallBuffer(anotherBuf)
}

func TestBufferPoolReuse(t *testing.T) {
	// Get a buffer
	buf1 := GetSmallBuffer()
	if len(buf1) != constants.SmallBufferSize {
		t.Fatalf("Expected buffer size %d, got %d", constants.SmallBufferSize, len(buf1))
	}

	// Put it back
	PutSmallBuffer(buf1)

	// Get another buffer - it might be the same one from the pool
	buf2 := GetSmallBuffer()
	if len(buf2) != constants.SmallBufferSize {
		t.Fatalf("Expected buffer size %d, got %d", constants.SmallBufferSize, len(buf2))
	}

	// Put it back
	PutSmallBuffer(buf2)
}
