// Package logger provides logging functionality for the yap application.
package logger

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"regexp"
	"strings"
	"sync"

	"golang.org/x/term"

	"github.com/M0Rf30/yap/v2/pkg/color"
)

// ansiEscape matches ANSI escape sequences for stripping when measuring visible width.
var ansiEscape = regexp.MustCompile(`\x1b\[[0-9;]*m`)

// visibleLen returns the number of visible (non-ANSI) bytes in s.
func visibleLen(s string) int {
	return len(ansiEscape.ReplaceAllString(s, ""))
}

// termWidth returns the current terminal width, defaulting to 120 if unavailable.
func termWidth() int {
	w, _, err := term.GetSize(int(os.Stderr.Fd()))
	if err != nil || w <= 0 {
		return 120
	}

	return w
}

// KeyColorMap defines the color for each known key category.
var KeyColorMap = map[string]func(string) string{
	// Package and artifact identifiers - Green
	"package":        color.Green,
	"distro":         color.Green,
	"release":        color.Green,
	"pkgver":         color.Green,
	"pkgrel":         color.Green,
	"source":         color.Green,
	"total_packages": color.Green,
	"total_batches":  color.Green,
	"count":          color.Green,
	// Numbers, counts, and progress - Blue
	"batch_number":     color.Blue,
	"batch_size":       color.Blue,
	"packages":         color.Blue,
	"parallel_workers": color.Blue,
	"progress":         color.Blue,
	"duration":         color.Blue,
	"timestamp":        color.Blue,
	// Paths and commands - Light Blue
	"path":    color.HiBlue,
	"command": color.HiBlue,
	"dir":     color.HiBlue,
	"args":    color.HiBlue,
	// Status and state - Cyan
	"name":      color.Cyan,
	"success":   color.Cyan,
	"operation": color.Cyan,
}

// MultiPrinterImpl provides concurrent-safe output handling.
// Writer is the destination for all log output; Start is a no-op kept for
// API compatibility with pkg/shell/exec.go.
type MultiPrinterImpl struct {
	Writer io.Writer
	mu     sync.Mutex
}

// Start returns the writer for concurrent output (no-op; kept for API compat).
func (m *MultiPrinterImpl) Start() (io.Writer, error) {
	return m.Writer, nil
}

var (
	// MultiPrinter is the default multiprinter for concurrent logging.
	MultiPrinter = &MultiPrinterImpl{Writer: os.Stderr}

	// Logger is the global YapLogger instance.
	Logger = &YapLogger{}

	colorDisabled  = false
	verboseEnabled = false

	// slogHandler is lazily initialised on first use via getHandler().
	slogHandler slog.Handler //nolint:gochecknoglobals
)

// getHandler returns the slog handler, initialising it on first call.
// Using a function avoids the need for an init().
func getHandler() slog.Handler {
	if slogHandler == nil {
		slogHandler = &CustomHandler{
			writer: MultiPrinter.Writer,
			mu:     &MultiPrinter.mu,
		}
	}

	return slogHandler
}

// CustomHandler implements slog.Handler for yap's custom log format.
type CustomHandler struct {
	writer io.Writer
	mu     *sync.Mutex
}

// Handle formats and writes a log record.
//
// When all key-value pairs fit on one line within the terminal width they are
// appended inline. When the line would overflow, each pair is rendered on its
// own indented line with tree connectors (├ / └).
//
//nolint:gocritic // hugeParam: slog.Record is required by the slog.Handler interface; cannot pass by pointer
func (h *CustomHandler) Handle(_ context.Context, record slog.Record) error {
	h.mu.Lock()
	defer h.mu.Unlock()

	timestamp := color.Gray(record.Time.Format("2006-01-02 15:04:05"))

	var levelFormatted string

	switch record.Level {
	case slog.LevelInfo:
		levelFormatted = color.BoldGreen("INFO ")
	case slog.LevelWarn:
		levelFormatted = color.BoldYellow("WARN ")
	case slog.LevelError:
		levelFormatted = color.Red("ERROR")
	default:
		levelFormatted = color.BoldBlue("DEBUG")
	}

	prefix := color.Bracket("yap")
	header := fmt.Sprintf("%s %s %s %s", timestamp, levelFormatted, prefix, record.Message)

	// Collect formatted key=value pairs.
	type kv struct{ colored, plain string }

	var pairs []kv

	record.Attrs(func(a slog.Attr) bool {
		keyColor, ok := KeyColorMap[a.Key]
		if !ok {
			keyColor = color.White
		}

		// For inline fitting check, collapse newlines to a single space.
		rawVal := a.Value.String()
		inlineVal := strings.ReplaceAll(rawVal, "\n", " ")
		colored := keyColor(a.Key+": ") + rawVal
		plain := a.Key + ": " + inlineVal
		pairs = append(pairs, kv{colored, plain})

		return true
	})

	if len(pairs) == 0 {
		_, err := fmt.Fprintln(h.writer, header)

		return err
	}

	// Try inline: "header  k1=v1 k2=v2"
	inlineParts := make([]string, len(pairs))
	for i, p := range pairs {
		inlineParts[i] = p.plain
	}

	inlineLine := header + " " + strings.Join(inlineParts, " ")

	if visibleLen(inlineLine) <= termWidth() {
		// Fits — render with colors inline.
		coloredParts := make([]string, len(pairs))
		for i, p := range pairs {
			coloredParts[i] = p.colored
		}

		_, err := fmt.Fprintf(h.writer, "%s %s\n", header, strings.Join(coloredParts, " "))

		return err
	}

	// Doesn't fit — tree layout.
	// Indent = len("2006-01-02 15:04:05") + 3 = 22 spaces, aligning under the message.
	const treeIndent = 22

	indent := strings.Repeat(" ", treeIndent)

	var sb strings.Builder

	sb.WriteString(header)
	sb.WriteByte('\n')

	for i, p := range pairs {
		var connector string
		if i < len(pairs)-1 {
			connector = "├"
		} else {
			connector = "└"
		}

		lines := strings.Split(p.colored, "\n")

		sb.WriteString(indent)
		sb.WriteString(color.Gray(connector + " "))
		sb.WriteString(lines[0])
		sb.WriteByte('\n')

		// Continuation lines for multi-line values, indented to align under the value.
		contIndent := indent + "  "

		for _, contLine := range lines[1:] {
			if strings.TrimSpace(contLine) == "" {
				continue
			}

			sb.WriteString(contIndent)
			sb.WriteString(contLine)
			sb.WriteByte('\n')
		}
	}

	_, err := h.writer.Write([]byte(sb.String()))

	return err
}

// WithAttrs returns a new handler with the given attributes (no-op for simplicity).
func (h *CustomHandler) WithAttrs(_ []slog.Attr) slog.Handler { return h }

// WithGroup returns a new handler with the given group (no-op for simplicity).
func (h *CustomHandler) WithGroup(_ string) slog.Handler { return h }

// Enabled reports whether the handler handles records at the given level.
func (h *CustomHandler) Enabled(_ context.Context, level slog.Level) bool {
	if verboseEnabled {
		return level >= slog.LevelDebug
	}

	return level >= slog.LevelInfo
}

// YapLogger provides yap-specific logging functionality.
type YapLogger struct{}

// Info logs an informational message.
func (y *YapLogger) Info(msg string, args ...any) {
	logWithLevel(slog.LevelInfo, msg, args...)
}

// Debug logs a debug message (no-op unless verbose is enabled).
func (y *YapLogger) Debug(msg string, args ...any) {
	if !verboseEnabled {
		return
	}

	logWithLevel(slog.LevelDebug, msg, args...)
}

// Warn logs a warning message.
func (y *YapLogger) Warn(msg string, args ...any) {
	logWithLevel(slog.LevelWarn, msg, args...)
}

// Error logs an error message.
func (y *YapLogger) Error(msg string, args ...any) {
	logWithLevel(slog.LevelError, msg, args...)
}

// Tips logs a tip message with TIPS prefix.
func (y *YapLogger) Tips(msg string, _ ...any) {
	MultiPrinter.mu.Lock()
	defer MultiPrinter.mu.Unlock()

	line := fmt.Sprintf("%s %s %s\n", color.Magenta("TIPS"), color.Bracket("yap"), msg)
	_, _ = MultiPrinter.Writer.Write([]byte(line))
}

// Fatal logs a fatal message and exits with code 1.
func (y *YapLogger) Fatal(msg string, args ...any) {
	logWithLevel(slog.LevelError, msg, args...)
	os.Exit(1)
}

// logWithLevel logs a message at the given level with key-value pairs.
func logWithLevel(level slog.Level, msg string, args ...any) {
	l := slog.New(getHandler())

	var attrs []slog.Attr

	for i := 0; i < len(args)-1; i += 2 {
		key := fmt.Sprintf("%v", args[i])
		attrs = append(attrs, slog.Any(key, args[i+1]))
	}

	if len(attrs) > 0 {
		l.LogAttrs(context.TODO(), level, msg, attrs...)
	} else {
		l.Log(context.TODO(), level, msg)
	}
}

// SetVerbose configures the logger verbosity level.
func SetVerbose(verbose bool) {
	verboseEnabled = verbose
}

// SetWriter redirects the underlying logger's output to the given writer.
// Not goroutine-safe; call before spinning up any concurrent loggers.
func SetWriter(w io.Writer) {
	MultiPrinter.Writer = w
	// Reset handler so it picks up the new writer on next use.
	slogHandler = nil
}

// IsVerboseEnabled returns true if verbose logging is enabled.
func IsVerboseEnabled() bool {
	return verboseEnabled
}

// IsColorDisabled checks if color output is disabled.
func IsColorDisabled() bool {
	if colorDisabled {
		return true
	}

	return os.Getenv("NO_COLOR") != ""
}

// SetColorDisabled enables or disables color output.
func SetColorDisabled(disabled bool) {
	colorDisabled = disabled

	if disabled {
		color.Disable()
	} else {
		color.Enable()
	}
}

// Debug logs a debug message using the global logger.
func Debug(msg string, args ...any) { Logger.Debug(msg, args...) }

// Info logs an informational message using the global logger.
func Info(msg string, args ...any) { Logger.Info(msg, args...) }

// Warn logs a warning message using the global logger.
func Warn(msg string, args ...any) { Logger.Warn(msg, args...) }

// Error logs an error message using the global logger.
func Error(msg string, args ...any) { Logger.Error(msg, args...) }

// Fatal logs a fatal message using the global logger and exits.
func Fatal(msg string, args ...any) { Logger.Fatal(msg, args...) }

// Tips logs a tip message using the global logger.
func Tips(msg string, args ...any) { Logger.Tips(msg, args...) }
