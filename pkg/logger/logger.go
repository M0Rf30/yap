// Package logger provides logging functionality for the yap application.
package logger

import (
	"context"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/pterm/pterm"
)

const (
	timestampFormat = "2006-01-02T15:04:05.000Z07:00"
	logLevelInfo    = "INFO "
)

// LogLevel represents the severity level of log messages for compatibility.
type LogLevel int

const (
	// LevelDebug represents the debug log level.
	LevelDebug LogLevel = iota
	// LevelInfo represents the info log level.
	LevelInfo
	// LevelWarn represents the warning log level.
	LevelWarn
	// LevelError represents the error log level.
	LevelError
	// LevelFatal represents the fatal log level.
	LevelFatal
)

// Config holds configuration for the logger for compatibility.
type Config struct {
	Level      LogLevel
	Format     string
	Output     io.Writer
	TimeFormat string
	AddSource  bool
}

// DefaultConfig returns a default logger configuration.
func DefaultConfig() *Config {
	return &Config{
		Level:      LevelInfo,
		Format:     "text",
		Output:     os.Stdout,
		TimeFormat: time.RFC3339,
		AddSource:  false,
	}
}

var (
	// MultiPrinter is the default multiprinter for concurrent logging.
	MultiPrinter = pterm.DefaultMultiPrinter
	baseLogger   = pterm.DefaultLogger.WithLevel(pterm.LogLevelInfo).WithWriter(MultiPrinter.Writer)
	// Logger is the global YapLogger instance.
	Logger        = &YapLogger{baseLogger}
	globalLogger  = &YapLogger{baseLogger}
	colorDisabled = false
)

// YapLogger wraps pterm.Logger with yap-specific formatting.
type YapLogger struct {
	*pterm.Logger
}

// Info logs an informational message with yap prefix.
func (y *YapLogger) Info(msg string, args ...[]pterm.LoggerArgument) {
	y.Logger.Info("[yap] "+msg, args...)
}

// Tips logs a tip message with custom formatting and yap prefix.
func (y *YapLogger) Tips(msg string, _ ...[]pterm.LoggerArgument) {
	timestamp := time.Now().Format(timestampFormat)

	var logMsg string
	if IsColorDisabled() {
		logMsg = fmt.Sprintf("%s %s  [yap] %s", timestamp, logLevelInfo, msg)
	} else {
		logMsg = fmt.Sprintf("%s %s  %s %s",
			pterm.FgGray.Sprint(timestamp),
			pterm.FgCyan.Sprint(logLevelInfo),
			pterm.FgBlue.Sprint("[yap]"),
			msg,
		)
	}

	pterm.Println(logMsg)
}

// Warn logs a warning message with yap prefix.
func (y *YapLogger) Warn(msg string, args ...[]pterm.LoggerArgument) {
	y.Logger.Warn("[yap] "+msg, args...)
}

func (y *YapLogger) Error(msg string, args ...[]pterm.LoggerArgument) {
	y.Logger.Error("[yap] "+msg, args...)
}

// Debug logs a debug message with yap prefix.
func (y *YapLogger) Debug(msg string, args ...[]pterm.LoggerArgument) {
	y.Logger.Debug("[yap] "+msg, args...)
}

// Fatal logs a fatal message with yap prefix and exits.
func (y *YapLogger) Fatal(msg string, args ...[]pterm.LoggerArgument) {
	y.Logger.Fatal("[yap] "+msg, args...)
}

// Args converts arguments to pterm logger arguments.
func (y *YapLogger) Args(args ...any) []pterm.LoggerArgument {
	return y.Logger.Args(args...)
}

// SetVerbose configures the logger verbosity level.
func SetVerbose(verbose bool) {
	var level pterm.LogLevel
	if verbose {
		level = pterm.LogLevelDebug
	} else {
		level = pterm.LogLevelInfo
	}

	baseLogger = pterm.DefaultLogger.WithLevel(level).WithWriter(MultiPrinter.Writer)
	Logger = &YapLogger{baseLogger}
	globalLogger = &YapLogger{baseLogger}
}

// ComponentLogger wraps a logger with component-specific formatting.
type ComponentLogger struct {
	*pterm.Logger
	Component string
}

// WithComponent creates a new ComponentLogger with the specified component name.
func WithComponent(component string) *ComponentLogger {
	return &ComponentLogger{
		Logger:    baseLogger,
		Component: component,
	}
}

// ServiceLogger creates a ComponentLogger for the yap service.
func ServiceLogger() *ComponentLogger {
	return WithComponent("yap")
}

// Info logs an informational message with component prefix.
func (cl *ComponentLogger) Info(msg string, args ...[]pterm.LoggerArgument) {
	prefixedMsg := fmt.Sprintf("[%s] %s", cl.Component, msg)
	cl.Logger.Info(prefixedMsg, args...)
}

// Warn logs a warning message with component prefix.
func (cl *ComponentLogger) Warn(msg string, args ...[]pterm.LoggerArgument) {
	prefixedMsg := fmt.Sprintf("[%s] %s", cl.Component, msg)
	cl.Logger.Warn(prefixedMsg, args...)
}

func (cl *ComponentLogger) Error(msg string, args ...[]pterm.LoggerArgument) {
	prefixedMsg := fmt.Sprintf("[%s] %s", cl.Component, msg)
	cl.Logger.Error(prefixedMsg, args...)
}

// Fatal logs a fatal message with component prefix and exits.
func (cl *ComponentLogger) Fatal(msg string, args ...[]pterm.LoggerArgument) {
	prefixedMsg := fmt.Sprintf("[%s] %s", cl.Component, msg)
	cl.Logger.Fatal(prefixedMsg, args...)
}

// Debug logs a debug message with component prefix.
func (cl *ComponentLogger) Debug(msg string, args ...[]pterm.LoggerArgument) {
	prefixedMsg := fmt.Sprintf("[%s] %s", cl.Component, msg)
	cl.Logger.Debug(prefixedMsg, args...)
}

// Args converts arguments to pterm logger arguments.
func (cl *ComponentLogger) Args(args ...any) []pterm.LoggerArgument {
	return cl.Logger.Args(args...)
}

// IsColorDisabled checks if color output is disabled.
func IsColorDisabled() bool {
	// Check programmatically set state first
	if colorDisabled {
		return true
	}

	// Check environment variables
	colorTerm := os.Getenv("COLORTERM")
	term := os.Getenv("TERM")
	noColor := os.Getenv("NO_COLOR")

	if noColor != "" {
		return true
	}

	if colorTerm == "" && term == "" {
		return true
	}

	return false
}

// SetColorDisabled enables or disables color output.
func SetColorDisabled(disabled bool) {
	colorDisabled = disabled

	// Configure pterm color settings
	if disabled {
		pterm.DisableColor()
	} else {
		pterm.EnableColor()
	}
}

// CompatLogger provides compatibility with the old slog-based logger interface
type CompatLogger struct {
	yapLogger *YapLogger
	config    *Config
}

// New creates a logger with the given configuration for compatibility.
func New(config *Config) *CompatLogger {
	if config == nil {
		config = DefaultConfig()
	}

	return &CompatLogger{
		yapLogger: globalLogger,
		config:    config,
	}
}

// NewDefault creates a logger with default configuration for compatibility.
func NewDefault() *CompatLogger {
	return New(DefaultConfig())
}

// Debug logs a debug message for compatibility.
func (l *CompatLogger) Debug(msg string, args ...any) {
	l.yapLogger.Debug(msg, convertArgsToLoggerArgs(args...)...)
}

// Info logs an informational message for compatibility.
func (l *CompatLogger) Info(msg string, args ...any) {
	l.yapLogger.Info(msg, convertArgsToLoggerArgs(args...)...)
}

// Warn logs a warning message for compatibility.
func (l *CompatLogger) Warn(msg string, args ...any) {
	l.yapLogger.Warn(msg, convertArgsToLoggerArgs(args...)...)
}

func (l *CompatLogger) Error(msg string, args ...any) {
	l.yapLogger.Error(msg, convertArgsToLoggerArgs(args...)...)
}

// Fatal logs a fatal message for compatibility and exits.
func (l *CompatLogger) Fatal(msg string, args ...any) {
	l.yapLogger.Fatal(msg, convertArgsToLoggerArgs(args...)...)
}

// WithError returns a logger with error context for compatibility.
func (l *CompatLogger) WithError(err error) *CompatLogger {
	// For compatibility, just return the same logger
	return l
}

// WithFields returns a logger with field context for compatibility.
func (l *CompatLogger) WithFields(fields map[string]any) *CompatLogger {
	// For compatibility, just return the same logger
	return l
}

// DebugContext logs a debug message with context for compatibility.
func (l *CompatLogger) DebugContext(ctx context.Context, msg string, args ...any) {
	l.Debug(msg, args...)
}

// InfoContext logs an informational message with context for compatibility.
func (l *CompatLogger) InfoContext(ctx context.Context, msg string, args ...any) {
	l.Info(msg, args...)
}

// WarnContext logs a warning message with context for compatibility.
func (l *CompatLogger) WarnContext(ctx context.Context, msg string, args ...any) {
	l.Warn(msg, args...)
}

// ErrorContext logs an error message with context for compatibility.
func (l *CompatLogger) ErrorContext(ctx context.Context, msg string, args ...any) {
	l.Error(msg, args...)
}

// LogOperation logs an operation with timing and error handling.
func (l *CompatLogger) LogOperation(name string, fn func() error) error {
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

// SetGlobal sets the global logger instance for compatibility.
func SetGlobal(logger *CompatLogger) {
	if logger != nil {
		globalLogger = logger.yapLogger
	}
}

// Global returns the global logger instance for compatibility.
func Global() *CompatLogger {
	return &CompatLogger{
		yapLogger: globalLogger,
		config:    DefaultConfig(),
	}
}

// Debug logs a debug message using the global logger.
func Debug(msg string, args ...any) {
	globalLogger.Debug(msg, convertArgsToLoggerArgs(args...)...)
}

// Info logs an informational message using the global logger.
func Info(msg string, args ...any) {
	globalLogger.Info(msg, convertArgsToLoggerArgs(args...)...)
}

// Warn logs a warning message using the global logger.
func Warn(msg string, args ...any) {
	globalLogger.Warn(msg, convertArgsToLoggerArgs(args...)...)
}

// Error logs an error message using the global logger.
func Error(msg string, args ...any) {
	globalLogger.Error(msg, convertArgsToLoggerArgs(args...)...)
}

// Fatal logs a fatal message using the global logger and exits.
func Fatal(msg string, args ...any) {
	globalLogger.Fatal(msg, convertArgsToLoggerArgs(args...)...)
}

// Tips logs a tip message using the global logger.
func Tips(msg string, args ...any) {
	globalLogger.Tips(msg, convertArgsToLoggerArgs(args...)...)
}

// WithError returns a global logger with error context.
func WithError(err error) *CompatLogger {
	return Global().WithError(err)
}

// WithFields returns a global logger with field context.
func WithFields(fields map[string]any) *CompatLogger {
	return Global().WithFields(fields)
}

// Helper function to convert args to pterm logger args
func convertArgsToLoggerArgs(args ...any) [][]pterm.LoggerArgument {
	if len(args) == 0 {
		return nil
	}

	// Convert to pterm logger arguments
	return [][]pterm.LoggerArgument{globalLogger.Args(args...)}
}
