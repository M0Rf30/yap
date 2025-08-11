// Package files provides unified file system operations for package building.
package files

import (
	"os"
	"time"
)

// File type constants
const (
	TagLink             = 0o120000
	TagDirectory        = 0o40000
	TypeFile            = "file"
	TypeDir             = "dir"
	TypeImplicitDir     = "implicit dir"
	TypeSymlink         = "symlink"
	TypeTree            = "tree"
	TypeConfig          = "config"
	TypeConfigNoReplace = "config|noreplace"
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
	return e.Type == TypeConfig || e.Type == TypeConfigNoReplace
}
