//nolint:testpackage,err113 // Internal testing requires access to private functions, test errors can be dynamic
package logger

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	yaperrors "github.com/M0Rf30/yap/pkg/errors"
)

func TestLogLevel_String(t *testing.T) {
	t.Parallel()

	tests := []struct {
		level    LogLevel
		expected string
	}{
		{LevelDebug, "DEBUG"},
		{LevelInfo, "INFO"},
		{LevelWarn, "WARN"},
		{LevelError, "ERROR"},
		{LevelFatal, "FATAL"},
		{LogLevel(999), "UNKNOWN"},
	}

	for _, testCase := range tests {
		t.Run(testCase.expected, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, testCase.expected, testCase.level.String())
		})
	}
}

func TestNewLogger(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer

	config := &LoggerConfig{
		Level:      LevelDebug,
		Format:     "json",
		Output:     &buf,
		TimeFormat: time.RFC3339,
		AddSource:  true,
	}

	logger := New(config)
	require.NotNil(t, logger)
	assert.Equal(t, config, logger.config)
}

func TestNewDefault(t *testing.T) {
	t.Parallel()

	logger := NewDefault()
	require.NotNil(t, logger)
	assert.Equal(t, LevelInfo, logger.config.Level)
	assert.Equal(t, "text", logger.config.Format)
}

func TestLogger_With(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer

	config := &LoggerConfig{
		Level:  LevelDebug,
		Format: "json",
		Output: &buf,
	}

	logger := New(config)
	childLogger := logger.With("key", "value", "number", 42)

	childLogger.Info("test message")

	output := buf.String()
	assert.Contains(t, output, "test message")
	assert.Contains(t, output, "key")
	assert.Contains(t, output, "value")
	assert.Contains(t, output, "number")
	assert.Contains(t, output, "42")
}

func TestLogger_WithError(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer

	config := &LoggerConfig{
		Level:  LevelDebug,
		Format: "json",
		Output: &buf,
	}

	logger := New(config)

	// Test with regular error
	regularErr := errors.New("regular error")
	errorLogger := logger.WithError(regularErr)
	errorLogger.Error("something went wrong")

	output := buf.String()
	assert.Contains(t, output, "regular error")

	// Test with YapError
	buf.Reset()

	yapErr := yaperrors.NewValidationError("validation failed").
		WithOperation("parseFile").
		WithContext("file", "test.go")

	yapErrorLogger := logger.WithError(yapErr)
	yapErrorLogger.Error("yap error occurred")

	output = buf.String()
	assert.Contains(t, output, "validation failed")
	assert.Contains(t, output, "validation")
	assert.Contains(t, output, "parseFile")
}

func TestLogger_WithFields(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer

	config := &LoggerConfig{
		Level:  LevelDebug,
		Format: "json",
		Output: &buf,
	}

	logger := New(config)

	fields := map[string]any{
		"user_id": "123",
		"action":  "login",
		"ip":      "192.168.1.1",
	}

	fieldsLogger := logger.WithFields(fields)
	fieldsLogger.Info("user logged in")

	output := buf.String()
	assert.Contains(t, output, "user logged in")
	assert.Contains(t, output, "user_id")
	assert.Contains(t, output, "123")
	assert.Contains(t, output, "action")
	assert.Contains(t, output, "login")
}

func TestLogger_LogLevels(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		logFn func(*Logger, string, ...any)
		level string
		msg   string
	}{
		{"debug", (*Logger).Debug, "DEBUG", "debug message"},
		{"info", (*Logger).Info, "INFO", "info message"},
		{"warn", (*Logger).Warn, "WARN", "warning message"},
		{"error", (*Logger).Error, "ERROR", "error message"},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			var buf bytes.Buffer

			config := &LoggerConfig{
				Level:  LevelDebug,
				Format: "json",
				Output: &buf,
			}
			logger := New(config)

			testCase.logFn(logger, testCase.msg, "key", "value")

			output := buf.String()
			assert.Contains(t, output, testCase.msg)
			assert.Contains(t, output, "key")
			assert.Contains(t, output, "value")

			// Parse JSON to verify level
			var logEntry map[string]any

			err := json.Unmarshal([]byte(strings.TrimSpace(output)), &logEntry)
			require.NoError(t, err)
			assert.Equal(t, testCase.level, logEntry["level"])
		})
	}
}

func TestLogger_LogOperation(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer

	config := &LoggerConfig{
		Level:  LevelDebug,
		Format: "json",
		Output: &buf,
	}

	logger := New(config)

	// Test successful operation
	err := logger.LogOperation("test_operation", func() error {
		time.Sleep(10 * time.Millisecond)

		return nil
	})

	require.NoError(t, err)

	output := buf.String()
	assert.Contains(t, output, "operation started")
	assert.Contains(t, output, "operation completed")
	assert.Contains(t, output, "test_operation")
	assert.Contains(t, output, "success")

	// Test failed operation
	buf.Reset()

	testErr := errors.New("operation failed")
	err = logger.LogOperation("failed_operation", func() error {
		return testErr
	})

	require.Error(t, err)
	assert.Equal(t, testErr, err)

	output = buf.String()
	assert.Contains(t, output, "operation started")
	assert.Contains(t, output, "operation failed")
	assert.Contains(t, output, "failed_operation")
}

func TestLogger_LogOperationContext(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer

	config := &LoggerConfig{
		Level:  LevelDebug,
		Format: "json",
		Output: &buf,
	}

	logger := New(config)
	ctx := context.Background()

	err := logger.LogOperationContext(ctx, "context_operation", func(ctx context.Context) error {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(10 * time.Millisecond):
			return nil
		}
	})

	require.NoError(t, err)

	output := buf.String()
	assert.Contains(t, output, "operation started")
	assert.Contains(t, output, "operation completed")
	assert.Contains(t, output, "context_operation")
}

func TestLogger_ContextMethods(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer

	config := &LoggerConfig{
		Level:  LevelDebug,
		Format: "json",
		Output: &buf,
	}

	logger := New(config)
	ctx := context.Background()

	logger.DebugContext(ctx, "debug context message")
	logger.InfoContext(ctx, "info context message")
	logger.WarnContext(ctx, "warn context message")
	logger.ErrorContext(ctx, "error context message")

	output := buf.String()
	assert.Contains(t, output, "debug context message")
	assert.Contains(t, output, "info context message")
	assert.Contains(t, output, "warn context message")
	assert.Contains(t, output, "error context message")
}

func TestLogger_Trace(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer

	config := &LoggerConfig{
		Level:  LevelDebug,
		Format: "json",
		Output: &buf,
	}

	logger := New(config)

	func() {
		defer logger.Trace("arg1", "arg2")()

		time.Sleep(10 * time.Millisecond)
	}()

	output := buf.String()
	assert.Contains(t, output, "entering function")
	assert.Contains(t, output, "exiting function")
	assert.Contains(t, output, "TestLogger_Trace")
}

func TestConvertArgs(t *testing.T) {
	t.Parallel()

	// Test even number of args
	result := convertArgs("key1", "value1", "key2", 42)
	expected := []any{"key1", "value1", "key2", 42}
	assert.Equal(t, expected, result)

	// Test odd number of args
	result = convertArgs("key1", "value1", "orphan")
	expected = []any{"extra", "key1", "value1", "orphan"}
	assert.Equal(t, expected, result)

	// Test non-string keys
	result = convertArgs(123, "value1")
	assert.Equal(t, "arg_0", result[0])
	assert.Equal(t, "value1", result[1])
	assert.Len(t, result, 2)
}

func TestGlobalLogger(t *testing.T) {
	t.Parallel()

	// Save original global logger
	originalLogger := Global()
	defer SetGlobal(originalLogger)

	var buf bytes.Buffer

	config := &LoggerConfig{
		Level:  LevelDebug,
		Format: "text",
		Output: &buf,
	}

	testLogger := New(config)
	SetGlobal(testLogger)

	assert.Equal(t, testLogger, Global())

	// Test global functions
	Info("global info message", "key", "value")
	Debug("global debug message")
	Warn("global warn message")
	Error("global error message")

	output := buf.String()
	assert.Contains(t, output, "global info message")
	assert.Contains(t, output, "global debug message")
	assert.Contains(t, output, "global warn message")
	assert.Contains(t, output, "global error message")
}

func TestLoggerLevelFiltering(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer

	config := &LoggerConfig{
		Level:  LevelWarn, // Only warn and above should be logged
		Format: "text",
		Output: &buf,
	}

	logger := New(config)

	logger.Debug("debug message")
	logger.Info("info message")
	logger.Warn("warn message")
	logger.Error("error message")

	output := buf.String()
	assert.NotContains(t, output, "debug message")
	assert.NotContains(t, output, "info message")
	assert.Contains(t, output, "warn message")
	assert.Contains(t, output, "error message")
}

func TestTextVsJSONFormat(t *testing.T) {
	t.Parallel()

	// Test text format
	var textBuf bytes.Buffer

	textConfig := &LoggerConfig{
		Level:  LevelInfo,
		Format: "text",
		Output: &textBuf,
	}
	textLogger := New(textConfig)
	textLogger.Info("test message", "key", "value")

	textOutput := textBuf.String()
	assert.Contains(t, textOutput, "test message")
	assert.Contains(t, textOutput, "key=value")

	// Test JSON format
	var jsonBuf bytes.Buffer

	jsonConfig := &LoggerConfig{
		Level:  LevelInfo,
		Format: "json",
		Output: &jsonBuf,
	}
	jsonLogger := New(jsonConfig)
	jsonLogger.Info("test message", "key", "value")

	jsonOutput := jsonBuf.String()
	assert.Contains(t, jsonOutput, "test message")

	// Verify it's valid JSON
	var logEntry map[string]any

	err := json.Unmarshal([]byte(strings.TrimSpace(jsonOutput)), &logEntry)
	require.NoError(t, err)
	assert.Equal(t, "[yap] test message", logEntry["msg"])
	assert.Equal(t, "value", logEntry["key"])
}
