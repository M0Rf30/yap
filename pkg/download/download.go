// Package download provides file download functionality with resume capability,
// concurrent downloads, and progress tracking.
package download

import (
	"context"
	stderrors "errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/cavaliergopher/grab/v3"

	"github.com/M0Rf30/yap/v2/pkg/errors"
	"github.com/M0Rf30/yap/v2/pkg/httpclient"
	"github.com/M0Rf30/yap/v2/pkg/i18n"
	"github.com/M0Rf30/yap/v2/pkg/logger"
)

// backoffBase is the first retry delay; it doubles on each subsequent
// attempt (1s, 2s, 4s, …). Package-level var so tests can shrink it.
var backoffBase = time.Second

// DefaultMaxRetries is the retry budget used when nothing overrides it.
const DefaultMaxRetries = 3

// MaxRetriesEnv names the environment variable that overrides DefaultMaxRetries.
const MaxRetriesEnv = "YAP_SOURCE_MAX_RETRIES"

var (
	maxRetriesMu   sync.RWMutex
	maxRetries     = DefaultMaxRetries
	maxRetriesOnce sync.Once
)

// MaxRetries returns the configured retry budget: retries after the first
// attempt (so total attempts == MaxRetries()+1). Resolution order is an
// explicit SetMaxRetries call, then the YAP_SOURCE_MAX_RETRIES environment
// variable, then DefaultMaxRetries. The environment variable is resolved
// lazily (on first call, not at package init) because i18n/logger are not
// ready at init time.
func MaxRetries() int {
	maxRetriesOnce.Do(resolveMaxRetriesFromEnv)

	maxRetriesMu.RLock()
	defer maxRetriesMu.RUnlock()

	return maxRetries
}

// SetMaxRetries overrides the budget; negative values clamp to 0. Consumes
// the same lazy-init guard as MaxRetries so an explicit override (e.g. from
// a CLI flag) is never clobbered by a later environment resolution.
func SetMaxRetries(retries int) {
	maxRetriesOnce.Do(func() {})

	if retries < 0 {
		retries = 0
	}

	maxRetriesMu.Lock()
	defer maxRetriesMu.Unlock()

	maxRetries = retries
}

// resolveMaxRetriesFromEnv applies YAP_SOURCE_MAX_RETRIES over
// DefaultMaxRetries; an empty, unparseable, or negative value keeps the
// default (silently for empty, with a warning otherwise).
func resolveMaxRetriesFromEnv() {
	raw := os.Getenv(MaxRetriesEnv)

	value, ok := parseMaxRetriesEnv(raw)
	if !ok {
		if strings.TrimSpace(raw) != "" {
			logger.Warn(i18n.T("logger.download.warn.invalid_max_retries"),
				"value", raw, "default", DefaultMaxRetries)
		}

		return
	}

	maxRetriesMu.Lock()
	defer maxRetriesMu.Unlock()

	maxRetries = value
}

// parseMaxRetriesEnv parses raw as a non-negative retry count. An empty or
// invalid value reports ok=false so the caller keeps the current default.
func parseMaxRetriesEnv(raw string) (int, bool) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return 0, false
	}

	value, err := strconv.Atoi(trimmed)
	if err != nil || value < 0 {
		return 0, false
	}

	return value, true
}

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
		return errors.Wrap(err, errors.ErrTypeNetwork, i18n.T("errors.download.download_failed")).
			WithOperation("Download").
			WithContext("uri", uri)
	}

	resp := client.Do(req)
	if resp.HTTPResponse == nil {
		return errors.New(errors.ErrTypeNetwork, i18n.T("errors.download.download_failed_no_response")).
			WithOperation("Download").
			WithContext("uri", uri).
			WithContext("error", resp.Err())
	}

	// start download
	logger.Info(i18n.T("logger.download.info.downloading"), "url", req.URL())
	logger.Info(i18n.T("logger.download.info.response_status"), "status", resp.HTTPResponse.Status)

	// Create enhanced progress bar using the progress helper
	progressBar := createProgressBar(resp, "yap", "", uri, writer)

	return monitorDownload(resp, progressBar, destination)
}

// WithResume downloads a file with resume capability and retry logic.
// It extends the basic Download function with the ability to resume interrupted downloads.
//
//   - ctx: context for cancellation.
//   - destination: the path where the downloaded file will be saved.
//   - uri: the URL of the file to download.
//   - maxRetries: retries after the first attempt; 0 = no retries; pass
//     MaxRetries() for the configured budget.
//   - writer: writer for progress output (can be nil)
func WithResume(ctx context.Context, destination, uri string, maxRetries int, writer io.Writer) error {
	return retryDownload(ctx, destination, uri, maxRetries, "", "", writer,
		"WithResume", "logger.download.info.retrying_download")
}

// WithResumeContext downloads a file with context information for enhanced
// progress reporting.
//
//   - destination: local file path where the downloaded content will be saved.
//   - uri: source URL to download from.
//   - maxRetries: retries after the first attempt; 0 = no retries; pass
//     MaxRetries() for the configured budget.
//   - packageName: package name for progress reporting (if empty, uses logger component or "yap").
//   - sourceName: source name for progress reporting (if empty, uses filename from URI).
//   - writer: writer for progress output (can be nil)
func WithResumeContext(
	destination, uri string,
	maxRetries int,
	packageName, sourceName string,
	writer io.Writer,
) error {
	return retryDownload(context.Background(), destination, uri, maxRetries,
		packageName, sourceName, writer,
		"WithResumeContext", "logger.download.info.retrying_download_2")
}

// retryDownload is the attempt loop shared by WithResume and
// WithResumeContext: exponential backoff between attempts, retry only on
// transient errors, and partial-file cleanup when the partial cannot be
// resumed (length mismatch).
func retryDownload(
	ctx context.Context, destination, uri string, maxRetries int,
	packageName, sourceName string, writer io.Writer,
	op, retryMsgID string,
) error {
	if maxRetries < 0 {
		maxRetries = 0
	}

	var lastErr error

	for attempt := 0; attempt <= maxRetries; attempt++ {
		if attempt > 0 {
			logger.Info(i18n.T(retryMsgID),
				"attempt", attempt+1,
				"max_retries", maxRetries+1,
				"url", uri)

			// Exponential backoff: 1s, 2s, 4s, 8s — abandoned early when
			// the context is cancelled.
			backoff := backoffBase << (attempt - 1)

			select {
			case <-ctx.Done():
				return errors.Wrap(lastErr, errors.ErrTypeNetwork,
					i18n.T("errors.download.download_failed")).
					WithOperation(op).
					WithContext("uri", uri)
			case <-time.After(backoff):
			}
		}

		err := downloadWithResumeInternal(ctx, destination, uri, packageName, sourceName, writer)
		if err == nil {
			return nil
		}

		lastErr = err

		// Check if it's a retryable error
		if !IsRetryableGrabError(err) {
			break
		}

		// A length mismatch means the on-disk partial cannot be resumed
		// (half-synced mirror or corrupt partial) — clear it so the next
		// attempt starts from scratch instead of failing the same way.
		if stderrors.Is(err, grab.ErrBadLength) {
			_ = os.Remove(destination)
		}
	}

	return errors.Wrap(lastErr, errors.ErrTypeNetwork,
		fmt.Sprintf(i18n.T("errors.download.download_failed_after_attempts"), maxRetries+1)).
		WithOperation(op).
		WithContext("uri", uri).
		WithContext("max_retries", maxRetries)
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
		return errors.New(errors.ErrTypeNetwork, i18n.T("errors.download.download_failed_no_response")).
			WithOperation("downloadWithResumeInternal").
			WithContext("uri", uri)
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
		return nil, nil, errors.Wrap(err, errors.ErrTypeNetwork, i18n.T("errors.download.download_failed")).
			WithOperation("prepareDownloadRequest").
			WithContext("uri", uri)
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

		logger.Info(i18n.T("logger.download.info.resuming_download"),
			"url", uri,
			"existing_size", formatBytes(info.Size()))
	}
}

// logDownloadStart logs the initial download information.
func logDownloadStart(uri string, resp *grab.Response) {
	if resp.CanResume {
		logger.Info(i18n.T("logger.download.info.server_supports_resume"), "url", uri)
	}

	logger.Info(i18n.T("logger.download.info.downloading"), "url", resp.Request.URL())
	logger.Info(i18n.T("logger.download.info.response_status_2"), "status", resp.HTTPResponse.Status)
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
	const downloadDefault = "download"

	if sourceName != "" {
		return sourceName
	}

	if uri == "" {
		return downloadDefault
	}

	filename := filepath.Base(uri)
	// Handle special cases where filepath.Base doesn't return what we want
	if filename == "." || filename == "/" || strings.HasSuffix(uri, "/") {
		return downloadDefault
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

			logger.Info(i18n.T("logger.download.info.download_completed"), "path", destination)

			return nil

		case <-ticker.C:
			if progressBar != nil && resp.Size() > 0 {
				progressBar.Update(resp.BytesComplete())
			}
		}
	}
}

// IsRetryableGrabError determines if a grab-based download error is
// transient and worth retrying. Typed checks run first: context
// cancellation is never retried; grab status codes retry on 408/429/5xx;
// transport-level errors (timeouts, resets, truncated bodies) defer to
// httpclient.IsRetryable. A conservative string fallback catches errors
// that lost their type through fmt-style wrapping.
//
// Exported so other grab-based downloaders (e.g. pkg/aptcache's .deb
// fetch) can reuse the same classification instead of duplicating it.
func IsRetryableGrabError(err error) bool {
	if err == nil {
		return false
	}

	if stderrors.Is(err, context.Canceled) || stderrors.Is(err, context.DeadlineExceeded) {
		return false
	}

	if statusErr, ok := stderrors.AsType[grab.StatusCodeError](err); ok {
		code := int(statusErr)

		return code == http.StatusRequestTimeout ||
			code == http.StatusTooManyRequests ||
			code >= http.StatusInternalServerError
	}

	// Length mismatch: half-synced mirror or corrupt partial; the retry
	// loop clears the partial file and starts fresh.
	if stderrors.Is(err, grab.ErrBadLength) {
		return true
	}

	// Transport-level classification (net.Error, url.Error, unexpected
	// EOF, ECONNRESET/ECONNREFUSED, …).
	if httpclient.IsRetryable(err) {
		return true
	}

	// Fallback for errors that lost their type through string wrapping.
	errStr := strings.ToLower(err.Error())
	for _, retryable := range []string{
		"connection reset",
		"connection refused",
		"timeout",
		"temporary failure",
		"network is unreachable",
		"no route to host",
		"unexpected eof",
	} {
		if strings.Contains(errStr, retryable) {
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
