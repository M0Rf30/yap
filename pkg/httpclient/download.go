package httpclient

import (
	"context"
	"io"
	"net/http"
	"os"

	"github.com/M0Rf30/yap/v2/pkg/errors"
)

// FetchBytes downloads url and returns the body, capped at maxBytes.
// A non-positive maxBytes falls back to DefaultMaxBytes.
func FetchBytes(ctx context.Context, url string, maxBytes int64) ([]byte, error) {
	if maxBytes <= 0 {
		maxBytes = DefaultMaxBytes
	}

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
func FetchToFile(ctx context.Context, url, destPath string, maxBytes int64) error {
	if maxBytes <= 0 {
		maxBytes = DefaultMaxBytes
	}

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
