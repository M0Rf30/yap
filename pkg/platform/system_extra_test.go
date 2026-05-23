package platform

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestIsPrivilegedHost verifies the function returns a consistent bool
// and matches the actual effective UID.
func TestIsPrivilegedHost(t *testing.T) {
	got := IsPrivilegedHost()
	want := os.Geteuid() == 0
	assert.Equal(t, want, got, "IsPrivilegedHost should match os.Geteuid()==0")
}

// TestParseOSReleaseFromTempFile exercises the parser against a synthetic
// os-release file so we don't depend on the host's /etc/os-release content.
func TestParseOSReleaseFromTempFile(t *testing.T) {
	tests := []struct {
		name         string
		content      string
		wantID       string
		wantCodename string
	}{
		{
			name: "ubuntu jammy",
			content: `# This is a comment
ID=ubuntu
VERSION_CODENAME=jammy
NAME="Ubuntu"
`,
			wantID:       "ubuntu",
			wantCodename: "jammy",
		},
		{
			name: "quoted values",
			content: `ID="debian"
VERSION_CODENAME="bookworm"
`,
			wantID:       "debian",
			wantCodename: "bookworm",
		},
		{
			name: "missing codename",
			content: `ID=fedora
VERSION_ID=39
`,
			wantID:       "fedora",
			wantCodename: "",
		},
		{
			name:         "empty file",
			content:      "",
			wantID:       "",
			wantCodename: "",
		},
		{
			name: "only comments and blanks",
			content: `# comment
# another comment

`,
			wantID:       "",
			wantCodename: "",
		},
		{
			name: "whitespace around key",
			content: `  ID  =alpine
VERSION_CODENAME=edge
`,
			wantID:       "alpine",
			wantCodename: "edge",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Write a temp os-release file and point ParseOSRelease at it via
			// a symlink trick: we can't change the hard-coded path, so we use
			// the internal parseOSReleaseFromReader helper exposed via the
			// package-internal test helper below.
			dir := t.TempDir()
			fpath := filepath.Join(dir, "os-release")
			require.NoError(t, os.WriteFile(fpath, []byte(tc.content), 0o644))

			got, err := parseOSReleaseFile(fpath)
			require.NoError(t, err)
			assert.Equal(t, tc.wantID, got.ID)
			assert.Equal(t, tc.wantCodename, got.Codename)
		})
	}
}

// TestParseOSReleaseFileNotFound verifies the error path when the file is absent.
func TestParseOSReleaseFileNotFound(t *testing.T) {
	_, err := parseOSReleaseFile("/nonexistent/path/os-release-does-not-exist")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "os-release")
}

// TestGetArchitectureKnownArches verifies that the current GOARCH maps to a
// non-empty string (the mapping table covers all tier-1 Go platforms).
func TestGetArchitectureNonEmpty(t *testing.T) {
	arch := GetArchitecture()
	assert.NotEmpty(t, arch, "GetArchitecture must never return empty string")
}
