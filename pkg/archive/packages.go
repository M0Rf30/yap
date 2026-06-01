// Package archive provides archive creation and manipulation functionality.
package archive

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/m0rf30/ar"
	rpmutils "github.com/sassoftware/go-rpmutils"

	"github.com/M0Rf30/yap/v2/pkg/errors"
	"github.com/M0Rf30/yap/v2/pkg/i18n"
	"github.com/M0Rf30/yap/v2/pkg/logger"
)

// ExtractDEB extracts a Debian package (.deb) to the destination directory.
// DEB format: AR archive containing control.tar.gz and data.tar.{gz,xz,zst}
// We need to extract data.tar from the AR archive and then extract its contents.
func ExtractDEB(packagePath, destDir string) error {
	file, err := os.Open(packagePath) //nolint:gosec
	if err != nil {
		return errors.Wrap(err, errors.ErrTypeFileSystem, "failed to open DEB package").
			WithContext("path", packagePath).
			WithOperation("ExtractDEB")
	}

	defer func() {
		_ = file.Close()
	}()

	arReader, err := ar.NewReader(file)
	if err != nil {
		return errors.Wrap(err, errors.ErrTypePackaging, "failed to parse AR archive").
			WithOperation("ExtractDEB")
	}

	// Iterate through AR archive members to find data.tar
	var dataTarPath string

	for {
		header, err := arReader.Next()
		if err != nil {
			if err == io.EOF { //nolint:errorlint // io.EOF is not a wrapped error
				break
			}

			return errors.Wrap(err, errors.ErrTypePackaging, "failed to read AR header").
				WithOperation("ExtractDEB")
		}

		// Extract only the data.tar.* file (skip control.tar.* and debian-binary)
		if strings.HasPrefix(header.Name, "data.tar") {
			// Create a temporary file for data.tar
			tmpFile, err := os.CreateTemp("", "data.tar.*")
			if err != nil {
				return errors.Wrap(err, errors.ErrTypeFileSystem, "failed to create temp file").
					WithOperation("ExtractDEB")
			}

			dataTarPath = tmpFile.Name()

			// Copy data.tar to temp file
			if _, err := io.Copy(tmpFile, arReader); err != nil {
				_ = tmpFile.Close()
				_ = os.Remove(dataTarPath) //nolint:gosec

				return errors.Wrap(err, errors.ErrTypeFileSystem, "failed to write temp file").
					WithOperation("ExtractDEB")
			}

			if err := tmpFile.Close(); err != nil {
				_ = os.Remove(dataTarPath) //nolint:gosec

				return errors.Wrap(err, errors.ErrTypeFileSystem, "failed to close temp file").
					WithOperation("ExtractDEB")
			}

			break
		}
	}

	if dataTarPath == "" {
		return errors.New(errors.ErrTypePackaging, "data.tar not found in DEB package").
			WithContext("package", packagePath).
			WithOperation("ExtractDEB")
	}

	defer func() {
		_ = os.Remove(dataTarPath)
	}()

	// Use Extract to handle the tar extraction (with compression auto-detection)
	return Extract(context.Background(), dataTarPath, destDir)
}

// RPM cpio file-type bits (top bits of the mode field). Mirrors stdlib
// syscall.S_IF* constants without depending on platform-specific packages.
const (
	rpmFileTypeMask = 0o170000
	rpmFileTypeReg  = 0o100000
	rpmFileTypeDir  = 0o040000
	rpmFileTypeLink = 0o120000
)

// ExtractRPM extracts an RPM package payload (cpio.{gz,xz,zst,...}) to the
// destination directory using github.com/sassoftware/go-rpmutils.
//
// We iterate the cpio payload via PayloadReaderExtended rather than calling
// ExpandPayload because the bundled cpio.Extract refuses entries whose names
// begin with "/" once destDir is "/" (its containment check compares against
// dest+"/", which becomes "//"). YAP's rpmpack emits absolute paths like
// "/opt/vendor/common/bin/x264", so ExpandPayload errors out with
// 'invalid cpio path "/opt/..."' on every package targeted to /.
//
// Streaming entries ourselves also lets us reuse SafeJoin for
// traversal protection and skip risky entry types (char/block/FIFO).
func ExtractRPM(packagePath, destDir string) error {
	file, err := os.Open(packagePath) //nolint:gosec
	if err != nil {
		return errors.Wrap(err, errors.ErrTypeFileSystem, "failed to open RPM package").
			WithContext("path", packagePath).
			WithOperation("ExtractRPM")
	}

	defer func() {
		_ = file.Close()
	}()

	rpm, err := rpmutils.ReadRpm(file)
	if err != nil {
		return errors.Wrap(err, errors.ErrTypePackaging, "failed to read RPM header").
			WithContext("path", packagePath).
			WithOperation("ExtractRPM")
	}

	pr, err := rpm.PayloadReaderExtended()
	if err != nil {
		return errors.Wrap(err, errors.ErrTypePackaging, "failed to open RPM payload").
			WithContext("package", packagePath).
			WithOperation("ExtractRPM")
	}

	dirMap := make(map[string]bool)

	for {
		fi, err := pr.Next()
		if err == io.EOF { //nolint:errorlint // io.EOF is the sentinel
			return nil
		}

		if err != nil {
			return errors.Wrap(err, errors.ErrTypePackaging, "failed to read RPM payload entry").
				WithContext("package", packagePath).
				WithOperation("ExtractRPM")
		}

		if err := extractRPMEntry(pr, fi, destDir, dirMap); err != nil {
			return errors.Wrap(err, errors.ErrTypePackaging, "failed to extract RPM entry").
				WithContext("package", packagePath).
				WithContext("entry", fi.Name()).
				WithOperation("ExtractRPM")
		}
	}
}

// extractRPMEntry writes one payload entry to destDir, normalising absolute
// cpio paths (".//opt/..." or "/opt/...") to relative joins under destDir and
// rejecting traversal via SafeJoin.
func extractRPMEntry(
	pr rpmutils.PayloadReader,
	fi rpmutils.FileInfo,
	destDir string,
	dirMap map[string]bool,
) error {
	// rpmpack writes absolute names ("/opt/...") while the historical cpio
	// convention is "./opt/...". Strip either form so SafeJoin treats the
	// remainder as a relative entry under destDir.
	name := strings.TrimPrefix(fi.Name(), "./")
	name = strings.TrimPrefix(name, "/")

	if name == "" {
		return nil
	}

	target, err := SafeJoin(destDir, name)
	if err != nil {
		logger.Warn(i18n.T("logger.archive.warn.path_traversal_rejected"), "entry", fi.Name(), "destination", destDir)

		return err
	}

	mode := fi.Mode()
	// (0o777) discards the type bits and any out-of-range data before use.
	perm := os.FileMode(uint32(mode)) & os.ModePerm //nolint:gosec

	switch mode & rpmFileTypeMask {
	case rpmFileTypeDir:
		dirMap[target] = true

		return os.MkdirAll(target, perm|0o100) // ensure exec bit so we can descend

	case rpmFileTypeLink:
		if err := SafeSymlinkTarget(fi.Name(), fi.Linkname()); err != nil {
			logger.Warn(i18n.T("logger.archive.warn.symlink_rejected"), "entry", fi.Name(), "target", fi.Linkname())

			return err
		}

		parent := filepath.Dir(target)
		if _, seen := dirMap[parent]; !seen {
			dirMap[parent] = true
			_ = os.MkdirAll(parent, 0o755)
		}

		_ = os.Remove(target)

		return os.Symlink(fi.Linkname(), target)

	case rpmFileTypeReg:
		// Hardlinks: payload only ships content for the last entry of a link
		// group; preceding entries are empty placeholders. Skip them — the
		// final entry will materialise on disk.
		if pr.IsLink() {
			return nil
		}

		parent := filepath.Dir(target)
		if _, seen := dirMap[parent]; !seen {
			dirMap[parent] = true
			_ = os.MkdirAll(parent, 0o755)
		}

		return writeRPMRegularFile(pr, target, perm)

	default:
		// Skip FIFOs, char/block devices, sockets — neither produced by YAP
		// nor safe to extract under / outside a controlled chroot.
		return nil
	}
}

// writeRPMRegularFile streams the current payload entry's content to path.
func writeRPMRegularFile(pr rpmutils.PayloadReader, path string, perm os.FileMode) error {
	out, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, perm) //nolint:gosec
	if err != nil {
		return errors.Wrap(err, errors.ErrTypeFileSystem, "open file failed").
			WithContext("path", path).
			WithOperation("writeRPMRegularFile")
	}

	defer func() {
		_ = out.Close()
	}()

	if _, err := io.Copy(out, pr); err != nil { //nolint:gosec // size bounded by cpio header
		return errors.Wrap(err, errors.ErrTypeFileSystem, "write file failed").
			WithContext("path", path).
			WithOperation("writeRPMRegularFile")
	}

	return nil
}
