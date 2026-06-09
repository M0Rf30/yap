package httpclient

import (
	"context"
	"errors"
	"io"
	"math/rand/v2"
	"net"
	"net/url"
	"sync"
	"syscall"
	"time"

	"github.com/M0Rf30/yap/v2/pkg/i18n"
	"github.com/M0Rf30/yap/v2/pkg/logger"
)

// Retry policy for transient fetch failures. A "logical fetch" (FetchBytes,
// FetchToFile, …) is attempted up to retryAttempts times; between attempts
// the caller sleeps an exponentially growing delay with ±25% jitter so
// concurrent workers hitting the same flaky mirror don't retry in lockstep.
//
// Defaults mirror dnf/apt behaviour for metadata fetches: a blip (connection
// reset, mid-body EOF, 502/503 from an overloaded mirror) is retried; a
// definitive answer (404, 403, checksum/validation failure) is not.
var (
	retryMu        sync.RWMutex
	retryAttempts  = 3
	retryBaseDelay = 500 * time.Millisecond
)

// SetRetryPolicy overrides the global retry policy. attempts is the total
// number of tries (minimum 1); baseDelay is the first backoff interval
// (doubled on each subsequent retry). Intended for tests and callers that
// need faster failure (e.g. offline detection).
func SetRetryPolicy(attempts int, baseDelay time.Duration) {
	retryMu.Lock()
	defer retryMu.Unlock()

	if attempts < 1 {
		attempts = 1
	}

	if baseDelay < 0 {
		baseDelay = 0
	}

	retryAttempts = attempts
	retryBaseDelay = baseDelay
}

// retryPolicy returns the current (attempts, baseDelay) pair.
func retryPolicy() (int, time.Duration) {
	retryMu.RLock()
	defer retryMu.RUnlock()

	return retryAttempts, retryBaseDelay
}

// IsRetryable reports whether err looks like a transient network failure
// worth retrying: transport-level errors (DNS, connection reset/refused,
// timeouts, mid-body EOF) and HTTP 408/429/5xx responses.
//
// Context cancellation, response-size caps, and all other HTTP 4xx codes
// are NOT retryable: they are definitive answers, not blips.
func IsRetryable(err error) bool {
	if err == nil {
		return false
	}

	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return false
	}

	if errors.Is(err, ErrTooLarge) {
		return false
	}

	var statusErr *HTTPStatusError
	if errors.As(err, &statusErr) {
		code := statusErr.Code

		return code == 408 || code == 429 || code >= 500
	}

	// Mid-body truncation (server closed early) surfaces as unexpected EOF.
	if errors.Is(err, io.ErrUnexpectedEOF) || errors.Is(err, io.EOF) {
		return true
	}

	if errors.Is(err, syscall.ECONNRESET) || errors.Is(err, syscall.ECONNREFUSED) ||
		errors.Is(err, syscall.EPIPE) {
		return true
	}

	var netErr net.Error
	if errors.As(err, &netErr) {
		return true
	}

	// Any other transport-level failure (http.Client.Do wraps everything in
	// *url.Error): DNS hiccups, TLS handshake resets, proxy errors.
	var urlErr *url.Error

	return errors.As(err, &urlErr)
}

// WithRetry runs fn until it succeeds, fails with a non-retryable error, or
// the retry budget is exhausted. label identifies the fetch in retry logs
// (typically the URL). The last error is returned verbatim so callers can
// still classify it (errors.As on HTTPStatusError, etc.).
//
// fn must be safe to re-run from scratch: helpers in this package re-issue
// the full request and rewrite output atomically, so a half-read body from
// a failed attempt is never observable.
func WithRetry(ctx context.Context, label string, fn func() error) error {
	attempts, baseDelay := retryPolicy()

	var err error

	for attempt := 1; attempt <= attempts; attempt++ {
		err = fn()
		if err == nil || !IsRetryable(err) {
			return err
		}

		if attempt == attempts {
			break
		}

		delay := backoffDelay(baseDelay, attempt)

		logger.Warn(i18n.T("logger.httpclient.warn.transient_fetch_retry"),
			"url", label,
			"attempt", attempt,
			"max_attempts", attempts,
			"retry_in", delay.Round(time.Millisecond).String(),
			"error", err)

		select {
		case <-ctx.Done():
			return err
		case <-time.After(delay):
		}
	}

	return err
}

// backoffDelay computes the sleep before retry number attempt (1-based):
// baseDelay·2^(attempt−1) with ±25% jitter.
func backoffDelay(baseDelay time.Duration, attempt int) time.Duration {
	if baseDelay <= 0 {
		return 0
	}

	delay := baseDelay << (attempt - 1)

	// ±25% jitter; math/rand is fine here — this is backoff spreading,
	// not a security boundary.
	jitter := time.Duration(rand.Int64N(int64(delay)/2+1)) - delay/4 //nolint:gosec

	return delay + jitter
}
