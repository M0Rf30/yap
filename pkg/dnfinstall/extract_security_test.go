package dnfinstall //nolint:testpackage

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/M0Rf30/rpmpack"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func buildFilesRPM(t *testing.T, files []rpmpack.RPMFile) string {
	t.Helper()

	rpm, err := rpmpack.NewRPM(rpmpack.RPMMetaData{
		Name: "sec-pkg", Version: "1", Release: "1", Arch: "x86_64",
		Compressor: "gzip", BuildTime: time.Now(),
	})
	require.NoError(t, err)

	for _, f := range files {
		rpm.AddFile(f)
	}

	p := filepath.Join(t.TempDir(), "sec.rpm")
	out, err := os.Create(p)
	require.NoError(t, err)

	defer func() { _ = out.Close() }()

	require.NoError(t, rpm.Write(out))

	return p
}

func TestExtractSymlinkCannotEscapeRoot(t *testing.T) {
	base := t.TempDir()
	root := filepath.Join(base, "root")
	outside := filepath.Join(base, "outside")

	require.NoError(t, os.MkdirAll(root, 0o755))
	require.NoError(t, os.MkdirAll(outside, 0o755))

	rpmPath := buildFilesRPM(t, []rpmpack.RPMFile{
		{Name: "/link", Body: []byte(outside), Mode: 0o120777},
		{Name: "/link/x", Body: []byte("pwn"), Mode: 0o644},
	})

	_, err := extractRPM(context.Background(), rpmPath, root, Options{})
	require.NoError(t, err)

	_, statErr := os.Stat(filepath.Join(outside, "x"))
	assert.True(t, os.IsNotExist(statErr), "write escaped rootDir")
}

func TestExtractPreservesSetuidAndRootRelativePaths(t *testing.T) {
	root := t.TempDir()
	rpmPath := buildFilesRPM(t, []rpmpack.RPMFile{
		{Name: "/usr/bin/su", Body: []byte("x"), Mode: 0o104755},
	})

	entry, err := extractRPM(context.Background(), rpmPath, root, Options{})
	require.NoError(t, err)

	fi, err := os.Stat(filepath.Join(root, "usr", "bin", "su"))
	require.NoError(t, err)
	assert.NotZero(t, fi.Mode()&os.ModeSetuid)

	var found bool

	for _, f := range entry.Files {
		if f.Path == "/usr/bin/su" {
			found = true

			assert.Equal(t, uint32(0o4755), posixMode(f.Mode))
		}
	}

	assert.True(t, found, "root-relative path recorded")
}

func TestLooksLuaNotShellPaths(t *testing.T) {
	assert.False(t, looksLikeLua("cp x /etc/profile.d/path.sh\n"))
	assert.True(t, looksLikeLua("local p = path.exists('/x')\n"))
}
