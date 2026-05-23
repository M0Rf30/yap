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

		// Write to /var/lib/apt/lists/.
		encoded := encodeListFilename(src.URL, src.Suite, relPath)
		destPath := filepath.Join(aptListsDir, encoded)
		// #nosec G306 — file permissions 0o644 are appropriate for apt list files
		if err := os.WriteFile(destPath, data, 0o644); err != nil {
			return err
		}

		return nil
	}

	return errors.New(errors.ErrTypeValidation, "no Packages variant found").
		WithOperation("fetchComponentIndex").
		WithContext("component", comp).
		WithContext("arch", arch)
}
