package httpclient

import (
	"context"
	"io"
	"net/http"
	"os"
	"time"

	"github.com/M0Rf30/yap/v2/pkg/errors"
)

// FetchBytesConditional is like FetchBytes but sends If-Modified-Since when
// ifModSince is non-zero. On HTTP 304 it returns (nil, true, nil) so the
// caller can use its on-disk copy. On 200 it returns (data, false, nil).
// On any other error it returns (nil, false, err).
//
// This is the warm-cache primitive that lets dnfcache / aptrepo / aptcache
// skip re-downloading metadata files whose upstream hasn't moved.
//
// Transient failures (network errors, HTTP 5xx) are retried per the
// package retry policy.
func FetchBytesConditional(
	ctx context.Context,
	url string,
	maxBytes int64,
	ifModSince time.Time,
) (body []byte, notModified bool, err error) {
	if maxBytes <= 0 {
		maxBytes = DefaultMaxBytes
	}

	err = WithRetry(ctx, url, func() error {
		body, notModified, err = fetchBytesConditionalOnce(ctx, url, maxBytes, ifModSince)

		return err
	})

	return body, notModified, err
}

// fetchBytesConditionalOnce performs a single conditional GET attempt.
func fetchBytesConditionalOnce(
	ctx context.Context,
	url string,
	maxBytes int64,
	ifModSince time.Time,
) (body []byte, notModified bool, err error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, http.NoBody)
	if err != nil {
		return nil, false, err
	}

	if !ifModSince.IsZero() {
		// RFC 7232 If-Modified-Since uses HTTP-date in UTC.
		req.Header.Set("If-Modified-Since", ifModSince.UTC().Format(http.TimeFormat))
	}

	resp, err := Client().Do(req)
	if err != nil {
		return nil, false, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode == http.StatusNotModified {
		return nil, true, nil
	}

	if err := CheckStatus(resp, url); err != nil {
		return nil, false, err
	}

	data, err := io.ReadAll(io.LimitReader(resp.Body, maxBytes+1))
	if err != nil {
		return nil, false, err
	}

	if int64(len(data)) > maxBytes {
		return nil, false, errors.Wrap(ErrTooLarge, errors.ErrTypeValidation, "response body exceeds size cap").
			WithOperation("FetchBytesConditional").
			WithContext("url", url).
			WithContext("cap", maxBytes)
	}

	return data, false, nil
}

// FetchBytes downloads url and returns the body, capped at maxBytes.
// A non-positive maxBytes falls back to DefaultMaxBytes.
// Transient failures are retried per the package retry policy.
func FetchBytes(ctx context.Context, url string, maxBytes int64) ([]byte, error) {
	if maxBytes <= 0 {
		maxBytes = DefaultMaxBytes
	}

	var data []byte

	err := WithRetry(ctx, url, func() error {
		var err error

		data, err = fetchBytesOnce(ctx, url, maxBytes)

		return err
	})

	return data, err
}

// fetchBytesOnce performs a single GET attempt.
func fetchBytesOnce(ctx context.Context, url string, maxBytes int64) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, http.NoBody)
	if err != nil {
		return nil, err
	}

	resp, err := Client().Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	if err := CheckStatus(resp, url); err != nil {
		return nil, err
	}

	data, err := io.ReadAll(io.LimitReader(resp.Body, maxBytes+1))
	if err != nil {
		return nil, err
	}

	if int64(len(data)) > maxBytes {
		return nil, errors.Wrap(ErrTooLarge, errors.ErrTypeValidation, "response body exceeds size cap").
			WithOperation("FetchBytes").
			WithContext("url", url).
			WithContext("cap", maxBytes)
	}

	return data, nil
}

// FetchToFile downloads url and writes it atomically to destPath, capped at maxBytes.
// Content-Length is checked as a cheap preflight when available.
// A non-positive maxBytes falls back to DefaultMaxBytes.
// Transient failures are retried per the package retry policy; the atomic
// temp-file + rename write guarantees a failed attempt leaves no partial file.
func FetchToFile(ctx context.Context, url, destPath string, maxBytes int64) error {
	if maxBytes <= 0 {
		maxBytes = DefaultMaxBytes
	}

	return WithRetry(ctx, url, func() error {
		return fetchToFileOnce(ctx, url, destPath, maxBytes)
	})
}

// fetchToFileOnce performs a single download attempt.
func fetchToFileOnce(ctx context.Context, url, destPath string, maxBytes int64) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, http.NoBody)
	if err != nil {
		return err
	}

	resp, err := Client().Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()

	if err := CheckStatus(resp, url); err != nil {
		return err
	}

	// Cheap preflight: reject if Content-Length already exceeds cap.
	if resp.ContentLength > 0 && resp.ContentLength > maxBytes {
		return errors.Wrap(ErrTooLarge, errors.ErrTypeValidation, "response body exceeds size cap").
			WithOperation("FetchToFile").
			WithContext("url", url).
			WithContext("content-length", resp.ContentLength).
			WithContext("cap", maxBytes)
	}

	return AtomicWrite(destPath, func(w io.Writer) error {
		written, err := io.Copy(w, io.LimitReader(resp.Body, maxBytes+1))
		if err != nil {
			return err
		}

		if written > maxBytes {
			return errors.Wrap(ErrTooLarge, errors.ErrTypeValidation, "response body exceeds size cap").
				WithOperation("FetchToFile").
				WithContext("url", url).
				WithContext("cap", maxBytes)
		}

		return nil
	})
}

// AtomicWrite writes to destPath via a temp file + rename so readers never
// see a partial file. fn receives a writer for the temp file; if fn returns
// an error the temp file is removed.
func AtomicWrite(destPath string, fn func(io.Writer) error) error {
	tmpPath := destPath + ".tmp"

	f, err := os.Create(tmpPath) //nolint:gosec
	if err != nil {
		return err
	}

	if err := fn(f); err != nil {
		_ = f.Close()
		_ = os.Remove(tmpPath)

		return err
	}

	if err := f.Close(); err != nil {
		_ = os.Remove(tmpPath)
		return err
	}

	if err := os.Rename(tmpPath, destPath); err != nil {
		_ = os.Remove(tmpPath)
		return err
	}

	return nil
}
