// Package filesystem provides unified file system operations for package building.
package filesystem

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/M0Rf30/yap/v2/pkg/osutils"
)

// Walker provides unified directory walking functionality for package managers.
type Walker struct {
	packageDir  string
	backupFiles []string
}

// NewWalker creates a new filesystem walker.
func NewWalker(packageDir string, backupFiles []string) *Walker {
	return &Walker{
		packageDir:  packageDir,
		backupFiles: normalizeBackupFiles(backupFiles),
	}
}

// WalkPackageDirectory traverses a package directory and collects file contents.
// This consolidates the duplicated walkPackageDirectory logic from makepkg and rpm.
func (w *Walker) WalkPackageDirectory() ([]*osutils.FileContent, error) {
	var contents []*osutils.FileContent

	err := filepath.WalkDir(w.packageDir, func(path string, dirEntry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		// Skip the root package directory itself
		if path == w.packageDir {
			return nil
		}

		// Skip metadata files that start with '.' (same as pacman's handle_simple_path)
		filename := filepath.Base(path)
		if filename != "" && filename[0] == '.' {
			return nil
		}

		if dirEntry.IsDir() {
			if osutils.IsEmptyDir(path, dirEntry) {
				contents = append(contents, w.createContent(path, osutils.TypeDir))
			}
			return nil
		}

		return w.handleFileEntry(path, &contents)
	})

	return contents, err
}

// WalkForMakePkg provides makepkg-specific walking with FileContent slice and directory handling.
func (w *Walker) WalkForMakePkg() ([]osutils.FileContent, error) {
	var entries []osutils.FileContent

	err := filepath.WalkDir(w.packageDir, func(path string, dirEntry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if path == w.packageDir {
			return nil
		}

		// Skip metadata files that start with '.' (same as pacman's handle_simple_path)
		filename := filepath.Base(path)
		if filename != "" && filename[0] == '.' {
			return nil
		}

		if dirEntry.IsDir() {
			fileInfo, err := dirEntry.Info()
			if err != nil {
				return err
			}

			entries = append(entries, osutils.FileContent{
				Destination: strings.TrimPrefix(path, w.packageDir),
				FileInfo: &osutils.FileInfo{
					Mode:    uint32(fileInfo.Mode().Perm()),
					Size:    fileInfo.Size(),
					ModTime: fileInfo.ModTime().Unix(),
				},
				Source: "",
				Type:   osutils.TypeDir,
				SHA256: nil,
			})
			return nil
		}

		return w.handleFileEntryForMakePkg(path, &entries)
	})

	return entries, err
}

// createContent creates a FileContent object with the specified parameters.
func (w *Walker) createContent(path, contentType string) *osutils.FileContent {
	return &osutils.FileContent{
		Source:      path,
		Destination: strings.TrimPrefix(path, w.packageDir),
		Type:        contentType,
	}
}

// handleFileEntry processes a file entry and determines its type.
func (w *Walker) handleFileEntry(path string, contents *[]*osutils.FileContent) error {
	fileInfo, err := os.Lstat(path)
	if err != nil {
		return err
	}

	switch {
	case fileInfo.Mode()&os.ModeSymlink != 0:
		*contents = append(*contents, w.createContent(path, osutils.TypeSymlink))
	case w.isBackupFile(path):
		*contents = append(*contents, w.createContent(path, osutils.TypeConfigNoReplace))
	default:
		*contents = append(*contents, w.createContent(path, osutils.TypeFile))
	}

	return nil
}

// handleFileEntryForMakePkg processes a file entry for makepkg format.
func (w *Walker) handleFileEntryForMakePkg(path string, entries *[]osutils.FileContent) error {
	fileInfo, err := os.Lstat(path)
	if err != nil {
		return err
	}

	var source string
	var contentType string
	var sha256 []byte

	if fileInfo.Mode()&os.ModeSymlink != 0 {
		readlink, err := os.Readlink(path)
		if err != nil {
			return err
		}
		source = readlink
		contentType = osutils.TypeSymlink
	} else {
		sha256, err = osutils.CalculateSHA256(path)
		if err != nil {
			return err
		}
		contentType = osutils.TypeFile
	}

	*entries = append(*entries, osutils.FileContent{
		Destination: strings.TrimPrefix(path, w.packageDir),
		FileInfo: &osutils.FileInfo{
			Mode:    uint32(fileInfo.Mode().Perm()),
			Size:    fileInfo.Size(),
			ModTime: fileInfo.ModTime().Unix(),
		},
		Source: source,
		Type:   contentType,
		SHA256: sha256,
	})

	return nil
}

// isBackupFile checks if a file is in the backup files list.
func (w *Walker) isBackupFile(path string) bool {
	relativePath := strings.TrimPrefix(path, w.packageDir)
	return osutils.Contains(w.backupFiles, relativePath)
}

// normalizeBackupFiles ensures all backup file paths have a leading slash.
func normalizeBackupFiles(backupFiles []string) []string {
	normalized := make([]string, len(backupFiles))
	for i, filePath := range backupFiles {
		if !strings.HasPrefix(filePath, "/") {
			normalized[i] = "/" + filePath
		} else {
			normalized[i] = filePath
		}
	}
	return normalized
}
