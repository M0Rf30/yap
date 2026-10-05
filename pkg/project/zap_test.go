package project

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/M0Rf30/yap/v2/pkg/builder"
	"github.com/M0Rf30/yap/v2/pkg/pkgbuild"
)

// TestZapSingleProjectKeepsLocalSources ensures zap never deletes tarballs,
// logs or signatures that sit next to the PKGBUILD (legitimate source=() entries).
func TestZapSingleProjectKeepsLocalSources(t *testing.T) {
	dir := t.TempDir()

	keep := []string{"src.tar.gz", "src.tar.xz", "src.tar.bz2", "src.tar.gz.sig", "x.log"}
	drop := []string{"foo_1.0_amd64.deb", "foo-1.0.rpm", "foo-1.0.pkg.tar.zst"}

	for _, name := range append(append([]string{}, keep...), drop...) {
		require.NoError(t, os.WriteFile(filepath.Join(dir, name), []byte("x"), 0o644))
	}

	require.NoError(t, os.MkdirAll(filepath.Join(dir, "src", "sub"), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "pkg"), 0o755))

	mpc := &MultipleProject{
		singleProject: true,
		Projects: []*Project{{
			Builder: &builder.Builder{PKGBUILD: &pkgbuild.PKGBUILD{StartDir: dir}},
		}},
		Opts: BuildOptions{Zap: true},
	}

	require.NoError(t, mpc.Clean())

	for _, name := range keep {
		assert.FileExists(t, filepath.Join(dir, name), name)
	}

	for _, name := range drop {
		assert.NoFileExists(t, filepath.Join(dir, name), name)
	}

	assert.NoDirExists(t, filepath.Join(dir, "src"))
	assert.NoDirExists(t, filepath.Join(dir, "pkg"))
}

// TestCleanHonoursFromTo ensures Clean only touches projects inside the
// --from/--to range.
func TestCleanHonoursFromTo(t *testing.T) {
	root := t.TempDir()
	names := []string{"a", "b", "c"}
	projs := make([]*Project, 0, len(names))

	for _, n := range names {
		start := filepath.Join(root, n)
		require.NoError(t, os.MkdirAll(start, 0o755))

		projs = append(projs, &Project{
			Builder: &builder.Builder{PKGBUILD: &pkgbuild.PKGBUILD{
				PkgName: n, StartDir: start,
			}},
		})
	}

	mpc := &MultipleProject{
		Projects: projs,
		Opts:     BuildOptions{Zap: true, FromPkgName: "b", ToPkgName: "b"},
	}

	require.NoError(t, mpc.Clean())

	assert.DirExists(t, filepath.Join(root, "a"))
	assert.NoDirExists(t, filepath.Join(root, "b"))
	assert.DirExists(t, filepath.Join(root, "c"))
}
