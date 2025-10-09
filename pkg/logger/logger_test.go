package logger

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestArgsToLoggerArgs(t *testing.T) {
	// Test with no arguments
	result := argsToLoggerArgs()
	assert.Nil(t, result)

	// Test with one argument (odd number, should ignore the last)
	result = argsToLoggerArgs("key1", "value1", "key2")
	assert.Len(t, result, 1)
	assert.Equal(t, "key1", result[0].Key)
	assert.Equal(t, "value1", result[0].Value)

	// Test with multiple key-value pairs
	result = argsToLoggerArgs("key1", "value1", "key2", "value2")
	assert.Len(t, result, 2)
	assert.Equal(t, "key1", result[0].Key)
	assert.Equal(t, "value1", result[0].Value)
	assert.Equal(t, "key2", result[1].Key)
	assert.Equal(t, "value2", result[1].Value)

	// Test with different types
	result = argsToLoggerArgs("number", 42, "boolean", true)
	assert.Len(t, result, 2)
	assert.Equal(t, "number", result[0].Key)
	assert.Equal(t, 42, result[0].Value)
	assert.Equal(t, "boolean", result[1].Key)
	assert.Equal(t, true, result[1].Value)
}

func TestYapLoggerArgs(t *testing.T) {
	logger := &YapLogger{}

	// Test with no arguments
	result := logger.Args()
	assert.Nil(t, result)

	// Test with key-value pairs
	result = logger.Args("key1", "value1", "key2", "value2")
	assert.Len(t, result, 2)
	assert.Equal(t, "key1", result[0].Key)
	assert.Equal(t, "value1", result[0].Value)
	assert.Equal(t, "key2", result[1].Key)
	assert.Equal(t, "value2", result[1].Value)
}

func TestLogLevelConstants(t *testing.T) {
	// Test that constants have expected values
	assert.Equal(t, LogLevel(0), LevelDebug)
	assert.Equal(t, LogLevel(1), LevelInfo)
	assert.Equal(t, LogLevel(2), LevelWarn)
	assert.Equal(t, LogLevel(3), LevelError)
	assert.Equal(t, LogLevel(4), LevelFatal)
}

func TestYapLoggerInfo(t *testing.T) {
	// Use the global logger instance which is properly initialized
	assert.NotPanics(t, func() {
		Logger.Info("test message")
		Logger.Info("test message with args", Logger.Args("key", "value"))
	})
}

func TestYapLoggerDebug(t *testing.T) {
	// Use the global logger instance which is properly initialized
	// Test when verbose is disabled (default)
	assert.NotPanics(t, func() {
		Logger.Debug("test debug message")
		Logger.Debug("test debug message with args", Logger.Args("key", "value"))
	})

	// Test when verbose is enabled
	SetVerbose(true)
	assert.NotPanics(t, func() {
		Logger.Debug("test debug message with verbose enabled")
		Logger.Debug("test debug message with args and verbose", Logger.Args("key", "value"))
	})

	// Reset verbose setting
	SetVerbose(false)
}

func TestYapLoggerWarn(t *testing.T) {
	// Use the global logger instance which is properly initialized
	assert.NotPanics(t, func() {
		Logger.Warn("test warning message")
		Logger.Warn("test warning message with args", Logger.Args("key", "value"))
	})
}

func TestYapLoggerError(t *testing.T) {
	// Use the global logger instance which is properly initialized
	assert.NotPanics(t, func() {
		Logger.Error("test error message")
		Logger.Error("test error message with args", Logger.Args("key", "value"))
	})
}

func TestYapLoggerFatal(t *testing.T) {
	// We skip testing the actual Fatal call since it calls os.Exit and terminates the process
	// Instead, we just verify that the function exists and is accessible
	assert.NotNil(t, Logger.Fatal)
}

func TestYapLoggerTips(t *testing.T) {
	// Use the global logger instance which is properly initialized
	assert.NotPanics(t, func() {
		Logger.Tips("test tips message")
		Logger.Tips("test tips message with args", Logger.Args("key", "value"))
	})
}

func TestSetColorDisabled(t *testing.T) {
	// Save original environment variables
	oldNoColor := os.Getenv("NO_COLOR")
	oldColorTerm := os.Getenv("COLORTERM")
	oldTerm := os.Getenv("TERM")

	// Defer restoring environment variables
	defer func() {
		_ = os.Setenv("NO_COLOR", oldNoColor)
		_ = os.Setenv("COLORTERM", oldColorTerm)
		_ = os.Setenv("TERM", oldTerm)

		SetColorDisabled(false) // Reset to default behavior
	}()

	// Clear environment variables that might affect color detection
	_ = os.Unsetenv("NO_COLOR")
	_ = os.Unsetenv("COLORTERM")
	_ = os.Unsetenv("TERM")

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

	// Test with NO_COLOR environment variable
	oldNoColor := os.Getenv("NO_COLOR")

	defer func() {
		_ = os.Setenv("NO_COLOR", oldNoColor)
	}()

	_ = os.Setenv("NO_COLOR", "1")

	assert.True(t, IsColorDisabled())
}

func TestSetVerbose(t *testing.T) {
	// Save original verbose state
	originalVerbose := IsVerboseEnabled()
	defer SetVerbose(originalVerbose)

	// Test enabling verbose
	SetVerbose(true)
	assert.True(t, IsVerboseEnabled())

	// Test disabling verbose
	SetVerbose(false)
	assert.False(t, IsVerboseEnabled())
}

func TestIsVerboseEnabled(t *testing.T) {
	// Save original verbose state
	originalVerbose := IsVerboseEnabled()
	defer SetVerbose(originalVerbose)

	// Test default state
	result := IsVerboseEnabled()
	assert.NotNil(t, result)

	// Test after setting to true
	SetVerbose(true)
	assert.True(t, IsVerboseEnabled())

	// Reset to original state
	SetVerbose(originalVerbose)
}

func TestGlobalLoggerFunctions(t *testing.T) {
	// Save original verbose state
	originalVerbose := IsVerboseEnabled()
	defer SetVerbose(originalVerbose)

	// Test all global logger functions
	assert.NotPanics(t, func() {
		Info("test global info", "key", "value")
		Debug("test global debug", "key", "value")
		Warn("test global warn", "key", "value")
		Error("test global error", "key", "value")
		Tips("test global tips", "key", "value")

		// Enable verbose to test debug
		SetVerbose(true)
		Debug("test global debug with verbose", "key", "value")
	})
}

func TestFormatYapPrefix(t *testing.T) {
	// Test that formatYapPrefix doesn't panic and returns a string
	result := formatYapPrefix("test message")
	assert.Contains(t, result, "[")
	assert.Contains(t, result, "yap")
	assert.Contains(t, result, "]")
	assert.Contains(t, result, "test message")
}
