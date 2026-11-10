// SPDX-FileCopyrightText: 2021-2026 Gianluca Boiano
// SPDX-License-Identifier: GPL-3.0-only

package aptinstall

import (
	"archive/tar"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/M0Rf30/pkgformat/safepath"

	"github.com/M0Rf30/yap/v2/pkg/errors"
	"github.com/M0Rf30/yap/v2/pkg/i18n"
	"github.com/M0Rf30/yap/v2/pkg/logger"
)

// extractDataTar extracts the data archive of a .deb to the destination directory.
// It handles conffile collisions: if a conffile already exists on disk, it is NOT
// overwritten (dpkg behavior with DEBIAN_FRONTEND=noninteractive).
func extractDataTar(debPath, destDir string, conffiles []string) error {
	deb, closeFile, err := openDEB(debPath)
	if err != nil {
		return err
	}

	defer closeFile()

	data, err := deb.OpenData()
	if err != nil {
		return errors.Wrap(err, errors.ErrTypeParser, "open data archive").
			WithOperation("extractDataTar").WithContext("path", debPath)
	}

	defer func() { _ = data.Close() }()

	return extractTarWithConffiles(data, destDir, conffiles)
}

// extractTarWithConffiles extracts a decompressed data tar stream, skipping
// conffiles that already exist.
func extractTarWithConffiles(data io.Reader, destDir string, conffiles []string) error {
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

	tr := tar.NewReader(data)
	dirMap := make(map[string]bool)

	for {
		hdr, err := tr.Next()
		if err != nil {
			if err == io.EOF { //nolint:errorlint
				break
			}

			return errors.Wrap(err, errors.ErrTypeParser, "read tar entry").
				WithOperation("extractTarWithConffiles")
		}

		// Strip leading "./" from tar entry names.
		path := strings.TrimPrefix(hdr.Name, "./")
		if path == "" {
			continue
		}

		// Compute the destination path while rejecting traversal attempts.
		fullPath, err := safepath.Join(destDir, path)
		if err != nil {
			logger.Warn(i18n.T("logger.aptinstall.warn.skipping_path_traversal_attempt"), "path", path, "error", err)

			continue
		}

		fullPath, err = confineEntry(destDir, fullPath, path, hdr.Typeflag)
		if err != nil {
			logger.Warn(i18n.T("logger.aptinstall.warn.skipping_path_traversal_attempt"),
				"path", path, "error", err)

			continue
		}

		if err := extractTarEntry(tr, hdr, destDir, fullPath, conffileSet, dirMap); err != nil {
			return err
		}
	}

	return nil
}

// confineEntry re-resolves fullPath so no pre-existing symlink in destDir can
// redirect the write outside it. For root "/" the lexical path is kept. For
// directories the whole path is resolved (usrmerge links like lib -> usr/lib
// keep working); for other entries only the parent is resolved so the final
// component is never followed.
func confineEntry(destDir, fullPath, rel string, typeflag byte) (string, error) {
	if filepath.Clean(destDir) == "/" {
		return fullPath, nil
	}

	if typeflag == tar.TypeDir {
		return safepath.ResolveInRoot(destDir, rel)
	}

	parent, err := safepath.ResolveInRoot(destDir, filepath.Dir(rel))
	if err != nil {
		return "", err
	}

	return filepath.Join(parent, filepath.Base(fullPath)), nil
}

// tarMode returns the permission bits plus setuid/setgid/sticky from hdr.
func tarMode(hdr *tar.Header) os.FileMode {
	m := hdr.FileInfo().Mode()

	return m.Perm() | m&(os.ModeSetuid|os.ModeSetgid|os.ModeSticky)
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
	case tar.TypeLink:
		return extractTarHardlink(hdr, destDir, fullPath, dirMap)
	case tar.TypeReg, tar.TypeRegA: //nolint:staticcheck
		return extractTarFile(tr, hdr, fullPath, dirMap, conffileSet)
	default:
		// Skip other types (devices, fifos, etc.).
		return nil
	}
}

// extractTarDir creates a directory from a tar entry.
func extractTarDir(hdr *tar.Header, fullPath string, dirMap map[string]bool) error {
	dirMap[fullPath] = true

	// nolint:gosec // G301: mode is from tar header, constrained by safepath.Join
	if err := os.MkdirAll(fullPath, tarMode(hdr)); err != nil {
		return errors.Wrap(err, errors.ErrTypeFileSystem, "mkdir").
			WithOperation("extractTarDir").WithContext("path", fullPath)
	}

	if err := os.Chmod(fullPath, tarMode(hdr)); err != nil {
		return errors.Wrap(err, errors.ErrTypeFileSystem, "chmod dir").
			WithOperation("extractTarDir").WithContext("path", fullPath)
	}

	return nil
}

// extractTarSymlink creates a symlink from a tar entry after validating the
// link target cannot escape destDir.
func extractTarSymlink(hdr *tar.Header, destDir, fullPath string, dirMap map[string]bool) error {
	if err := safepath.SymlinkTarget(destDir, fullPath, hdr.Linkname); err != nil {
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

// extractTarHardlink materialises a tar hardlink entry (typeflag '1').
//
// Debian packages use hardlinks for multi-call binaries — bzip2 ships
// ./bin/bunzip2 as the regular file and ./bin/bzip2, ./bin/bzcat as
// hardlinks to it. Dropping these entries (the previous behaviour) left
// the package "installed" with its main binary absent.
//
// Linkname is a path relative to the archive root, so it is joined with
// destDir through the same containment check as the entry itself. When
// os.Link fails (target skipped as an existing conffile, cross-device
// destDir, filesystem without hardlink support) the target is copied
// instead: a divergent copy beats a missing file.
func extractTarHardlink(hdr *tar.Header, destDir, fullPath string, dirMap map[string]bool) error {
	target, err := safepath.Join(destDir, strings.TrimPrefix(hdr.Linkname, "./"))
	if err == nil && filepath.Clean(destDir) != "/" {
		target, err = safepath.ResolveInRoot(destDir, strings.TrimPrefix(hdr.Linkname, "./"))
	}

	if err != nil {
		logger.Warn(i18n.T("logger.aptinstall.warn.skipping_path_traversal_attempt"),
			"path", hdr.Name, "error", err)

		return nil
	}

	parentDir := filepath.Dir(fullPath)
	if _, seen := dirMap[parentDir]; !seen {
		dirMap[parentDir] = true
		_ = os.MkdirAll(parentDir, 0o755)
	}

	_ = os.Remove(fullPath)

	if err := os.Link(target, fullPath); err == nil {
		return nil
	}

	if err := copyFile(target, fullPath, tarMode(hdr)); err != nil { //nolint:gosec
		return errors.Wrap(err, errors.ErrTypeFileSystem, "hardlink").
			WithOperation("extractTarHardlink").
			WithContext("path", fullPath).
			WithContext("target", target)
	}

	return nil
}

// copyFile duplicates src to dst with the given mode.
func copyFile(src, dst string, mode os.FileMode) error {
	in, err := os.Open(src) //nolint:gosec // path validated by safepath.Join
	if err != nil {
		return err
	}

	defer func() { _ = in.Close() }()

	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, mode) //nolint:gosec
	if err != nil {
		return err
	}

	if _, err := io.Copy(out, in); err != nil { //nolint:gosec // trusted package payload
		_ = out.Close()

		return err
	}

	if err := out.Close(); err != nil {
		return err
	}

	return os.Chmod(dst, mode)
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

	// Never write through a symlink at the final component.
	if fi, lerr := os.Lstat(fullPath); lerr == nil && fi.Mode()&os.ModeSymlink != 0 {
		_ = os.Remove(fullPath)
	}

	// Create the file.
	// nolint:gosec // G304: fullPath is constrained by safepath.Join; G306: mode is from tar header
	f, err := os.OpenFile(fullPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, tarMode(hdr))
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

	if err := os.Chmod(fullPath, tarMode(hdr)); err != nil {
		return errors.Wrap(err, errors.ErrTypeFileSystem, "chmod file").
			WithOperation("extractTarFile").WithContext("path", fullPath)
	}

	return nil
}

// fileExists checks if a file exists on disk.
func fileExists(path string) bool {
	_, err := os.Stat(path)

	return err == nil
}
