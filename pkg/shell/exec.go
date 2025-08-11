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

	"github.com/pkg/errors"
	"github.com/pterm/pterm"
	"mvdan.cc/sh/v3/expand"
	"mvdan.cc/sh/v3/interp"
	"mvdan.cc/sh/v3/syntax"

	"github.com/M0Rf30/yap/v2/pkg/logger"
)

const (
	timestampFormat = "2006-01-02 15:04:05"
	logLevelInfo    = "INFO "
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
		buffer:      make([]byte, 0, 1024),
	}
}

// NewGitProgressWriter creates a new GitProgressWriter instance.
func NewGitProgressWriter(writer io.Writer, packageName string) *GitProgressWriter {
	return &GitProgressWriter{
		writer:      writer,
		packageName: packageName,
		buffer:      make([]byte, 0, 1024),
		lastLine:    make([]byte, 0, 256),
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

func (pdw *PackageDecoratedWriter) writeLine(line []byte) error {
	lineContent := strings.TrimRight(string(line), "\n\r")

	if strings.TrimSpace(lineContent) == "" {
		_, err := pdw.writer.Write(line)
		return err
	}

	timestamp := time.Now().Format(timestampFormat)

	var decoratedLine string
	if logger.IsColorDisabled() {
		decoratedLine = fmt.Sprintf("%s %s  [%s] %s\n", timestamp, logLevelInfo,
			pdw.packageName, lineContent)
	} else {
		decoratedLine = pterm.Sprintf("%s %s  [%s] %s\n",
			pterm.FgGray.Sprint(timestamp),
			pterm.NewStyle(pterm.FgGreen, pterm.Bold).Sprint(logLevelInfo),
			pterm.FgYellow.Sprint(pdw.packageName),
			lineContent,
		)
	}

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
	timestamp := time.Now().Format(timestampFormat)

	var decoratedLine string
	if logger.IsColorDisabled() {
		decoratedLine = fmt.Sprintf("%s %s  [%s] %s\n", timestamp, logLevelInfo,
			gpw.packageName, lineContent)
	} else {
		decoratedLine = pterm.Sprintf("%s %s  %s %s\n",
			pterm.FgGray.Sprint(timestamp),
			pterm.NewStyle(pterm.FgGreen, pterm.Bold).Sprint(logLevelInfo),
			pterm.FgYellow.Sprintf("[%s]", gpw.packageName),
			lineContent,
		)
	}

	_, err := gpw.writer.Write([]byte(decoratedLine))

	return err
}

// Exec executes a command in the specified directory with optional stdout exclusion.
func Exec(excludeStdout bool, dir, name string, args ...string) error {
	return ExecWithContext(context.Background(), excludeStdout, dir, name, args...)
}

// ExecWithContext executes a command with context for cancellation control.
func ExecWithContext(
	ctx context.Context, excludeStdout bool, dir, name string, args ...string,
) error {
	cmd := exec.CommandContext(ctx, name, args...)

	if !excludeStdout {
		_, err := MultiPrinter.Start()
		if err != nil {
			return errors.Wrap(err, "failed to start multiprinter")
		}

		decoratedWriter := NewPackageDecoratedWriter(MultiPrinter.Writer, "yap")
		cmd.Stdout = decoratedWriter
		cmd.Stderr = decoratedWriter
	}

	if dir != "" {
		cmd.Dir = dir
	}

	logger.Debug("executing command", "command", name, "args", args, "dir", dir)

	start := time.Now()
	err := cmd.Run()
	duration := time.Since(start)

	if err != nil {
		logger.Error("command execution failed",
			"command", name,
			"args", args,
			"dir", dir,
			"duration", duration,
			"error", err)

		return errors.Wrapf(err, "failed to execute command %s", name)
	}

	logger.Debug("command execution completed",
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

		if strings.HasSuffix(trimmed, "\\") {
			commandPart := strings.TrimSuffix(trimmed, "\\")
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
	_, err := MultiPrinter.Start()
	if err != nil {
		return
	}

	timestamp := time.Now().Format("2006-01-02 15:04:05")
	headerLine := pterm.Sprintf("%s %s %s %s\n",
		pterm.FgGray.Sprint(timestamp),
		pterm.NewStyle(pterm.FgBlue, pterm.Bold).Sprint("DEBUG "), pterm.FgBlue.Sprint("[yap]"),
		"script content:",
	)
	_, _ = MultiPrinter.Writer.Write([]byte(headerLine))

	normalizedScript := normalizeScriptContent(cmds)
	lines := strings.SplitSeq(normalizedScript, "\n")

	for line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed != "" {
			timestamp := time.Now().Format("2006-01-02 15:04:05")
			scriptLine := pterm.Sprintf("%s %s %s   %s\n",
				pterm.FgGray.Sprint(timestamp),
				pterm.NewStyle(pterm.FgBlue, pterm.Bold).Sprint("DEBUG "), pterm.FgBlue.Sprint("[yap]"),
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
		logger.Info("executing shell script", "package", packageName)
	} else {
		logger.Info("executing shell script")
	}

	if cmds != "" {
		logScriptContent(cmds)
	}

	script, err := syntax.NewParser().Parse(strings.NewReader(cmds), "")
	if err != nil {
		return errors.Wrap(err, "failed to parse script")
	}

	_, err = MultiPrinter.Start()
	if err != nil {
		return errors.Wrap(err, "failed to start multiprinter")
	}

	writer := MultiPrinter.Writer
	if packageName != "" {
		writer = NewPackageDecoratedWriter(MultiPrinter.Writer, packageName)
	}

	runner, err := interp.New(
		interp.Env(expand.ListEnviron(os.Environ()...)),
		interp.StdIO(nil, writer, writer),
	)
	if err != nil {
		return errors.Wrap(err, "failed to create script runner")
	}

	logger.Debug("starting script execution")

	err = runner.Run(context.Background(), script)
	duration := time.Since(start)

	if err != nil {
		if packageName != "" {
			logger.Error("script execution failed",
				"error", err,
				"duration", duration,
				"package", packageName)
		} else {
			logger.Error("script execution failed",
				"error", err,
				"duration", duration)
		}

		return errors.Wrap(err, "script execution failed")
	}

	if packageName != "" {
		logger.Info("shell script execution completed successfully",
			"duration", duration,
			"package", packageName)
	} else {
		logger.Info("shell script execution completed successfully",
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
		return fmt.Errorf("command '%s' is not allowed for sudo execution", name)
	}

	// Check if we need sudo (not running as root and not already under sudo)
	needsSudo := os.Geteuid() != 0 && os.Getenv("SUDO_USER") == ""

	var cmd *exec.Cmd

	if needsSudo {
		// Prepend sudo to the command
		sudoArgs := append([]string{name}, args...)
		// #nosec G204 - command name is validated against allowlist
		cmd = exec.CommandContext(ctx, "sudo", sudoArgs...)

		logger.Debug("executing command with sudo", "command", name, "args", args, "dir", dir)
	} else {
		cmd = exec.CommandContext(ctx, name, args...)
		logger.Debug("executing command", "command", name, "args", args, "dir", dir)
	}

	if !excludeStdout {
		_, err := MultiPrinter.Start()
		if err != nil {
			return errors.Wrap(err, "failed to start multiprinter")
		}

		decoratedWriter := NewPackageDecoratedWriter(MultiPrinter.Writer, "yap")
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
		logger.Error("command execution failed",
			"command", name,
			"args", args,
			"dir", dir,
			"duration", duration,
			"error", err,
			"sudo", needsSudo)

		return errors.Wrapf(err, "failed to execute command %s", name)
	}

	logger.Debug("command execution completed",
		"command", name,
		"duration", duration,
		"sudo", needsSudo)

	return nil
}
