package options

import (
	"os"
	"path/filepath"
	"slices"

	"github.com/M0Rf30/yap/v2/pkg/errors"
	"github.com/M0Rf30/yap/v2/pkg/i18n"
	"github.com/M0Rf30/yap/v2/pkg/logger"
)

// RemoveEmptyDirs removes empty directories from the package directory,
// mirroring makepkg's !emptydirs option.
//
// Discovery and mutation are kept in separate passes. filepath.WalkDir only
// collects the subdirectories under packageDir (the package root itself is
// never a candidate); nothing is removed while the walk is in progress.
// Removing a directory from inside a filepath.WalkDir callback makes WalkDir
// try to read the directory it just deleted to recurse into it, which fails
// the whole walk with an opaque "no such file or directory" error - that is
// the bug this rewrite fixes.
//
// The collected directories are then checked for removal in the reverse of
// their walk order. filepath.WalkDir visits a directory before any of its
// descendants, and a directory's entire subtree occupies a contiguous run
// immediately after it in the visitation order, so reversing that order
// guarantees every directory is (re-)checked only after all of its
// descendants have already been checked and, if empty, removed. A directory
// that becomes empty solely because such a descendant was just removed is
// therefore still caught in this same pass, so no repeat-until-stable outer
// loop is required.
func RemoveEmptyDirs(packageDir string) error {
	logger.Info(i18n.T("logger.options.info.removing_empty_dirs"))

	dirs, err := collectSubdirs(packageDir)
	if err != nil {
		return err
	}

	for _, path := range slices.Backward(dirs) {
		entries, err := os.ReadDir(path)
		if err != nil {
			return errors.Wrap(err, errors.ErrTypeFileSystem, "failed to read directory").
				WithOperation("RemoveEmptyDirs").
				WithContext("path", path)
		}

		if len(entries) != 0 {
			continue
		}

		logger.Debug(i18n.T("logger.options.debug.removing_empty_directory"), "path", path)

		if err := os.Remove(path); err != nil { //nolint:gosec
			return errors.Wrap(err, errors.ErrTypeFileSystem, "failed to remove empty directory").
				WithOperation("RemoveEmptyDirs").
				WithContext("path", path)
		}
	}

	return nil
}

// collectSubdirs returns every subdirectory of packageDir, excluding the
// root itself, in filepath.WalkDir visitation order.
func collectSubdirs(packageDir string) ([]string, error) {
	var dirs []string

	err := filepath.WalkDir(packageDir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return errors.Wrap(err, errors.ErrTypeFileSystem, "failed to walk package directory").
				WithOperation("RemoveEmptyDirs").
				WithContext("path", path)
		}

		if path == packageDir || !d.IsDir() {
			return nil
		}

		dirs = append(dirs, path)

		return nil
	})
	if err != nil {
		return nil, err
	}

	return dirs, nil
}
