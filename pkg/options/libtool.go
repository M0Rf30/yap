package options

import (
	"os"
	"path/filepath"

	"github.com/M0Rf30/yap/v2/pkg/i18n"
	"github.com/M0Rf30/yap/v2/pkg/logger"
)

// removeByExtension walks packageDir and removes all regular files whose
// extension matches ext (e.g. ".la", ".a").
func removeByExtension(packageDir, ext, logMsg string) error {
	return filepath.WalkDir(packageDir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if !d.IsDir() && filepath.Ext(path) == ext {
			logger.Debug(logMsg, "path", path)

			if err := os.Remove(path); err != nil { // #nosec G122 -- trusted packageDir
				return err
			}
		}

		return nil
	})
}

// RemoveLibtool removes libtool .la files from the package directory,
// mirroring makepkg's !libtool option.
func RemoveLibtool(packageDir string) error {
	logger.Info(i18n.T("logger.options.info.removing_libtool"))

	return removeByExtension(packageDir, ".la", "removing libtool file")
}
