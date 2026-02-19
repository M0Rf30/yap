// Package shell provides process execution and shell operations.
package shell

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/pterm/pterm"
	"mvdan.cc/sh/v3/expand"
	"mvdan.cc/sh/v3/interp"
	"mvdan.cc/sh/v3/syntax"

	"github.com/M0Rf30/yap/v2/pkg/buffers"
	"github.com/M0Rf30/yap/v2/pkg/constants"
	"github.com/M0Rf30/yap/v2/pkg/i18n"
	"github.com/M0Rf30/yap/v2/pkg/logger"
)

const (
	logLevelInfo = "INFO "

	// Buffer size constants for writer management
	minBufferPoolSize = 1024 // Minimum buffer size to return to pool
	lastLineCapacity  = 256  // Initial capacity for git progress lastLine buffer
)

var (
	// SetVerbose configures verbose logging output.
	SetVerbose = logger.SetVerbose
	// MultiPrinter handles concurrent output formatting.
	MultiPrinter = logger.MultiPrinter
)

// PackageDecoratedWriter decorates output with package name prefixes.
type PackageDecoratedWriter struct {
	writer      io.Writer
	packageName string
	buffer      []byte
}

// GitProgressWriter handles git command output with progress formatting.
type GitProgressWriter struct {
	writer      io.Writer
	packageName string
	buffer      []byte
	lastLine    []byte
}

// NewPackageDecoratedWriter creates a new PackageDecoratedWriter instance.
func NewPackageDecoratedWriter(writer io.Writer, packageName string) *PackageDecoratedWriter {
	return &PackageDecoratedWriter{
		writer:      writer,
		packageName: packageName,
		buffer:      buffers.GetSmallBuffer()[:0], // Start with empty slice but allocated backing array
	}
}

// NewGitProgressWriter creates a new GitProgressWriter instance.
func NewGitProgressWriter(writer io.Writer, packageName string) *GitProgressWriter {
	return &GitProgressWriter{
		writer:      writer,
		packageName: packageName,
		buffer:      buffers.GetSmallBuffer()[:0], // Start with empty slice but allocated backing array
		lastLine:    make([]byte, 0, lastLineCapacity),
	}
}

func (pdw *PackageDecoratedWriter) Write(p []byte) (int, error) {
	originalLen := len(p)
	pdw.buffer = append(pdw.buffer, p...)

	for {
		lineEnd := bytes.IndexByte(pdw.buffer, '\n')
		if lineEnd == -1 {
			break
		}

		line := pdw.buffer[:lineEnd+1]
		pdw.buffer = pdw.buffer[lineEnd+1:]

		if err := pdw.writeLine(line); err != nil {
			return originalLen, err
		}
	}

	return originalLen, nil
}

// Close returns the buffer to the pool and should be called when done with the writer.
func (pdw *PackageDecoratedWriter) Close() error {
	if pdw.buffer != nil {
		// Reset buffer to original capacity before returning to pool
		if cap(pdw.buffer) >= minBufferPoolSize {
			pdw.buffer = pdw.buffer[:minBufferPoolSize]
			buffers.PutSmallBuffer(pdw.buffer)
		}

		pdw.buffer = nil
	}

	return nil
}

// formatDecoratedLine creates a decorated log line with timestamp and package name.
func formatDecoratedLine(packageName, lineContent string) string {
	timestamp := time.Now().Format(constants.TimestampFormat)

	if logger.IsColorDisabled() {
		return fmt.Sprintf("%s %s [%s] %s\n", timestamp, logLevelInfo,
			packageName, lineContent)
	}

	return pterm.Sprintf("%s %s %s%s%s %s\n",
		pterm.FgGray.Sprint(timestamp),
		pterm.NewStyle(pterm.FgGreen, pterm.Bold).Sprint(logLevelInfo),
		pterm.NewStyle(pterm.FgWhite).Sprint("["),
		pterm.NewStyle(pterm.FgYellow).Sprint(packageName),
		pterm.NewStyle(pterm.FgWhite).Sprint("]"),
		lineContent,
	)
}

func (pdw *PackageDecoratedWriter) writeLine(line []byte) error {
	lineContent := strings.TrimRight(string(line), "\n\r")

	if strings.TrimSpace(lineContent) == "" {
		_, err := pdw.writer.Write(line)
		return err
	}

	decoratedLine := formatDecoratedLine(pdw.packageName, lineContent)
	_, err := pdw.writer.Write([]byte(decoratedLine))

	return err
}

func (gpw *GitProgressWriter) Write(p []byte) (int, error) {
	originalLen := len(p)
	gpw.buffer = append(gpw.buffer, p...)

	for {
		crIndex := bytes.IndexByte(gpw.buffer, '\r')
		nlIndex := bytes.IndexByte(gpw.buffer, '\n')

		var (
			lineEnd          int
			isCarriageReturn bool
		)

		switch {
		case crIndex != -1 && (nlIndex == -1 || crIndex < nlIndex):
			lineEnd = crIndex
			isCarriageReturn = true
		case nlIndex != -1:
			lineEnd = nlIndex
			isCarriageReturn = false
		default:
			return originalLen, nil
		}

		line := gpw.buffer[:lineEnd]
		gpw.buffer = gpw.buffer[lineEnd+1:]

		if err := gpw.handleLine(line, isCarriageReturn); err != nil {
			return originalLen, err
		}
	}
}

func (gpw *GitProgressWriter) handleLine(line []byte, isCarriageReturn bool) error {
	lineContent := string(line)

	if lineContent == "" {
		return nil
	}

	if isCarriageReturn {
		gpw.lastLine = make([]byte, len(line))
		copy(gpw.lastLine, line)

		return nil
	}

	return gpw.writeDecoratedLine(lineContent)
}

func (gpw *GitProgressWriter) writeDecoratedLine(lineContent string) error {
	decoratedLine := formatDecoratedLine(gpw.packageName, lineContent)
	_, err := gpw.writer.Write([]byte(decoratedLine))

	return err
}

// Close returns the buffer to the pool and should be called when done with the writer.
func (gpw *GitProgressWriter) Close() error {
	if gpw.buffer != nil {
		// Reset buffer to original capacity before returning to pool
		if cap(gpw.buffer) >= minBufferPoolSize {
			gpw.buffer = gpw.buffer[:minBufferPoolSize]
			buffers.PutSmallBuffer(gpw.buffer)
		}

		gpw.buffer = nil
	}

	return nil
}

// Exec executes a command in the specified directory with optional stdout exclusion.
func Exec(excludeStdout bool, dir, name string, args ...string) error {
	return ExecWithContext(context.Background(), excludeStdout, dir, name, args...)
}

// ExecWithContext executes a command with context for cancellation control.
func ExecWithContext(
	ctx context.Context, excludeStdout bool, dir, name string, args ...string,
) error {
	cmd := exec.CommandContext(ctx, name, args...) // #nosec G204 //nolint:gosec -- internal build system cmd

	if !excludeStdout {
		_, err := MultiPrinter.Start()
		if err != nil {
			return fmt.Errorf("%s: %w", i18n.T("errors.shell.failed_to_start_multiprinter"), err)
		}

		decoratedWriter := NewPackageDecoratedWriter(MultiPrinter.Writer, "yap")

		defer func() {
			_ = decoratedWriter.Close()
		}()

		cmd.Stdout = decoratedWriter
		cmd.Stderr = decoratedWriter
	}

	if dir != "" {
		cmd.Dir = dir
	}

	logger.Debug(i18n.T("logger.shell.debug.exec_cmd"), "command", name, "args", args, "dir", dir)

	start := time.Now()
	err := cmd.Run()
	duration := time.Since(start)

	if err != nil {
		logger.Debug(i18n.T("logger.execwithcontext.error.command_execution_failed_1"),
			"command", name,
			"args", args,
			"dir", dir,
			"duration", duration,
			"error", err)

		return fmt.Errorf("failed to execute command %s: %w", name, err)
	}

	logger.Debug(i18n.T("logger.execwithcontext.debug.command_execution_completed_1"),
		"command", name,
		"duration", duration)

	return nil
}

func normalizeScriptContent(script string) string {
	lines := strings.Split(script, "\n")

	var (
		normalized     []string
		currentCommand strings.Builder
	)

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)

		if trimmed == "" {
			if currentCommand.Len() > 0 {
				normalized = append(normalized, currentCommand.String())
				currentCommand.Reset()
			}

			continue
		}

		if before, ok := strings.CutSuffix(trimmed, "\\"); ok {
			commandPart := before
			commandPart = strings.TrimRight(commandPart, " ")

			if currentCommand.Len() > 0 {
				currentCommand.WriteString(" ")
				currentCommand.WriteString(commandPart)
			} else {
				currentCommand.WriteString(commandPart)
			}

			continue
		}

		if currentCommand.Len() > 0 {
			currentCommand.WriteString(" ")
			currentCommand.WriteString(trimmed)
			normalized = append(normalized, currentCommand.String())
			currentCommand.Reset()
		} else {
			normalized = append(normalized, trimmed)
		}
	}

	if currentCommand.Len() > 0 {
		normalized = append(normalized, currentCommand.String())
	}

	return strings.Join(normalized, "\n")
}

func logScriptContent(cmds string) {
	// Only log script content in verbose mode
	if !logger.IsVerboseEnabled() {
		return
	}

	_, err := MultiPrinter.Start()
	if err != nil {
		return
	}

	timestamp := time.Now().Format(constants.TimestampFormat)
	headerLine := pterm.Sprintf("%s %s %s%s%s %s\n",
		pterm.FgGray.Sprint(timestamp),
		pterm.NewStyle(pterm.FgBlue, pterm.Bold).Sprint("DEBUG"),
		pterm.NewStyle(pterm.FgWhite).Sprint("["),
		pterm.NewStyle(pterm.FgYellow).Sprint("yap"),
		pterm.NewStyle(pterm.FgWhite).Sprint("]"),
		"script content:",
	)
	_, _ = MultiPrinter.Writer.Write([]byte(headerLine))

	normalizedScript := normalizeScriptContent(cmds)
	lines := strings.SplitSeq(normalizedScript, "\n")

	for line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed != "" {
			timestamp := time.Now().Format(constants.TimestampFormat)
			scriptLine := pterm.Sprintf("%s %s %s%s%s   %s\n",
				pterm.FgGray.Sprint(timestamp),
				pterm.NewStyle(pterm.FgBlue, pterm.Bold).Sprint("DEBUG"),
				pterm.NewStyle(pterm.FgWhite).Sprint("["),
				pterm.NewStyle(pterm.FgYellow).Sprint("yap"),
				pterm.NewStyle(pterm.FgWhite).Sprint("]"),
				trimmed,
			)
			_, _ = MultiPrinter.Writer.Write([]byte(scriptLine))
		}
	}
}

// RunScript executes a shell script from a string.
func RunScript(cmds string) error {
	return RunScriptWithPackage(cmds, "")
}

// RunScriptWithPackage executes a shell script with package-specific output formatting.
func RunScriptWithPackage(cmds, packageName string) error {
	start := time.Now()

	if packageName != "" {
		logger.Info(i18n.T("logger.shell.info.exec_script"), "package", packageName)
	} else {
		logger.Info(i18n.T("logger.runscriptwithpackage.info.executing_shell_script_3"))
	}

	if cmds != "" {
		logScriptContent(cmds)
	}

	// Use LangBash so that bash-specific syntax (arrays, [[ ]], process substitution,
	// etc.) is accepted by the parser. Without this, bash array expansions like
	// "${_modules[@]}" cause a parse error in the default POSIX mode.
	script, err := syntax.NewParser(syntax.Variant(syntax.LangBash)).Parse(strings.NewReader(cmds), "")
	if err != nil {
		return fmt.Errorf("%s: %w", i18n.T("errors.shell.failed_to_parse_script"), err)
	}

	_, err = MultiPrinter.Start()
	if err != nil {
		return fmt.Errorf("%s: %w", i18n.T("errors.shell.failed_to_start_multiprinter"), err)
	}

	writer := MultiPrinter.Writer

	var decoratedWriter *PackageDecoratedWriter
	if packageName != "" {
		decoratedWriter = NewPackageDecoratedWriter(MultiPrinter.Writer, packageName)

		defer func() {
			_ = decoratedWriter.Close()
		}()

		writer = decoratedWriter
	}

	// Tee stdout+stderr into a buffer so we can include the output in the
	// error message when the script fails. mvdan/sh routes both stdout and
	// stderr of the script through the writers passed to StdIO; capturing
	// both gives us the most complete failure context.
	var outputBuf bytes.Buffer

	teeWriter := io.MultiWriter(writer, &outputBuf)

	runner, err := interp.New(
		interp.Env(expand.ListEnviron(os.Environ()...)),
		interp.StdIO(nil, teeWriter, teeWriter),
	)
	if err != nil {
		return fmt.Errorf("%s: %w", i18n.T("errors.shell.failed_to_create_script_runner"), err)
	}

	logger.Debug(i18n.T("logger.runscriptwithpackage.debug.starting_script_execution_1"))

	err = runner.Run(context.Background(), script)
	duration := time.Since(start)

	if err != nil {
		scriptErr := err.Error()
		if captured := strings.TrimSpace(outputBuf.String()); captured != "" {
			scriptErr = captured
		}

		if packageName != "" {
			logger.Error(i18n.T("logger.runscriptwithpackage.error.script_execution_failed_1"),
				"error", scriptErr,
				"duration", duration,
				"package", packageName)
		} else {
			logger.Error(i18n.T("logger.runscriptwithpackage.error.script_execution_failed_3"),
				"error", scriptErr,
				"duration", duration)
		}

		return fmt.Errorf("%s: %w", i18n.T("errors.shell.script_execution_failed"), err)
	}

	if packageName != "" {
		logger.Info(i18n.T("logger.unknown.info.shell_script_execution_completed_1"),
			"duration", duration,
			"package", packageName)
	} else {
		logger.Info(i18n.T("logger.unknown.info.shell_script_execution_completed_3"),
			"duration", duration)
	}

	return nil
}

// ExecWithSudo executes a command with sudo if the user is not running as root.
// This is specifically for package manager commands that need elevated privileges.
func ExecWithSudo(excludeStdout bool, dir, name string, args ...string) error {
	return ExecWithSudoContext(context.Background(), excludeStdout, dir, name, args...)
}

// ExecWithSudoContext executes a command with sudo if needed, with context for cancellation.
func ExecWithSudoContext(
	ctx context.Context, excludeStdout bool, dir, name string, args ...string,
) error {
	// Validate command name for security - only allow known package managers
	allowedCommands := map[string]bool{
		"pacman":  true,
		"dnf":     true,
		"yum":     true,
		"apt-get": true,
		"apt":     true,
		"apk":     true,
		"dpkg":    true,
		"rpm":     true,
		"makepkg": true,
		"zypper":  true,
	}

	if !allowedCommands[name] {
		return fmt.Errorf(i18n.T("errors.shell.command_not_allowed_for_sudo"), name)
	}

	// Check if we need sudo (not running as root and not already under sudo)
	needsSudo := os.Geteuid() != 0 && os.Getenv("SUDO_USER") == ""

	var cmd *exec.Cmd

	if needsSudo {
		// Prepend sudo to the command
		sudoArgs := append([]string{name}, args...)
		// #nosec G204 - command name is validated against allowlist
		cmd = exec.CommandContext(ctx, "sudo", sudoArgs...)

		logger.Debug(i18n.T("logger.shell.debug.exec_sudo"), "command", name, "args", args, "dir", dir)
	} else {
		cmd = exec.CommandContext(ctx, name, args...) // #nosec G204 //nolint:gosec -- internal build system cmd
		logger.Debug(i18n.T("logger.shell.debug.exec_sudo_cmd"),
			"command", name, "args", args, "dir", dir)
	}

	if !excludeStdout {
		_, err := MultiPrinter.Start()
		if err != nil {
			return fmt.Errorf("%s: %w", i18n.T("errors.shell.failed_to_start_multiprinter"), err)
		}

		decoratedWriter := NewPackageDecoratedWriter(MultiPrinter.Writer, "yap")

		defer func() {
			_ = decoratedWriter.Close()
		}()

		cmd.Stdout = decoratedWriter
		cmd.Stderr = decoratedWriter
	}

	if dir != "" {
		cmd.Dir = dir
	}

	start := time.Now()
	err := cmd.Run()
	duration := time.Since(start)

	if err != nil {
		logger.Debug(i18n.T("logger.unknown.error.command_execution_failed_1"),
			"command", name,
			"args", args,
			"dir", dir,
			"duration", duration,
			"error", err,
			"sudo", needsSudo)

		return fmt.Errorf("failed to execute command %s: %w", name, err)
	}

	logger.Debug(i18n.T("logger.unknown.debug.command_execution_completed_1"),
		"command", name,
		"duration", duration,
		"sudo", needsSudo)

	return nil
}
