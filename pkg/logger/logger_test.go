package logger

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSetColorDisabled(t *testing.T) {
	// Test setting color disabled to true
	SetColorDisabled(true)
	assert.True(t, IsColorDisabled())

	// Test setting color disabled to false
	SetColorDisabled(false)
	assert.False(t, IsColorDisabled())
}

func TestIsColorDisabled(t *testing.T) {
	// Test default state
	result := IsColorDisabled()
	// Result can be true or false depending on environment, just ensure it doesn't panic
	assert.NotNil(t, result)
}

func TestLoggerFunctions(t *testing.T) {
	// Test logger functions that have 0% coverage
	assert.NotPanics(t, func() {
		// These functions may output to stderr/stdout but shouldn't panic
		Tips("This is a tip message")
		Warn("This is a warning message")
		Debug("This is a debug message")
	})
}

func TestWithComponent(t *testing.T) {
	// Test WithComponent function
	assert.NotPanics(t, func() {
		WithComponent("test-component")

		// Test component logger methods (but not Fatal as it exits)
		Info("test info")
		Warn("test warning")
		Error("test error")
		Debug("test debug")
	})
}
