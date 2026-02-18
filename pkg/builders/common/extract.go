// Package common provides shared interfaces and base implementations for package builders.
package common

import (
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/blakesmith/ar"

	"github.com/M0Rf30/yap/v2/pkg/archive"
	"github.com/M0Rf30/yap/v2/pkg/constants"
	"github.com/M0Rf30/yap/v2/pkg/errors"
	"github.com/M0Rf30/yap/v2/pkg/i18n"
	"github.com/M0Rf30/yap/v2/pkg/logger"
)

// ExtractToStaging extracts a package to a staging directory that mirrors the filesystem layout.
// This allows cross-compiled packages to provide their files for dependent packages
// without installation. The staging directory acts as a filesystem overlay for
// build-time dependencies.
func (bb *BaseBuilder) ExtractToStaging(packagePath, stagingRoot string) error {
	// Get package info for logging
	pkgInfo, _ := os.Stat(packagePath)

	var packageSize int64

	if pkgInfo != nil {
		packageSize = pkgInfo.Size()
	}

	logger.Debug(i18n.T("logger.extract.extracting_package"),
		"package", filepath.Base(packagePath),
		"package_size_mb", packageSize/(1024*1024),
		"staging_root", stagingRoot,
		"format", bb.Format)

	var extractErr error

	switch bb.Format {
	case constants.FormatDEB:
		// DEB packages need special handling to extract data.tar from AR archive
		extractErr = extractDEB(packagePath, stagingRoot)
	case constants.FormatRPM, constants.FormatAPK, constants.FormatPacman:
		// Use generic archive extraction for RPM, APK, and Pacman formats
		extractErr = archive.Extract(packagePath, stagingRoot)
	default:
		return errors.New(errors.ErrTypePackaging, "unsupported package format for extraction").
			WithContext("format", bb.Format).
			WithOperation("ExtractToStaging")
	}

	if extractErr != nil {
		return errors.Wrap(extractErr, errors.ErrTypePackaging, "failed to extract package").
			WithContext("package", packagePath).
			WithContext("format", bb.Format).
			WithOperation("ExtractToStaging")
	}

	// Calculate extraction statistics
	fileCount, stagingSize := countFilesAndSize(stagingRoot)

	logger.Info(i18n.T("logger.extract.package_extracted"),
		"package", filepath.Base(packagePath),
		"format", bb.Format)

	logger.Debug(i18n.T("logger.extract.extraction_stats"),
		"files_extracted", fileCount,
		"staging_size_mb", stagingSize/(1024*1024),
		"staging_root", stagingRoot)

	return nil
}

// countFilesAndSize walks the staging directory to count files and calculate total size.
func countFilesAndSize(root string) (fileCount int, totalSize int64) {
	_ = filepath.Walk(root, func(_ string, info os.FileInfo, err error) error {
		if err != nil {
			// Skip errors during walk
			return err //nolint:wrapcheck // Intentionally returning error for Walk to skip
		}

		if !info.IsDir() {
			fileCount++
			totalSize += info.Size()
		}

		return nil
	})

	return fileCount, totalSize
}

// extractDEB extracts a Debian package (.deb) to the staging directory.
// DEB format: AR archive containing control.tar.gz and data.tar.{gz,xz,zst}
// We need to extract data.tar from the AR archive and then extract its contents.
func extractDEB(packagePath, stagingRoot string) error {
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
				_ = os.Remove(dataTarPath) //nolint:gosec // path comes from os.CreateTemp, not user input

				return errors.Wrap(err, errors.ErrTypeFileSystem, "failed to write temp file").
					WithOperation("extractDEB")
			}

			if err := tmpFile.Close(); err != nil {
				_ = os.Remove(dataTarPath) //nolint:gosec // path comes from os.CreateTemp, not user input

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
	return archive.Extract(dataTarPath, stagingRoot)
}

// GetStagingRoot returns the staging directory path for cross-compilation.
func GetStagingRoot(buildDir string) string {
	return filepath.Join(buildDir, "yap-cross-staging")
}

// CleanupStaging removes the staging directory.
func CleanupStaging(buildDir string) error {
	stagingRoot := GetStagingRoot(buildDir)

	if _, err := os.Stat(stagingRoot); os.IsNotExist(err) {
		return nil // Already clean
	}

	// Log staging directory statistics before cleanup
	fileCount, stagingSize := countFilesAndSize(stagingRoot)
	if fileCount > 0 {
		logger.Debug(i18n.T("logger.extract.staging_cleaned"),
			"staging_root", stagingRoot,
			"files_cleaned", fileCount,
			"space_freed_mb", stagingSize/(1024*1024))
	}

	if err := os.RemoveAll(stagingRoot); err != nil {
		return errors.Wrap(err, errors.ErrTypeFileSystem, "failed to cleanup staging directory").
			WithContext("staging_root", stagingRoot).
			WithOperation("CleanupStaging")
	}

	return nil
}

// InstallOrExtract installs the package normally, or extracts to staging during cross-compilation.
func (bb *BaseBuilder) InstallOrExtract(artifactsPath, buildDir, targetArch string) error {
	// Detect cross-compilation: target arch differs from build host arch
	isCrossCompiling := targetArch != "" && targetArch != bb.PKGBUILD.ArchComputed

	if isCrossCompiling {
		logger.Info(i18n.T("logger.extract.cross_compilation_detected"),
			"target_arch", targetArch,
			"build_arch", bb.PKGBUILD.ArchComputed,
			"package", bb.PKGBUILD.PkgName)

		// Extract to staging instead of installing
		stagingRoot := GetStagingRoot(buildDir)

		// Check if staging directory already exists (from previous packages)
		stagingExists := false
		if stat, err := os.Stat(stagingRoot); err == nil && stat.IsDir() {
			stagingExists = true

			logger.Debug(i18n.T("logger.extract.staging_directory_reused"),
				"staging_root", stagingRoot,
				"package", bb.PKGBUILD.PkgName)
		}

		if !stagingExists {
			// #nosec G301 - staging directory needs standard permissions for build artifacts
			if err := os.MkdirAll(stagingRoot, 0o755); err != nil {
				return errors.Wrap(err, errors.ErrTypeFileSystem, "failed to create staging directory").
					WithContext("staging_root", stagingRoot).
					WithOperation("InstallOrExtract")
			}

			logger.Info(i18n.T("logger.extract.staging_directory_created"),
				"staging_root", stagingRoot,
				"package", bb.PKGBUILD.PkgName)
		}

		pkgName := bb.BuildPackageName(getExtension(bb.Format))
		pkgPath := filepath.Join(artifactsPath, pkgName)

		return bb.ExtractToStaging(pkgPath, stagingRoot)
	}

	// Normal installation for native builds
	return bb.Install(artifactsPath)
}
