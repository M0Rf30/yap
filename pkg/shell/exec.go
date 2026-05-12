// Package shell provides process execution and shell operations.
package shell

import (
	"bytes"
	"context"
	stderrors "errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"syscall"
	"time"

	"github.com/pterm/pterm"
	"mvdan.cc/sh/v3/expand"
	"mvdan.cc/sh/v3/interp"
	"mvdan.cc/sh/v3/syntax"

	"github.com/M0Rf30/yap/v2/pkg/buffers"
	"github.com/M0Rf30/yap/v2/pkg/constants"
	"github.com/M0Rf30/yap/v2/pkg/errors"
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
func Exec(ctx context.Context, excludeStdout bool, dir, name string, args ...string) error {
	return ExecWithContext(ctx, excludeStdout, dir, name, args...)
}

// ExecWithContext executes a command with context for cancellation control.
func ExecWithContext(
	ctx context.Context, excludeStdout bool, dir, name string, args ...string,
) error {
	cmd := exec.CommandContext(ctx, name, args...) // #nosec G204 //nolint:gosec -- internal build system cmd

	if !excludeStdout {
		_, err := MultiPrinter.Start()
		if err != nil {
			return errors.Wrap(err, errors.ErrTypeBuild, i18n.T("errors.shell.failed_to_start_multiprinter")).
				WithOperation("ExecWithContext")
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

		return errors.Wrap(err, errors.ErrTypeBuild, "failed to execute command").
			WithOperation("ExecWithContext").
			WithContext("command", name)
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
// An optional extraEnv slice of "KEY=VALUE" pairs may be supplied; each entry overrides
// or extends the inherited process environment for this script invocation only,
// without mutating os.Environ().  This makes the function safe to call concurrently
// from parallel build goroutines.
//
//nolint:gocyclo,cyclop // RunScriptWithPackage handles multiple script edge cases inline
func RunScriptWithPackage(cmds, packageName string, extraEnv ...[]string) error {
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
		return errors.Wrap(err, errors.ErrTypeBuild, i18n.T("errors.shell.failed_to_parse_script")).
			WithOperation("RunScriptWithPackage")
	}

	_, err = MultiPrinter.Start()
	if err != nil {
		return errors.Wrap(err, errors.ErrTypeBuild, i18n.T("errors.shell.failed_to_start_multiprinter")).
			WithOperation("RunScriptWithPackage")
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

	// Build the effective environment: start with the inherited process env, then
	// overlay any per-package overrides supplied by the caller.  Using a map-then-slice
	// approach guarantees that package-specific values (e.g. pkgdir, srcdir) shadow
	// the global ones without touching os.Setenv, making this safe for parallel builds.
	envMap := make(map[string]string)

	for _, kv := range os.Environ() {
		if k, v, ok := strings.Cut(kv, "="); ok {
			envMap[k] = v
		}
	}

	for _, extra := range extraEnv {
		for _, kv := range extra {
			if k, v, ok := strings.Cut(kv, "="); ok {
				envMap[k] = v
			}
		}
	}

	mergedEnv := make([]string, 0, len(envMap))

	for k, v := range envMap {
		mergedEnv = append(mergedEnv, k+"="+v)
	}

	runner, err := interp.New(
		interp.Env(expand.ListEnviron(mergedEnv...)),
		interp.StdIO(nil, teeWriter, teeWriter),
		interp.ExecHandlers(archiveExecHandler),
	)
	if err != nil {
		return errors.Wrap(err, errors.ErrTypeBuild, i18n.T("errors.shell.failed_to_create_script_runner")).
			WithOperation("RunScriptWithPackage")
	}

	logger.Debug(i18n.T("logger.runscriptwithpackage.debug.starting_script_execution_1"))

	err = runner.Run(context.Background(), script)
	duration := time.Since(start)

	return logScriptResult(err, packageName, duration, &outputBuf, "RunScriptWithPackage")
}

// fakerootExecHandler returns an interp.ExecHandlerFunc middleware that applies Linux
// user-namespace fakeroot to every subprocess spawned by the mvdan/sh interpreter.
// This allows package() scripts to call `install -o root -g root` (and similar) without
// actually being root: the kernel maps UID/GID 0 inside the namespace back to the real caller.
//
// Commands that cannot be resolved to a binary on PATH are passed to next (e.g. shell
// built-ins handled by a prior handler in the chain).
// Modelled after lure.sh/fakeroot and the default mvdan/sh exec handler.
func fakerootExecHandler(killTimeout time.Duration) func(next interp.ExecHandlerFunc) interp.ExecHandlerFunc {
	return func(next interp.ExecHandlerFunc) interp.ExecHandlerFunc {
		return func(ctx context.Context, args []string) error {
			hc := interp.HandlerCtx(ctx)

			path, err := interp.LookPathDir(hc.Dir, hc.Env, args[0])
			if err != nil {
				// Not a binary on PATH — let the next handler deal with it.
				return next(ctx, args)
			}

			cmd := &exec.Cmd{
				Path:   path,
				Args:   args,
				Env:    fakerootExecEnv(hc.Env),
				Dir:    hc.Dir,
				Stdin:  hc.Stdin,
				Stdout: hc.Stdout,
				Stderr: hc.Stderr,
			}

			applyFakeroot(cmd)

			if err = cmd.Start(); err == nil {
				if done := ctx.Done(); done != nil {
					go func() {
						<-done

						if killTimeout <= 0 || runtime.GOOS == "windows" {
							_ = cmd.Process.Signal(os.Kill)

							return
						}

						go func() {
							time.Sleep(killTimeout)

							_ = cmd.Process.Signal(os.Kill)
						}()

						_ = cmd.Process.Signal(os.Interrupt)
					}()
				}

				err = cmd.Wait()
			}

			return interpretCmdError(ctx, hc.Stderr, err)
		}
	}
}

// interpretCmdError converts an exec error into the appropriate interp exit status.
// Extracted to avoid duplication between fakerootExecHandler and any future handlers.
func interpretCmdError(ctx context.Context, stderr interface{ Write([]byte) (int, error) }, err error) error {
	var exitErr *exec.ExitError

	var execErr *exec.Error

	switch {
	case stderrors.As(err, &exitErr):
		if status, ok := exitErr.Sys().(syscall.WaitStatus); ok {
			if status.Signaled() {
				if ctx.Err() != nil {
					return ctx.Err()
				}

				sig := int(status.Signal())
				//nolint:gosec // signal values are bounded 0-127
				return interp.ExitStatus(uint8(128 + sig))
			}

			exitCode := status.ExitStatus()
			//nolint:gosec // exit codes are bounded 0-255
			return interp.ExitStatus(uint8(exitCode))
		}

		return interp.ExitStatus(1)
	case stderrors.As(err, &execErr):
		_, _ = fmt.Fprintf(stderr, "%v\n", execErr)

		return interp.ExitStatus(127)
	default:
		return err
	}
}

// fakerootExecEnv converts the mvdan/sh environment into a []string suitable for exec.Cmd.Env.
// Extracted from mvdan/sh interp/vars.go (same approach as lure-sh).
func fakerootExecEnv(env expand.Environ) []string {
	list := make([]string, 0, 64)

	env.Each(func(name string, vr expand.Variable) bool {
		if !vr.IsSet() {
			for i, kv := range list {
				if strings.HasPrefix(kv, name+"=") {
					list[i] = ""
				}
			}
		}

		if vr.Exported && vr.Kind == expand.String {
			list = append(list, name+"="+vr.String())
		}

		return true
	})

	return list
}

// RunScriptInFakeroot executes a shell script identically to RunScriptWithPackage
// but wraps every subprocess in a Linux user-namespace fakeroot so that ownership
// operations (install -o root, chown, etc.) succeed without real root privileges.
// Use this for the package() stage of PKGBUILD execution.
//
//nolint:gocyclo,cyclop // mirrors RunScriptWithPackage complexity
func RunScriptInFakeroot(cmds, packageName string, extraEnv ...[]string) error {
	start := time.Now()

	if packageName != "" {
		logger.Info(i18n.T("logger.shell.info.exec_script"), "package", packageName)
	} else {
		logger.Info(i18n.T("logger.runscriptwithpackage.info.executing_shell_script_3"))
	}

	if cmds != "" {
		logScriptContent(cmds)
	}

	script, err := syntax.NewParser(syntax.Variant(syntax.LangBash)).Parse(strings.NewReader(cmds), "")
	if err != nil {
		return errors.Wrap(err, errors.ErrTypeBuild, i18n.T("errors.shell.failed_to_parse_script")).
			WithOperation("RunScriptInFakeroot")
	}

	_, err = MultiPrinter.Start()
	if err != nil {
		return errors.Wrap(err, errors.ErrTypeBuild, i18n.T("errors.shell.failed_to_start_multiprinter")).
			WithOperation("RunScriptInFakeroot")
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

	var outputBuf bytes.Buffer

	teeWriter := io.MultiWriter(writer, &outputBuf)

	envMap := make(map[string]string)

	for _, kv := range os.Environ() {
		if k, v, ok := strings.Cut(kv, "="); ok {
			envMap[k] = v
		}
	}

	for _, extra := range extraEnv {
		for _, kv := range extra {
			if k, v, ok := strings.Cut(kv, "="); ok {
				envMap[k] = v
			}
		}
	}

	mergedEnv := make([]string, 0, len(envMap))

	for k, v := range envMap {
		mergedEnv = append(mergedEnv, k+"="+v)
	}

	runner, err := interp.New(
		interp.Env(expand.ListEnviron(mergedEnv...)),
		interp.StdIO(nil, teeWriter, teeWriter),
		interp.ExecHandlers(archiveExecHandler, fakerootExecHandler(0)),
	)
	if err != nil {
		return errors.Wrap(err, errors.ErrTypeBuild, i18n.T("errors.shell.failed_to_create_script_runner")).
			WithOperation("RunScriptInFakeroot")
	}

	logger.Debug(i18n.T("logger.runscriptwithpackage.debug.starting_script_execution_1"))

	err = runner.Run(context.Background(), script)
	duration := time.Since(start)

	return logScriptResult(err, packageName, duration, &outputBuf, "RunScriptInFakeroot")
}

// logScriptResult logs the outcome of a script run and returns a wrapped error on failure.
// Extracted to eliminate duplication between RunScriptWithPackage and RunScriptInFakeroot.
func logScriptResult(err error, packageName string, duration time.Duration, outputBuf *bytes.Buffer, op string) error {
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

		return errors.Wrap(err, errors.ErrTypeBuild, i18n.T("errors.shell.script_execution_failed")).
			WithOperation(op)
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
func ExecWithSudo(ctx context.Context, excludeStdout bool, dir, name string, args ...string) error {
	return ExecWithSudoContext(ctx, excludeStdout, dir, name, args...)
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
		return errors.New(errors.ErrTypeBuild, fmt.Sprintf(i18n.T("errors.shell.command_not_allowed_for_sudo"), name)).
			WithOperation("ExecWithSudoContext").
			WithContext("command", name)
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
			return errors.Wrap(err, errors.ErrTypeBuild, i18n.T("errors.shell.failed_to_start_multiprinter")).
				WithOperation("ExecWithSudoContext")
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

		return errors.Wrap(err, errors.ErrTypeBuild, "failed to execute command").
			WithOperation("ExecWithSudoContext").
			WithContext("command", name)
	}

	logger.Debug(i18n.T("logger.unknown.debug.command_execution_completed_1"),
		"command", name,
		"duration", duration,
		"sudo", needsSudo)

	return nil
}
