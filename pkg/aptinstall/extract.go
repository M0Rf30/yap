package aptinstall

import (
	"archive/tar"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/blakesmith/ar"

	"github.com/M0Rf30/yap/v2/pkg/logger"
)

// extractDataTar extracts the data.tar member of a .deb to the destination directory.
// It handles conffile collisions: if a conffile already exists on disk, it is NOT
// overwritten (dpkg behavior with DEBIAN_FRONTEND=noninteractive).
func extractDataTar(debPath, destDir string, conffiles []string) error {
	file, err := os.Open(debPath) // #nosec G304 - debPath is from trusted apt index metadata
	if err != nil {
		return fmt.Errorf("open DEB: %w", err)
	}

	defer func() { _ = file.Close() }()

	// Find and extract data.tar.
	// We need to read the AR archive again to find data.tar.
	// For simplicity, we'll use the archive.Extract function which handles this.
	// But first, we need to extract data.tar to a temp file.

	// Actually, let's use a simpler approach: read the AR archive, find data.tar,
	// extract it to a temp file, then use archive.Extract.

	arReader := ar.NewReader(file)

	var dataTarPath string

	for {
		header, err := arReader.Next()
		if err != nil {
			if err == io.EOF { //nolint:errorlint
				break
			}

			return fmt.Errorf("read AR header: %w", err)
		}

		if strings.HasPrefix(header.Name, "data.tar") {
			// Create a temporary file for data.tar.
			tmpFile, err := os.CreateTemp("", "data.tar.*")
			if err != nil {
				return fmt.Errorf("create temp file: %w", err)
			}

			dataTarPath = tmpFile.Name()

			// Copy data.tar to temp file.
			if _, err := io.Copy(tmpFile, arReader); err != nil {
				_ = tmpFile.Close()
				_ = os.Remove(dataTarPath) // #nosec G703

				return fmt.Errorf("write temp file: %w", err)
			}

			if err := tmpFile.Close(); err != nil {
				_ = os.Remove(dataTarPath) // #nosec G703

				return fmt.Errorf("close temp file: %w", err)
			}

			break
		}
	}

	if dataTarPath == "" {
		return fmt.Errorf("data.tar not found in DEB")
	}

	defer func() {
		_ = os.Remove(dataTarPath)
	}()

	// Now extract the data.tar, handling conffiles.
	return extractDataTarWithConffiles(dataTarPath, destDir, conffiles)
}

// extractDataTarWithConffiles extracts a data.tar file, skipping conffiles that already exist.
func extractDataTarWithConffiles(dataTarPath, destDir string, conffiles []string) error {
	// Build a set of conffiles for quick lookup.
	conffileSet := make(map[string]bool)

	for _, cf := range conffiles {
		cf = strings.TrimSpace(cf)
		if cf != "" {
			conffileSet[cf] = true
		}
	}

	// Open and decompress the data.tar.
	file, err := os.Open(dataTarPath) // #nosec G304 - dataTarPath is from os.CreateTemp
	if err != nil {
		return fmt.Errorf("open data.tar: %w", err)
	}

	defer func() { _ = file.Close() }()

	decompressed, err := decompressStream(file, dataTarPath)
	if err != nil {
		return fmt.Errorf("decompress data.tar: %w", err)
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

			return fmt.Errorf("read tar entry: %w", err)
		}

		// Strip leading "./" from tar entry names.
		path := strings.TrimPrefix(hdr.Name, "./")
		if path == "" {
			continue
		}

		// Ensure the path is relative (no leading /).
		path = strings.TrimPrefix(path, "/")

		// Full destination path.
		fullPath := filepath.Join(destDir, path)

		// Ensure the path stays within destDir (security check).
		if !strings.HasPrefix(fullPath, destDir) {
			logger.Warn("Skipping path traversal attempt", "path", path)

			continue
		}

		if err := extractTarEntry(tr, hdr, fullPath, conffileSet, dirMap); err != nil {
			return err
		}
	}

	return nil
}

// extractTarEntry dispatches tar entry extraction based on type.
func extractTarEntry(
	tr *tar.Reader,
	hdr *tar.Header,
	fullPath string,
	conffileSet, dirMap map[string]bool,
) error {
	switch hdr.Typeflag {
	case tar.TypeDir:
		return extractTarDir(hdr, fullPath, dirMap)
	case tar.TypeSymlink:
		return extractTarSymlink(hdr, fullPath, dirMap)
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

	// nolint:gosec // G301: mode is from tar header, constrained by SafeJoin
	if err := os.MkdirAll(fullPath, os.FileMode(hdr.Mode)); err != nil {
		return fmt.Errorf("mkdir %s: %w", fullPath, err)
	}

	return nil
}

// extractTarSymlink creates a symlink from a tar entry.
func extractTarSymlink(hdr *tar.Header, fullPath string, dirMap map[string]bool) error {
	// Remove existing symlink/file.
	_ = os.Remove(fullPath)

	// Create parent directory if needed.
	parentDir := filepath.Dir(fullPath)
	if _, seen := dirMap[parentDir]; !seen {
		dirMap[parentDir] = true
		_ = os.MkdirAll(parentDir, 0o755) // #nosec G301
	}

	if err := os.Symlink(hdr.Linkname, fullPath); err != nil {
		return fmt.Errorf("symlink %s: %w", fullPath, err)
	}

	return nil
}

// extractTarFile extracts a regular file from a tar entry, respecting conffiles.
func extractTarFile(
	tr *tar.Reader,
	hdr *tar.Header,
	fullPath string,
	dirMap, conffileSet map[string]bool,
) error {
	// Check if this is a conffile that already exists.
	if conffileSet["/"+filepath.Base(fullPath)] && fileExists(fullPath) {
		logger.Info("Skipping existing conffile", "path", fullPath)

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
	// nolint:gosec // G304: fullPath is constrained by SafeJoin; G306: mode is from tar header
	f, err := os.OpenFile(fullPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, os.FileMode(hdr.Mode))
	if err != nil {
		return fmt.Errorf("create %s: %w", fullPath, err)
	}

	// Limit file size to prevent decompression bombs (2GB max per file).
	const maxFileSize = 2 * 1024 * 1024 * 1024

	if _, err := io.Copy(f, io.LimitReader(tr, maxFileSize)); err != nil {
		_ = f.Close()

		return fmt.Errorf("write %s: %w", fullPath, err)
	}

	if err := f.Close(); err != nil {
		return fmt.Errorf("close %s: %w", fullPath, err)
	}

	return nil
}

// fileExists checks if a file exists on disk.
func fileExists(path string) bool {
	_, err := os.Stat(path)

	return err == nil
}
