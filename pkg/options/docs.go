package options

import (
	"os"
	"path/filepath"

	"github.com/M0Rf30/yap/v2/pkg/i18n"
	"github.com/M0Rf30/yap/v2/pkg/logger"
)

// docDirs mirrors makepkg's DOC_DIRS: directories considered documentation.
var docDirs = []string{
	"usr/share/doc",
	"usr/share/gtk-doc",
	"usr/local/share/doc",
	"usr/local/share/gtk-doc",
}

// RemoveDocs removes documentation directories from the package directory,
// mirroring makepkg's !docs option.
func RemoveDocs(packageDir string) error {
	logger.Info(i18n.T("logger.options.info.removing_docs"))

	for _, dir := range docDirs {
		target := filepath.Join(packageDir, filepath.FromSlash(dir))

		info, err := os.Lstat(target)
		if os.IsNotExist(err) {
			continue
		}

		if err != nil {
			return err
		}

		if !info.IsDir() {
			continue
		}

		logger.Debug("removing doc directory", "path", target)

		if err := os.RemoveAll(target); err != nil {
			return err
		}
	}

	return nil
}
