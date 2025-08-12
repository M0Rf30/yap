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

func TestWrapf(t *testing.T) {
	t.Parallel()

	cause := errors.New("original error")
	err := Wrapf(cause, ErrTypeFileSystem, "failed to process %s at line %d", "file.go", 42)

	assert.Equal(t, "failed to process file.go at line 42", err.Message)
	assert.Equal(t, cause, err.Cause)
}

func TestNewf(t *testing.T) {
	t.Parallel()

	err := Newf(ErrTypeValidation, "validation failed for %s", "username")

	assert.Equal(t, "validation failed for username", err.Message)
}

func TestPreDefinedErrorConstructors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		fn       func(string) *YapError
		expected ErrorType
	}{
		{"NewValidationError", NewValidationError, ErrTypeValidation},
		{"NewFileSystemError", NewFileSystemError, ErrTypeFileSystem},
		{"NewNetworkError", NewNetworkError, ErrTypeNetwork},
		{"NewPackagingError", NewPackagingError, ErrTypePackaging},
		{"NewConfigurationError", NewConfigurationError, ErrTypeConfiguration},
		{"NewBuildError", NewBuildError, ErrTypeBuild},
		{"NewParserError", NewParserError, ErrTypeParser},
		{"NewInternalError", NewInternalError, ErrTypeInternal},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := tt.fn("test message")
			assert.Equal(t, tt.expected, err.Type)
			assert.Equal(t, "test message", err.Message)
		})
	}
}

func TestIsType(t *testing.T) {
	t.Parallel()

	err := NewValidationError("test")
	assert.True(t, IsType(err, ErrTypeValidation))
	assert.False(t, IsType(err, ErrTypeFileSystem))
	assert.False(t, IsType(errors.New("regular error"), ErrTypeValidation))
}

func TestGetContext(t *testing.T) {
	t.Parallel()

	err := New(ErrTypeValidation, "test").WithContext("key", "value")
	context := GetContext(err)

	assert.NotNil(t, context)
	assert.Equal(t, "value", context["key"])

	context = GetContext(errors.New("regular error"))
	assert.Nil(t, context)
}

func TestErrorChain(t *testing.T) {
	t.Parallel()

	chain := NewChain()
	assert.False(t, chain.HasErrors())
	assert.Equal(t, "no errors", chain.Error())
	assert.NoError(t, chain.First())
	assert.NoError(t, chain.Last())

	err1 := errors.New("error 1")
	err2 := errors.New("error 2")

	_ = chain.Add(err1).Add(err2).Add(nil) // nil should be ignored

	assert.True(t, chain.HasErrors())
	assert.Len(t, chain.Errors(), 2)
	assert.Equal(t, err1, chain.First())
	assert.Equal(t, err2, chain.Last())

	expectedError := "multiple errors (2):\n  1: error 1\n  2: error 2"
	assert.Equal(t, expectedError, chain.Error())
}

func TestErrorChain_SingleError(t *testing.T) {
	t.Parallel()

	chain := NewChain()
	err := errors.New("single error")
	_ = chain.Add(err)

	assert.Equal(t, err.Error(), chain.Error())
}

func TestErrorChain_ErrorsIntegration(t *testing.T) {
	t.Parallel()

	// Test that ErrorChain works with errors.Is and errors.As
	chain := NewChain()
	yapErr := NewValidationError("validation failed")
	_ = chain.Add(yapErr)

	// Test that we can unwrap the error
	var unwrapped *YapError
	require.ErrorAs(t, chain.First(), &unwrapped)
	assert.Equal(t, ErrTypeValidation, unwrapped.Type)
}
