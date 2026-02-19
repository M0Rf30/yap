// Package download provides file download functionality with resume capability,
// concurrent downloads, and progress tracking.
package download

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/cavaliergopher/grab/v3"

	"github.com/M0Rf30/yap/v2/pkg/i18n"
	"github.com/M0Rf30/yap/v2/pkg/logger"
)

// Download downloads a file from the given URL and saves it to the specified destination.
// Uses a simple writer for output.
//
// Parameters:
// - destination: the path where the downloaded file will be saved.
// - url: the URL of the file to download.
// - writer: writer for progress output (can be nil)
func Download(destination, uri string, writer io.Writer) error {
	// create client
	client := grab.NewClient()

	req, err := grab.NewRequest(destination, uri)
	if err != nil {
		return fmt.Errorf(i18n.T("errors.download.download_failed")+" %s", err)
	}

	resp := client.Do(req)
	if resp.HTTPResponse == nil {
		logger.Fatal(i18n.T("errors.download.download_failed_no_response"), "error", resp.Err())
	}

	// start download
	logger.Info("downloading", "url", req.URL())
	logger.Info(i18n.T("logger.download.info.response_status") + resp.HTTPResponse.Status)

	// Create enhanced progress bar using the progress helper
	progressBar := createProgressBar(resp, "yap", "", uri, writer)

	return monitorDownload(resp, progressBar, destination)
}

// WithResume downloads a file with resume capability and retry logic.
// It extends the basic Download function with the ability to resume interrupted downloads.
//
// Parameters:
// - ctx: context for cancellation.
// - destination: the path where the downloaded file will be saved.
// - uri: the URL of the file to download.
// - maxRetries: maximum number of retry attempts (0 = no retries, default: 3).
// - writer: writer for progress output (can be nil)
func WithResume(ctx context.Context, destination, uri string, maxRetries int, writer io.Writer) error {
	if maxRetries <= 0 {
		maxRetries = 3
	}

	var lastErr error

	for attempt := 0; attempt <= maxRetries; attempt++ {
		if attempt > 0 {
			logger.Info(i18n.T("logger.withresume.info.retrying_download_1"),
				"attempt", attempt+1,
				"max_retries", maxRetries+1,
				"url", uri)

			// Exponential backoff: 1s, 2s, 4s, 8s
			backoff := time.Duration(1<<(attempt-1)) * time.Second
			time.Sleep(backoff)
		}

		err := downloadWithResumeInternal(ctx, destination, uri, "", "", writer)
		if err == nil {
			return nil
		}

		lastErr = err

		// Check if it's a retryable error
		if !isRetryableDownloadError(err) {
			break
		}
	}

	return fmt.Errorf(i18n.T("errors.download.download_failed_after_attempts"),
		maxRetries+1, lastErr)
}

// WithResumeContext downloads a file with context information for enhanced
// progress reporting.
//
// Parameters:
//   - destination: local file path where the downloaded content will be saved.
//   - uri: source URL to download from.
//   - maxRetries: maximum number of retry attempts (0 = no retries, default: 3).
//   - packageName: package name for progress reporting (if empty, uses logger component or "yap").
//   - sourceName: source name for progress reporting (if empty, uses filename from URI).
//   - writer: writer for progress output (can be nil)
func WithResumeContext(
	destination, uri string,
	maxRetries int,
	packageName, sourceName string,
	writer io.Writer,
) error {
	if maxRetries <= 0 {
		maxRetries = 3
	}

	var lastErr error

	for attempt := 0; attempt <= maxRetries; attempt++ {
		if attempt > 0 {
			logger.Info(i18n.T("logger.withresumecontext.info.retrying_download_1"),
				"attempt", attempt+1,
				"max_retries", maxRetries+1,
				"url", uri)

			// Exponential backoff: 1s, 2s, 4s, 8s
			backoff := time.Duration(1<<(attempt-1)) * time.Second
			time.Sleep(backoff)
		}

		err := downloadWithResumeInternal(
			context.Background(),
			destination, uri, packageName, sourceName, writer)
		if err == nil {
			return nil
		}

		lastErr = err

		// Check if it's a retryable error
		if !isRetryableDownloadError(err) {
			break
		}
	}

	return fmt.Errorf(i18n.T("errors.download.download_failed_after_attempts"),
		maxRetries+1, lastErr)
}

// downloadWithResumeInternal performs the actual download with resume capability.
func downloadWithResumeInternal(
	ctx context.Context, destination, uri string,
	packageName, sourceName string, writer io.Writer,
) error {
	client, req, err := prepareDownloadRequest(ctx, destination, uri)
	if err != nil {
		return err
	}

	resp := client.Do(req)
	if resp.HTTPResponse == nil {
		return fmt.Errorf("%s", i18n.T("errors.download.download_failed_no_response"))
	}

	logDownloadStart(uri, resp)

	progressBar := createProgressBar(resp, packageName, sourceName, uri, writer)

	return monitorDownload(resp, progressBar, destination)
}

// prepareDownloadRequest creates and configures the download request.
func prepareDownloadRequest(
	ctx context.Context, destination, uri string) (*grab.Client, *grab.Request, error) {
	client := grab.NewClient()
	client.UserAgent = "YAP/1.0 (Yet Another Packager)"

	req, err := grab.NewRequest(destination, uri)
	if err != nil {
		return nil, nil, fmt.Errorf(i18n.T("errors.download.download_failed")+" %s", err)
	}

	configureResumeIfPossible(req, destination, uri)
	req.WithContext(ctx)

	return client, req, nil
}

// configureResumeIfPossible checks for partial files and enables resume.
func configureResumeIfPossible(req *grab.Request, destination, uri string) {
	info, err := os.Stat(destination)
	if err == nil && info.Size() > 0 {
		req.NoResume = false // Enable resume

		logger.Info(i18n.T("logger.configureresumeifpossible.info.resuming_download_1"),
			"url", uri,
			"existing_size", formatBytes(info.Size()))
	}
}

// logDownloadStart logs the initial download information.
func logDownloadStart(uri string, resp *grab.Response) {
	if resp.CanResume {
		logger.Info(i18n.T("logger.logdownloadstart.info.server_supports_resume_1"), "url", uri)
	}

	logger.Info("downloading", "url", resp.Request.URL())
	logger.Info(i18n.T("logger.logdownloadstart.info.response_status") + resp.HTTPResponse.Status)
}

// createProgressBar creates an enhanced progress bar if the response size is known.
func createProgressBar(
	resp *grab.Response, packageName, sourceName, uri string, writer io.Writer,
) *ProgressBar {
	if resp.Size() <= 0 || writer == nil {
		return nil
	}

	pkgName := determinePackageName(packageName)
	srcName := determineSourceName(sourceName, uri)

	return NewProgressBar(writer, pkgName, srcName, resp.Size())
}

// determinePackageName resolves the package name for progress display.
func determinePackageName(packageName string) string {
	if packageName != "" {
		return packageName
	}

	return "yap"
}

// determineSourceName resolves the source name for progress display.
func determineSourceName(sourceName, uri string) string {
	if sourceName != "" {
		return sourceName
	}

	if uri == "" {
		return "download"
	}

	filename := filepath.Base(uri)
	// Handle special cases where filepath.Base doesn't return what we want
	if filename == "." || filename == "/" || strings.HasSuffix(uri, "/") {
		return "download"
	}

	return filename
}

// monitorDownload handles the download monitoring loop.
func monitorDownload(
	resp *grab.Response, progressBar *ProgressBar,
	destination string,
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

			logger.Info(i18n.T("logger.monitordownload.info.download_completed_1"), "path", destination)

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
		"deadline exceeded",
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

// formatBytes formats bytes into human-readable format.
func formatBytes(size int64) string {
	const unit = 1024
	if size < unit {
		return fmt.Sprintf("%d B", size)
	}

	units := []string{"B", "KB", "MB", "GB", "TB", "PB", "EB"}
	div, exp := int64(unit), 0

	for n := size / unit; n >= unit && exp < len(units)-2; n /= unit {
		div *= unit
		exp++
	}

	// Bounds check to prevent index out of range
	if exp+1 >= len(units) {
		exp = len(units) - 2
	}

	return fmt.Sprintf("%.1f %s", float64(size)/float64(div), units[exp+1])
}
