// SPDX-FileCopyrightText: 2021-2026 Gianluca Boiano
// SPDX-License-Identifier: GPL-3.0-only

// Package fetch is the application front end of pkgformat/download: it owns the
// source-download retry budget (flag/environment), renders the progress bar, and
// wraps failures with yap's error and logging conventions.
package fetch

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/M0Rf30/pkgformat/download"
	"github.com/M0Rf30/pkgformat/httpclient"

	"github.com/M0Rf30/yap/v2/pkg/errors"
	"github.com/M0Rf30/yap/v2/pkg/i18n"
	"github.com/M0Rf30/yap/v2/pkg/logger"
)

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

// userAgent identifies yap on source and toolchain downloads.
const userAgent = "YAP/1.0 (Yet Another Packager)"

// defaultBackoffBase is the first retry delay; it doubles on every retry.
const defaultBackoffBase = time.Second

// Options configures File.
type Options struct {
	// MaxRetries is the number of retries after the first attempt; 0 means no
	// retries. Pass MaxRetries() for the configured budget.
	MaxRetries int
	// BackoffBase is the first retry delay; zero selects one second.
	BackoffBase time.Duration
	// PackageName labels the progress bar; empty selects "yap".
	PackageName string
	// SourceName titles the progress bar; empty uses the file name of the URL.
	SourceName string
	// Progress receives the progress bar; nil disables it.
	Progress io.Writer
}

// File downloads uri to destination with resume, retry and context-aware
// progress reporting. Cancelling ctx aborts the in-flight transfer and any
// pending retry backoff; the partial file is kept for the next run.
func File(ctx context.Context, destination, uri string, opts Options) error {
	retries := max(opts.MaxRetries, 0)

	backoff := opts.BackoffBase
	if backoff <= 0 {
		backoff = defaultBackoffBase
	}

	client := httpclient.New(httpclient.Options{
		Timeout:   -1,
		UserAgent: userAgent,
		Logger:    logger.Slog(),
		Retry:     httpclient.RetryPolicy{Attempts: retries + 1, BaseDelay: backoff},
	})

	var bar *ProgressBar

	req := download.Request{
		URL:  uri,
		Dest: destination,
		Progress: func(done, total int64) {
			if opts.Progress == nil || total <= 0 {
				return
			}

			if bar == nil {
				bar = NewProgressBar(opts.Progress,
					determinePackageName(opts.PackageName),
					determineSourceName(opts.SourceName, uri), total)
			}

			bar.Update(done)
		},
	}

	logger.Info(i18n.T("logger.download.info.downloading"), "url", uri)

	if err := download.Download(ctx, req, download.Options{Client: client}); err != nil {
		if bar != nil {
			bar.Abort(bar.current)
		}

		return errors.Wrap(err, errors.ErrTypeNetwork,
			fmt.Sprintf(i18n.T("errors.download.download_failed_after_attempts"), retries+1)).
			WithOperation("File").
			WithContext("uri", uri).
			WithContext("max_retries", retries)
	}

	if bar != nil {
		bar.Finish()
	}

	logger.Info(i18n.T("logger.download.info.download_completed"), "path", destination)

	return nil
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
