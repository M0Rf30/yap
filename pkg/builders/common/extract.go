// Package common provides shared interfaces and base implementations for package builders.
package common

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/blakesmith/ar"
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
		extractErr = extractDEB(packagePath, "/")
	case constants.FormatRPM:
		// RPM format: header + cpio payload — the generic archive identifier
		// does not recognize RPM, so we must decode the payload ourselves.
		extractErr = extractRPM(packagePath, "/")
	case constants.FormatAPK, constants.FormatPacman:
		// APK and Pacman: extract to root, then clean up metadata files
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

// cleanupMetadataFiles removes package metadata files that were extracted to root.
// These files should not be present on the actual filesystem.
func cleanupMetadataFiles(format string) {
	var metadataPatterns []string

	switch format {
	case constants.FormatAPK:
		metadataPatterns = []string{
			"/.PKGINFO",
			"/.SIGN.*",
			"/.pre-*",
			"/.post-*",
			"/.install",
			"/.trigger*",
		}
	case constants.FormatPacman:
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

// extractDEB extracts a Debian package (.deb) to the destination directory.
// DEB format: AR archive containing control.tar.gz and data.tar.{gz,xz,zst}
// We need to extract data.tar from the AR archive and then extract its contents.
func extractDEB(packagePath, destDir string) error {
	file, err := os.Open(packagePath) // #nosec G304 - packagePath is from trusted build artifacts
	if err != nil {
		return errors.Wrap(err, errors.ErrTypeFileSystem, "failed to open DEB package").
			WithContext("path", packagePath).
			WithOperation("extractDEB")
	}

	defer func() {
		_ = file.Close()
	}()

	arReader := ar.NewReader(file)

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

// extractRPM extracts an RPM package payload (cpio.{gz,xz,zst,...}) to the
// destination directory using github.com/sassoftware/go-rpmutils. We can't use
// the generic archive.Extract path because the mholt/archives Identify routine
// does not recognize the RPM lead+header envelope and returns no extractor,
// which would silently no-op the install.
func extractRPM(packagePath, destDir string) error {
	file, err := os.Open(packagePath) // #nosec G304 - packagePath is from trusted build artifacts
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

	// destDir is expected to exist (callers pass "/" or a test temp dir).
	// ExpandPayload -> cpio.Extract creates subdirectories from the payload
	// itself, using per-entry modes baked into the RPM.
	if err := rpm.ExpandPayload(destDir); err != nil {
		return errors.Wrap(err, errors.ErrTypePackaging, "failed to expand RPM payload").
			WithContext("package", packagePath).
			WithContext("dest", destDir).
			WithOperation("extractRPM")
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
