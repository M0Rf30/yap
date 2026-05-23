package rpmdb

import "time"

// InstalledFile represents a file installed by an RPM package.
// Used by the Writer to record file metadata in the rpmdb.
type InstalledFile struct {
	// Path is the absolute path on disk (e.g., "/usr/bin/foo").
	Path string
	// Size is the file size in bytes.
	Size int64
	// Mode is the POSIX file mode (permissions + type bits).
	Mode uint32
	// SHA256 is the file digest in hex format (for digest algo 8).
	SHA256 string
	// LinkTarget is the symlink target if this is a symlink, empty otherwise.
	LinkTarget string
	// User is the file owner username.
	User string
	// Group is the file owner group name.
	Group string
	// MTime is the file modification time.
	MTime time.Time
	// Flags is the RPMFILE_* bitmask (config, doc, ghost, etc.).
	Flags uint32
}
