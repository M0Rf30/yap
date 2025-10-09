// Package logger provides logging functionality for the yap application.
package logger

import (
	"fmt"
	"os"

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

// formatYapPrefix creates a formatted prefix with white brackets and yellow yap text.
func formatYapPrefix(msg string) string {
	return fmt.Sprintf("%s%s%s %s",
		pterm.NewStyle(pterm.FgWhite).Sprint("["),
		pterm.NewStyle(pterm.FgYellow).Sprint("yap"),
		pterm.NewStyle(pterm.FgWhite).Sprint("]"),
		msg)
}

// Info logs an informational message with yap prefix.
func (y *YapLogger) Info(msg string, args ...[]pterm.LoggerArgument) {
	prefix := formatYapPrefix(msg)
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

	prefix := formatYapPrefix(msg)
	if len(args) > 0 && len(args[0]) > 0 {
		y.ptermLogger.Debug(prefix, args...)
	} else {
		y.ptermLogger.Debug(prefix)
	}
}

// Warn logs a warning message with yap prefix.
func (y *YapLogger) Warn(msg string, args ...[]pterm.LoggerArgument) {
	prefix := formatYapPrefix(msg)
	if len(args) > 0 && len(args[0]) > 0 {
		y.ptermLogger.Warn(prefix, args...)
	} else {
		y.ptermLogger.Warn(prefix)
	}
}

// Error logs an error message with yap prefix.
func (y *YapLogger) Error(msg string, args ...[]pterm.LoggerArgument) {
	prefix := formatYapPrefix(msg)
	if len(args) > 0 && len(args[0]) > 0 {
		y.ptermLogger.Error(prefix, args...)
	} else {
		y.ptermLogger.Error(prefix)
	}
}

// Tips logs a tip message with custom formatting and yap prefix.
func (y *YapLogger) Tips(msg string, args ...[]pterm.LoggerArgument) {
	// Use pterm's info printer with custom styling for tips
	pterm.Info.WithPrefix(pterm.Prefix{
		Text:  "TIPS",
		Style: pterm.NewStyle(pterm.FgMagenta),
	}).Println(formatYapPrefix(msg))
}

// Fatal logs a fatal message with yap prefix and exits.
func (y *YapLogger) Fatal(msg string, args ...[]pterm.LoggerArgument) {
	prefix := formatYapPrefix(msg)
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

// IsVerboseEnabled returns true if verbose logging is enabled.
func IsVerboseEnabled() bool {
	return verboseEnabled
}

// IsColorDisabled checks if color output is disabled.
func IsColorDisabled() bool {
	// Check programmatically set state first
	if colorDisabled {
		return true
	}

	// Check environment variables
	noColor := os.Getenv("NO_COLOR")

	return noColor != ""
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

// Helper function to convert args to pterm logger args
func convertArgsToLoggerArgs(args ...any) [][]pterm.LoggerArgument {
	if len(args) == 0 {
		return nil
	}

	// Convert to pterm logger arguments
	return [][]pterm.LoggerArgument{Logger.Args(args...)}
}
