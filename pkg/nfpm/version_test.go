package nfpm_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/M0Rf30/yap/v2/pkg/nfpm"
)

func TestSplitVersion(t *testing.T) {
	tests := []struct {
		name           string
		input          string
		wantVersion    string
		wantPrerelease string
		wantMetadata   string
	}{
		{
			name:        "plain semver",
			input:       "1.2.3",
			wantVersion: "1.2.3",
		},
		{
			name:        "v-prefixed semver",
			input:       "v1.2.3",
			wantVersion: "1.2.3",
		},
		{
			name:           "prerelease only",
			input:          "1.2.3-beta.1",
			wantVersion:    "1.2.3",
			wantPrerelease: "beta.1",
		},
		{
			name:         "metadata only",
			input:        "1.2.3+deadbeef",
			wantVersion:  "1.2.3",
			wantMetadata: "deadbeef",
		},
		{
			name:           "prerelease and metadata",
			input:          "v1.2.3-beta.1+deadbeef",
			wantVersion:    "1.2.3",
			wantPrerelease: "beta.1",
			wantMetadata:   "deadbeef",
		},
		{
			name:           "prerelease containing a dash",
			input:          "1.2.3-rc-1+build.5",
			wantVersion:    "1.2.3",
			wantPrerelease: "rc-1",
			wantMetadata:   "build.5",
		},
		{
			name:        "non-semver returned unchanged",
			input:       "notsemver",
			wantVersion: "notsemver",
		},
		{
			name:        "non-semver with dashes returned unchanged",
			input:       "2025-01-15-git",
			wantVersion: "2025-01-15-git",
		},
		{
			name:        "empty string",
			input:       "",
			wantVersion: "",
		},
		{
			name:        "only major minor, not semver-shaped",
			input:       "1.2",
			wantVersion: "1.2",
		},
		{
			name:        "calendar-versioning-shaped triple has no prerelease",
			input:       "2024.01.15",
			wantVersion: "2024.01.15",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			version, prerelease, metadata := nfpm.SplitVersion(tt.input)
			assert.Equal(t, tt.wantVersion, version, "version")
			assert.Equal(t, tt.wantPrerelease, prerelease, "prerelease")
			assert.Equal(t, tt.wantMetadata, metadata, "metadata")
		})
	}
}
