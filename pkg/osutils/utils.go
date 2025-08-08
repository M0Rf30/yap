package osutils

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/cavaliergopher/grab/v3"
	ggit "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/transport/ssh"
	"github.com/mholt/archives"
	"github.com/pkg/errors"
	"github.com/pterm/pterm"
	"mvdan.cc/sh/v3/expand"
	"mvdan.cc/sh/v3/interp"
	"mvdan.cc/sh/v3/syntax"

	"github.com/M0Rf30/yap/v2/pkg/constants"
	ycontext "github.com/M0Rf30/yap/v2/pkg/context"
)

const (
	goArchivePath = "/tmp/go.tar.gz"
	goExecutable  = "/usr/bin/go"

	// Log formatting constants.
	timestampFormat = "2006-01-02 15:04:05"
	logLevelInfo    = "INFO"
)

// Global flag to disable colored output.
var colorDisabled = false

// SetColorDisabled sets the global color preference.
func SetColorDisabled(disabled bool) {
	colorDisabled = disabled

	// Configure pterm color settings
	if disabled {
		pterm.DisableColor()
	} else {
		pterm.EnableColor()
	}
}

// IsColorDisabled returns true if colors should be disabled.
func IsColorDisabled() bool {
	// Check environment variable first (common convention)
	envDisabled := os.Getenv("NO_COLOR") != "" || os.Getenv("TERM") == "dumb"

	if envDisabled {
		// Ensure pterm is configured to match environment
		if !colorDisabled {
			SetColorDisabled(true)
		}

		return true
	}

	return colorDisabled
}

// YapLogger wraps pterm.Logger to add [yap] prefix to all messages.
type YapLogger struct {
	*pterm.Logger
}

// Info logs an info message with [yap] prefix.
func (y *YapLogger) Info(msg string, args ...[]pterm.LoggerArgument) {
	y.Logger.Info("[yap] "+msg, args...)
}

// Tips logs a tips message with [yap] prefix and TIPS level.
func (y *YapLogger) Tips(msg string, _ ...[]pterm.LoggerArgument) {
	// Create a custom log entry with TIPS level
	timestamp := time.Now().Format(timestampFormat)

	var logMsg string
	if IsColorDisabled() {
		// Plain text format without colors
		logMsg = fmt.Sprintf("%s %s  [yap] %s", timestamp, logLevelInfo, msg)
	} else {
		// Colored format
		logMsg = fmt.Sprintf("%s %s  %s %s",
			pterm.FgGray.Sprint(timestamp),
			pterm.FgCyan.Sprint(logLevelInfo),
			pterm.FgBlue.Sprint("[yap]"),
			msg,
		)
	}

	pterm.Println(logMsg)
}

// Warn logs a warning message with [yap] prefix.
func (y *YapLogger) Warn(msg string, args ...[]pterm.LoggerArgument) {
	y.Logger.Warn("[yap] "+msg, args...)
}

// Error logs an error message with [yap] prefix.
func (y *YapLogger) Error(msg string, args ...[]pterm.LoggerArgument) {
	y.Logger.Error("[yap] "+msg, args...)
}

// Debug logs a debug message with [yap] prefix.
func (y *YapLogger) Debug(msg string, args ...[]pterm.LoggerArgument) {
	y.Logger.Debug("[yap] "+msg, args...)
}

// Fatal logs a fatal message with [yap] prefix.
func (y *YapLogger) Fatal(msg string, args ...[]pterm.LoggerArgument) {
	y.Logger.Fatal("[yap] "+msg, args...)
}

// Args creates logger arguments (delegate to embedded logger).
func (y *YapLogger) Args(args ...any) []pterm.LoggerArgument {
	return y.Logger.Args(args...)
}

// SetVerbose configures the logger level based on verbose flag.
func SetVerbose(verbose bool) {
	var level pterm.LogLevel
	if verbose {
		level = pterm.LogLevelDebug
	} else {
		level = pterm.LogLevelInfo
	}

	baseLogger = pterm.DefaultLogger.WithLevel(level).WithWriter(MultiPrinter.Writer)
	Logger = &YapLogger{baseLogger}
}

var (
	// MultiPrinter is the default multi printer.
	MultiPrinter = pterm.DefaultMultiPrinter
	// baseLogger is the underlying pterm logger.
	baseLogger = pterm.DefaultLogger.WithLevel(pterm.LogLevelInfo).WithWriter(MultiPrinter.Writer)
	// Logger is the default logger with information level logging.
	// It writes to the MultiPrinter's writer and adds [yap] prefix.
	Logger = &YapLogger{baseLogger}
)

// ComponentLogger provides component-specific logging with identifiers.
type ComponentLogger struct {
	*pterm.Logger

	component string
}

// WithComponent creates a logger with a component identifier.
func WithComponent(component string) *ComponentLogger {
	return &ComponentLogger{
		Logger:    baseLogger,
		component: component,
	}
}

// ServiceLogger returns a logger with "yap" as component identifier for service messages.
func ServiceLogger() *ComponentLogger {
	return WithComponent("yap")
}

// Info logs an info message with component identifier.
func (cl *ComponentLogger) Info(msg string, args ...[]pterm.LoggerArgument) {
	prefixedMsg := "[" + cl.component + "] " + msg
	cl.Logger.Info(prefixedMsg, args...)
}

// Warn logs a warning message with component identifier.
func (cl *ComponentLogger) Warn(msg string, args ...[]pterm.LoggerArgument) {
	prefixedMsg := "[" + cl.component + "] " + msg
	cl.Logger.Warn(prefixedMsg, args...)
}

// Error logs an error message with component identifier.
func (cl *ComponentLogger) Error(msg string, args ...[]pterm.LoggerArgument) {
	prefixedMsg := "[" + cl.component + "] " + msg
	cl.Logger.Error(prefixedMsg, args...)
}

// Fatal logs a fatal message with component identifier and exits.
func (cl *ComponentLogger) Fatal(msg string, args ...[]pterm.LoggerArgument) {
	prefixedMsg := "[" + cl.component + "] " + msg
	cl.Logger.Fatal(prefixedMsg, args...)
}

// Debug logs a debug message with component identifier.
func (cl *ComponentLogger) Debug(msg string, args ...[]pterm.LoggerArgument) {
	prefixedMsg := "[" + cl.component + "] " + msg
	cl.Logger.Debug(prefixedMsg, args...)
}

// Args creates logger arguments for ComponentLogger (delegate to embedded logger).
func (cl *ComponentLogger) Args(args ...any) []pterm.LoggerArgument {
	return cl.Logger.Args(args...)
}

// PackageDecoratedWriter wraps an io.Writer to decorate each line with package name and timestamp.
type PackageDecoratedWriter struct {
	writer      io.Writer
	packageName string
	buffer      []byte
}

// GitProgressWriter wraps an io.Writer to handle git progress output with carriage returns.
type GitProgressWriter struct {
	writer      io.Writer
	packageName string
	buffer      []byte
	lastLine    []byte
}

// NewPackageDecoratedWriter creates a new PackageDecoratedWriter with the specified package name.
func NewPackageDecoratedWriter(writer io.Writer, packageName string) *PackageDecoratedWriter {
	return &PackageDecoratedWriter{
		writer:      writer,
		packageName: packageName,
		buffer:      make([]byte, 0, 1024), // Pre-allocate buffer
	}
}

// NewGitProgressWriter creates a new GitProgressWriter with the specified package name.
func NewGitProgressWriter(writer io.Writer, packageName string) *GitProgressWriter {
	return &GitProgressWriter{
		writer:      writer,
		packageName: packageName,
		buffer:      make([]byte, 0, 1024),
		lastLine:    make([]byte, 0, 256),
	}
}

// Write implements io.Writer interface and decorates each line with package name and timestamp.
func (pdw *PackageDecoratedWriter) Write(p []byte) (int, error) {
	originalLen := len(p)

	// Add incoming bytes to buffer
	pdw.buffer = append(pdw.buffer, p...)

	// Process complete lines
	for {
		lineEnd := bytes.IndexByte(pdw.buffer, '\n')
		if lineEnd == -1 {
			// No complete line found, keep buffering
			break
		}

		// Extract line including newline
		line := pdw.buffer[:lineEnd+1]
		pdw.buffer = pdw.buffer[lineEnd+1:]

		// Process the line
		err := pdw.writeLine(line)
		if err != nil {
			return originalLen, err
		}
	}

	return originalLen, nil
}

// writeLine processes and writes a single line with decoration.
func (pdw *PackageDecoratedWriter) writeLine(line []byte) error {
	// Get the line content without newline for processing
	lineContent := strings.TrimRight(string(line), "\n\r")

	// Skip empty lines - write them as-is
	if strings.TrimSpace(lineContent) == "" {
		_, err := pdw.writer.Write(line)

		return err
	}

	// Add the decorated line with timestamp and colors for consistency with logger output
	timestamp := time.Now().Format(timestampFormat)

	var decoratedLine string
	if IsColorDisabled() {
		// Plain text format without colors
		decoratedLine = fmt.Sprintf("%s %s  [%s] %s\n", timestamp, logLevelInfo,
			pdw.packageName, lineContent)
	} else {
		// Colored format
		decoratedLine = pterm.Sprintf("%s %s  [%s] %s\n",
			pterm.FgGray.Sprint(timestamp),
			pterm.FgCyan.Sprint(logLevelInfo),
			pterm.FgYellow.Sprint(pdw.packageName),
			lineContent,
		)
	}

	_, err := pdw.writer.Write([]byte(decoratedLine))

	return err
}

// DownloadWithResume downloads a file with resume capability and retry logic.
// It extends the basic Download function with the ability to resume interrupted downloads.
//
// Parameters:
// - destination: the path where the downloaded file will be saved.
// - uri: the URL of the file to download.
// - logger: optional component logger for context-aware logging. If nil, uses default logger.
// - maxRetries: maximum number of retry attempts (0 = no retries, default: 3).
func DownloadWithResume(destination, uri string, logger *ComponentLogger, maxRetries int) error {
	if maxRetries <= 0 {
		maxRetries = 3
	}

	var lastErr error

	for attempt := 0; attempt <= maxRetries; attempt++ {
		if attempt > 0 {
			activeLogger := logger
			if activeLogger == nil {
				activeLogger = ServiceLogger()
			}

			activeLogger.Info("retrying download", Logger.Args(
				"attempt", attempt+1,
				"max_retries", maxRetries+1,
				"url", uri))

			// Exponential backoff: 1s, 2s, 4s, 8s
			backoff := time.Duration(1<<(attempt-1)) * time.Second
			time.Sleep(backoff)
		}

		err := downloadWithResumeInternal(context.Background(), destination, uri, logger, "", "")
		if err == nil {
			return nil
		}

		lastErr = err

		// Check if it's a retryable error
		if !isRetryableDownloadError(err) {
			break
		}
	}

	return errors.Errorf("download failed after %d attempts: %v", maxRetries+1, lastErr)
}

// DownloadWithResumeContext downloads a file with context information for enhanced
// progress reporting.
//
// Parameters:
//   - destination: local file path where the downloaded content will be saved.
//   - uri: source URL to download from.
//   - logger: optional component logger for context-aware logging. If nil, uses default logger.
//   - maxRetries: maximum number of retry attempts (0 = no retries, default: 3).
//   - packageName: package name for progress reporting (if empty, uses logger component or "yap").
//   - sourceName: source name for progress reporting (if empty, uses filename from URI).
func DownloadWithResumeContext(
	destination, uri string,
	logger *ComponentLogger,
	maxRetries int,
	packageName, sourceName string,
) error {
	if maxRetries <= 0 {
		maxRetries = 3
	}

	var lastErr error

	for attempt := 0; attempt <= maxRetries; attempt++ {
		if attempt > 0 {
			activeLogger := logger
			if activeLogger == nil {
				activeLogger = ServiceLogger()
			}

			activeLogger.Info("retrying download", Logger.Args(
				"attempt", attempt+1,
				"max_retries", maxRetries+1,
				"url", uri))

			// Exponential backoff: 1s, 2s, 4s, 8s
			backoff := time.Duration(1<<(attempt-1)) * time.Second
			time.Sleep(backoff)
		}

		err := downloadWithResumeInternal(
			context.Background(),
			destination, uri, logger, packageName, sourceName)
		if err == nil {
			return nil
		}

		lastErr = err

		// Check if it's a retryable error
		if !isRetryableDownloadError(err) {
			break
		}
	}

	return errors.Errorf("download failed after %d attempts: %v", maxRetries+1, lastErr)
}

// downloadWithResumeInternal performs the actual download with resume capability.
func downloadWithResumeInternal(
	ctx context.Context, destination, uri string, logger *ComponentLogger,
	packageName, sourceName string,
) error {
	client, req, err := prepareDownloadRequest(ctx, destination, uri, logger)
	if err != nil {
		return err
	}

	resp := client.Do(req)
	if resp.HTTPResponse == nil {
		return errors.Errorf("download failed: no response")
	}

	activeLogger := getActiveLogger(logger)
	logDownloadStart(activeLogger, uri, resp)

	_, err = MultiPrinter.Start()
	if err != nil {
		return err
	}

	progressBar := createProgressBar(resp, logger, packageName, sourceName, uri)

	return monitorDownload(resp, progressBar, activeLogger, destination)
}

// prepareDownloadRequest creates and configures the download request.
func prepareDownloadRequest(
	ctx context.Context, destination, uri string, logger *ComponentLogger,
) (*grab.Client, *grab.Request, error) {
	client := grab.NewClient()
	client.UserAgent = "YAP/1.0 (Yet Another Packager)"

	req, err := grab.NewRequest(destination, uri)
	if err != nil {
		return nil, nil, errors.Errorf("download failed %s", err)
	}

	configureResumeIfPossible(req, destination, uri, logger)
	req.WithContext(ctx)

	return client, req, nil
}

// configureResumeIfPossible checks for partial files and enables resume.
func configureResumeIfPossible(req *grab.Request, destination, uri string,
	logger *ComponentLogger) {
	info, err := os.Stat(destination)
	if err == nil && info.Size() > 0 {
		req.NoResume = false // Enable resume

		if logger != nil {
			logger.Info("resuming download", Logger.Args(
				"url", uri,
				"existing_size", formatBytes(info.Size())))
		}
	}
}

// getActiveLogger returns the provided logger or default logger.
func getActiveLogger(logger *ComponentLogger) *ComponentLogger {
	if logger != nil {
		return logger
	}

	return ServiceLogger()
}

// logDownloadStart logs the initial download information.
func logDownloadStart(activeLogger *ComponentLogger, uri string, resp *grab.Response) {
	if resp.CanResume {
		activeLogger.Info("server supports resume", Logger.Args("url", uri))
	}

	activeLogger.Info("downloading", Logger.Args("url", resp.Request.URL()))
	activeLogger.Info("response status: " + resp.HTTPResponse.Status)
}

// createProgressBar creates an enhanced progress bar if the response size is known.
func createProgressBar(
	resp *grab.Response, logger *ComponentLogger, packageName, sourceName, uri string,
) *EnhancedProgressBar {
	if resp.Size() <= 0 {
		return nil
	}

	pkgName := determinePackageName(packageName, logger)
	srcName := determineSourceName(sourceName, uri)

	return NewEnhancedProgressBar(MultiPrinter.Writer, pkgName, srcName, resp.Size())
}

// determinePackageName resolves the package name for progress display.
func determinePackageName(packageName string, logger *ComponentLogger) string {
	if packageName != "" {
		return packageName
	}

	if logger != nil && logger.component != "" {
		return logger.component
	}

	return "yap"
}

// determineSourceName resolves the source name for progress display.
func determineSourceName(sourceName, uri string) string {
	if sourceName != "" {
		return sourceName
	}

	if filename := Filename(uri); filename != "" {
		return filename
	}

	return "download"
}

// monitorDownload handles the download monitoring loop.
func monitorDownload(
	resp *grab.Response, progressBar *EnhancedProgressBar,
	activeLogger *ComponentLogger, destination string,
) error {
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-resp.Done:
			if progressBar != nil {
				progressBar.Finish()
			}

			if resp.Err() != nil {
				return resp.Err()
			}

			activeLogger.Info("download completed", Logger.Args("path", destination))

			return nil

		case <-ticker.C:
			if progressBar != nil && resp.Size() > 0 {
				progressBar.Update(resp.BytesComplete())
			}
		}
	}
}

// isRetryableDownloadError determines if a download error is retryable.
func isRetryableDownloadError(err error) bool {
	if err == nil {
		return false
	}

	errStr := err.Error()
	// Network-related errors that are typically retryable
	retryableErrors := []string{
		"connection reset",
		"connection refused",
		"timeout",
		"temporary failure",
		"network is unreachable",
		"no route to host",
		"502", "503", "504", // HTTP server errors
	}

	for _, retryable := range retryableErrors {
		if strings.Contains(strings.ToLower(errStr), retryable) {
			return true
		}
	}

	return false
}

// ConcurrentDownloadManager manages multiple downloads concurrently.
type ConcurrentDownloadManager struct {
	workerPool *ycontext.WorkerPool
	activeJobs map[string]*DownloadJob
	jobResults map[string]error
	mutex      sync.RWMutex
}

// DownloadJob represents a single download task.
type DownloadJob struct {
	Destination string
	URL         string
	Logger      *ComponentLogger
	MaxRetries  int
	Done        chan error
}

// NewConcurrentDownloadManager creates a new concurrent download manager.
func NewConcurrentDownloadManager(maxConcurrent int) *ConcurrentDownloadManager {
	if maxConcurrent <= 0 {
		maxConcurrent = 3 // default to 3 concurrent downloads
	}

	if maxConcurrent > 8 {
		maxConcurrent = 8 // cap at 8 to avoid overwhelming servers
	}

	return &ConcurrentDownloadManager{
		workerPool: ycontext.NewWorkerPool(maxConcurrent),
		activeJobs: make(map[string]*DownloadJob),
		jobResults: make(map[string]error),
	}
}

// SubmitDownload submits a download job to the concurrent manager.
func (cdm *ConcurrentDownloadManager) SubmitDownload(
	destination, downloadURL string,
	logger *ComponentLogger,
	maxRetries int,
) error {
	job := &DownloadJob{
		Destination: destination,
		URL:         downloadURL,
		Logger:      logger,
		MaxRetries:  maxRetries,
		Done:        make(chan error, 1),
	}

	jobKey := destination // use destination as unique key

	cdm.mutex.Lock()
	cdm.activeJobs[jobKey] = job
	cdm.mutex.Unlock()

	// Submit work to the worker pool
	return cdm.workerPool.Submit(context.Background(), func(ctx context.Context) error {
		defer func() {
			cdm.mutex.Lock()
			delete(cdm.activeJobs, jobKey)
			cdm.mutex.Unlock()
		}()

		err := downloadWithResumeInternal(ctx, job.Destination, job.URL, job.Logger, "", "")

		cdm.mutex.Lock()
		cdm.jobResults[jobKey] = err
		cdm.mutex.Unlock()

		job.Done <- err

		return err
	})
}

// WaitForJob waits for a specific download job to complete.
func (cdm *ConcurrentDownloadManager) WaitForJob(destination string) error {
	cdm.mutex.RLock()
	job, exists := cdm.activeJobs[destination]
	cdm.mutex.RUnlock()

	if !exists {
		// Check if already completed
		cdm.mutex.RLock()
		result, hasResult := cdm.jobResults[destination]
		cdm.mutex.RUnlock()

		if hasResult {
			return result
		}

		return errors.Errorf("download job not found: %s", destination)
	}

	return <-job.Done
}

// WaitForAll waits for all active downloads to complete and returns any errors.
func (cdm *ConcurrentDownloadManager) WaitForAll() map[string]error {
	// Get current active jobs
	cdm.mutex.RLock()

	jobs := make([]*DownloadJob, 0, len(cdm.activeJobs))
	for _, job := range cdm.activeJobs {
		jobs = append(jobs, job)
	}

	cdm.mutex.RUnlock()

	// Wait for all jobs to complete
	for _, job := range jobs {
		<-job.Done
	}

	// Return results
	cdm.mutex.RLock()

	results := make(map[string]error)
	for dest, err := range cdm.jobResults {
		results[dest] = err
	}

	cdm.mutex.RUnlock()

	return results
}

// Shutdown gracefully shuts down the download manager.
func (cdm *ConcurrentDownloadManager) Shutdown(timeout time.Duration) error {
	return cdm.workerPool.Shutdown(timeout)
}

// DownloadConcurrently downloads multiple files concurrently with resume capability.
//
// Parameters:
//   - downloads: map of destination -> URL for files to download
//   - logger: optional component logger for context-aware logging
//   - maxConcurrent: maximum number of concurrent downloads (0 = default)
//   - maxRetries: maximum retry attempts per download (0 = default)
//
// Returns a map of destination -> error for any failed downloads.
func DownloadConcurrently(
	downloads map[string]string,
	logger *ComponentLogger,
	maxConcurrent, maxRetries int,
) map[string]error {
	if len(downloads) == 0 {
		return make(map[string]error)
	}

	manager := NewConcurrentDownloadManager(maxConcurrent)

	defer func() {
		err := manager.Shutdown(30 * time.Second)
		if err != nil {
			Logger.Warn("failed to shutdown download manager", Logger.Args("error", err))
		}
	}()

	// Submit all downloads
	for destination, url := range downloads {
		err := manager.SubmitDownload(destination, url, logger, maxRetries)
		if err != nil {
			// If we can't submit, record the error immediately
			manager.mutex.Lock()
			manager.jobResults[destination] = err
			manager.mutex.Unlock()
		}
	}

	// Wait for all to complete and return results
	return manager.WaitForAll()
}

// Write implements io.Writer interface for GitProgressWriter and handles carriage returns properly.
func (gpw *GitProgressWriter) Write(p []byte) (int, error) {
	originalLen := len(p)

	// Add incoming bytes to buffer
	gpw.buffer = append(gpw.buffer, p...)

	// Process lines, handling both \n and \r
	for {
		crIndex := bytes.IndexByte(gpw.buffer, '\r')
		nlIndex := bytes.IndexByte(gpw.buffer, '\n')

		var lineEnd int

		var isCarriageReturn bool

		switch {
		case crIndex != -1 && (nlIndex == -1 || crIndex < nlIndex):
			// Found \r before \n (or no \n)
			lineEnd = crIndex
			isCarriageReturn = true
		case nlIndex != -1:
			// Found \n
			lineEnd = nlIndex
			isCarriageReturn = false
		default:
			// No complete line found
			return originalLen, nil
		}

		// Extract line
		line := gpw.buffer[:lineEnd]
		gpw.buffer = gpw.buffer[lineEnd+1:]

		// Handle the line
		err := gpw.handleLine(line, isCarriageReturn)
		if err != nil {
			return originalLen, err
		}
	}
}

// handleLine processes a single line from git progress output.
func (gpw *GitProgressWriter) handleLine(line []byte, isCarriageReturn bool) error {
	lineContent := string(line)

	// Skip empty lines
	if lineContent == "" {
		return nil
	}

	if isCarriageReturn {
		// This is a progress update line, store it but don't output yet
		gpw.lastLine = make([]byte, len(line))
		copy(gpw.lastLine, line)

		return nil
	}

	// This is a final line (ends with \n), output it with decoration
	return gpw.writeDecoratedLine(lineContent)
}

// writeDecoratedLine writes a line with timestamp and package decoration.
func (gpw *GitProgressWriter) writeDecoratedLine(lineContent string) error {
	timestamp := time.Now().Format(timestampFormat)

	var decoratedLine string
	if IsColorDisabled() {
		// Plain text format without colors
		decoratedLine = fmt.Sprintf("%s %s  [%s] %s\n", timestamp, logLevelInfo,
			gpw.packageName, lineContent)
	} else {
		// Colored format
		decoratedLine = pterm.Sprintf("%s %s  %s %s\n",
			pterm.FgGray.Sprint(timestamp),
			pterm.FgCyan.Sprint(logLevelInfo),
			pterm.FgYellow.Sprintf("[%s]", gpw.packageName),
			lineContent,
		)
	}

	_, err := gpw.writer.Write([]byte(decoratedLine))

	return err
}

// normalizeScriptContent normalizes multiline script content by joining line continuations
// and properly formatting commands for readable logging.
func normalizeScriptContent(script string) string {
	lines := strings.Split(script, "\n")

	var normalized []string

	var currentCommand strings.Builder

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)

		// Skip empty lines
		if trimmed == "" {
			if currentCommand.Len() > 0 {
				normalized = append(normalized, currentCommand.String())
				currentCommand.Reset()
			}

			continue
		}

		// Check if this line continues the previous one (ends with \)
		if strings.HasSuffix(trimmed, "\\") {
			// Remove the backslash and add to current command
			commandPart := strings.TrimSuffix(trimmed, "\\")
			if currentCommand.Len() > 0 {
				currentCommand.WriteString(" " + commandPart)
			} else {
				currentCommand.WriteString(commandPart)
			}

			continue
		}

		// This line completes the command
		if currentCommand.Len() > 0 {
			currentCommand.WriteString(" " + trimmed)
			normalized = append(normalized, currentCommand.String())
			currentCommand.Reset()
		} else {
			normalized = append(normalized, trimmed)
		}
	}

	// Add any remaining command
	if currentCommand.Len() > 0 {
		normalized = append(normalized, currentCommand.String())
	}

	return strings.Join(normalized, "\n")
}

// logScriptContent logs script content using direct writer to avoid line wrapping.
func logScriptContent(cmds string) {
	// Only log script content if debug level is enabled
	if baseLogger.Level > pterm.LogLevelDebug {
		return
	}

	// Start multiprinter for consistent output handling
	_, err := MultiPrinter.Start()
	if err != nil {
		return
	}

	// Write script content header
	timestamp := time.Now().Format(timestampFormat)
	headerLine := pterm.Sprintf("%s %s  %s %s\n",
		pterm.FgGray.Sprint(timestamp),
		pterm.FgCyan.Sprint("DEBUG"),
		pterm.FgBlue.Sprint("[yap]"),
		"script content:",
	)
	_, _ = MultiPrinter.Writer.Write([]byte(headerLine))

	normalizedScript := normalizeScriptContent(cmds)
	lines := strings.Split(normalizedScript, "\n")

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed != "" {
			timestamp := time.Now().Format(timestampFormat)
			scriptLine := pterm.Sprintf("%s %s  %s   %s\n",
				pterm.FgGray.Sprint(timestamp),
				pterm.FgCyan.Sprint("DEBUG"),
				pterm.FgBlue.Sprint("[yap]"),
				trimmed,
			)
			_, _ = MultiPrinter.Writer.Write([]byte(scriptLine))
		}
	}
}

// EnhancedProgressBar provides a more precise and wrapped progress bar implementation.
type EnhancedProgressBar struct {
	writer      io.Writer
	packageName string
	title       string
	total       int64
	current     int64
	lastPercent int
	startTime   time.Time
	lastUpdate  time.Time
}

// NewEnhancedProgressBar creates a new enhanced progress bar.
func NewEnhancedProgressBar(writer io.Writer, packageName, title string,
	total int64) *EnhancedProgressBar {
	return &EnhancedProgressBar{
		writer:      writer,
		packageName: packageName,
		title:       title,
		total:       total,
		startTime:   time.Now(),
		lastUpdate:  time.Now(),
		lastPercent: -1,
	}
}

// Update updates the progress bar with new current value.
func (epb *EnhancedProgressBar) Update(current int64) {
	epb.current = current
	percent := int((current * 100) / epb.total)

	// Only update if progress changed by at least 1% or if it's been more than 2 seconds
	now := time.Now()
	if percent != epb.lastPercent || now.Sub(epb.lastUpdate) > 2*time.Second {
		epb.render(percent)
		epb.lastPercent = percent
		epb.lastUpdate = now
	}
}

// Finish completes the progress bar.
func (epb *EnhancedProgressBar) Finish() {
	epb.current = epb.total
	epb.render(100)

	// Log completion
	duration := time.Since(epb.startTime)
	timestamp := time.Now().Format(timestampFormat)

	var completionLine string
	if IsColorDisabled() {
		// Plain text format without colors
		completionLine = fmt.Sprintf("%s %s  [%s] %s completed in %v\n",
			timestamp, logLevelInfo, epb.packageName, epb.title, duration)
	} else {
		// Colored format
		completionLine = pterm.Sprintf("%s %s  %s %s completed in %v\n",
			pterm.FgGray.Sprint(timestamp),
			pterm.FgCyan.Sprint(logLevelInfo),
			pterm.FgYellow.Sprintf("[%s]", epb.packageName),
			epb.title,
			duration,
		)
	}

	_, _ = epb.writer.Write([]byte(completionLine))
}

// render renders the progress bar.
func (epb *EnhancedProgressBar) render(percent int) {
	// Calculate human-readable sizes
	currentSize := formatBytes(epb.current)
	totalSize := formatBytes(epb.total)

	// Calculate speed
	duration := time.Since(epb.startTime)

	var speed string

	if duration.Seconds() > 0 {
		bytesPerSec := float64(epb.current) / duration.Seconds()
		speed = formatBytes(int64(bytesPerSec)) + "/s"
	}

	// Create progress bar visualization
	barWidth := 30
	filled := (percent * barWidth) / 100
	bar := strings.Repeat("█", filled) + strings.Repeat("░", barWidth-filled)

	timestamp := time.Now().Format(timestampFormat)

	var progressLine string
	if IsColorDisabled() {
		// Plain text format without colors
		progressLine = fmt.Sprintf("%s %s  [%s] %s: [%s] %d%% (%s/%s) %s\n",
			timestamp, logLevelInfo, epb.packageName, epb.title, bar, percent, currentSize, totalSize, speed)
	} else {
		// Colored format
		progressLine = pterm.Sprintf("%s %s  %s %s: [%s] %d%% (%s/%s) %s\n",
			pterm.FgGray.Sprint(timestamp),
			pterm.FgCyan.Sprint(logLevelInfo),
			pterm.FgYellow.Sprintf("[%s]", epb.packageName),
			epb.title,
			pterm.FgGreen.Sprint(bar),
			percent,
			currentSize,
			totalSize,
			speed,
		)
	}

	_, _ = epb.writer.Write([]byte(progressLine))
}

// formatBytes formats bytes into human-readable format.
func formatBytes(size int64) string {
	const unit = 1024
	if size < unit {
		return fmt.Sprintf("%d B", size)
	}

	div, exp := int64(unit), 0
	for n := size / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}

	units := []string{"B", "KB", "MB", "GB", "TB"}

	return fmt.Sprintf("%.1f %s", float64(size)/float64(div), units[exp+1])
}

// CheckGO checks if the GO executable is already installed.//
// It does not take any parameters.
// It returns a boolean value indicating whether the GO executable is already installed.
func CheckGO() bool {
	_, err := os.Stat(goExecutable)
	if err == nil {
		Logger.Info("go is already installed")

		return true
	}

	return false
}

// CreateTarZst creates a compressed tar.zst archive from the specified source
// directory. It takes the source directory and the output file path as
// arguments and returns an error if any occurs.
func CreateTarZst(sourceDir, outputFile string, formatGNU bool) error {
	ctx := context.TODO()
	options := &archives.FromDiskOptions{
		FollowSymlinks: false,
	}

	// Retrieve the list of files from the source directory on disk.
	// The map specifies that the files should be read from the sourceDir
	// and the output path in the archive should be empty.
	files, err := archives.FilesFromDisk(ctx, options, map[string]string{
		sourceDir + string(os.PathSeparator): "",
	})
	if err != nil {
		return err
	}

	// Add trailing slashes to directory entries for pacman compatibility
	for i := range files {
		if files[i].IsDir() && !strings.HasSuffix(files[i].NameInArchive, "/") {
			files[i].NameInArchive += "/"
		}
	}

	cleanFilePath := filepath.Clean(outputFile)

	out, err := os.Create(cleanFilePath)
	if err != nil {
		return err
	}

	defer func() {
		err := out.Close()
		if err != nil {
			Logger.Warn("failed to close output file", Logger.Args("path", cleanFilePath, "error", err))
		}
	}()

	format := archives.CompressedArchive{
		Compression: archives.Zstd{},
		Archival: archives.Tar{
			FormatGNU: formatGNU,
			Uid:       0,
			Gid:       0,
			Uname:     "root",
			Gname:     "root",
		},
	}

	return format.Archive(ctx, out, files)
}

// Download downloads a file from the given URL and saves it to the specified destination.
//
// Parameters:
// - destination: the path where the downloaded file will be saved.
// - url: the URL of the file to download.
// - logger: optional component logger for context-aware logging. If nil, uses default logger.
func Download(destination, uri string, logger *ComponentLogger) error {
	// create client
	client := grab.NewClient()

	req, err := grab.NewRequest(destination, uri)
	if err != nil {
		return errors.Errorf("download failed %s", err)
	}

	resp := client.Do(req)
	if resp.HTTPResponse == nil {
		Logger.Fatal("download failed: no response", Logger.Args("error", resp.Err()))
	}

	// Use provided logger or default logger
	activeLogger := logger
	if activeLogger == nil {
		activeLogger = ServiceLogger()
	}

	// start download
	activeLogger.Info("downloading", Logger.Args("url", req.URL()))
	activeLogger.Info("response status: " + resp.HTTPResponse.Status)

	// Start multiprinter for consistent output handling
	_, err = MultiPrinter.Start()
	if err != nil {
		return err
	}

	// Create enhanced progress bar with logger context
	var progressBar *EnhancedProgressBar

	if resp.Size() > 0 {
		// Use logger component as package name if available
		pkgName := "yap"
		if logger != nil && logger.component != "" {
			pkgName = logger.component
		}

		// Extract filename from URI
		srcName := Filename(uri)
		if srcName == "" {
			srcName = "download"
		}

		progressBar = NewEnhancedProgressBar(MultiPrinter.Writer, pkgName, srcName, resp.Size())
	}

	// start UI loop with more frequent updates for precision
	ticker := time.NewTicker(100 * time.Millisecond)

Loop:
	for {
		select {
		case <-resp.Done:
			if progressBar != nil {
				progressBar.Finish()
			}

			ticker.Stop()
			activeLogger.Info("download completed", Logger.Args("path", destination))

			break Loop

		case <-ticker.C:
			if progressBar != nil && resp.Size() > 0 {
				progressBar.Update(resp.BytesComplete())
			}
		}
	}

	return err
}

// GitClone clones a Git repository from the given sourceItemURI to the specified dloadFilePath.
//
// Parameters:
// - sourceItemURI: the URI of the Git repository to clone.
// - dloadFilePath: the file path to clone the repository into.
// - sshPassword: the password for SSH authentication (optional).
// - referenceName: the reference name for the clone operation.
// - logger: optional component logger for context-aware logging. If nil, uses default logger.
func GitClone(dloadFilePath, sourceItemURI, sshPassword string,
	referenceName plumbing.ReferenceName, logger *ComponentLogger,
) error {
	// Start multiprinter for consistent output handling
	_, err := MultiPrinter.Start()
	if err != nil {
		return err
	}

	// Create git progress writer for properly formatted git clone output
	gitProgressWriter := NewGitProgressWriter(MultiPrinter.Writer, "yap")

	cloneOptions := &ggit.CloneOptions{
		Progress: gitProgressWriter,
		URL:      sourceItemURI,
	}

	// If a specific branch or tag is requested, set it as the reference to clone
	if referenceName != "" {
		cloneOptions.ReferenceName = referenceName
		cloneOptions.SingleBranch = true
	}

	plainOpenOptions := &ggit.PlainOpenOptions{
		DetectDotGit:          true,
		EnableDotGitCommonDir: true,
	}

	// Use provided logger or default logger
	activeLogger := logger
	if activeLogger == nil {
		activeLogger = ServiceLogger()
	}

	activeLogger.Info("cloning",
		Logger.Args("repo", sourceItemURI))

	if Exists(dloadFilePath) {
		return handleExistingRepo(dloadFilePath, referenceName, plainOpenOptions)
	}

	repo, err := ggit.PlainClone(dloadFilePath, false, cloneOptions)
	if err != nil && strings.Contains(err.Error(), "authentication required") {
		sourceURL, _ := url.Parse(sourceItemURI)
		sshKeyPath := os.Getenv("HOME") + "/.ssh/id_rsa"

		publicKey, err := ssh.NewPublicKeysFromFile("git", sshKeyPath, sshPassword)
		if err != nil {
			Logger.Error("failed to load ssh key")
			Logger.Warn("try to use an ssh-password with the -p")

			return err
		}

		sshURL := constants.Git + "@" + sourceURL.Hostname() +
			strings.Replace(sourceURL.EscapedPath(), "/", ":", 1)
		cloneOptions.Auth = publicKey
		cloneOptions.URL = sshURL

		repo, err = ggit.PlainClone(dloadFilePath, false, cloneOptions)
		if err != nil {
			return err
		}
	} else if err != nil {
		return err
	}

	// After successful clone, ensure we're on the correct branch if specified
	if referenceName != "" && repo != nil {
		return checkoutReference(repo, referenceName)
	}

	return nil
}

// handleExistingRepo handles the case where a git repository already exists
// and potentially needs to checkout a specific branch or tag.
func handleExistingRepo(dloadFilePath string, referenceName plumbing.ReferenceName,
	plainOpenOptions *ggit.PlainOpenOptions,
) error {
	repo, err := ggit.PlainOpenWithOptions(dloadFilePath, plainOpenOptions)
	if err != nil {
		return err
	}

	if referenceName == "" {
		return nil
	}

	return checkoutReference(repo, referenceName)
}

// checkoutReference attempts to checkout the specified reference,
// fetching it first if necessary.
func checkoutReference(repo *ggit.Repository, referenceName plumbing.ReferenceName) error {
	workTree, err := repo.Worktree()
	if err != nil {
		return err
	}

	branchName := referenceName.Short()

	// First, try to fetch the latest changes from remote
	fetchOptions := &ggit.FetchOptions{}
	_ = repo.Fetch(fetchOptions) // Ignore fetch errors

	// Try to checkout the specified reference directly first
	checkoutOptions := &ggit.CheckoutOptions{
		Branch: referenceName,
	}

	err = workTree.Checkout(checkoutOptions)
	if err == nil {
		return nil // Success
	}

	// If direct checkout fails, the local branch might not exist
	// Try to create a local branch that tracks the remote branch
	remoteBranchRef := plumbing.NewRemoteReferenceName("origin", branchName)

	// Check if the remote branch exists
	remoteRef, err := repo.Reference(remoteBranchRef, true)
	if err != nil {
		return errors.Errorf("remote branch %s not found: %v", branchName, err)
	}

	// Create a new local branch that tracks the remote branch
	localBranchRef := plumbing.NewBranchReferenceName(branchName)
	localRef := plumbing.NewHashReference(localBranchRef, remoteRef.Hash())

	err = repo.Storer.SetReference(localRef)
	if err != nil {
		return err
	}

	// Now checkout the newly created local branch
	checkoutOptions = &ggit.CheckoutOptions{
		Branch: localBranchRef,
	}

	err = workTree.Checkout(checkoutOptions)
	if err != nil {
		return err
	}

	return nil
}

// GOSetup sets up the Go environment.
//
// It checks if Go is installed and if not, it downloads and installs it.
// The function takes no parameters and does not return anything.
func GOSetup() error {
	if CheckGO() {
		return nil
	}

	err := Download(goArchivePath, constants.GoArchiveURL, nil)
	if err != nil {
		Logger.Fatal("download failed",
			Logger.Args("error", err))
	}

	err = Unarchive(goArchivePath, "/usr/lib")
	if err != nil {
		return err
	}

	err = os.Symlink("/usr/lib/go/bin/go", goExecutable)
	if err != nil {
		return err
	}

	err = os.Symlink("/usr/lib/go/bin/gofmt", "/usr/bin/gofmt")
	if err != nil {
		return err
	}

	err = os.RemoveAll(goArchivePath)
	if err != nil {
		return err
	}

	Logger.Info("go successfully installed")

	return err
}

// PullContainers pulls the specified container image.
//
// distro: the name of the container image to pull.
// error: returns an error if the container image cannot be pulled.
func PullContainers(distro string) error {
	var containerApp string

	switch {
	case Exists("/usr/bin/podman"):
		containerApp = "/usr/bin/podman"
	case Exists("/usr/bin/docker"):
		containerApp = "/usr/bin/docker"
	default:
		return errors.Errorf("no container application found")
	}

	args := []string{
		"pull",
		constants.DockerOrg + distro,
	}

	_, err := os.Stat(containerApp)
	if err == nil {
		return Exec(false, "", containerApp, args...)
	}

	return nil
}

// RunScript runs a shell script with logger decorations.
//
// It takes a string parameter `cmds` which represents the shell script to be executed.
// The function returns an error if there was an issue running the script.
func RunScript(cmds string) error {
	return RunScriptWithPackage(cmds, "")
}

// RunScriptWithPackage runs a shell script with package-specific decorations.
//
// It takes a string parameter `cmds` which represents the shell script to be executed
// and an optional `packageName` to decorate output lines with timestamps and package
// identification.
// The function returns an error if there was an issue running the script.
func RunScriptWithPackage(cmds, packageName string) error {
	start := time.Now()

	// Log script execution start
	if packageName != "" {
		Logger.Info("executing shell script", Logger.Args("package", packageName))
	} else {
		Logger.Info("executing shell script")
	}

	// Log script content with proper multiline handling
	if cmds != "" {
		// Use direct writer to avoid pterm's line wrapping
		logScriptContent(cmds)
	}

	// Parse the script
	script, err := syntax.NewParser().Parse(strings.NewReader(cmds), "")
	if err != nil {
		Logger.Error("failed to parse script", Logger.Args("error", err))

		return err
	}

	// Start multiprinter
	_, err = MultiPrinter.Start()
	if err != nil {
		Logger.Error("failed to start multiprinter", Logger.Args("error", err))

		return err
	}

	// Create decorated writer if package name is provided
	writer := MultiPrinter.Writer
	if packageName != "" {
		writer = NewPackageDecoratedWriter(MultiPrinter.Writer, packageName)
	}

	// Create and configure script runner
	runner, err := interp.New(
		interp.Env(expand.ListEnviron(os.Environ()...)),
		interp.StdIO(nil, writer, writer),
	)
	if err != nil {
		Logger.Error("failed to create script runner", Logger.Args("error", err))

		return err
	}

	Logger.Debug("starting script execution")

	// Execute script
	err = runner.Run(context.TODO(), script)
	duration := time.Since(start)

	// Log results with consistent formatting
	if err != nil {
		if packageName != "" {
			Logger.Error("script execution failed",
				Logger.Args("error", err, "duration", duration, "package", packageName))
		} else {
			Logger.Error("script execution failed",
				Logger.Args("error", err, "duration", duration))
		}

		return err
	}

	if packageName != "" {
		Logger.Info("shell script execution completed successfully",
			Logger.Args("duration", duration, "package", packageName))
	} else {
		Logger.Info("shell script execution completed successfully",
			Logger.Args("duration", duration))
	}

	return nil
}

// Unarchive is a function that takes a source file and a destination. It opens
// the source archive file, identifies its format, and extracts it to the
// destination.
//
// Returns an error if there was a problem extracting the files.
func Unarchive(source, destination string) error {
	ctx := context.TODO()

	// Open the source archive file
	archive, err := Open(source)
	if err != nil {
		return err
	}

	// Identify the archive file's format
	format, archiveReader, _ := archives.Identify(ctx, "", archive)

	dirMap := make(map[string]bool)

	// Check if the format is an extractor. If not, skip the archive file.
	extractor, ok := format.(archives.Extractor)

	if !ok {
		return nil
	}

	return extractor.Extract(
		ctx,
		archiveReader,
		func(_ context.Context, archiveFile archives.FileInfo) error {
			fileName := archiveFile.NameInArchive
			newPath := filepath.Join(destination, fileName)

			if archiveFile.IsDir() {
				dirMap[newPath] = true

				return os.MkdirAll(newPath, 0o755) // #nosec
			}

			fileDir := filepath.Dir(newPath)
			_, seenDir := dirMap[fileDir]

			if !seenDir {
				dirMap[fileDir] = true

				_ = os.MkdirAll(fileDir, 0o755) // #nosec
			}

			cleanNewPath := filepath.Clean(newPath)

			newFile, err := os.OpenFile(cleanNewPath,
				os.O_CREATE|os.O_WRONLY,
				archiveFile.Mode())
			if err != nil {
				return err
			}

			defer func() {
				err := newFile.Close()
				if err != nil {
					Logger.Warn("failed to close new file", Logger.Args("path", cleanNewPath, "error", err))
				}
			}()

			archiveFileTemp, err := archiveFile.Open()
			if err != nil {
				return err
			}

			defer func() {
				err := archiveFileTemp.Close()
				if err != nil {
					Logger.Warn("failed to close archive file", Logger.Args("error", err))
				}
			}()

			_, err = io.Copy(newFile, archiveFileTemp)

			return err
		})
}
