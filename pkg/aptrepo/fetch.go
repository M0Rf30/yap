package aptrepo

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"

	"github.com/M0Rf30/yap/v2/pkg/aptcache"
	"github.com/M0Rf30/yap/v2/pkg/errors"
)

// fetchComponentIndex downloads the Packages index for a component+arch combination.
// It tries compression formats in size order: .xz, .gz, .bz2, uncompressed.
func fetchComponentIndex(ctx context.Context, src *aptcache.SourceEntry, comp, arch string, rel *Release) error {
	// Try compression formats in size order: .xz, .gz, .bz2, uncompressed.
	candidates := []string{
		comp + "/binary-" + arch + "/Packages.xz",
		comp + "/binary-" + arch + "/Packages.gz",
		comp + "/binary-" + arch + "/Packages.bz2",
		comp + "/binary-" + arch + "/Packages",
	}

	for _, relPath := range candidates {
		entry, ok := rel.SHA256[relPath]
		if !ok {
			continue
		}

		// Warm-cache fast path: if /var/lib/apt/lists/ already holds a file
		// for this (src, suite, relPath) whose size and SHA256 match the
		// freshly-verified Release entry, skip the download entirely. This
		// avoids re-fetching unchanged Packages.xz files on every refresh —
		// the typical case when the repo's InRelease hasn't moved.
		encoded := encodeListFilename(src.URL, src.Suite, relPath)
		destPath := filepath.Join(aptListsDir, encoded)

		if cached, ok := readMatchingFile(destPath, entry.Size, entry.Hash); ok {
			_ = cached // already on disk and verified; nothing to do

			return nil
		}

		url := strings.TrimRight(src.URL, "/") + "/dists/" + src.Suite + "/" + relPath

		data, err := httpFetch(ctx, url)
		if err != nil {
			continue
		}

		// Verify size + SHA256.
		if int64(len(data)) != entry.Size {
			return errors.New(errors.ErrTypeValidation, "size mismatch").
				WithOperation("fetchComponentIndex").
				WithContext("url", url).
				WithContext("got", len(data)).
				WithContext("expected", entry.Size)
		}

		sum := sha256.Sum256(data)

		got := hex.EncodeToString(sum[:])
		if got != entry.Hash {
			return errors.New(errors.ErrTypeValidation, "SHA256 mismatch").
				WithOperation("fetchComponentIndex").
				WithContext("url", url).
				WithContext("got", got).
				WithContext("expected", entry.Hash)
		}

		if err := os.WriteFile(destPath, data, 0o644); err != nil { //nolint:gosec
			return err
		}

		return nil
	}

	return errors.New(errors.ErrTypeValidation, "no Packages variant found").
		WithOperation("fetchComponentIndex").
		WithContext("component", comp).
		WithContext("arch", arch)
}

// readMatchingFile returns (data, true) when destPath exists on disk with
// the expected size and SHA256. It's the warm-cache shortcut that avoids
// re-downloading unchanged Packages indexes. Returns (nil, false) on any
// stat / read / hash mismatch — the caller will then fall through to the
// normal HTTP fetch path.
func readMatchingFile(destPath string, expectedSize int64, expectedHash string) ([]byte, bool) {
	fi, err := os.Stat(destPath)
	if err != nil || fi.Size() != expectedSize {
		return nil, false
	}

	data, err := os.ReadFile(destPath) //nolint:gosec
	if err != nil {
		return nil, false
	}

	sum := sha256.Sum256(data)
	if hex.EncodeToString(sum[:]) != expectedHash {
		return nil, false
	}

	return data, true
}
