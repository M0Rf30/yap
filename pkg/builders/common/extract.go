// Package common provides shared interfaces and base implementations for package builders.
package common

import (
	"archive/tar"
	"bufio"
	"compress/gzip"
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/m0rf30/ar"
	rpmutils "github.com/sassoftware/go-rpmutils"

	"github.com/M0Rf30/yap/v2/pkg/archive"
	"github.com/M0Rf30/yap/v2/pkg/constants"
	"github.com/M0Rf30/yap/v2/pkg/errors"
	"github.com/M0Rf30/yap/v2/pkg/i18n"
	"github.com/M0Rf30/yap/v2/pkg/logger"
)

// ExtractToRoot extracts a package directly to the root filesystem (/).
// This extracts the package contents to the actual filesystem without
// using a sysroot directory.
func (bb *BaseBuilder) ExtractToRoot(packagePath string) error {
	// Get package info for logging
	pkgInfo, _ := os.Stat(packagePath)

	var packageSize int64

	if pkgInfo != nil {
		packageSize = pkgInfo.Size()
	}

	logger.Debug(i18n.T("logger.extract.extracting_package"),
		"package", filepath.Base(packagePath),
		"package_size_mb", packageSize/(1024*1024),
		"format", bb.Format)

	var extractErr error

	switch bb.Format {
	case constants.FormatDEB:
		// DEB packages need special handling to extract data.tar from AR archive
		extractErr = ExtractDEB(packagePath, "/")
	case constants.FormatRPM:
		// RPM format: header + cpio payload — the generic archive identifier
		// does not recognize RPM, so we must decode the payload ourselves.
		extractErr = extractRPM(packagePath, "/")
	case constants.FormatAPK:
		// APK is a concatenation of (optional sig.tar.gz +) control.tar.gz +
		// data.tar.gz. The generic archive extractor only sees the first gzip
		// member, silently dropping the data payload. Walk every gzip stream
		// explicitly and skip control entries.
		extractErr = extractAPK(packagePath, "/")
	case constants.FormatPacman:
		// Pacman packages are plain tar.zst; the generic extractor works.
		// Clean up metadata files afterwards.
		extractErr = archive.Extract(context.Background(), packagePath, "/")
		if extractErr == nil {
			cleanupMetadataFiles(bb.Format)
		}
	default:
		return errors.New(errors.ErrTypePackaging, "unsupported package format for extraction").
			WithContext("format", bb.Format).
			WithOperation("ExtractToRoot")
	}

	if extractErr != nil {
		return errors.Wrap(extractErr, errors.ErrTypePackaging, "failed to extract package").
			WithContext("package", packagePath).
			WithContext("format", bb.Format).
			WithOperation("ExtractToRoot")
	}

	logger.Info(i18n.T("logger.extract.package_extracted"),
		"package", filepath.Base(packagePath),
		"format", bb.Format)

	return nil
}

// cleanupMetadataFiles removes package metadata files that were extracted to
// root. APK metadata is handled in-line by extractAPK (which never writes
// control entries to disk), so this function only handles Pacman.
func cleanupMetadataFiles(format string) {
	var metadataPatterns []string

	if format == constants.FormatPacman {
		metadataPatterns = []string{
			"/.PKGINFO",
			"/.BUILDINFO",
			"/.MTREE",
			"/.INSTALL",
		}
	}

	for _, pattern := range metadataPatterns {
		// Handle glob patterns
		if strings.Contains(pattern, "*") {
			matches, err := filepath.Glob(pattern)
			if err != nil {
				continue
			}

			for _, match := range matches {
				_ = os.Remove(match) // Best-effort removal
			}
		} else {
			_ = os.Remove(pattern) // Best-effort removal
		}
	}
}

// ExtractDEB extracts a Debian package (.deb) to the destination directory.
// DEB format: AR archive containing control.tar.gz and data.tar.{gz,xz,zst}
// We need to extract data.tar from the AR archive and then extract its contents.
func ExtractDEB(packagePath, destDir string) error {
	file, err := os.Open(packagePath) // #nosec G304 - packagePath is from trusted build artifacts
	if err != nil {
		return errors.Wrap(err, errors.ErrTypeFileSystem, "failed to open DEB package").
			WithContext("path", packagePath).
			WithOperation("extractDEB")
	}

	defer func() {
		_ = file.Close()
	}()

	arReader, err := ar.NewReader(file)
	if err != nil {
		return errors.Wrap(err, errors.ErrTypePackaging, "failed to parse AR archive").
			WithOperation("extractDEB")
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
				WithOperation("extractDEB")
		}

		// Extract only the data.tar.* file (skip control.tar.* and debian-binary)
		if strings.HasPrefix(header.Name, "data.tar") {
			// Create a temporary file for data.tar
			tmpFile, err := os.CreateTemp("", "data.tar.*")
			if err != nil {
				return errors.Wrap(err, errors.ErrTypeFileSystem, "failed to create temp file").
					WithOperation("extractDEB")
			}

			dataTarPath = tmpFile.Name()

			// Copy data.tar to temp file
			if _, err := io.Copy(tmpFile, arReader); err != nil {
				_ = tmpFile.Close()
				_ = os.Remove(dataTarPath) // #nosec G703 -- path comes from os.CreateTemp, not user input //nolint:gosec

				return errors.Wrap(err, errors.ErrTypeFileSystem, "failed to write temp file").
					WithOperation("extractDEB")
			}

			if err := tmpFile.Close(); err != nil {
				_ = os.Remove(dataTarPath) // #nosec G703 -- path comes from os.CreateTemp, not user input //nolint:gosec

				return errors.Wrap(err, errors.ErrTypeFileSystem, "failed to close temp file").
					WithOperation("extractDEB")
			}

			break
		}
	}

	if dataTarPath == "" {
		return errors.New(errors.ErrTypePackaging, "data.tar not found in DEB package").
			WithContext("package", packagePath).
			WithOperation("extractDEB")
	}

	defer func() {
		_ = os.Remove(dataTarPath)
	}()

	// Use archive.Extract to handle the tar extraction (with compression auto-detection)
	return archive.Extract(context.Background(), dataTarPath, destDir)
}

// isAPKControlEntry reports whether the given tar entry name belongs to APK
// metadata streams (signature / control / scriptlets) and must NOT be
// extracted to the live filesystem.
func isAPKControlEntry(name string) bool {
	// strip a leading "./" or "/" so the prefix checks work regardless of
	// archive convention.
	trimmed := strings.TrimPrefix(name, "./")
	trimmed = strings.TrimPrefix(trimmed, "/")

	return strings.HasPrefix(trimmed, ".PKGINFO") ||
		strings.HasPrefix(trimmed, ".SIGN") ||
		strings.HasPrefix(trimmed, ".pre-") ||
		strings.HasPrefix(trimmed, ".post-") ||
		strings.HasPrefix(trimmed, ".install") ||
		strings.HasPrefix(trimmed, ".trigger")
}

// extractAPK walks every concatenated gzip stream inside an APK package and
// extracts data entries to destDir, skipping APK control/signature metadata.
//
// APK layout (Alpine spec): [signature.tar.gz +] control.tar.gz + data.tar.gz.
// The mholt/archives extractor stops after the first gzip member, so the
// data payload was previously dropped silently. This is the APK analogue of
// the RPM regression fixed by extractRPM.
func extractAPK(packagePath, destDir string) error {
	file, err := os.Open(packagePath) // #nosec G304 - packagePath is from trusted build artifacts
	if err != nil {
		return errors.Wrap(err, errors.ErrTypeFileSystem, "failed to open APK package").
			WithContext("path", packagePath).
			WithOperation("extractAPK")
	}

	defer func() {
		_ = file.Close()
	}()

	br := bufio.NewReader(file)

	gz, err := gzip.NewReader(br)
	if err != nil {
		return errors.Wrap(err, errors.ErrTypePackaging, "failed to open APK gzip stream").
			WithContext("path", packagePath).
			WithOperation("extractAPK")
	}

	defer func() {
		_ = gz.Close()
	}()

	// Disable transparent multistream so we can iterate gzip members
	// explicitly and apply per-member tar parsing.
	gz.Multistream(false)

	dirMap := make(map[string]bool)

	for memberIdx := 0; ; memberIdx++ {
		if err := extractAPKMember(gz, destDir, dirMap); err != nil {
			return err
		}

		// Advance to the next concatenated gzip member; io.EOF means we
		// finished the last one.
		if err := gz.Reset(br); err != nil {
			if err == io.EOF { //nolint:errorlint // io.EOF is the sentinel, not a wrapped error
				return nil
			}

			return errors.Wrap(err, errors.ErrTypePackaging, "failed to advance APK gzip member").
				WithContext("path", packagePath).
				WithContext("member", memberIdx).
				WithOperation("extractAPK")
		}

		gz.Multistream(false)
	}
}

// extractAPKMember reads tar entries from a single gzip member and writes
// non-control files to destDir.
//
//nolint:gocyclo,cyclop // tar entry dispatch is inherently branchy
func extractAPKMember(r io.Reader, destDir string, dirMap map[string]bool) error {
	tr := tar.NewReader(r)

	for {
		hdr, err := tr.Next()
		if err == io.EOF { //nolint:errorlint // io.EOF is the sentinel
			return nil
		}

		if err != nil {
			return errors.Wrap(err, errors.ErrTypePackaging, "failed to read APK tar entry").
				WithOperation("extractAPKMember")
		}

		if isAPKControlEntry(hdr.Name) {
			continue
		}

		cleanPath, err := archive.SafeJoin(destDir, hdr.Name)
		if err != nil {
			logger.Warn(i18n.T("logger.archive.warn.path_traversal_rejected"),
				"entry", hdr.Name, "destination", destDir)

			return err
		}

		switch hdr.Typeflag {
		case tar.TypeDir:
			dirMap[cleanPath] = true
			if err := os.MkdirAll(cleanPath, apkTarMode(hdr.Mode, 0o755)); err != nil {
				return errors.Wrap(err, errors.ErrTypeFileSystem, "mkdir failed").
					WithContext("path", cleanPath).
					WithOperation("extractAPKMember")
			}

		case tar.TypeSymlink:
			if err := archive.SafeSymlinkTarget(hdr.Name, hdr.Linkname); err != nil {
				logger.Warn(i18n.T("logger.archive.warn.symlink_rejected"),
					"entry", hdr.Name, "target", hdr.Linkname)

				return err
			}

			_ = os.Remove(cleanPath)

			if err := os.Symlink(hdr.Linkname, cleanPath); err != nil {
				return errors.Wrap(err, errors.ErrTypeFileSystem, "symlink failed").
					WithContext("path", cleanPath).
					WithOperation("extractAPKMember")
			}

		case tar.TypeReg, tar.TypeRegA: //nolint:staticcheck // TypeRegA is deprecated but still appears in older tarballs
			fileDir := filepath.Dir(cleanPath)
			if _, seen := dirMap[fileDir]; !seen {
				dirMap[fileDir] = true
				_ = os.MkdirAll(fileDir, 0o755) // #nosec G301 -- intermediate dirs need read+exec for installed binaries/libs
			}

			if err := writeAPKRegularFile(cleanPath, hdr, tr); err != nil {
				return err
			}

		default:
			// Skip hardlinks, char/block devices, FIFOs — not produced by YAP
			// and rarely meaningful when extracted to a foreign root.
			continue
		}
	}
}

// apkTarMode safely converts a tar header mode (int64) to an os.FileMode,
// masking to the permission bits and falling back to the supplied default if
// the input is out of range. Defends against gosec G115 (int64 -> uint32).
func apkTarMode(mode int64, fallback os.FileMode) os.FileMode {
	if mode < 0 || mode > 0o7777 {
		return fallback
	}

	return os.FileMode(uint32(mode))
}

// writeAPKRegularFile creates a regular file at path with content streamed
// from tr, honoring the tar header mode.
func writeAPKRegularFile(path string, hdr *tar.Header, tr io.Reader) error {
	// #nosec G304 -- path is constrained by archive.SafeJoin to stay inside destDir.
	out, err := os.OpenFile(path,
		os.O_CREATE|os.O_WRONLY|os.O_TRUNC,
		apkTarMode(hdr.Mode, 0o644))
	if err != nil {
		return errors.Wrap(err, errors.ErrTypeFileSystem, "open file failed").
			WithContext("path", path).
			WithOperation("writeAPKRegularFile")
	}

	defer func() {
		_ = out.Close()
	}()

	if _, err := io.Copy(out, tr); err != nil { //nolint:gosec // size bounded by tar header
		return errors.Wrap(err, errors.ErrTypeFileSystem, "write file failed").
			WithContext("path", path).
			WithOperation("writeAPKRegularFile")
	}

	return nil
}

// RPM cpio file-type bits (top bits of the mode field). Mirrors stdlib
// syscall.S_IF* constants without depending on platform-specific packages.
const (
	rpmFileTypeMask = 0o170000
	rpmFileTypeReg  = 0o100000
	rpmFileTypeDir  = 0o040000
	rpmFileTypeLink = 0o120000
)

// extractRPM extracts an RPM package payload (cpio.{gz,xz,zst,...}) to the
// destination directory using github.com/sassoftware/go-rpmutils.
//
// We iterate the cpio payload via PayloadReaderExtended rather than calling
// ExpandPayload because the bundled cpio.Extract refuses entries whose names
// begin with "/" once destDir is "/" (its containment check compares against
// dest+"/", which becomes "//"). YAP's rpmpack emits absolute paths like
// "/opt/zextras/common/bin/x264", so ExpandPayload errors out with
// 'invalid cpio path "/opt/..."' on every package targeted to /.
//
// Streaming entries ourselves also lets us reuse archive.SafeJoin for
// traversal protection and skip risky entry types (char/block/FIFO).
func extractRPM(packagePath, destDir string) error {
	file, err := os.Open(packagePath) // #nosec G304,G703 -- packagePath is from trusted build artifacts
	if err != nil {
		return errors.Wrap(err, errors.ErrTypeFileSystem, "failed to open RPM package").
			WithContext("path", packagePath).
			WithOperation("extractRPM")
	}

	defer func() {
		_ = file.Close()
	}()

	rpm, err := rpmutils.ReadRpm(file)
	if err != nil {
		return errors.Wrap(err, errors.ErrTypePackaging, "failed to read RPM header").
			WithContext("path", packagePath).
			WithOperation("extractRPM")
	}

	pr, err := rpm.PayloadReaderExtended()
	if err != nil {
		return errors.Wrap(err, errors.ErrTypePackaging, "failed to open RPM payload").
			WithContext("package", packagePath).
			WithOperation("extractRPM")
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
				WithOperation("extractRPM")
		}

		if err := extractRPMEntry(pr, fi, destDir, dirMap); err != nil {
			return errors.Wrap(err, errors.ErrTypePackaging, "failed to extract RPM entry").
				WithContext("package", packagePath).
				WithContext("entry", fi.Name()).
				WithOperation("extractRPM")
		}
	}
}

// extractRPMEntry writes one payload entry to destDir, normalising absolute
// cpio paths (".//opt/..." or "/opt/...") to relative joins under destDir and
// rejecting traversal via archive.SafeJoin.
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

	target, err := archive.SafeJoin(destDir, name)
	if err != nil {
		logger.Warn(i18n.T("logger.archive.warn.path_traversal_rejected"),
			"entry", fi.Name(), "destination", destDir)

		return err
	}

	mode := fi.Mode()
	// #nosec G115 -- POSIX mode is a 32-bit field; masking with os.ModePerm
	// (0o777) discards the type bits and any out-of-range data before use.
	perm := os.FileMode(uint32(mode)) & os.ModePerm

	switch mode & rpmFileTypeMask {
	case rpmFileTypeDir:
		dirMap[target] = true

		// #nosec G703 -- target is constrained by archive.SafeJoin above.
		return os.MkdirAll(target, perm|0o100) // ensure exec bit so we can descend

	case rpmFileTypeLink:
		if err := archive.SafeSymlinkTarget(fi.Name(), fi.Linkname()); err != nil {
			logger.Warn(i18n.T("logger.archive.warn.symlink_rejected"),
				"entry", fi.Name(), "target", fi.Linkname())

			return err
		}

		parent := filepath.Dir(target)
		if _, seen := dirMap[parent]; !seen {
			dirMap[parent] = true
			_ = os.MkdirAll(parent, 0o755) // #nosec G301,G703 -- intermediate dirs need read+exec; path is SafeJoin-constrained
		}

		_ = os.Remove(target) // #nosec G703 -- target is constrained by archive.SafeJoin above.

		return os.Symlink(fi.Linkname(), target) // #nosec G703 -- target is constrained by archive.SafeJoin above.

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
			_ = os.MkdirAll(parent, 0o755) // #nosec G301,G703 -- intermediate dirs need read+exec; path is SafeJoin-constrained
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
	// #nosec G304,G703 -- path is constrained by archive.SafeJoin to stay inside destDir.
	out, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, perm)
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

// InstallOrExtract extracts the built package to the root filesystem (/).
// This applies to both native and cross-compilation builds.
func (bb *BaseBuilder) InstallOrExtract(artifactsPath, buildDir, targetArch string) error {
	// targetArch is accepted for interface compatibility (packer.InstallOrExtractor)
	// but unused: extraction always goes to root filesystem.
	_ = targetArch
	_ = buildDir

	pkgName := bb.BuildPackageName(getExtension(bb.Format))
	pkgPath := filepath.Join(artifactsPath, pkgName)

	logger.Info(i18n.T("logger.extract.extracting_to_sysroot"),
		"package", filepath.Base(pkgPath))

	return bb.ExtractToRoot(pkgPath)
}
