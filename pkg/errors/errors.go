// Package errors provides custom error types and error handling utilities for YAP.
package errors

import (
	"errors"
	"fmt"
)

// ErrorType represents different categories of errors in the application.
type ErrorType string

const (
	// ErrTypeValidation represents validation errors.
	ErrTypeValidation ErrorType = "validation"
	// ErrTypeFileSystem represents filesystem errors.
	ErrTypeFileSystem ErrorType = "filesystem"
	// ErrTypeNetwork represents network errors.
	ErrTypeNetwork ErrorType = "network"
	// ErrTypePackaging represents packaging errors.
	ErrTypePackaging ErrorType = "packaging"
	// ErrTypeConfiguration represents configuration errors.
	ErrTypeConfiguration ErrorType = "configuration"
	// ErrTypeBuild represents build errors.
	ErrTypeBuild ErrorType = "build"
	// ErrTypeParser represents parser errors.
	ErrTypeParser ErrorType = "parser"
	// ErrTypeInternal represents internal errors.
	ErrTypeInternal ErrorType = "internal"
)

// YapError represents a structured error with context.
type YapError struct {
	Type      ErrorType
	Message   string
	Cause     error
	Operation string
	Context   map[string]any
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
