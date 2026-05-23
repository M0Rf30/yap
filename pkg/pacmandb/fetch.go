package pacmandb

import (
	"context"
	"io"
	"net/http"
	"os"

	"github.com/M0Rf30/yap/v2/pkg/errors"
	"github.com/M0Rf30/yap/v2/pkg/httpclient"
)

// maxPacmanDBBytes caps a downloaded <repo>.db (or .db.sig) at 256 MiB.
// Real Arch / extra / community DBs are well under 50 MB.
const maxPacmanDBBytes = 256 << 20

func downloadFile(ctx context.Context, url, dest string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, http.NoBody)
	if err != nil {
		return err
	}

	resp, err := httpclient.Client().Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()

	if err := httpclient.CheckStatus(resp, url); err != nil {
		return err
	}

	if resp.ContentLength > maxPacmanDBBytes {
		return errors.New(errors.ErrTypeValidation, "response body too large").
			WithOperation("downloadFile").
			WithContext("url", url).
			WithContext("size", resp.ContentLength).
			WithContext("cap", maxPacmanDBBytes)
	}

	// Atomic write: write to tmp, then rename.
	tmpDest := dest + ".tmp"

	f, err := os.Create(tmpDest) // #nosec G304 — dest is constructed from trusted constants
	if err != nil {
		return err
	}

	written, err := io.Copy(f, io.LimitReader(resp.Body, maxPacmanDBBytes+1))
	if err != nil {
		_ = f.Close()
		_ = os.Remove(tmpDest)

		return err
	}

	if written > maxPacmanDBBytes {
		_ = f.Close()
		_ = os.Remove(tmpDest)

		return errors.New(errors.ErrTypeValidation, "downloaded size exceeded cap").
			WithOperation("downloadFile").
			WithContext("url", url).
			WithContext("size", written).
			WithContext("cap", maxPacmanDBBytes)
	}

	if err := f.Close(); err != nil {
		_ = os.Remove(tmpDest)
		return err
	}

	return os.Rename(tmpDest, dest)
}
