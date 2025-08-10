// Package files provides unified file system operations for package building.
package files

import (
	"os"
	"time"

	"github.com/M0Rf30/yap/v2/pkg/osutils"
)

// Entry represents a file or directory entry in a package.
// This consolidates the various file entry types used across package formats.
type Entry struct {
	Source      string      // Absolute path to the source file
	Destination string      // Relative path in the package (starts with /)
	Type        string      // File type (file, dir, symlink, config, etc.)
	Mode        os.FileMode // File permissions
	Size        int64       // File size in bytes
	ModTime     time.Time   // Last modification time
	LinkTarget  string      // Target path for symlinks
	SHA256      []byte      // SHA256 hash for regular files
	IsBackup    bool        // Whether this file should be treated as a config backup
}

// ConvertToLegacyFormat converts Entry to the legacy osutils.FileContent format.
// This provides backward compatibility during the transition period.
func (e *Entry) ConvertToLegacyFormat() osutils.FileContent {
	return osutils.FileContent{
		Source:      e.Source,
		Destination: e.Destination,
		Type:        e.Type,
		SHA256:      e.SHA256,
		FileInfo: &osutils.FileInfo{
			Mode:    uint32(e.Mode.Perm()),
			Size:    e.Size,
			ModTime: e.ModTime.Unix(),
		},
	}
}

// IsRegularFile returns true if this entry represents a regular file.
func (e *Entry) IsRegularFile() bool {
	return e.Mode.IsRegular()
}

// IsDirectory returns true if this entry represents a directory.
func (e *Entry) IsDirectory() bool {
	return e.Mode.IsDir()
}

// IsSymlink returns true if this entry represents a symbolic link.
func (e *Entry) IsSymlink() bool {
	return e.Mode&os.ModeSymlink != 0
}

// IsConfigFile returns true if this entry represents a configuration file.
func (e *Entry) IsConfigFile() bool {
	return e.Type == osutils.TypeConfig || e.Type == osutils.TypeConfigNoReplace
}
