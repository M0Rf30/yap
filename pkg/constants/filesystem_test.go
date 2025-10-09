// Package constants provides tests for the constants package.
package constants

import (
	"testing"
	"time"
)

func TestFilesystemConstants(t *testing.T) {
	// Test buffer sizes
	if DefaultBufferSize != 32*1024 {
		t.Errorf("DefaultBufferSize = %d; want %d", DefaultBufferSize, 32*1024)
	}

	if SmallBufferSize != 1024 {
		t.Errorf("SmallBufferSize = %d; want %d", SmallBufferSize, 1024)
	}

	// Test file permissions
	if DefaultDirPerm != 0o755 {
		t.Errorf("DefaultDirPerm = %d; want %d", DefaultDirPerm, 0o755)
	}

	if DefaultFilePerm != 0o644 {
		t.Errorf("DefaultFilePerm = %d; want %d", DefaultFilePerm, 0o644)
	}

	if WritePerm != 0o200 {
		t.Errorf("WritePerm = %d; want %d", WritePerm, 0o200)
	}

	// Test time formats
	if TimestampFormat != "2006-01-02 15:04:05" {
		t.Errorf("TimestampFormat = %s; want %s", TimestampFormat, "2006-01-02 15:04:05")
	}

	// Test intervals
	if ProgressUpdateInterval != 500*time.Millisecond {
		t.Errorf("ProgressUpdateInterval = %v; want %v", ProgressUpdateInterval, 500*time.Millisecond)
	}

	if TickerInterval != 100*time.Millisecond {
		t.Errorf("TickerInterval = %v; want %v", TickerInterval, 100*time.Millisecond)
	}

	// Test conversions
	if KBDivisor != 1024 {
		t.Errorf("KBDivisor = %d; want %d", KBDivisor, 1024)
	}
}
