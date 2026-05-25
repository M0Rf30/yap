// Package errors provides custom error types and error handling utilities for YAP.
//
//nolint:revive // Intentional wrapper around stdlib errors with structured error handling
package errors

import (
	"fmt"
	"sort"
	"strings"
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
	var b strings.Builder

	fmt.Fprintf(&b, "%s: %s", e.Type, e.Message)

	if e.Operation != "" {
		fmt.Fprintf(&b, " [op=%s]", e.Operation)
	}

	if len(e.Context) > 0 {
		keys := make([]string, 0, len(e.Context))
		for k := range e.Context {
			keys = append(keys, k)
		}

		sort.Strings(keys)

		b.WriteString(" {")

		for i, k := range keys {
			if i > 0 {
				b.WriteString(", ")
			}

			fmt.Fprintf(&b, "%s=%v", k, e.Context[k])
		}

		b.WriteString("}")
	}

	if e.Cause != nil {
		fmt.Fprintf(&b, " (caused by: %v)", e.Cause)
	}

	return b.String()
}

// Unwrap returns the underlying cause for error unwrapping.
func (e *YapError) Unwrap() error {
	return e.Cause
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
