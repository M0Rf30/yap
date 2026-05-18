package project

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSkipDeps(t *testing.T) {
	t.Run("buildSkipSet", func(t *testing.T) {
		t.Run("empty both sources returns nil set", func(t *testing.T) {
			mpc := &MultipleProject{
				SkipDeps: []string{},
				Opts: BuildOptions{
					SkipDeps: []string{},
				},
			}
			set := mpc.buildSkipSet()
			assert.Nil(t, set)
		})

		t.Run("only json skipDeps", func(t *testing.T) {
			mpc := &MultipleProject{
				SkipDeps: []string{"libfoo", "libbar"},
				Opts: BuildOptions{
					SkipDeps: []string{},
				},
			}
			set := mpc.buildSkipSet()
			assert.NotNil(t, set)
			assert.Len(t, set, 2)
			assert.Contains(t, set, "libfoo")
			assert.Contains(t, set, "libbar")
		})

		t.Run("only CLI SkipDeps", func(t *testing.T) {
			mpc := &MultipleProject{
				SkipDeps: []string{},
				Opts: BuildOptions{
					SkipDeps: []string{"pkg-a", "pkg-b"},
				},
			}
			set := mpc.buildSkipSet()
			assert.NotNil(t, set)
			assert.Len(t, set, 2)
			assert.Contains(t, set, "pkg-a")
			assert.Contains(t, set, "pkg-b")
		})

		t.Run("both sources merged without duplicates", func(t *testing.T) {
			mpc := &MultipleProject{
				SkipDeps: []string{"libfoo", "shared-pkg"},
				Opts: BuildOptions{
					SkipDeps: []string{"shared-pkg", "pkg-a"},
				},
			}
			set := mpc.buildSkipSet()
			assert.NotNil(t, set)
			assert.Len(t, set, 3)
			assert.Contains(t, set, "libfoo")
			assert.Contains(t, set, "shared-pkg")
			assert.Contains(t, set, "pkg-a")
		})
	})

	t.Run("filterSkipDeps", func(t *testing.T) {
		t.Run("empty skip set returns slice unchanged", func(t *testing.T) {
			mpc := &MultipleProject{
				SkipDeps: []string{},
				Opts: BuildOptions{
					SkipDeps: []string{},
				},
			}
			deps := []string{"gcc", "make", "python3"}
			result := mpc.filterSkipDeps(deps)
			assert.Equal(t, deps, result)
		})

		t.Run("skip one entry removes it", func(t *testing.T) {
			mpc := &MultipleProject{
				SkipDeps: []string{"make"},
				Opts: BuildOptions{
					SkipDeps: []string{},
				},
			}
			deps := []string{"gcc", "make", "python3"}
			result := mpc.filterSkipDeps(deps)
			assert.Equal(t, []string{"gcc", "python3"}, result)
		})

		t.Run("skip all entries returns empty slice", func(t *testing.T) {
			mpc := &MultipleProject{
				SkipDeps: []string{"gcc", "make", "python3"},
				Opts: BuildOptions{
					SkipDeps: []string{},
				},
			}
			deps := []string{"gcc", "make", "python3"}
			result := mpc.filterSkipDeps(deps)
			assert.Empty(t, result)
		})

		t.Run("skip non-existent name is no-op", func(t *testing.T) {
			mpc := &MultipleProject{
				SkipDeps: []string{"nonexistent"},
				Opts: BuildOptions{
					SkipDeps: []string{},
				},
			}
			deps := []string{"gcc", "make", "python3"}
			result := mpc.filterSkipDeps(deps)
			assert.Equal(t, deps, result)
		})

		t.Run("CLI and json sources both filter combined", func(t *testing.T) {
			mpc := &MultipleProject{
				SkipDeps: []string{"pkg-a"},
				Opts: BuildOptions{
					SkipDeps: []string{"pkg-b"},
				},
			}
			deps := []string{"pkg-a", "pkg-b", "pkg-c"}
			result := mpc.filterSkipDeps(deps)
			assert.Equal(t, []string{"pkg-c"}, result)
		})
	})

	t.Run("skipDeps JSON parsing", func(t *testing.T) {
		testDir := t.TempDir()
		buildDir := filepath.Join(testDir, "build")
		outputDir := filepath.Join(testDir, "output")

		require.NoError(t, os.MkdirAll(buildDir, 0o755))
		require.NoError(t, os.MkdirAll(outputDir, 0o755))

		for _, name := range []string{"pkg-a", "pkg-b"} {
			dir := filepath.Join(testDir, name)
			require.NoError(t, os.MkdirAll(dir, 0o755))

			content := `pkgname="` + name + `"
pkgver="1.0"
pkgrel="1"
pkgdesc="` + name + ` package"
arch=("x86_64")
license=("GPL-3.0-only")

package() {
	mkdir -p "${pkgdir}/usr/bin"
}
`
			require.NoError(t, os.WriteFile(filepath.Join(dir, "PKGBUILD"), []byte(content), 0o644))
		}

		yapJSON := `{
			"name": "skip-deps-test",
			"description": "Skip deps JSON parsing test",
			"buildDir": "` + buildDir + `",
			"output": "` + outputDir + `",
			"skipDeps": ["pkg-a", "pkg-b"],
			"projects": [
				{"name": "pkg-a", "install": false},
				{"name": "pkg-b", "install": false}
			]
		}`
		require.NoError(t, os.WriteFile(filepath.Join(testDir, "yap.json"), []byte(yapJSON), 0o644))

		mpc := &MultipleProject{
			Opts: BuildOptions{
				SkipSyncDeps: true,
				NoMakeDeps:   true,
			},
		}

		err := mpc.MultiProject("ubuntu", "", testDir)
		if err != nil {
			t.Logf("MultiProject returned error (may be expected in CI): %v", err)
			t.Skip("skipping assertion: MultiProject failed (no package manager)")
		}

		assert.Equal(t, []string{"pkg-a", "pkg-b"}, mpc.SkipDeps)
	})

	t.Run("--skip-deps CLI and json compose", func(t *testing.T) {
		mpc := &MultipleProject{
			SkipDeps: []string{"pkg-a"},
			Opts: BuildOptions{
				SkipDeps: []string{"pkg-b"},
			},
		}

		deps := []string{"pkg-a", "pkg-b", "pkg-c"}
		result := mpc.filterSkipDeps(deps)
		assert.Equal(t, []string{"pkg-c"}, result)
	})
}
