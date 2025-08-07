package logger

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"runtime"
	"time"

	yaperrors "github.com/M0Rf30/yap/pkg/errors"
)

// LogLevel represents the severity level of log messages.
type LogLevel int

// yapHandler wraps slog.Handler to add [yap] prefix to messages.
type yapHandler struct {
	handler slog.Handler
}

func (h *yapHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.handler.Enabled(ctx, level)
}

//nolint:gocritic // slog.Handler interface requires slog.Record by value
func (h *yapHandler) Handle(ctx context.Context, record slog.Record) error {
	// Create a copy to modify without affecting the original
	recordCopy := record
	recordCopy.Message = "[yap] " + recordCopy.Message

	return h.handler.Handle(ctx, recordCopy)
}

func (h *yapHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &yapHandler{handler: h.handler.WithAttrs(attrs)}
}

func (h *yapHandler) WithGroup(name string) slog.Handler {
	return &yapHandler{handler: h.handler.WithGroup(name)}
}

const (
	LevelDebug LogLevel = iota
	LevelInfo
	LevelWarn
	LevelError
	LevelFatal
)

// String returns the string representation of the log level.
func (l LogLevel) String() string {
	switch l {
	case LevelDebug:
		return "DEBUG"
	case LevelInfo:
		return "INFO"
	case LevelWarn:
		return "WARN"
	case LevelError:
		return "ERROR"
	case LevelFatal:
		return "FATAL"
	default:
		return "UNKNOWN"
	}
}

// ToSlogLevel converts LogLevel to slog.Level.
func (l LogLevel) ToSlogLevel() slog.Level {
	switch l {
	case LevelDebug:
		return slog.LevelDebug
	case LevelInfo:
		return slog.LevelInfo
	case LevelWarn:
		return slog.LevelWarn
	case LevelError:
		return slog.LevelError
	case LevelFatal:
		return slog.LevelError // slog doesn't have fatal, use error
	default:
		return slog.LevelInfo
	}
}

// LoggerConfig holds configuration for the logger.
//

type LoggerConfig struct {
	Level      LogLevel
	Format     string    // "json" or "text"
	Output     io.Writer // defaults to os.Stdout
	TimeFormat string    // time format for logs
	AddSource  bool      // include source location
}

// DefaultConfig returns a default logger configuration.
func DefaultConfig() *LoggerConfig {
	return &LoggerConfig{
		Level:      LevelInfo,
		Format:     "text",
		Output:     os.Stdout,
		TimeFormat: time.RFC3339,
		AddSource:  false,
	}
}

// Logger wraps slog.Logger with additional functionality.
type Logger struct {
	slog   *slog.Logger
	config *LoggerConfig
}

// New creates a new Logger with the given configuration.
func New(config *LoggerConfig) *Logger {
	if config == nil {
		config = DefaultConfig()
	}

	var handler slog.Handler

	opts := &slog.HandlerOptions{
		Level:     config.Level.ToSlogLevel(),
		AddSource: config.AddSource,
		ReplaceAttr: func(_ []string, attr slog.Attr) slog.Attr {
			// Customize time format
			if attr.Key == slog.TimeKey {
				return slog.Attr{
					Key:   attr.Key,
					Value: slog.StringValue(attr.Value.Time().Format(config.TimeFormat)),
				}
			}

			return attr
		},
	}

	var baseHandler slog.Handler

	switch config.Format {
	case "json":
		baseHandler = slog.NewJSONHandler(config.Output, opts)
	default:
		baseHandler = slog.NewTextHandler(config.Output, opts)
	}

	// Wrap with our custom handler to add [yap] prefix
	handler = &yapHandler{handler: baseHandler}

	return &Logger{
		slog:   slog.New(handler),
		config: config,
	}
}

// NewDefault creates a logger with default configuration.
func NewDefault() *Logger {
	return New(DefaultConfig())
}

// WithContext returns a logger with context values.
func (l *Logger) WithContext(ctx context.Context) *Logger {
	return &Logger{
		slog:   l.slog.With(slog.Any("context", ctx)),
		config: l.config,
	}
}

// With returns a logger with additional key-value pairs.
func (l *Logger) With(args ...any) *Logger {
	return &Logger{
		slog:   l.slog.With(convertArgs(args...)...),
		config: l.config,
	}
}

// WithError returns a logger with error information.
func (l *Logger) WithError(err error) *Logger {
	if err == nil {
		return l
	}

	attrs := []slog.Attr{
		slog.String("error", err.Error()),
	}

	// Add YapError-specific attributes
	var yapErr *yaperrors.YapError
	if errors.As(err, &yapErr) {
		attrs = append(attrs,
			slog.String("error_type", string(yapErr.Type)),
			slog.String("operation", yapErr.Operation),
		)

		if len(yapErr.Context) > 0 {
			contextJSON, err := json.Marshal(yapErr.Context)
			if err == nil {
				attrs = append(attrs, slog.String("error_context", string(contextJSON)))
			}
		}
	}

	return &Logger{
		slog:   l.slog.With(slog.Group("error_info", convertAttrsToAny(attrs)...)),
		config: l.config,
	}
}

// WithFields returns a logger with structured fields.
func (l *Logger) WithFields(fields map[string]any) *Logger {
	args := make([]any, 0, len(fields)*2)
	for k, v := range fields {
		args = append(args, k, v)
	}

	return l.With(args...)
}

// Debug logs a debug message.
func (l *Logger) Debug(msg string, args ...any) {
	l.slog.Debug(msg, convertArgs(args...)...)
}

// Info logs an info message.
func (l *Logger) Info(msg string, args ...any) {
	l.slog.Info(msg, convertArgs(args...)...)
}

// Warn logs a warning message.
func (l *Logger) Warn(msg string, args ...any) {
	l.slog.Warn(msg, convertArgs(args...)...)
}

// Error logs an error message.
func (l *Logger) Error(msg string, args ...any) {
	l.slog.Error(msg, convertArgs(args...)...)
}

// Fatal logs a fatal message and exits.
func (l *Logger) Fatal(msg string, args ...any) {
	l.slog.Error(msg, convertArgs(args...)...)
	os.Exit(1)
}

// DebugContext logs a debug message with context.
func (l *Logger) DebugContext(ctx context.Context, msg string, args ...any) {
	l.slog.DebugContext(ctx, msg, convertArgs(args...)...)
}

// InfoContext logs an info message with context.
func (l *Logger) InfoContext(ctx context.Context, msg string, args ...any) {
	l.slog.InfoContext(ctx, msg, convertArgs(args...)...)
}

// WarnContext logs a warning message with context.
func (l *Logger) WarnContext(ctx context.Context, msg string, args ...any) {
	l.slog.WarnContext(ctx, msg, convertArgs(args...)...)
}

// ErrorContext logs an error message with context.
func (l *Logger) ErrorContext(ctx context.Context, msg string, args ...any) {
	l.slog.ErrorContext(ctx, msg, convertArgs(args...)...)
}

// LogOperation logs the start and completion of an operation.
func (l *Logger) LogOperation(name string, fn func() error) error {
	start := time.Now()
	l.Info("operation started", "name", name, "timestamp", start)

	err := fn()
	duration := time.Since(start)

	if err != nil {
		l.WithError(err).Error("operation failed",
			"name", name,
			"duration", duration,
			"success", false,
		)

		return err
	}

	l.Info("operation completed",
		"name", name,
		"duration", duration,
		"success", true,
	)

	return nil
}

// LogOperationContext logs the start and completion of an operation with context.
func (l *Logger) LogOperationContext(ctx context.Context, name string, fn func(context.Context) error) error {
	start := time.Now()
	l.InfoContext(ctx, "operation started", "name", name, "timestamp", start)

	err := fn(ctx)
	duration := time.Since(start)

	if err != nil {
		l.WithError(err).ErrorContext(ctx, "operation failed",
			"name", name,
			"duration", duration,
			"success", false,
		)

		return err
	}

	l.InfoContext(ctx, "operation completed",
		"name", name,
		"duration", duration,
		"success", true,
	)

	return nil
}

// Trace logs function entry and exit for debugging.
func (l *Logger) Trace(args ...any) func() {
	pc, file, line, _ := runtime.Caller(1)
	funcName := runtime.FuncForPC(pc).Name()

	l.Debug("entering function",
		"function", funcName,
		"file", file,
		"line", line,
		"args", args,
	)

	start := time.Now()

	return func() {
		l.Debug("exiting function",
			"function", funcName,
			"duration", time.Since(start),
		)
	}
}

// convertArgs converts interface{} args to slog.Attr.
func convertArgs(args ...any) []any {
	if len(args)%2 != 0 {
		// If odd number of args, treat the last one as a value with empty key
		args = append([]any{"extra"}, args...)
	}

	result := make([]any, 0, len(args))
	for index := 0; index < len(args); index += 2 {
		key, ok := args[index].(string)
		if !ok {
			key = fmt.Sprintf("arg_%d", index/2)
		}

		result = append(result, key, args[index+1])
	}

	return result
}

// convertAttrsToAny converts slog.Attr slice to []any.
func convertAttrsToAny(attrs []slog.Attr) []any {
	result := make([]any, len(attrs))
	for i, attr := range attrs {
		result[i] = attr
	}

	return result
}

// SetGlobalLogger sets the global logger instance.
var globalLogger *Logger

//nolint:gochecknoinits // Required for global logger initialization
func init() {
	globalLogger = NewDefault()
}

// SetGlobal sets the global logger.
func SetGlobal(logger *Logger) {
	globalLogger = logger
}

// Global returns the global logger.
func Global() *Logger {
	return globalLogger
}

// Package-level convenience functions that use the global logger

// Debug logs a debug message using the global logger.
func Debug(msg string, args ...any) {
	globalLogger.Debug(msg, args...)
}

// Info logs an info message using the global logger.
func Info(msg string, args ...any) {
	globalLogger.Info(msg, args...)
}

// Warn logs a warning message using the global logger.
func Warn(msg string, args ...any) {
	globalLogger.Warn(msg, args...)
}

// Error logs an error message using the global logger.
func Error(msg string, args ...any) {
	globalLogger.Error(msg, args...)
}

// Fatal logs a fatal message and exits using the global logger.
func Fatal(msg string, args ...any) {
	globalLogger.Fatal(msg, args...)
}

// WithError returns a logger with error information using the global logger.
func WithError(err error) *Logger {
	return globalLogger.WithError(err)
}

// WithFields returns a logger with structured fields using the global logger.
func WithFields(fields map[string]any) *Logger {
	return globalLogger.WithFields(fields)
}
