package options

import (
	"os"
	"path/filepath"

	"github.com/M0Rf30/yap/v2/pkg/i18n"
	"github.com/M0Rf30/yap/v2/pkg/logger"
)

// RemoveEmptyDirs removes empty directories from the package directory,
// mirroring makepkg's !emptydirs option.
// It walks bottom-up so that newly emptied parent dirs are also removed.
func RemoveEmptyDirs(packageDir string) error {
	logger.Info(i18n.T("logger.options.info.removing_empty_dirs"))

	// Repeat until no more empty dirs are found (handles nested empty dirs).
	for {
		removed := 0

		err := filepath.WalkDir(packageDir, func(path string, d os.DirEntry, err error) error {
			if err != nil {
				return err
			}

			// Skip the root package dir itself.
			if path == packageDir {
				return nil
			}

			if !d.IsDir() {
				return nil
			}

			entries, err := os.ReadDir(path)
			if err != nil {
				return err
			}

			if len(entries) == 0 {
				logger.Debug("removing empty directory", "path", path)

				if err := os.Remove(path); err != nil { //nolint:gosec
					return err
				}

				removed++
			}

			return nil
		})
		if err != nil {
			return err
		}

		if removed == 0 {
			break
		}
	}

	return nil
}
