//nolint:err113,testpackage // Test errors can be dynamic, internal testing requires access to private functions
package errors

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestYapError_Error(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		err      *YapError
		expected string
	}{
		{
			name: "error without cause",
			err: &YapError{
				Type:    ErrTypeValidation,
				Message: "invalid input",
			},
			expected: "validation: invalid input",
		},
		{
			name: "error with cause",
			err: &YapError{
				Type:    ErrTypeFileSystem,
				Message: "failed to read file",
				Cause:   errors.New("permission denied"),
			},
			expected: "filesystem: failed to read file (caused by: permission denied)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.expected, tt.err.Error())
		})
	}
}

func TestYapError_Unwrap(t *testing.T) {
	t.Parallel()

	cause := errors.New("underlying error")
	err := &YapError{
		Type:    ErrTypeNetwork,
		Message: "network failed",
		Cause:   cause,
	}

	assert.Equal(t, cause, err.Unwrap())
}

func TestYapError_Is(t *testing.T) {
	t.Parallel()

	err1 := &YapError{Type: ErrTypeValidation, Message: "test"}
	err2 := &YapError{Type: ErrTypeValidation, Message: "different"}
	err3 := &YapError{Type: ErrTypeFileSystem, Message: "test"}

	assert.True(t, err1.Is(err2))
	assert.False(t, err1.Is(err3))
	assert.False(t, err1.Is(errors.New("regular error")))
}

func TestYapError_WithContext(t *testing.T) {
	t.Parallel()

	err := New(ErrTypeValidation, "test error")
	_ = err.WithContext("file", "test.go").WithContext("line", 42)

	assert.Equal(t, "test.go", err.Context["file"])
	assert.Equal(t, 42, err.Context["line"])
}

func TestYapError_WithOperation(t *testing.T) {
	t.Parallel()

	err := New(ErrTypeValidation, "test error")
	_ = err.WithOperation("parseFile")

	assert.Equal(t, "parseFile", err.Operation)
}

func TestNew(t *testing.T) {
	t.Parallel()

	err := New(ErrTypeValidation, "test message")

	assert.Equal(t, ErrTypeValidation, err.Type)
	assert.Equal(t, "test message", err.Message)
	require.NoError(t, err.Cause)
	assert.NotNil(t, err.Context)
}

func TestWrap(t *testing.T) {
	t.Parallel()

	cause := errors.New("original error")
	err := Wrap(cause, ErrTypeFileSystem, "wrapped message")

	assert.Equal(t, ErrTypeFileSystem, err.Type)
	assert.Equal(t, "wrapped message", err.Message)
	assert.Equal(t, cause, err.Cause)
}
