package osutils

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"time"
)

// ResolveSourceDateEpoch returns a deterministic build timestamp for
// reproducible builds.
//
// Resolution order:
//  1. If SOURCE_DATE_EPOCH is already set in the environment, parse and
//     return it (honouring explicit user/CI override).
//  2. Otherwise, use the modification time of the PKGBUILD file located
//     in pkgbuildDir (matching makepkg behaviour).
//
// The resolved value is also exported into the process environment so
// that child processes (gcc, tar, gzip, …) that natively support
// SOURCE_DATE_EPOCH will use it.
func ResolveSourceDateEpoch(pkgbuildDir string) (time.Time, error) {
	if env := os.Getenv("SOURCE_DATE_EPOCH"); env != "" {
		epoch, err := strconv.ParseInt(env, 10, 64)
		if err != nil {
			return time.Time{}, fmt.Errorf("invalid SOURCE_DATE_EPOCH %q: %w", env, err)
		}

		return time.Unix(epoch, 0).UTC(), nil
	}

	pkgbuildPath := filepath.Join(pkgbuildDir, "PKGBUILD")

	info, statErr := os.Stat(pkgbuildPath)
	if statErr != nil {
		// If the PKGBUILD file doesn't exist (e.g. during tests or when
		// Home is empty), fall back to time.Now() without exporting.
		return time.Now(), nil //nolint:nilerr // intentional fallback
	}

	epoch := info.ModTime().Unix()

	err := os.Setenv("SOURCE_DATE_EPOCH", strconv.FormatInt(epoch, 10))
	if err != nil {
		return time.Time{}, fmt.Errorf("failed to export SOURCE_DATE_EPOCH: %w", err)
	}

	return time.Unix(epoch, 0).UTC(), nil
}
