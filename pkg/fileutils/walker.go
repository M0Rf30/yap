// Package fileutils provides common file processing utilities for package managers.
package fileutils

import (
	"crypto/sha256"
	"encoding/hex"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/M0Rf30/yap/v2/pkg/osutils"
)

// FileEntry represents a file or directory entry in a package.
type FileEntry struct {
	Source      string
	Destination string
	Type        string
	Mode        os.FileMode
	Size        int64
	ModTime     time.Time
	LinkTarget  string
	SHA256      []byte
	IsBackup    bool
}

// FileWalker provides common file walking functionality.
type FileWalker struct {
	BaseDir      string
	BackupFiles  []string
	SkipDotFiles bool
}

// NewFileWalker creates a new file walker.
func NewFileWalker(baseDir string, backupFiles []string, skipDotFiles bool) *FileWalker {
	return &FileWalker{
		BaseDir:      baseDir,
		BackupFiles:  backupFiles,
		SkipDotFiles: skipDotFiles,
	}
}

// Walk traverses the directory and returns file entries.
func (fw *FileWalker) Walk() ([]*FileEntry, error) {
	var entries []*FileEntry

	err := filepath.WalkDir(fw.BaseDir, func(path string, dirEntry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		// Skip the base directory itself
		if path == fw.BaseDir {
			return nil
		}

		// Skip dot files if requested (common for makepkg)
		if fw.SkipDotFiles {
			filename := filepath.Base(path)
			if filename != "" && filename[0] == '.' {
				return nil
			}
		}

		entry, err := fw.createFileEntry(path, dirEntry)
		if err != nil {
			return err
		}

		entries = append(entries, entry)

		return nil
	})

	return entries, err
}

// createFileEntry creates a FileEntry from a file system entry.
func (fw *FileWalker) createFileEntry(path string, dirEntry fs.DirEntry) (*FileEntry, error) {
	fileInfo, err := dirEntry.Info()
	if err != nil {
		return nil, err
	}

	relPath, err := filepath.Rel(fw.BaseDir, path)
	if err != nil {
		return nil, err
	}

	// Ensure destination starts with /
	destination := "/" + strings.TrimPrefix(relPath, "/")

	entry := &FileEntry{
		Source:      path,
		Destination: destination,
		Mode:        fileInfo.Mode(),
		Size:        fileInfo.Size(),
		ModTime:     fileInfo.ModTime(),
		IsBackup:    fw.isBackupFile(destination),
	}

	// Determine file type and handle special cases
	switch {
	case fileInfo.Mode()&os.ModeSymlink != 0:
		entry.Type = osutils.TypeSymlink

		linkTarget, err := os.Readlink(path)
		if err != nil {
			return nil, err
		}

		entry.LinkTarget = linkTarget

	case fileInfo.IsDir():
		entry.Type = osutils.TypeDir

	case entry.IsBackup:
		entry.Type = osutils.TypeConfigNoReplace

	default:
		entry.Type = osutils.TypeFile
		// Calculate SHA256 for regular files
		if fileInfo.Mode().IsRegular() {
			sha256Hash, err := fw.calculateSHA256(path)
			if err != nil {
				return nil, err
			}

			entry.SHA256 = sha256Hash
		}
	}

	return entry, nil
}

// isBackupFile checks if a file path is in the backup list.
func (fw *FileWalker) isBackupFile(path string) bool {
	for _, backupFile := range fw.BackupFiles {
		// Normalize backup file path to start with /
		normalizedBackup := backupFile
		if !strings.HasPrefix(normalizedBackup, "/") {
			normalizedBackup = "/" + normalizedBackup
		}

		if path == normalizedBackup {
			return true
		}
	}

	return false
}

// calculateSHA256 calculates the SHA256 hash of a file.
func (fw *FileWalker) calculateSHA256(filePath string) ([]byte, error) {
	// #nosec G304 - File paths are controlled within package build process
	file, err := os.Open(filepath.Clean(filePath))
	if err != nil {
		return nil, err
	}

	defer func() {
		if closeErr := file.Close(); closeErr != nil {
			// Log error but don't fail the operation
			_ = closeErr
		}
	}()

	hasher := sha256.New()
	if _, err := io.Copy(hasher, file); err != nil {
		return nil, err
	}

	return hasher.Sum(nil), nil
}

// ConvertToOldFormat converts FileEntry to the old osutils.FileContent format.
// This provides backward compatibility during the transition period.
func (entry *FileEntry) ConvertToOldFormat() osutils.FileContent {
	return osutils.FileContent{
		Source:      entry.Source,
		Destination: entry.Destination,
		Type:        entry.Type,
		SHA256:      entry.SHA256,
		FileInfo: &osutils.FileInfo{
			Mode:    uint32(entry.Mode.Perm()),
			Size:    entry.Size,
			ModTime: entry.ModTime.Unix(),
		},
	}
}

// CalculateDataHash calculates a hash of all data files for package metadata.
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
			// #nosec G304 - File paths are controlled within package build process
			file, err := os.Open(filepath.Clean(path))
			if err != nil {
				return err
			}

			defer func() {
				if closeErr := file.Close(); closeErr != nil {
					// Log error but don't fail the operation
					_ = closeErr
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
