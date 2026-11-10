// SPDX-FileCopyrightText: 2021-2026 Gianluca Boiano
// SPDX-License-Identifier: GPL-3.0-only

// Package files provides unified file system operations for package building.
package files

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/M0Rf30/yap/v2/pkg/crypto"
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

		// Skip dot files if requested (common for makepkg/pacman); a skipped
		// directory's contents are skipped with it.
		if w.Options.SkipDotFiles {
			filename := filepath.Base(path)
			if filename != "" && filename[0] == '.' {
				return skipEntry(dirEntry)
			}
		}

		// Skip files matching patterns
		if w.shouldSkipFile(filepath.Base(path)) {
			return skipEntry(dirEntry)
		}

		entry, err := w.createEntry(path, dirEntry)
		if err != nil {
			return err
		}

		// Include all entries (files, directories, symlinks)
		entries = append(entries, entry)

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
			sha256Hash, err := crypto.CalculateSHA256(path)
			if err != nil {
				return nil, err
			}

			entry.SHA256 = sha256Hash
		}
	}

	return entry, nil
}

// skipEntry returns filepath.SkipDir for directories (pruning their contents)
// and nil for other entries.
func skipEntry(dirEntry fs.DirEntry) error {
	if dirEntry.IsDir() {
		return filepath.SkipDir
	}

	return nil
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
