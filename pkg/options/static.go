package options

import (
	"github.com/M0Rf30/yap/v2/pkg/i18n"
	"github.com/M0Rf30/yap/v2/pkg/logger"
)

// RemoveStatic removes static library .a files from the package directory,
// mirroring makepkg's !static option.
func RemoveStatic(packageDir string) error {
	logger.Info(i18n.T("logger.options.info.removing_static"))

	return removeByExtension(packageDir, ".a", "removing static library file")
}
