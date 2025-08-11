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

// argsToLoggerArgs converts arguments to pterm logger arguments.
func argsToLoggerArgs(args ...any) []pterm.LoggerArgument {
	if len(args) == 0 {
		return nil
	}

	var loggerArgs []pterm.LoggerArgument

	for i := 0; i < len(args)-1; i += 2 {
		key := fmt.Sprintf("%v", args[i])
		value := args[i+1]
		loggerArgs = append(loggerArgs, pterm.LoggerArgument{
			Key:   key,
			Value: value,
		})
	}

	return loggerArgs
}

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
	// ptermLogger is the underlying pterm logger configured for yap with uniform color scheme
	ptermLogger = pterm.DefaultLogger.
			WithLevel(pterm.LogLevelTrace).
			WithWriter(MultiPrinter.Writer).
			WithCaller(false).
			WithTime(true).
			WithKeyStyles(map[string]pterm.Style{
			// Package and artifact identifiers - Green
			"package":        *pterm.NewStyle(pterm.FgGreen),
			"distro":         *pterm.NewStyle(pterm.FgGreen),
			"release":        *pterm.NewStyle(pterm.FgGreen),
			"pkgver":         *pterm.NewStyle(pterm.FgGreen),
			"pkgrel":         *pterm.NewStyle(pterm.FgGreen),
			"source":         *pterm.NewStyle(pterm.FgGreen),
			"total_packages": *pterm.NewStyle(pterm.FgGreen),
			"total_batches":  *pterm.NewStyle(pterm.FgGreen),
			"count":          *pterm.NewStyle(pterm.FgGreen),
			// Numbers, counts, and progress - Blue
			"batch_number":     *pterm.NewStyle(pterm.FgBlue),
			"batch_size":       *pterm.NewStyle(pterm.FgBlue),
			"packages":         *pterm.NewStyle(pterm.FgBlue),
			"parallel_workers": *pterm.NewStyle(pterm.FgBlue),
			"progress":         *pterm.NewStyle(pterm.FgBlue),
			"duration":         *pterm.NewStyle(pterm.FgBlue),
			"timestamp":        *pterm.NewStyle(pterm.FgBlue),
			// Paths and commands - Light Blue
			"path":    *pterm.NewStyle(pterm.FgLightBlue),
			"command": *pterm.NewStyle(pterm.FgLightBlue),
			"dir":     *pterm.NewStyle(pterm.FgLightBlue),
			"args":    *pterm.NewStyle(pterm.FgLightBlue),
			// Status and state - Cyan
			"name":      *pterm.NewStyle(pterm.FgCyan),
			"success":   *pterm.NewStyle(pterm.FgCyan),
			"operation": *pterm.NewStyle(pterm.FgCyan),
		})
	// Logger is the global YapLogger instance.
	Logger         = &YapLogger{ptermLogger: ptermLogger}
	colorDisabled  = false
	verboseEnabled = false
)

// YapLogger provides yap-specific logging functionality.
type YapLogger struct {
	ptermLogger *pterm.Logger
}

// Info logs an informational message with yap prefix.
func (y *YapLogger) Info(msg string, args ...[]pterm.LoggerArgument) {
	prefix := fmt.Sprintf("[yap] %s", msg)
	if len(args) > 0 && len(args[0]) > 0 {
		y.ptermLogger.Info(prefix, args...)
	} else {
		y.ptermLogger.Info(prefix)
	}
}

// Debug logs a debug message with yap prefix.
func (y *YapLogger) Debug(msg string, args ...[]pterm.LoggerArgument) {
	if !verboseEnabled {
		return
	}

	prefix := fmt.Sprintf("[yap] %s", msg)
	if len(args) > 0 && len(args[0]) > 0 {
		y.ptermLogger.Debug(prefix, args...)
	} else {
		y.ptermLogger.Debug(prefix)
	}
}

// Warn logs a warning message with yap prefix.
func (y *YapLogger) Warn(msg string, args ...[]pterm.LoggerArgument) {
	prefix := fmt.Sprintf("[yap] %s", msg)
	if len(args) > 0 && len(args[0]) > 0 {
		y.ptermLogger.Warn(prefix, args...)
	} else {
		y.ptermLogger.Warn(prefix)
	}
}

// Error logs an error message with yap prefix.
func (y *YapLogger) Error(msg string, args ...[]pterm.LoggerArgument) {
	prefix := fmt.Sprintf("[yap] %s", msg)
	if len(args) > 0 && len(args[0]) > 0 {
		y.ptermLogger.Error(prefix, args...)
	} else {
		y.ptermLogger.Error(prefix)
	}
}

// WithKeyStyles sets custom styles for specific argument keys
func (y *YapLogger) WithKeyStyles(styles map[string]pterm.Style) *YapLogger {
	return &YapLogger{
		ptermLogger: y.ptermLogger.WithKeyStyles(styles),
	}
}

// AppendKeyStyle adds a style for a specific argument key
func (y *YapLogger) AppendKeyStyle(key string, style pterm.Style) *YapLogger {
	newLogger := *y
	newLogger.ptermLogger = y.ptermLogger.AppendKeyStyle(key, style)

	return &newLogger
}

// Tips logs a tip message with custom formatting and yap prefix.
func (y *YapLogger) Tips(msg string, args ...[]pterm.LoggerArgument) {
	// Use pterm's info printer with custom styling for tips
	pterm.Info.WithPrefix(pterm.Prefix{
		Text:  "TIPS",
		Style: pterm.NewStyle(pterm.FgMagenta),
	}).Println(fmt.Sprintf("[yap] %s", msg))
}

// Fatal logs a fatal message with yap prefix and exits.
func (y *YapLogger) Fatal(msg string, args ...[]pterm.LoggerArgument) {
	prefix := fmt.Sprintf("[yap] %s", msg)
	if len(args) > 0 && len(args[0]) > 0 {
		y.ptermLogger.Fatal(prefix, args...)
	} else {
		y.ptermLogger.Fatal(prefix)
	}
}

// Args converts arguments to pterm logger arguments.
func (y *YapLogger) Args(args ...any) []pterm.LoggerArgument {
	return argsToLoggerArgs(args...)
}

// SetVerbose configures the logger verbosity level.
func SetVerbose(verbose bool) {
	verboseEnabled = verbose
	if verbose {
		ptermLogger = ptermLogger.WithLevel(pterm.LogLevelTrace)
	} else {
		ptermLogger = ptermLogger.WithLevel(pterm.LogLevelInfo)
	}

	Logger.ptermLogger = ptermLogger
}

// ComponentLogger wraps a logger with component-specific formatting.
type ComponentLogger struct {
	Component   string
	ptermLogger *pterm.Logger
}

// WithComponent creates a new ComponentLogger with the specified component name.
func WithComponent(component string) *ComponentLogger {
	return &ComponentLogger{
		Component:   component,
		ptermLogger: ptermLogger,
	}
}

// ServiceLogger creates a ComponentLogger for the yap service.
func ServiceLogger() *ComponentLogger {
	return WithComponent("yap")
}

// Info logs an informational message with component prefix.
func (cl *ComponentLogger) Info(msg string, args ...[]pterm.LoggerArgument) {
	prefix := fmt.Sprintf("[%s] %s", cl.Component, msg)
	if len(args) > 0 && len(args[0]) > 0 {
		cl.ptermLogger.Info(prefix, args...)
	} else {
		cl.ptermLogger.Info(prefix)
	}
}

// Warn logs a warning message with component prefix.
func (cl *ComponentLogger) Warn(msg string, args ...[]pterm.LoggerArgument) {
	prefix := fmt.Sprintf("[%s] %s", cl.Component, msg)
	if len(args) > 0 && len(args[0]) > 0 {
		cl.ptermLogger.Warn(prefix, args...)
	} else {
		cl.ptermLogger.Warn(prefix)
	}
}

// Error logs an error message with component prefix.
func (cl *ComponentLogger) Error(msg string, args ...[]pterm.LoggerArgument) {
	prefix := fmt.Sprintf("[%s] %s", cl.Component, msg)
	if len(args) > 0 && len(args[0]) > 0 {
		cl.ptermLogger.Error(prefix, args...)
	} else {
		cl.ptermLogger.Error(prefix)
	}
}

// Fatal logs a fatal message with component prefix and exits.
func (cl *ComponentLogger) Fatal(msg string, args ...[]pterm.LoggerArgument) {
	prefix := fmt.Sprintf("[%s] %s", cl.Component, msg)
	if len(args) > 0 && len(args[0]) > 0 {
		cl.ptermLogger.Fatal(prefix, args...)
	} else {
		cl.ptermLogger.Fatal(prefix)
	}
}

// Debug logs a debug message with component prefix.
func (cl *ComponentLogger) Debug(msg string, args ...[]pterm.LoggerArgument) {
	if !verboseEnabled {
		return
	}

	prefix := fmt.Sprintf("[%s] %s", cl.Component, msg)
	if len(args) > 0 && len(args[0]) > 0 {
		cl.ptermLogger.Debug(prefix, args...)
	} else {
		cl.ptermLogger.Debug(prefix)
	}
}

// WithKeyStyles sets custom styles for specific argument keys for ComponentLogger
func (cl *ComponentLogger) WithKeyStyles(styles map[string]pterm.Style) *ComponentLogger {
	return &ComponentLogger{
		Component:   cl.Component,
		ptermLogger: cl.ptermLogger.WithKeyStyles(styles),
	}
}

// AppendKeyStyle adds a style for a specific argument key for ComponentLogger
func (cl *ComponentLogger) AppendKeyStyle(key string, style pterm.Style) *ComponentLogger {
	newLogger := *cl
	newLogger.ptermLogger = cl.ptermLogger.AppendKeyStyle(key, style)

	return &newLogger
}

// Args converts arguments to pterm logger arguments.
func (cl *ComponentLogger) Args(args ...any) []pterm.LoggerArgument {
	return argsToLoggerArgs(args...)
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
		yapLogger: Logger,
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

// Error logs an error message for compatibility.
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
		Logger = logger.yapLogger
	}
}

// Global returns the global logger instance for compatibility.
func Global() *CompatLogger {
	return &CompatLogger{
		yapLogger: Logger,
		config:    DefaultConfig(),
	}
}

// Debug logs a debug message using the global logger.
func Debug(msg string, args ...any) {
	Logger.Debug(msg, convertArgsToLoggerArgs(args...)...)
}

// Info logs an informational message using the global logger.
func Info(msg string, args ...any) {
	Logger.Info(msg, convertArgsToLoggerArgs(args...)...)
}

// Warn logs a warning message using the global logger.
func Warn(msg string, args ...any) {
	Logger.Warn(msg, convertArgsToLoggerArgs(args...)...)
}

// Error logs an error message using the global logger.
func Error(msg string, args ...any) {
	Logger.Error(msg, convertArgsToLoggerArgs(args...)...)
}

// Fatal logs a fatal message using the global logger and exits.
func Fatal(msg string, args ...any) {
	Logger.Fatal(msg, convertArgsToLoggerArgs(args...)...)
}

// Tips logs a tip message using the global logger.
func Tips(msg string, args ...any) {
	Logger.Tips(msg, convertArgsToLoggerArgs(args...)...)
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
	return [][]pterm.LoggerArgument{Logger.Args(args...)}
}
