package dnfinstall

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestSafeRPMPath tests path sanitization for CPIO entries.
func TestSafeRPMPath(t *testing.T) {
	rootDir := "/tmp/fakeroot"

	tests := []struct {
		name      string
		entryName string
		wantOK    bool
		wantPath  string
	}{
		{
			name:      "normal file",
			entryName: "bin/busybox",
			wantOK:    true,
			wantPath:  "/tmp/fakeroot/bin/busybox",
		},
		{
			name:      "nested directory",
			entryName: "usr/local/bin/myapp",
			wantOK:    true,
			wantPath:  "/tmp/fakeroot/usr/local/bin/myapp",
		},
		{
			name:      "absolute path stripped",
			entryName: "/etc/passwd",
			wantOK:    true,
			wantPath:  "/tmp/fakeroot/etc/passwd",
		},
		{
			name:      "parent traversal rejected",
			entryName: "../etc/passwd",
			wantOK:    false,
		},
		{
			name:      "parent traversal in middle rejected",
			entryName: "bin/../../../etc/passwd",
			wantOK:    false,
		},
		{
			name:      "dot rejected",
			entryName: ".",
			wantOK:    false,
		},
		{
			name:      "slash rejected",
			entryName: "/",
			wantOK:    false,
		},
		{
			name:      "empty string rejected",
			entryName: "",
			wantOK:    false,
		},
		{
			name:      "double dot rejected",
			entryName: "..",
			wantOK:    false,
		},
		{
			name:      "file with spaces",
			entryName: "usr/share/doc/my file.txt",
			wantOK:    true,
			wantPath:  "/tmp/fakeroot/usr/share/doc/my file.txt",
		},
		{
			name:      "symlink-like path",
			entryName: "bin/sh",
			wantOK:    true,
			wantPath:  "/tmp/fakeroot/bin/sh",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path, err := safeRPMPath(rootDir, tt.entryName)
			if tt.wantOK {
				require.NoError(t, err)
				assert.Equal(t, tt.wantPath, path)
			} else {
				require.Error(t, err)
			}
		})
	}
}

// TestSafeRPMPathWithRootSlash tests safeRPMPath with "/" as rootDir.
func TestSafeRPMPathWithRootSlash(t *testing.T) {
	rootDir := "/"

	tests := []struct {
		name      string
		entryName string
		wantOK    bool
		wantPath  string
	}{
		{
			name:      "normal file",
			entryName: "bin/busybox",
			wantOK:    true,
			wantPath:  "/bin/busybox",
		},
		{
			name:      "absolute path stripped",
			entryName: "/etc/passwd",
			wantOK:    true,
			wantPath:  "/etc/passwd",
		},
		{
			name:      "parent traversal resolves within root",
			entryName: "../etc/passwd",
			wantOK:    true,
			wantPath:  "/etc/passwd",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path, err := safeRPMPath(rootDir, tt.entryName)
			if tt.wantOK {
				require.NoError(t, err)
				assert.Equal(t, tt.wantPath, path)
			} else {
				require.Error(t, err)
			}
		})
	}
}

// TestSafeRPMSymlinkTarget tests symlink target validation.
func TestSafeRPMSymlinkTarget(t *testing.T) {
	rootDir := "/tmp/fakeroot"

	tests := []struct {
		name      string
		linkPath  string
		target    string
		wantErr   bool
		wantErrSub string
	}{
		{
			name:     "absolute target allowed",
			linkPath: "/tmp/fakeroot/bin/sh",
			target:   "/bin/bash",
			wantErr:  false,
		},
		{
			name:     "relative target within rootDir",
			linkPath: "/tmp/fakeroot/bin/sh",
			target:   "bash",
			wantErr:  false,
		},
		{
			name:     "relative target with ../ within rootDir",
			linkPath: "/tmp/fakeroot/usr/bin/sh",
			target:   "../../bin/bash",
			wantErr:  false,
		},
		{
			name:       "relative target escaping rootDir",
			linkPath:   "/tmp/fakeroot/bin/sh",
			target:     "../../../../../../etc/passwd",
			wantErr:    true,
			wantErrSub: "escapes rootDir",
		},
		{
			name:       "relative target with .. at root",
			linkPath:   "/tmp/fakeroot/sh",
			target:     "../etc/passwd",
			wantErr:    true,
			wantErrSub: "escapes rootDir",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := safeRPMSymlinkTarget(rootDir, tt.linkPath, tt.target)
			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErrSub)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

// TestSafeRPMSymlinkTargetWithRootSlash tests symlink validation with "/" as rootDir.
func TestSafeRPMSymlinkTargetWithRootSlash(t *testing.T) {
	rootDir := "/"

	tests := []struct {
		name      string
		linkPath  string
		target    string
		wantErr   bool
	}{
		{
			name:     "absolute target allowed",
			linkPath: "/bin/sh",
			target:   "/bin/bash",
			wantErr:  false,
		},
		{
			name:     "relative target within root",
			linkPath: "/bin/sh",
			target:   "bash",
			wantErr:  false,
		},
		{
			name:     "relative target with ../ within root",
			linkPath: "/usr/bin/sh",
			target:   "../../bin/bash",
			wantErr:  false,
		},
		{
			name:    "relative target resolves within root",
			linkPath: "/bin/sh",
			target:   "../../../../../../etc/passwd",
			wantErr:  false, // resolves to /etc/passwd which is under /
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := safeRPMSymlinkTarget(rootDir, tt.linkPath, tt.target)
			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}
