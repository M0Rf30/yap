package logger

import (
	"io"
	"os"
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
	oldVerbose := verboseEnabled

	defer func() {
		MultiPrinter.Writer = old
		verboseEnabled = oldVerbose
	}()

	MultiPrinter.Writer = io.Discard

	verboseEnabled = false

	assert.NotPanics(t, func() {
		Logger.Debug("test debug message")
		Logger.Debug("test debug message with args", "key", "value")
	})

	verboseEnabled = true

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
	oldColorDisabled := colorDisabled

	defer func() {
		_ = os.Setenv("NO_COLOR", oldNoColor)
		_ = os.Setenv("COLORTERM", oldColorTerm)
		_ = os.Setenv("TERM", oldTerm)

		colorDisabled = oldColorDisabled

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
	oldColorDisabled := colorDisabled

	defer func() {
		_ = os.Setenv("NO_COLOR", oldNoColor)
		colorDisabled = oldColorDisabled
	}()

	_ = os.Unsetenv("NO_COLOR")

	colorDisabled = false

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
