package pacmandb

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
)

func downloadFile(ctx context.Context, url, dest string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, http.NoBody)
	if err != nil {
		return err
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP %d for %s", resp.StatusCode, url)
	}

	// Atomic write: write to tmp, then rename.
	tmpDest := dest + ".tmp"

	f, err := os.Create(tmpDest) // #nosec G304 — dest is constructed from trusted constants
	if err != nil {
		return err
	}

	if _, err := io.Copy(f, resp.Body); err != nil {
		_ = f.Close()
		_ = os.Remove(tmpDest)

		return err
	}

	if err := f.Close(); err != nil {
		_ = os.Remove(tmpDest)
		return err
	}

	return os.Rename(tmpDest, dest)
}
