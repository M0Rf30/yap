package options

import (
	"compress/gzip"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/M0Rf30/yap/v2/pkg/i18n"
	"github.com/M0Rf30/yap/v2/pkg/logger"
)

// manDirs mirrors makepkg's MAN_DIRS.
var manDirs = []string{
	"usr/share/man",
	"usr/share/info",
	"usr/local/share/man",
	"usr/local/share/info",
}

// ZipMan compresses man and info pages with gzip,
// mirroring makepkg's zipman option.
func ZipMan(packageDir string) error {
	logger.Info(i18n.T("logger.options.info.compressing_man_pages"))

	for _, dir := range manDirs {
		target := filepath.Join(packageDir, filepath.FromSlash(dir))

		if _, err := os.Lstat(target); os.IsNotExist(err) {
			continue
		}

		var links []string

		err := filepath.WalkDir(target, func(path string, d os.DirEntry, err error) error {
			if err != nil {
				return err
			}

			if d.IsDir() {
				return nil
			}

			// Never follow symlinks: the target may already have been
			// compressed (dangling link) or live on the build host.
			if d.Type()&fs.ModeSymlink != 0 {
				links = append(links, path)

				return nil
			}

			// Skip already-compressed files.
			if strings.HasSuffix(path, ".gz") || strings.HasSuffix(path, ".bz2") ||
				strings.HasSuffix(path, ".xz") || strings.HasSuffix(path, ".zst") {
				return nil
			}

			return gzipFile(path)
		})
		if err != nil {
			return err
		}

		for _, link := range links {
			if err := retargetManSymlink(link); err != nil {
				return err
			}
		}
	}

	return nil
}

// retargetManSymlink re-points a relative symlink at the gzipped copy of its
// target (as makepkg does), renaming the link to carry the .gz suffix. Links
// whose target was not compressed inside the package are left untouched.
func retargetManSymlink(link string) error {
	dest, err := os.Readlink(link)
	if err != nil {
		return err
	}

	if filepath.IsAbs(dest) || strings.HasSuffix(dest, ".gz") {
		return nil
	}

	resolved := filepath.Join(filepath.Dir(link), dest) + ".gz"

	info, err := os.Lstat(resolved)
	if err != nil || !info.Mode().IsRegular() {
		return nil //nolint:nilerr // target was not compressed; leave link alone
	}

	if err := os.Symlink(dest+".gz", link+".gz"); err != nil {
		return err
	}

	return os.Remove(link)
}

// gzipFile compresses a single file in-place, replacing it with a .gz version.
func gzipFile(path string) error {
	logger.Debug(i18n.T("logger.options.debug.compressing_man_page"), "path", path)

	in, err := os.Open(path) //nolint:gosec
	if err != nil {
		return err
	}

	defer func() {
		_ = in.Close()
	}()

	outPath := path + ".gz"

	out, err := os.Create(outPath) //nolint:gosec
	if err != nil {
		return err
	}

	defer func() {
		_ = out.Close()
	}()

	gz, err := gzip.NewWriterLevel(out, gzip.BestCompression)
	if err != nil {
		return err
	}

	if _, err := io.Copy(gz, in); err != nil {
		_ = gz.Close()

		return err
	}

	if err := gz.Close(); err != nil {
		return err
	}

	// Preserve original permissions on the compressed file.
	info, err := os.Stat(path)
	if err == nil {
		_ = os.Chmod(outPath, info.Mode()) //nolint:gosec
	}

	return os.Remove(path)
}
