package errors

import (
	"errors"
	"fmt"
)

// ErrorType represents different categories of errors in the application.
type ErrorType string

const (
	// Core error types.
	ErrTypeValidation    ErrorType = "validation"
	ErrTypeFileSystem    ErrorType = "filesystem"
	ErrTypeNetwork       ErrorType = "network"
	ErrTypePackaging     ErrorType = "packaging"
	ErrTypeConfiguration ErrorType = "configuration"
	ErrTypeBuild         ErrorType = "build"
	ErrTypeParser        ErrorType = "parser"
	ErrTypeInternal      ErrorType = "internal"
)

// YapError represents a structured error with context.
type YapError struct {
	Type      ErrorType
	Message   string
	Cause     error
	Operation string
	Context   map[string]any
}

// Newf creates a new YapError with formatted message.
func Newf(errType ErrorType, format string, args ...any) *YapError {
	return New(errType, fmt.Sprintf(format, args...))
}

// Predefined error constructors for common scenarios

// NewValidationError creates a validation error.
func NewValidationError(message string) *YapError {
	return New(ErrTypeValidation, message)
}

// NewFileSystemError creates a filesystem error.
func NewFileSystemError(message string) *YapError {
	return New(ErrTypeFileSystem, message)
}

// NewNetworkError creates a network error.
func NewNetworkError(message string) *YapError {
	return New(ErrTypeNetwork, message)
}

// NewPackagingError creates a packaging error.
func NewPackagingError(message string) *YapError {
	return New(ErrTypePackaging, message)
}

// NewConfigurationError creates a configuration error.
func NewConfigurationError(message string) *YapError {
	return New(ErrTypeConfiguration, message)
}

// NewBuildError creates a build error.
func NewBuildError(message string) *YapError {
	return New(ErrTypeBuild, message)
}

// NewParserError creates a parser error.
func NewParserError(message string) *YapError {
	return New(ErrTypeParser, message)
}

// NewInternalError creates an internal error.
func NewInternalError(message string) *YapError {
	return New(ErrTypeInternal, message)
}

// Error implements the error interface.
func (e *YapError) Error() string {
	if e.Cause != nil {
		return fmt.Sprintf("%s: %s (caused by: %v)", e.Type, e.Message, e.Cause)
	}

	return fmt.Sprintf("%s: %s", e.Type, e.Message)
}

// Unwrap returns the underlying cause for error unwrapping.
func (e *YapError) Unwrap() error {
	return e.Cause
}

// Is implements error comparison for errors.Is.
func (e *YapError) Is(target error) bool {
	var yerr *YapError
	if errors.As(target, &yerr) {
		return e.Type == yerr.Type
	}

	return false
}

// WithContext adds context information to the error.
func (e *YapError) WithContext(key string, value any) *YapError {
	if e.Context == nil {
		e.Context = make(map[string]any)
	}

	e.Context[key] = value

	return e
}

// WithOperation sets the operation that caused the error.
func (e *YapError) WithOperation(op string) *YapError {
	e.Operation = op

	return e
}

// New creates a new YapError.
func New(errType ErrorType, message string) *YapError {
	return &YapError{
		Type:    errType,
		Message: message,
		Context: make(map[string]any),
	}
}

// Wrap wraps an existing error with YapError context.
func Wrap(err error, errType ErrorType, message string) *YapError {
	return &YapError{
		Type:    errType,
		Message: message,
		Cause:   err,
		Context: make(map[string]any),
	}
}

// Wrapf wraps an existing error with formatted message.
func Wrapf(err error, errType ErrorType, format string, args ...any) *YapError {
	return Wrap(err, errType, fmt.Sprintf(format, args...))
}

// IsType checks if an error is of a specific type.
func IsType(err error, errType ErrorType) bool {
	var yerr *YapError
	if errors.As(err, &yerr) {
		return yerr.Type == errType
	}

	return false
}

// GetContext retrieves context from a YapError.
func GetContext(err error) map[string]any {
	var yerr *YapError
	if errors.As(err, &yerr) {
		return yerr.Context
	}

	return nil
}

// Chain creates an error chain for related operations.
type ChainError struct {
	errors []error
}

// NewChain creates a new error chain.
func NewChain() *ChainError {
	return &ChainError{
		errors: make([]error, 0),
	}
}

// Add adds an error to the chain.
func (ec *ChainError) Add(err error) *ChainError {
	if err != nil {
		ec.errors = append(ec.errors, err)
	}

	return ec
}

// HasErrors returns true if the chain contains any errors.
func (ec *ChainError) HasErrors() bool {
	return len(ec.errors) > 0
}

// Errors returns all errors in the chain.
func (ec *ChainError) Errors() []error {
	return ec.errors
}

// Error implements the error interface for ChainError.
func (ec *ChainError) Error() string {
	if len(ec.errors) == 0 {
		return "no errors"
	}

	if len(ec.errors) == 1 {
		return ec.errors[0].Error()
	}

	result := fmt.Sprintf("multiple errors (%d):", len(ec.errors))
	for i, err := range ec.errors {
		result += fmt.Sprintf("\n  %d: %v", i+1, err)
	}

	return result
}

// First returns the first error in the chain.
func (ec *ChainError) First() error {
	if len(ec.errors) == 0 {
		return nil
	}

	return ec.errors[0]
}

// Last returns the last error in the chain.
func (ec *ChainError) Last() error {
	if len(ec.errors) == 0 {
		return nil
	}

	return ec.errors[len(ec.errors)-1]
}
