// Package files provides unified file system operations for package building.
package files

import (
	"crypto/sha256"
	"encoding/hex"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/M0Rf30/yap/v2/pkg/logger"
)

// WalkOptions configures the behavior of directory walking.
type WalkOptions struct {
	SkipDotFiles bool     // Skip files starting with '.'
	BackupFiles  []string // List of backup/config files
	SkipPatterns []string // File patterns to skip
}

// Walker provides unified directory walking functionality for all package managers.
type Walker struct {
	BaseDir string
	Options WalkOptions
}

// NewWalker creates a new filesystem walker.
func NewWalker(baseDir string, options WalkOptions) *Walker {
	return &Walker{
		BaseDir: baseDir,
		Options: options,
	}
}

// Walk traverses the directory and returns file entries.
// This consolidates all the walking logic from different package formats.
func (w *Walker) Walk() ([]*Entry, error) {
	var entries []*Entry

	err := filepath.WalkDir(w.BaseDir, func(path string, dirEntry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		// Skip the base directory itself
		if path == w.BaseDir {
			return nil
		}

		// Skip dot files if requested (common for makepkg/pacman)
		if w.Options.SkipDotFiles {
			filename := filepath.Base(path)
			if filename != "" && filename[0] == '.' {
				return nil
			}
		}

		// Skip files matching patterns
		if w.shouldSkipFile(filepath.Base(path)) {
			return nil
		}

		entry, err := w.createEntry(path, dirEntry)
		if err != nil {
			return err
		}

		// Skip empty directories unless they need explicit ownership
		if entry.IsDirectory() && w.isEmptyDirectory(path, dirEntry) {
			// Only include empty directories if they might need explicit ownership
			entries = append(entries, entry)
		} else if !entry.IsDirectory() {
			entries = append(entries, entry)
		}

		return nil
	})

	return entries, err
}

// createEntry creates an Entry from a file system entry.
func (w *Walker) createEntry(path string, dirEntry fs.DirEntry) (*Entry, error) {
	fileInfo, err := dirEntry.Info()
	if err != nil {
		return nil, err
	}

	relPath, err := filepath.Rel(w.BaseDir, path)
	if err != nil {
		return nil, err
	}

	// Ensure destination starts with /
	destination := "/" + strings.TrimPrefix(relPath, "/")

	entry := &Entry{
		Source:      path,
		Destination: destination,
		Mode:        fileInfo.Mode(),
		Size:        fileInfo.Size(),
		ModTime:     fileInfo.ModTime(),
		IsBackup:    w.isBackupFile(destination),
	}

	// Determine file type and handle special cases
	switch {
	case fileInfo.Mode()&os.ModeSymlink != 0:
		entry.Type = TypeSymlink

		linkTarget, err := os.Readlink(path)
		if err != nil {
			return nil, err
		}

		entry.LinkTarget = linkTarget

	case fileInfo.IsDir():
		entry.Type = TypeDir

	case entry.IsBackup:
		entry.Type = TypeConfigNoReplace

	default:
		entry.Type = TypeFile
		// Calculate SHA256 for regular files
		if fileInfo.Mode().IsRegular() {
			sha256Hash, err := w.calculateSHA256(path)
			if err != nil {
				return nil, err
			}

			entry.SHA256 = sha256Hash
		}
	}

	return entry, nil
}

// shouldSkipFile checks if a file should be skipped based on patterns.
func (w *Walker) shouldSkipFile(fileName string) bool {
	for _, pattern := range w.Options.SkipPatterns {
		if matched, _ := filepath.Match(pattern, fileName); matched {
			return true
		}
	}

	return false
}

// isBackupFile checks if a file path is in the backup list.
func (w *Walker) isBackupFile(path string) bool {
	normalizedPath := path
	if !strings.HasPrefix(normalizedPath, "/") {
		normalizedPath = "/" + normalizedPath
	}

	for _, backupFile := range w.Options.BackupFiles {
		normalizedBackup := backupFile
		if !strings.HasPrefix(normalizedBackup, "/") {
			normalizedBackup = "/" + normalizedBackup
		}

		if normalizedPath == normalizedBackup {
			return true
		}
	}

	return false
}

// isEmptyDirectory checks if a directory is empty.
func (w *Walker) isEmptyDirectory(path string, dirEntry fs.DirEntry) bool {
	if !dirEntry.IsDir() {
		return false
	}

	entries, err := os.ReadDir(filepath.Clean(path))
	if err != nil {
		return false
	}

	return len(entries) == 0
}

// calculateSHA256 calculates the SHA256 hash of a file.
func (w *Walker) calculateSHA256(filePath string) ([]byte, error) {
	file, err := os.Open(filepath.Clean(filePath))
	if err != nil {
		return nil, err
	}

	defer func() {
		if closeErr := file.Close(); closeErr != nil {
			// Log error but don't fail the operation
			logger.Warn("failed to close file during SHA256 calculation",
				"path", filePath,
				"error", closeErr)
		}
	}()

	hasher := sha256.New()
	if _, err := io.Copy(hasher, file); err != nil {
		return nil, err
	}

	return hasher.Sum(nil), nil
}

// CalculateDataHash calculates a hash of all data files for package metadata.
// This is used by formats like APK that need a hash of the entire data payload.
func CalculateDataHash(baseDir string, skipPatterns []string) (string, error) {
	hasher := sha256.New()

	err := filepath.WalkDir(baseDir, func(path string, dirEntry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if path == baseDir {
			return nil
		}

		// Skip files matching patterns
		fileName := filepath.Base(path)
		for _, pattern := range skipPatterns {
			if matched, _ := filepath.Match(pattern, fileName); matched {
				return nil
			}
		}

		relPath, err := filepath.Rel(baseDir, path)
		if err != nil {
			return err
		}

		fileInfo, err := dirEntry.Info()
		if err != nil {
			return err
		}

		// Hash file path and metadata
		hasher.Write([]byte(relPath))
		hasher.Write([]byte{byte(fileInfo.Mode())})

		// Hash file content if it's a regular file
		if fileInfo.Mode().IsRegular() {
			file, err := os.Open(filepath.Clean(path))
			if err != nil {
				return err
			}

			defer func() {
				if closeErr := file.Close(); closeErr != nil {
					logger.Warn("failed to close file during data hash calculation",
						"path", path,
						"error", closeErr)
				}
			}()

			_, err = io.Copy(hasher, file)
			if err != nil {
				return err
			}
		}

		return nil
	})
	if err != nil {
		return "", err
	}

	return hex.EncodeToString(hasher.Sum(nil)), nil
}
