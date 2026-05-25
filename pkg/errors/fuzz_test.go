package errors_test

import (
	"errors"
	"testing"

	yerrors "github.com/M0Rf30/yap/v2/pkg/errors"
)

// FuzzErrorNew tests creating errors with arbitrary types and messages.
// Must never panic. Error() must return non-empty string. Unwrap must not panic.
func FuzzErrorNew(f *testing.F) {
	// Seed corpus with various error types and messages
	f.Add(int(0), "validation error")
	f.Add(int(1), "filesystem error")
	f.Add(int(2), "network error")
	f.Add(int(3), "packaging error")
	f.Add(int(4), "configuration error")
	f.Add(int(5), "build error")
	f.Add(int(6), "parser error")
	f.Add(int(7), "internal error")
	f.Add(int(8), "unknown error type")
	f.Add(int(-1), "negative error type")
	f.Add(int(999), "very large error type")
	f.Add(int(0), "")
	f.Add(int(0), "very long error message "+string(make([]byte, 10000)))
	f.Add(int(0), "error with\nnewlines\nand\ttabs")
	f.Add(int(0), "error with special chars: !@#$%^&*()")

	f.Fuzz(func(t *testing.T, errTypeInt int, message string) {
		// Cast to ErrorType (will be one of the valid types or an unknown value)
		var errType yerrors.ErrorType

		switch errTypeInt {
		case 0:
			errType = yerrors.ErrTypeValidation
		case 1:
			errType = yerrors.ErrTypeFileSystem
		case 2:
			errType = yerrors.ErrTypeNetwork
		case 3:
			errType = yerrors.ErrTypePackaging
		case 4:
			errType = yerrors.ErrTypeConfiguration
		case 5:
			errType = yerrors.ErrTypeBuild
		case 6:
			errType = yerrors.ErrTypeParser
		case 7:
			errType = yerrors.ErrTypeInternal
		default:
			// Use arbitrary string for unknown types
			errType = yerrors.ErrorType("unknown")
		}

		// Should not panic
		err := yerrors.New(errType, message)

		// Error() must return non-empty string
		errStr := err.Error()
		if errStr == "" {
			t.Errorf("Error() returned empty string for type %q, message %q", errType, message)
		}

		// Unwrap must not panic
		_ = err.Unwrap()

		// WithContext must not panic
		_ = err.WithContext("key", "value")

		// WithOperation must not panic
		_ = err.WithOperation("TestOp")
	})
}

// FuzzErrorWrap tests wrapping arbitrary errors with arbitrary messages.
// Must never panic. Chain must be walkable.
func FuzzErrorWrap(f *testing.F) {
	f.Add(int(0), "original error", "wrap message")
	f.Add(int(1), "filesystem error", "wrapped")
	f.Add(int(2), "", "")
	f.Add(int(3), "error with\nnewlines", "wrap with\ttabs")

	f.Fuzz(func(t *testing.T, errTypeInt int, originalMsg, wrapMsg string) {
		// Create original error
		originalErr := errors.New(originalMsg)

		// Cast to ErrorType
		var errType yerrors.ErrorType

		switch errTypeInt {
		case 0:
			errType = yerrors.ErrTypeValidation
		case 1:
			errType = yerrors.ErrTypeFileSystem
		case 2:
			errType = yerrors.ErrTypeNetwork
		case 3:
			errType = yerrors.ErrTypePackaging
		default:
			errType = yerrors.ErrTypeConfiguration
		}

		// Should not panic
		err := yerrors.Wrap(originalErr, errType, wrapMsg)

		// Error() must return non-empty string
		errStr := err.Error()
		if errStr == "" {
			t.Errorf("Error() returned empty string")
		}

		// Unwrap must return the original error
		unwrapped := err.Unwrap()
		if !errors.Is(unwrapped, originalErr) {
			t.Errorf("Unwrap did not return original error")
		}

		// Chain must be walkable
		current := error(err)
		for current != nil {
			if unwrappable, ok := current.(interface{ Unwrap() error }); ok {
				current = unwrappable.Unwrap()
			} else {
				break
			}
		}
	})
}

// FuzzWithContext tests calling WithContext with arbitrary key/value pairs.
// Must never panic. Error() must still return non-empty string.
func FuzzWithContext(f *testing.F) {
	f.Add("key", "value")
	f.Add("", "")
	f.Add("key", "")
	f.Add("", "value")
	f.Add("key with spaces", "value with spaces")
	f.Add("key\nwith\nnewlines", "value\twith\ttabs")
	f.Add("very_long_key_"+string(make([]byte, 1000)), "very_long_value_"+string(make([]byte, 1000)))

	f.Fuzz(func(t *testing.T, key string, value string) {
		err := yerrors.New(yerrors.ErrTypeValidation, "test error")

		// Should not panic
		_ = err.WithContext(key, value)
		_ = err.WithContext("another_key", 42)
		_ = err.WithContext("third_key", true)

		// Error() must still return non-empty string
		errStr := err.Error()
		if errStr == "" {
			t.Errorf("Error() returned empty string after WithContext")
		}

		// Context should be accessible
		if err.Context != nil {
			if val, ok := err.Context[key]; ok {
				if val != value {
					t.Errorf("Context value mismatch: expected %q, got %q", value, val)
				}
			}
		}
	})
}

// FuzzErrorChaining tests complex error chains.
// Must never panic, chain must be walkable.
func FuzzErrorChaining(f *testing.F) {
	f.Add("msg1", "msg2", "msg3")
	f.Add("", "", "")
	f.Add("a", "b", "c")

	f.Fuzz(func(t *testing.T, msg1, msg2, msg3 string) {
		// Create a chain of errors
		err1 := errors.New(msg1)
		err2 := yerrors.Wrap(err1, yerrors.ErrTypeValidation, msg2)
		err3 := yerrors.Wrap(err2, yerrors.ErrTypeFileSystem, msg3)

		// Should not panic
		_ = err3.Error()

		// Chain must be walkable
		current := error(err3)

		depth := 0
		for current != nil && depth < 100 {
			if unwrappable, ok := current.(interface{ Unwrap() error }); ok {
				current = unwrappable.Unwrap()
			} else {
				break
			}

			depth++
		}

		// Should have walked at least 2 levels
		if depth < 2 {
			t.Errorf("Error chain too short: depth %d", depth)
		}
	})
}

// FuzzErrorIs validates errors.Is chain-walking via Unwrap. Must never panic.
func FuzzErrorIs(f *testing.F) {
	f.Add(int(0), int(0))
	f.Add(int(0), int(1))
	f.Add(int(1), int(1))
	f.Add(int(2), int(3))

	f.Fuzz(func(t *testing.T, type1Int, type2Int int) {
		typeMap := map[int]yerrors.ErrorType{
			0: yerrors.ErrTypeValidation,
			1: yerrors.ErrTypeFileSystem,
			2: yerrors.ErrTypeNetwork,
			3: yerrors.ErrTypePackaging,
		}

		type1 := typeMap[type1Int%4]
		_ = typeMap[type2Int%4]

		// Wrap a sentinel and confirm errors.Is finds it through Unwrap.
		sentinel := errors.New("sentinel")
		wrapped := yerrors.Wrap(sentinel, type1, "wrapper")

		if !errors.Is(wrapped, sentinel) {
			t.Errorf("errors.Is failed to find sentinel through Unwrap chain")
		}
	})
}
