package files

import (
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/M0Rf30/yap/v2/pkg/errors"
)

// ResolveSourceDateEpoch returns a deterministic build timestamp for
// reproducible builds.
//
// Resolution order:
//  1. If SOURCE_DATE_EPOCH is already set in the environment, parse and
//     return it (honouring explicit user/CI override).
//  2. Otherwise, use the modification time of the specfile located at
//     specPath — a PKGBUILD for the native dialect, or an nfpm.yaml for an
//     nfpm spec (matching makepkg's PKGBUILD-mtime behaviour). specPath must
//     name the specfile itself, not its containing directory.
//
// An empty specPath, a path that does not exist, or a path that is not a
// regular file (e.g. a directory) all fall back to time.Now() without
// exporting SOURCE_DATE_EPOCH.
//
// The resolved value is also exported into the process environment so
// that child processes (gcc, tar, gzip, …) that natively support
// SOURCE_DATE_EPOCH will use it.
func ResolveSourceDateEpoch(specPath string) (time.Time, error) {
	if env := os.Getenv("SOURCE_DATE_EPOCH"); env != "" {
		epoch, err := strconv.ParseInt(env, 10, 64)
		if err != nil {
			return time.Time{}, errors.Wrap(err, errors.ErrTypeConfiguration,
				fmt.Sprintf("invalid SOURCE_DATE_EPOCH %q", env)).
				WithOperation("ResolveSourceDateEpoch")
		}

		return time.Unix(epoch, 0).UTC(), nil
	}

	info, statErr := os.Stat(specPath)
	if statErr != nil || !info.Mode().IsRegular() {
		// If specPath is empty, doesn't exist, or isn't a regular file (e.g.
		// a directory — the old signature took pkgbuildDir and directories
		// DO have mtimes, so this guard matters), fall back to time.Now()
		// without exporting.
		return time.Now(), nil //nolint:nilerr // intentional fallback
	}

	epoch := info.ModTime().Unix()

	err := os.Setenv("SOURCE_DATE_EPOCH", strconv.FormatInt(epoch, 10))
	if err != nil {
		return time.Time{}, errors.Wrap(err, errors.ErrTypeConfiguration,
			"failed to export SOURCE_DATE_EPOCH").
			WithOperation("ResolveSourceDateEpoch")
	}

	return time.Unix(epoch, 0).UTC(), nil
}

// SourceDateEpochFromEnv reads SOURCE_DATE_EPOCH from the environment and
// returns the corresponding time. If the variable is absent or invalid, it
// returns time.Now(). This is a lightweight reader for use in packaging code
// that runs after ResolveSourceDateEpoch has already been called.
func SourceDateEpochFromEnv() time.Time {
	if env := os.Getenv("SOURCE_DATE_EPOCH"); env != "" {
		epoch, err := strconv.ParseInt(env, 10, 64)
		if err == nil {
			return time.Unix(epoch, 0).UTC()
		}
	}

	return time.Now()
}
