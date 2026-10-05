package logger

import (
	"bytes"
	"io"
	"os"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestYapLoggerInfo(t *testing.T) {
	old := MultiPrinter.Writer
	defer func() { MultiPrinter.Writer = old }()

	MultiPrinter.Writer = io.Discard

	assert.NotPanics(t, func() {
		Logger.Info("test message")
		Logger.Info("test message with args", "key", "value")
	})
}

func TestYapLoggerDebug(t *testing.T) {
	old := MultiPrinter.Writer
	oldVerbose := verboseEnabled.Load()

	defer func() {
		MultiPrinter.Writer = old

		verboseEnabled.Store(oldVerbose)
	}()

	MultiPrinter.Writer = io.Discard

	verboseEnabled.Store(false)

	assert.NotPanics(t, func() {
		Logger.Debug("test debug message")
		Logger.Debug("test debug message with args", "key", "value")
	})

	verboseEnabled.Store(true)

	assert.NotPanics(t, func() {
		Logger.Debug("test debug message with verbose enabled")
		Logger.Debug("test debug message with args and verbose", "key", "value")
	})
}

func TestYapLoggerWarn(t *testing.T) {
	old := MultiPrinter.Writer
	defer func() { MultiPrinter.Writer = old }()

	MultiPrinter.Writer = io.Discard

	assert.NotPanics(t, func() {
		Logger.Warn("test warning message")
		Logger.Warn("test warning message with args", "key", "value")
	})
}

func TestYapLoggerError(t *testing.T) {
	old := MultiPrinter.Writer
	defer func() { MultiPrinter.Writer = old }()

	MultiPrinter.Writer = io.Discard

	assert.NotPanics(t, func() {
		Logger.Error("test error message")
		Logger.Error("test error message with args", "key", "value")
	})
}

func TestYapLoggerFatal(t *testing.T) {
	// Fatal calls os.Exit — just verify the method is accessible.
	assert.NotNil(t, Logger.Fatal)
}

func TestYapLoggerTips(t *testing.T) {
	old := MultiPrinter.Writer
	defer func() { MultiPrinter.Writer = old }()

	MultiPrinter.Writer = io.Discard

	assert.NotPanics(t, func() {
		Logger.Tips("test tips message")
		Logger.Tips("test tips message with args")
	})
}

func TestSetColorDisabled(t *testing.T) {
	oldNoColor := os.Getenv("NO_COLOR")
	oldColorTerm := os.Getenv("COLORTERM")
	oldTerm := os.Getenv("TERM")
	oldColorDisabled := colorDisabled.Load()

	defer func() {
		_ = os.Setenv("NO_COLOR", oldNoColor)
		_ = os.Setenv("COLORTERM", oldColorTerm)
		_ = os.Setenv("TERM", oldTerm)

		colorDisabled.Store(oldColorDisabled)

		SetColorDisabled(false)
	}()

	_ = os.Unsetenv("NO_COLOR")
	_ = os.Unsetenv("COLORTERM")
	_ = os.Unsetenv("TERM")

	SetColorDisabled(true)
	assert.True(t, IsColorDisabled())

	SetColorDisabled(false)
	assert.False(t, IsColorDisabled())
}

func TestIsColorDisabled(t *testing.T) {
	oldNoColor := os.Getenv("NO_COLOR")
	oldColorDisabled := colorDisabled.Load()

	defer func() {
		_ = os.Setenv("NO_COLOR", oldNoColor)

		colorDisabled.Store(oldColorDisabled)
	}()

	_ = os.Unsetenv("NO_COLOR")

	colorDisabled.Store(false)

	assert.False(t, IsColorDisabled())

	_ = os.Setenv("NO_COLOR", "1")

	assert.True(t, IsColorDisabled())
}

func TestSetVerbose(t *testing.T) {
	orig := IsVerboseEnabled()
	defer SetVerbose(orig)

	SetVerbose(true)
	assert.True(t, IsVerboseEnabled())

	SetVerbose(false)
	assert.False(t, IsVerboseEnabled())
}

func TestIsVerboseEnabled(t *testing.T) {
	orig := IsVerboseEnabled()
	defer SetVerbose(orig)

	assert.NotNil(t, IsVerboseEnabled())

	SetVerbose(true)
	assert.True(t, IsVerboseEnabled())
}

func TestGlobalLoggerFunctions(t *testing.T) {
	orig := IsVerboseEnabled()
	old := MultiPrinter.Writer

	defer func() {
		SetVerbose(orig)

		MultiPrinter.Writer = old
	}()

	MultiPrinter.Writer = io.Discard

	assert.NotPanics(t, func() {
		Info("test global info", "key", "value")
		Debug("test global debug", "key", "value")
		Warn("test global warn", "key", "value")
		Error("test global error", "key", "value")
		Tips("test global tips")

		SetVerbose(true)
		Debug("test global debug with verbose", "key", "value")
	})
}

func TestMultiPrinterStart(t *testing.T) {
	writer, err := MultiPrinter.Start()

	assert.NoError(t, err)
	assert.NotNil(t, writer)
	assert.Equal(t, MultiPrinter.Writer, writer)
}

func TestSetWriter(t *testing.T) {
	old := MultiPrinter.Writer
	defer func() { MultiPrinter.Writer = old }()

	SetWriter(io.Discard)

	assert.Equal(t, io.Discard, MultiPrinter.Writer)
}

func TestSanitizeValue(t *testing.T) {
	assert.Equal(t, "plain", sanitizeValue("plain"))
	assert.Equal(t, "a\nb\tc", sanitizeValue("a\nb\tc"))
	assert.Equal(t, `\x1b[31mred`, sanitizeValue("\x1b[31mred"))
	assert.Equal(t, `a\rb`, sanitizeValue("a\rb"))
	assert.Equal(t, `\x00`, sanitizeValue("\x00"))
	assert.Equal(t, "héllo", sanitizeValue("héllo"))
}

func TestHandleEscapesControlCharacters(t *testing.T) {
	old := MultiPrinter.Writer
	defer SetWriter(old)

	var buf bytes.Buffer

	SetWriter(&buf)

	Info("rejected", "path", "evil\x1b[2J\rname")

	out := buf.String()
	assert.NotContains(t, out, "\x1b[2J")
	assert.NotContains(t, out, "\r")
	assert.Contains(t, out, `evil\x1b[2J\rname`)
}

func TestHandleMultilineValueCannotForgeLogLine(t *testing.T) {
	old := MultiPrinter.Writer
	defer SetWriter(old)

	var buf bytes.Buffer

	SetWriter(&buf)

	Info("rejected", "path", "a\n2026-01-01 00:00:00 INFO [yap] forged")

	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	for _, line := range lines[1:] {
		assert.True(t, strings.HasPrefix(line, " "), "continuation must be indented: %q", line)
	}
}

func TestLogOddArgsKeepsBadKey(t *testing.T) {
	old := MultiPrinter.Writer
	defer SetWriter(old)

	var buf bytes.Buffer

	SetWriter(&buf)

	Info("odd", "k", "v", "dangling")

	assert.Contains(t, buf.String(), "!BADKEY")
	assert.Contains(t, buf.String(), "dangling")
}

func TestLoggerConcurrentUse(t *testing.T) {
	old := MultiPrinter.Writer
	defer SetWriter(old)

	SetWriter(io.Discard)

	var wg sync.WaitGroup

	for i := range 8 {
		wg.Add(2)

		go func() {
			defer wg.Done()

			for range 50 {
				SetVerbose(i%2 == 0)
				Info("msg", "k", "v")

				_ = IsVerboseEnabled()
				_ = IsColorDisabled()
			}
		}()

		go func() {
			defer wg.Done()

			for range 20 {
				SetWriter(io.Discard)
			}
		}()
	}

	wg.Wait()
	SetVerbose(false)
}
