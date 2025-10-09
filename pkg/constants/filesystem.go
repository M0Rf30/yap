// Package constants defines global constants and configuration values used throughout YAP.
package constants

import (
	"time"
)

const (
	// DefaultBufferSize is the default buffer size for filesystem operations.
	DefaultBufferSize = 32 * 1024
	// SmallBufferSize is the buffer size for smaller operations.
	SmallBufferSize = 1024

	// DefaultDirPerm is the default permission for directories.
	DefaultDirPerm = 0o755
	// DefaultFilePerm is the default permission for files.
	DefaultFilePerm = 0o644
	// WritePerm is the write permission mask.
	WritePerm = 0o200

	// TimestampFormat is the standard timestamp format used throughout YAP.
	TimestampFormat = "2006-01-02 15:04:05"
	// ProgressUpdateInterval is the interval for updating progress indicators.
	ProgressUpdateInterval = 500 * time.Millisecond
	// TickerInterval is the default ticker interval.
	TickerInterval = 100 * time.Millisecond

	// KBDivisor is the divisor for converting bytes to kilobytes.
	KBDivisor = 1024
)
