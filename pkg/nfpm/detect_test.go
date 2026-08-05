package nfpm_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/M0Rf30/yap/v2/pkg/nfpm"
)

func TestDetectSpecFileNames_Order(t *testing.T) {
	assert.Equal(t, []string{"nfpm.yaml", "nfpm.yml", ".nfpm.yaml", ".nfpm.yml"}, nfpm.SpecFileNames)
}

func TestDetectFindSpec(t *testing.T) {
	tests := []struct {
		name   string
		files  []string
		want   string
		wantOk bool
	}{
		{
			name:   "no spec present",
			files:  nil,
			wantOk: false,
		},
		{
			name:   "nfpm.yaml wins first",
			files:  []string{"nfpm.yaml", "nfpm.yml", ".nfpm.yaml", ".nfpm.yml"},
			want:   "nfpm.yaml",
			wantOk: true,
		},
		{
			name:   "nfpm.yml used when nfpm.yaml absent",
			files:  []string{"nfpm.yml", ".nfpm.yaml", ".nfpm.yml"},
			want:   "nfpm.yml",
			wantOk: true,
		},
		{
			name:   "dotfile fallback",
			files:  []string{".nfpm.yaml", ".nfpm.yml"},
			want:   ".nfpm.yaml",
			wantOk: true,
		},
		{
			name:   "last dotfile fallback",
			files:  []string{".nfpm.yml"},
			want:   ".nfpm.yml",
			wantOk: true,
		},
		{
			name:   "unrelated files are ignored",
			files:  []string{"PKGBUILD", "README.md"},
			wantOk: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()

			for _, f := range tt.files {
				content := []byte("name: x\nversion: '1'\n")
				require.NoError(t, os.WriteFile(filepath.Join(dir, f), content, 0o644))
			}

			got, ok := nfpm.FindSpec(dir)

			assert.Equal(t, tt.wantOk, ok)

			if tt.wantOk {
				assert.Equal(t, filepath.Join(dir, tt.want), got)
			} else {
				assert.Empty(t, got)
			}
		})
	}
}

func TestDetectFindSpec_IgnoresDirectoriesNamedLikeASpec(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.Mkdir(filepath.Join(dir, "nfpm.yaml"), 0o750))

	_, ok := nfpm.FindSpec(dir)
	assert.False(t, ok, "a directory named nfpm.yaml must not be mistaken for a spec file")
}

func TestDetectIsSpecFile(t *testing.T) {
	tests := []struct {
		path string
		want bool
	}{
		{"nfpm.yaml", true},
		{"nfpm.yml", true},
		{".nfpm.yaml", true},
		{".nfpm.yml", true},
		{"/some/project/dir/nfpm.yaml", true},
		{"/some/project/dir/.nfpm.yml", true},
		{"PKGBUILD", false},
		{"nfpm.yaml.bak", false},
		{"other-nfpm.yaml", false},
		{"", false},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			assert.Equal(t, tt.want, nfpm.IsSpecFile(tt.path))
		})
	}
}
