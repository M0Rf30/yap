// Package httpclient is the shared HTTP client used by aptcache, aptrepo,
// apkindex, and pacmandb for short metadata fetches.
//
// It exposes one *http.Client with a global timeout so a stalled mirror
// cannot hang the build, plus helpers for size-capped body reads and a
// 2xx status check. Large package downloads use grab directly and don't
// route through this client.
package httpclient

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"
)

// DefaultMaxBytes caps individual HTTP responses to 2 GiB. Package indexes are
// typically a few MB; .deb / .apk / .rpm files top out around a few hundred
// MB. The cap prevents OOM from a malicious or buggy mirror serving an
// unbounded stream.
const DefaultMaxBytes int64 = 2 << 30 // 2 GiB

// DefaultTimeout bounds a complete HTTP request (connection, TLS handshake,
// header read, body read) at 10 minutes. Large .deb downloads over slow
// links should still fit; stalled mirrors will fail rather than hang.
const DefaultTimeout = 10 * time.Minute

// ErrTooLarge is returned when a response body exceeds the configured cap.
var ErrTooLarge = errors.New("httpclient: response body exceeds size cap")

// HTTPStatusError is returned by CheckStatus when the response is not 2xx.
// Callers can use errors.As to extract the status code without parsing the
// Error() string.
type HTTPStatusError struct {
	Code int
	URL  string
}

// Error returns the error message for HTTPStatusError.
func (e *HTTPStatusError) Error() string {
	return fmt.Sprintf("httpclient: HTTP %d for %s", e.Code, e.URL)
}

// IsClientError reports whether the status is in the 4xx range. Useful for
// classifying transient repo failures (auth, rate-limit, not-found) as
// non-fatal vs systemic failures (network, 5xx, disk full).
func (e *HTTPStatusError) IsClientError() bool {
	return e.Code >= 400 && e.Code < 500
}

var sharedClient = &http.Client{
	Timeout: DefaultTimeout,
	// Redirects: stdlib default (10) is fine.
}

// Client returns the package-wide *http.Client. Reuse by all aptcache /
// aptrepo / apkindex / pacmandb / aptinstall callers so timeout and redirect
// behaviour stays consistent.
func Client() *http.Client { return sharedClient }

// LimitedBody wraps resp.Body in an io.LimitReader with DefaultMaxBytes.
// Callers should still defer resp.Body.Close() on the original response.
func LimitedBody(resp *http.Response) io.Reader {
	return io.LimitReader(resp.Body, DefaultMaxBytes)
}

// LimitedBodyN wraps resp.Body in an io.LimitReader with the supplied cap.
// Use when the caller knows a tighter bound (e.g. control.tar is always
// small). A non-positive cap falls back to DefaultMaxBytes.
func LimitedBodyN(resp *http.Response, maxBytes int64) io.Reader {
	if maxBytes <= 0 {
		maxBytes = DefaultMaxBytes
	}

	return io.LimitReader(resp.Body, maxBytes)
}

// CheckStatus returns a descriptive error if resp.StatusCode is not 2xx.
// The caller still owns resp.Body.Close().
func CheckStatus(resp *http.Response, fetchURL string) error {
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return &HTTPStatusError{Code: resp.StatusCode, URL: fetchURL}
	}

	return nil
}
