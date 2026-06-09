package aptinstall

import (
	"archive/tar"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/m0rf30/ar"

	"github.com/M0Rf30/yap/v2/pkg/errors"
	"github.com/M0Rf30/yap/v2/pkg/i18n"
	"github.com/M0Rf30/yap/v2/pkg/logger"
	"github.com/M0Rf30/yap/v2/pkg/safepath"
)

// extractDataTar extracts the data.tar member of a .deb to the destination directory.
// It handles conffile collisions: if a conffile already exists on disk, it is NOT
// overwritten (dpkg behavior with DEBIAN_FRONTEND=noninteractive).
func extractDataTar(debPath, destDir string, conffiles []string) error {
	file, err := os.Open(debPath) //nolint:gosec
	if err != nil {
		return errors.Wrap(err, errors.ErrTypeFileSystem, "open DEB").
			WithOperation("extractDataTar").WithContext("path", debPath)
	}

	defer func() { _ = file.Close() }()

	arReader, err := ar.NewReader(file)
	if err != nil {
		return errors.Wrap(err, errors.ErrTypeParser, "parse AR archive").
			WithOperation("extractDataTar").WithContext("path", debPath)
	}

	var dataTarPath string

	for {
		header, err := arReader.Next()
		if err != nil {
			if err == io.EOF { //nolint:errorlint
				break
			}

			return errors.Wrap(err, errors.ErrTypeParser, "read AR header").
				WithOperation("extractDataTar")
		}

		if strings.HasPrefix(header.Name, "data.tar") {
			// Create a temporary file for data.tar.
			tmpFile, err := os.CreateTemp("", "data.tar.*")
			if err != nil {
				return errors.Wrap(err, errors.ErrTypeFileSystem, "create temp file").
					WithOperation("extractDataTar")
			}

			dataTarPath = tmpFile.Name()

			// Copy data.tar to temp file.
			if _, err := io.Copy(tmpFile, arReader); err != nil {
				_ = tmpFile.Close()
				_ = os.Remove(dataTarPath)

				return errors.Wrap(err, errors.ErrTypeFileSystem, "write temp file").
					WithOperation("extractDataTar")
			}

			if err := tmpFile.Close(); err != nil {
				_ = os.Remove(dataTarPath)

				return errors.Wrap(err, errors.ErrTypeFileSystem, "close temp file").
					WithOperation("extractDataTar")
			}

			break
		}
	}

	if dataTarPath == "" {
		return errors.New(errors.ErrTypeParser, "data.tar not found in DEB").
			WithOperation("extractDataTar").WithContext("path", debPath)
	}

	defer func() {
		_ = os.Remove(dataTarPath)
	}()

	// Now extract the data.tar, handling conffiles.
	return extractDataTarWithConffiles(dataTarPath, destDir, conffiles)
}

// safeJoin joins destDir with a tar-entry path, rejecting anything that
// escapes destDir via "..", absolute paths, or prefix-aliasing
// (`destDir="/tmp/foo"`, entry resolves to "/tmp/foobar/evil").
// Containment logic lives in pkg/safepath.
func safeJoin(destDir, entry string) (string, error) {
	return safepath.Join(destDir, entry)
}

// safeSymlinkTarget validates that a symlink's target stays under destDir.
// Absolute targets are permitted (common in Debian packages; they only
// resolve at runtime); relative targets are resolved against the symlink's
// own location. Containment logic lives in pkg/safepath.
func safeSymlinkTarget(destDir, linkPath, target string) error {
	return safepath.SymlinkTarget(destDir, linkPath, target)
}

// extractDataTarWithConffiles extracts a data.tar file, skipping conffiles that already exist.
func extractDataTarWithConffiles(dataTarPath, destDir string, conffiles []string) error {
	// Build a set of conffiles for quick lookup. Conffile entries in the
	// control file are absolute paths (e.g. "/etc/apt/sources.list"); we
	// store both the original form and the destDir-rooted form so the
	// lookup works whether destDir is "/" (real install) or a fakeroot.
	conffileSet := make(map[string]bool)

	for _, cf := range conffiles {
		cf = strings.TrimSpace(cf)
		if cf == "" {
			continue
		}

		conffileSet[cf] = true

		// Also store the destDir-rooted form for the post-join lookup.
		// e.g. destDir="/fakeroot" + cf="/etc/foo" → "/fakeroot/etc/foo".
		joined := filepath.Clean(filepath.Join(destDir, strings.TrimPrefix(cf, "/")))
		conffileSet[joined] = true
	}

	// Open and decompress the data.tar.
	file, err := os.Open(dataTarPath) //nolint:gosec
	if err != nil {
		return errors.Wrap(err, errors.ErrTypeFileSystem, "open data.tar").
			WithOperation("extractDataTarWithConffiles").WithContext("path", dataTarPath)
	}

	defer func() { _ = file.Close() }()

	decompressed, err := decompressStream(file, dataTarPath)
	if err != nil {
		return errors.Wrap(err, errors.ErrTypeParser, "decompress data.tar").
			WithOperation("extractDataTarWithConffiles")
	}

	defer func() { _ = decompressed.Close() }()

	tr := tar.NewReader(decompressed)
	dirMap := make(map[string]bool)

	for {
		hdr, err := tr.Next()
		if err != nil {
			if err == io.EOF { //nolint:errorlint
				break
			}

			return errors.Wrap(err, errors.ErrTypeParser, "read tar entry").
				WithOperation("extractDataTarWithConffiles")
		}

		// Strip leading "./" from tar entry names.
		path := strings.TrimPrefix(hdr.Name, "./")
		if path == "" {
			continue
		}

		// Compute the destination path while rejecting traversal attempts.
		fullPath, err := safeJoin(destDir, path)
		if err != nil {
			logger.Warn(i18n.T("logger.aptinstall.warn.skipping_path_traversal_attempt"), "path", path, "error", err)

			continue
		}

		if err := extractTarEntry(tr, hdr, destDir, fullPath, conffileSet, dirMap); err != nil {
			return err
		}
	}

	return nil
}

// extractTarEntry dispatches tar entry extraction based on type.
func extractTarEntry(
	tr *tar.Reader,
	hdr *tar.Header,
	destDir, fullPath string,
	conffileSet, dirMap map[string]bool,
) error {
	switch hdr.Typeflag {
	case tar.TypeDir:
		return extractTarDir(hdr, fullPath, dirMap)
	case tar.TypeSymlink:
		return extractTarSymlink(hdr, destDir, fullPath, dirMap)
	case tar.TypeReg, tar.TypeRegA: //nolint:staticcheck
		return extractTarFile(tr, hdr, fullPath, dirMap, conffileSet)
	default:
		// Skip other types (hardlinks, devices, etc.).
		return nil
	}
}

// extractTarDir creates a directory from a tar entry.
func extractTarDir(hdr *tar.Header, fullPath string, dirMap map[string]bool) error {
	dirMap[fullPath] = true

	// nolint:gosec // G301: mode is from tar header, constrained by safeJoin
	if err := os.MkdirAll(fullPath, os.FileMode(hdr.Mode)); err != nil {
		return errors.Wrap(err, errors.ErrTypeFileSystem, "mkdir").
			WithOperation("extractTarDir").WithContext("path", fullPath)
	}

	return nil
}

// extractTarSymlink creates a symlink from a tar entry after validating the
// link target cannot escape destDir.
func extractTarSymlink(hdr *tar.Header, destDir, fullPath string, dirMap map[string]bool) error {
	if err := safeSymlinkTarget(destDir, fullPath, hdr.Linkname); err != nil {
		logger.Warn(i18n.T("logger.aptinstall.warn.skipping_unsafe_symlink"),
			"path", hdr.Name, "target", hdr.Linkname, "error", err)

		return nil
	}

	// Remove existing symlink/file.
	_ = os.Remove(fullPath)

	// Create parent directory if needed.
	parentDir := filepath.Dir(fullPath)
	if _, seen := dirMap[parentDir]; !seen {
		dirMap[parentDir] = true
		_ = os.MkdirAll(parentDir, 0o755)
	}

	if err := os.Symlink(hdr.Linkname, fullPath); err != nil {
		return errors.Wrap(err, errors.ErrTypeFileSystem, "symlink").
			WithOperation("extractTarSymlink").WithContext("path", fullPath)
	}

	return nil
}

// extractTarFile extracts a regular file from a tar entry, respecting conffiles.
//
// `conffileSet` is keyed by both the absolute path as listed in the control
// file (e.g. "/etc/apt/sources.list") and the destDir-rooted form
// (e.g. "/fakeroot/etc/apt/sources.list") so the lookup works regardless of
// whether destDir is "/" or a sandbox. Previously the code looked up
// `"/"+filepath.Base(fullPath)` which never matched any real conffile and
// caused every conffile to be silently overwritten on upgrade.
func extractTarFile(
	tr *tar.Reader,
	hdr *tar.Header,
	fullPath string,
	dirMap, conffileSet map[string]bool,
) error {
	if conffileSet[fullPath] && fileExists(fullPath) {
		logger.Info(i18n.T("logger.aptinstall.info.skipping_existing_conffile"), "path", fullPath)

		return nil
	}

	// Create parent directory if needed.
	parentDir := filepath.Dir(fullPath)
	if _, seen := dirMap[parentDir]; !seen {
		dirMap[parentDir] = true
		// nolint:gosec // G301: intermediate dirs need read+exec
		_ = os.MkdirAll(parentDir, 0o755)
	}

	// Create the file.
	// nolint:gosec // G304: fullPath is constrained by safeJoin; G306: mode is from tar header
	f, err := os.OpenFile(fullPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, os.FileMode(hdr.Mode))
	if err != nil {
		return errors.Wrap(err, errors.ErrTypeFileSystem, "create file").
			WithOperation("extractTarFile").WithContext("path", fullPath)
	}

	// Limit file size to prevent decompression bombs (2GB max per file).
	const maxFileSize = 2 * 1024 * 1024 * 1024

	if _, err := io.Copy(f, io.LimitReader(tr, maxFileSize)); err != nil {
		_ = f.Close()

		return errors.Wrap(err, errors.ErrTypeFileSystem, "write file").
			WithOperation("extractTarFile").WithContext("path", fullPath)
	}

	if err := f.Close(); err != nil {
		return errors.Wrap(err, errors.ErrTypeFileSystem, "close file").
			WithOperation("extractTarFile").WithContext("path", fullPath)
	}

	return nil
}

// fileExists checks if a file exists on disk.
func fileExists(path string) bool {
	_, err := os.Stat(path)

	return err == nil
}
