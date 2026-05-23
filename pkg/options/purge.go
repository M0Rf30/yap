package options

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/M0Rf30/yap/v2/pkg/i18n"
	"github.com/M0Rf30/yap/v2/pkg/logger"
)

// purgeTargets mirrors makepkg's PURGE_TARGETS defaults.
var purgeTargets = []string{
	"usr/share/info/dir",
	"usr/local/share/info/dir",
}

// purgeGlobs are glob-matched filenames to remove anywhere in the tree.
var purgeGlobs = []string{
	".packlist",
	"*.pod",
}

// Purge removes files matching PURGE_TARGETS from the package directory,
// mirroring makepkg's purge option.
func Purge(packageDir string) error {
	logger.Info(i18n.T("logger.options.info.purging_files"))

	// Remove exact-path targets.
	for _, target := range purgeTargets {
		path := filepath.Join(packageDir, filepath.FromSlash(target))

		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			return err
		}

		logger.Debug("purged file", "path", path)
	}

	// Remove glob-matched files anywhere in the tree.
	return filepath.WalkDir(packageDir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if d.IsDir() {
			return nil
		}

		base := filepath.Base(path)

		for _, pattern := range purgeGlobs {
			matched, err := filepath.Match(pattern, base)
			if err != nil {
				return err
			}

			if matched {
				logger.Debug("purging file", "path", path)

				//nolint:gosec
				if removeErr := os.Remove(path); removeErr != nil && !os.IsNotExist(removeErr) {
					return removeErr
				}

				break
			}
		}

		// Also remove any file whose path contains the target suffix.
		rel, err := filepath.Rel(packageDir, path)
		if err == nil {
			rel = filepath.ToSlash(rel)

			for _, target := range purgeTargets {
				if strings.HasSuffix(rel, target) {
					_ = os.Remove(path) //nolint:gosec

					break
				}
			}
		}

		return nil
	})
}
