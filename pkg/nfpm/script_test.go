package nfpm

import (
	"bytes"
	"context"
	"os"
	"os/user"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"mvdan.cc/sh/v3/expand"
	"mvdan.cc/sh/v3/interp"
	"mvdan.cc/sh/v3/syntax"

	"github.com/M0Rf30/yap/v2/pkg/i18n"
)

func TestMain(m *testing.M) {
	_ = i18n.Init("en")

	os.Exit(m.Run())
}

// mustParseBash fails the test if script does not parse as valid bash.
func mustParseBash(t *testing.T, script string) {
	t.Helper()

	_, err := syntax.NewParser(syntax.Variant(syntax.LangBash)).Parse(strings.NewReader(script), "package")
	require.NoError(t, err, "generated script must parse as valid bash:\n%s", script)
}

// runPackageScript executes script through mvdan/sh with startdir/pkgdir set,
// returning any runtime error and the combined output for diagnostics.
func runPackageScript(t *testing.T, startDir, pkgDir, script string) error {
	t.Helper()

	parsed, err := syntax.NewParser(syntax.Variant(syntax.LangBash)).Parse(strings.NewReader(script), "package")
	require.NoError(t, err)

	env := os.Environ()
	env = append(env, "startdir="+startDir, "pkgdir="+pkgDir)

	var out bytes.Buffer

	runner, err := interp.New(
		interp.Env(expand.ListEnviron(env...)),
		interp.StdIO(nil, &out, &out),
	)
	require.NoError(t, err)

	runErr := runner.Run(context.Background(), parsed)
	if runErr != nil {
		t.Logf("script output:\n%s", out.String())
	}

	return runErr
}

func currentOwnerGroup(t *testing.T) (owner, group string) {
	t.Helper()

	u, err := user.Current()
	require.NoError(t, err)

	g, err := user.LookupGroupId(u.Gid)
	require.NoError(t, err)

	return u.Username, g.Name
}

func TestBuildPackageScript_Empty(t *testing.T) {
	script, msgs := BuildPackageScript(nil, true)
	assert.Empty(t, script)
	assert.Empty(t, msgs)
}

func TestBuildPackageScript_Golden(t *testing.T) {
	tests := []struct {
		name        string
		content     *Content
		expandGlobs bool
		want        string
	}{
		{
			name: "dir with explicit mode/owner/group",
			content: &Content{
				Type:        TypeDir,
				Destination: "/etc/app",
				FileInfo:    &ContentFileInfo{Mode: 0o750, Owner: "alice", Group: "users"},
			},
			want: `install -d -m0750 -o alice -g users "${pkgdir}"'/etc/app'` + "\n",
		},
		{
			name: "implicit dir defaults",
			content: &Content{
				Type:        TypeImplicitDir,
				Destination: "/var/lib/app",
			},
			want: `install -d -m0755 -o root -g root "${pkgdir}"'/var/lib/app'` + "\n",
		},
		{
			name: "symlink",
			content: &Content{
				Type:        TypeSymlink,
				Source:      "target-file",
				Destination: "/usr/lib/libfoo.so",
			},
			want: `install -d "${pkgdir}"'/usr/lib'` + "\n" +
				`ln -sfn 'target-file' "${pkgdir}"'/usr/lib/libfoo.so'` + "\n",
		},
		{
			name: "file relative source",
			content: &Content{
				Type:        TypeFile,
				Source:      "bin/app",
				Destination: "/usr/bin/app",
				FileInfo:    &ContentFileInfo{Mode: 0o755, Owner: "root", Group: "root"},
			},
			expandGlobs: true,
			want:        `install -D -m0755 -o root -g root "${startdir}/"'bin/app' "${pkgdir}"'/usr/bin/app'` + "\n",
		},
		{
			name: "file absolute source uses default mode",
			content: &Content{
				Type:        TypeFile,
				Source:      "/abs/path/app",
				Destination: "/usr/bin/app",
			},
			want: `install -D -m0644 -o root -g root '/abs/path/app' "${pkgdir}"'/usr/bin/app'` + "\n",
		},
		{
			name: "config type renders as a plain file install",
			content: &Content{
				Type:        TypeConfig,
				Source:      "etc/app.conf",
				Destination: "/etc/app.conf",
				FileInfo:    &ContentFileInfo{Mode: 0o644, Owner: "root", Group: "root"},
			},
			want: `install -D -m0644 -o root -g root "${startdir}/"'etc/app.conf' "${pkgdir}"'/etc/app.conf'` + "\n",
		},
		{
			name: "tree unexpanded",
			content: &Content{
				Type:        TypeTree,
				Source:      "docs",
				Destination: "/usr/share/doc/app",
			},
			want: `install -d -m0755 -o root -g root "${pkgdir}"'/usr/share/doc/app'` + "\n" +
				`cp -a "${startdir}/"'docs/.' "${pkgdir}"'/usr/share/doc/app/'` + "\n",
		},
		{
			name: "config tree unexpanded",
			content: &Content{
				Type:        TypeConfigTree,
				Source:      "/abs/etc/app.d",
				Destination: "/etc/app.d",
			},
			want: `install -d -m0755 -o root -g root "${pkgdir}"'/etc/app.d'` + "\n" +
				`cp -a '/abs/etc/app.d/.' "${pkgdir}"'/etc/app.d/'` + "\n",
		},
		{
			name: "unexpanded glob relative source",
			content: &Content{
				Type:        TypeFile,
				Source:      "conf/*.conf",
				Destination: "/etc/app/confd",
				FileInfo:    &ContentFileInfo{Mode: 0o644, Owner: "root", Group: "root"},
			},
			want: `for f in "${startdir}/"conf/*.conf; do` + "\n" +
				`  install -D -m0644 -o root -g root "$f" "${pkgdir}"'/etc/app/confd'"/$(basename "$f")"` + "\n" +
				`done` + "\n",
		},
		{
			name: "unexpanded glob absolute source",
			content: &Content{
				Type:        TypeFile,
				Source:      "/abs/conf/*.conf",
				Destination: "/etc/app/confd",
				FileInfo:    &ContentFileInfo{Mode: 0o644, Owner: "root", Group: "root"},
			},
			want: `for f in /abs/conf/*.conf; do` + "\n" +
				`  install -D -m0644 -o root -g root "$f" "${pkgdir}"'/etc/app/confd'"/$(basename "$f")"` + "\n" +
				`done` + "\n",
		},
		{
			name: "expandGlobs true treats a literal star as a filename",
			content: &Content{
				Type:        TypeFile,
				Source:      "weird/*literal",
				Destination: "/usr/share/app/weird",
				FileInfo:    &ContentFileInfo{Mode: 0o644, Owner: "root", Group: "root"},
			},
			expandGlobs: true,
			want:        `install -D -m0644 -o root -g root "${startdir}/"'weird/*literal' "${pkgdir}"'/usr/share/app/weird'` + "\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, msgs := BuildPackageScript(Contents{tt.content}, tt.expandGlobs)
			assert.Equal(t, tt.want, got)
			assert.Empty(t, msgs)
			mustParseBash(t, got)
		})
	}
}

func TestBuildPackageScript_DroppedContent(t *testing.T) {
	contents := Contents{
		{Type: TypeRPMGhost, Destination: "/var/run/app.pid"},
		{Type: TypeDebChangelog, Destination: "/usr/share/doc/app/changelog.gz"},
		{
			Type: TypeFile, Source: "locale/app.mo", Destination: "/usr/share/locale/en/app.mo",
			FileInfo: &ContentFileInfo{Lang: "en", Mode: 0o644, Owner: "root", Group: "root"},
		},
	}

	script, msgs := BuildPackageScript(contents, true)

	assert.NotContains(t, script, "app.pid")
	assert.NotContains(t, script, "changelog.gz")
	assert.Contains(t, script, "app.mo")

	require.Len(t, msgs, 3)
	assert.Contains(t, msgs[0], "/var/run/app.pid")
	assert.Contains(t, msgs[1], "/usr/share/doc/app/changelog.gz")
	assert.Contains(t, msgs[2], "/usr/share/locale/en/app.mo")
}

func TestBuildPackageScript_ParsesAsValidBash(t *testing.T) {
	contents := Contents{
		{Type: TypeDir, Destination: "/etc/app"},
		{Type: TypeSymlink, Source: "app-1.0", Destination: "/usr/bin/app"},
		{Type: TypeFile, Source: "bin/app-1.0", Destination: "/usr/bin/app-1.0"},
		{Type: TypeConfig, Source: "etc/app.conf", Destination: "/etc/app.conf"},
		{Type: TypeTree, Source: "docs", Destination: "/usr/share/doc/app"},
		{Type: TypeFile, Source: "conf/*.conf", Destination: "/etc/app/confd"},
		{Type: TypeRPMGhost, Destination: "/var/run/app.pid"},
	}

	for _, expandGlobs := range []bool{true, false} {
		script, _ := BuildPackageScript(contents, expandGlobs)
		mustParseBash(t, script)
		assert.True(t, strings.HasSuffix(script, "\n"))
	}
}

// TestBuildPackageScript_ExecutesAndInstallsFiles proves the synthesized
// package() body actually installs real files: it builds a temp startdir,
// runs the emitted body against a temp pkgdir through mvdan/sh, then
// inspects the resulting tree, modes and symlinks. Owner/group are set to
// the current user so `install -o/-g` needs no privileges.
func TestBuildPackageScript_ExecutesAndInstallsFiles(t *testing.T) {
	owner, group := currentOwnerGroup(t)

	startDir := t.TempDir()
	pkgDir := t.TempDir()

	writeFile(t, filepath.Join(startDir, "bin", "myapp"), "binary-payload", 0o755)
	writeFile(t, filepath.Join(startDir, "etc", "myapp.conf"), "key=value\n", 0o644)
	writeFile(t, filepath.Join(startDir, "docs", "readme.txt"), "readme\n", 0o644)
	writeFile(t, filepath.Join(startDir, "docs", "sub", "nested.txt"), "nested\n", 0o644)
	writeFile(t, filepath.Join(startDir, "conf", "a.conf"), "a\n", 0o644)
	writeFile(t, filepath.Join(startDir, "conf", "b.conf"), "b\n", 0o644)

	contents := Contents{
		{
			Type: TypeDir, Destination: "/etc/myapp",
			FileInfo: &ContentFileInfo{Mode: 0o755, Owner: owner, Group: group},
		},
		{
			Type: TypeFile, Source: "bin/myapp", Destination: "/usr/bin/myapp",
			FileInfo: &ContentFileInfo{Mode: 0o755, Owner: owner, Group: group},
		},
		{
			Type: TypeConfig, Source: "etc/myapp.conf", Destination: "/etc/myapp/myapp.conf",
			FileInfo: &ContentFileInfo{Mode: 0o644, Owner: owner, Group: group},
		},
		{
			Type: TypeSymlink, Source: "myapp", Destination: "/usr/bin/myapp-symlink",
			FileInfo: &ContentFileInfo{Owner: owner, Group: group},
		},
		{
			Type: TypeTree, Source: "docs", Destination: "/usr/share/doc/myapp",
			FileInfo: &ContentFileInfo{Mode: 0o755, Owner: owner, Group: group},
		},
		{
			Type: TypeFile, Source: "conf/*.conf", Destination: "/etc/myapp/confd",
			FileInfo: &ContentFileInfo{Mode: 0o644, Owner: owner, Group: group},
		},
	}

	script, msgs := BuildPackageScript(contents, false)
	require.Empty(t, msgs)
	mustParseBash(t, script)

	require.NoError(t, runPackageScript(t, startDir, pkgDir, script))

	// dir
	dirInfo, err := os.Stat(filepath.Join(pkgDir, "etc", "myapp"))
	require.NoError(t, err)
	assert.True(t, dirInfo.IsDir())
	assert.Equal(t, os.FileMode(0o755), dirInfo.Mode().Perm())

	// file
	appInfo, err := os.Stat(filepath.Join(pkgDir, "usr", "bin", "myapp"))
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o755), appInfo.Mode().Perm())
	assertFileContent(t, filepath.Join(pkgDir, "usr", "bin", "myapp"), "binary-payload")

	// config file
	assertFileContent(t, filepath.Join(pkgDir, "etc", "myapp", "myapp.conf"), "key=value\n")

	// symlink
	target, err := os.Readlink(filepath.Join(pkgDir, "usr", "bin", "myapp-symlink"))
	require.NoError(t, err)
	assert.Equal(t, "myapp", target)

	// tree copy
	assertFileContent(t, filepath.Join(pkgDir, "usr", "share", "doc", "myapp", "readme.txt"), "readme\n")
	assertFileContent(t, filepath.Join(pkgDir, "usr", "share", "doc", "myapp", "sub", "nested.txt"), "nested\n")

	// glob loop
	assertFileContent(t, filepath.Join(pkgDir, "etc", "myapp", "confd", "a.conf"), "a\n")
	assertFileContent(t, filepath.Join(pkgDir, "etc", "myapp", "confd", "b.conf"), "b\n")

	confInfo, err := os.Stat(filepath.Join(pkgDir, "etc", "myapp", "confd", "a.conf"))
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o644), confInfo.Mode().Perm())
}

func writeFile(t *testing.T, path, content string, mode os.FileMode) {
	t.Helper()

	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte(content), mode))
}

func assertFileContent(t *testing.T, path, want string) {
	t.Helper()

	got, err := os.ReadFile(path) //nolint:gosec // test fixture path
	require.NoError(t, err)
	assert.Equal(t, want, string(got))
}

func TestModeFor(t *testing.T) {
	assert.Equal(t, os.FileMode(0o755), modeFor(nil, true))
	assert.Equal(t, os.FileMode(0o644), modeFor(nil, false))
	assert.Equal(t, os.FileMode(0o600), modeFor(&ContentFileInfo{Mode: 0o600}, false))
}

func TestContentOwnerGroup(t *testing.T) {
	owner, group := contentOwnerGroup(nil)
	assert.Equal(t, "root", owner)
	assert.Equal(t, "root", group)

	owner, group = contentOwnerGroup(&ContentFileInfo{Owner: "alice"})
	assert.Equal(t, "alice", owner)
	assert.Equal(t, "root", group)
}

func TestNormalizeDest(t *testing.T) {
	assert.Equal(t, "/", normalizeDest(""))
	assert.Equal(t, "/etc/app", normalizeDest("etc/app"))
	assert.Equal(t, "/etc/app", normalizeDest("/etc/app/"))
	assert.Equal(t, "/etc/app", normalizeDest("/etc/./app"))
}
