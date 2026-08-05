package nfpm_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/M0Rf30/yap/v2/pkg/nfpm"
)

func TestContent_IsConfig(t *testing.T) {
	tests := []struct {
		typ  string
		want bool
	}{
		{nfpm.TypeConfig, true},
		{nfpm.TypeConfigNoReplace, true},
		{nfpm.TypeConfigMissingOK, true},
		{nfpm.TypeConfigTree, true},
		{nfpm.TypeConfigNoReplaceTree, true},
		{nfpm.TypeConfigMissingOKTree, true},
		{nfpm.TypeFile, false},
		{nfpm.TypeDir, false},
		{nfpm.TypeTree, false},
		{nfpm.TypeSymlink, false},
		{nfpm.TypeRPMGhost, false},
		{"", false},
	}

	for _, tt := range tests {
		t.Run(tt.typ, func(t *testing.T) {
			c := &nfpm.Content{Type: tt.typ}
			assert.Equal(t, tt.want, c.IsConfig())
		})
	}
}

func TestContent_IsTree(t *testing.T) {
	tests := []struct {
		typ  string
		want bool
	}{
		{nfpm.TypeTree, true},
		{nfpm.TypeConfigTree, true},
		{nfpm.TypeConfigNoReplaceTree, true},
		{nfpm.TypeConfigMissingOKTree, true},
		{nfpm.TypeFile, false},
		{nfpm.TypeConfig, false},
		{nfpm.TypeDir, false},
		{nfpm.TypeSymlink, false},
		{"", false},
	}

	for _, tt := range tests {
		t.Run(tt.typ, func(t *testing.T) {
			c := &nfpm.Content{Type: tt.typ}
			assert.Equal(t, tt.want, c.IsTree())
		})
	}
}

// writeFile writes content to a fresh file under dir/rel with the exact
// given permission bits (bypassing umask, unlike os.WriteFile's implicit
// process umask), so mode-masking tests are reproducible across machines.
func writeFile(t *testing.T, dir, rel string, mode os.FileMode) {
	t.Helper()

	path := filepath.Join(dir, rel)
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte("data"), 0o644))
	require.NoError(t, os.Chmod(path, mode))
}

func TestContentPrepareForPackager_FileDefaults(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "bin/tool", 0o765)

	mtime := time.Date(2025, 6, 1, 0, 0, 0, 0, time.UTC)

	cs := nfpm.Contents{
		{Source: "bin/tool", Destination: "/usr/bin/tool", Type: nfpm.TypeFile},
	}

	out, err := cs.PrepareForPackager(dir, nfpm.PackagerDeb, 0o022, false, mtime)
	require.NoError(t, err)
	require.Len(t, out, 1)

	entry := out[0]
	assert.Equal(t, "/usr/bin/tool", entry.Destination)
	require.NotNil(t, entry.FileInfo)
	assert.Equal(t, "root", entry.FileInfo.Owner)
	assert.Equal(t, "root", entry.FileInfo.Group)
	// 0o765 &^ 0o022 == 0o745
	assert.Equal(t, os.FileMode(0o745), entry.FileInfo.Mode)
	assert.True(t, entry.FileInfo.MTime.Equal(mtime))
	assert.EqualValues(t, len("data"), entry.FileInfo.Size)
}

func TestContentPrepareForPackager_DeclaredFileInfoWins(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "bin/tool", 0o765)

	declaredMTime := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
	cs := nfpm.Contents{
		{
			Source: "bin/tool", Destination: "/usr/bin/tool", Type: nfpm.TypeFile,
			FileInfo: &nfpm.ContentFileInfo{
				Owner: "alice", Group: "staff", Mode: 0o700, MTime: declaredMTime,
			},
		},
	}

	out, err := cs.PrepareForPackager(dir, nfpm.PackagerDeb, 0o022, false, time.Now())
	require.NoError(t, err)
	require.Len(t, out, 1)

	assert.Equal(t, "alice", out[0].FileInfo.Owner)
	assert.Equal(t, "staff", out[0].FileInfo.Group)
	assert.Equal(t, os.FileMode(0o700), out[0].FileInfo.Mode)
	assert.True(t, out[0].FileInfo.MTime.Equal(declaredMTime))
}

func TestContentPrepareForPackager_DirAndImplicitDirDefaults(t *testing.T) {
	dir := t.TempDir()
	mtime := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)

	cs := nfpm.Contents{
		{Destination: "/var/lib/example", Type: nfpm.TypeDir},
		{Destination: "/var/lib/example/sub", Type: nfpm.TypeImplicitDir},
	}

	out, err := cs.PrepareForPackager(dir, nfpm.PackagerDeb, 0o022, false, mtime)
	require.NoError(t, err)
	require.Len(t, out, 2)

	for _, entry := range out {
		assert.Equal(t, os.FileMode(0o755), entry.FileInfo.Mode,
			"standalone dir/implicit-dir entries have no disk backing: "+
				"hardcoded 0o755, umask does not apply")
		assert.Equal(t, "root", entry.FileInfo.Owner)
		assert.True(t, entry.FileInfo.MTime.Equal(mtime))
	}
}

func TestContentPrepareForPackager_SymlinkNoModeDefault(t *testing.T) {
	dir := t.TempDir()

	cs := nfpm.Contents{
		{Source: "/opt/real", Destination: "/usr/bin/link", Type: nfpm.TypeSymlink},
	}

	out, err := cs.PrepareForPackager(dir, nfpm.PackagerDeb, 0o022, false, time.Now())
	require.NoError(t, err)
	require.Len(t, out, 1)

	assert.Equal(t, "/opt/real", out[0].Source, "symlink Source is the link target, left untouched")
	assert.Equal(t, os.FileMode(0), out[0].FileInfo.Mode,
		"symlink mode bits are meaningless; never defaulted")
	assert.Equal(t, "root", out[0].FileInfo.Owner)
}

func TestContentPrepareForPackager_GhostDefaults(t *testing.T) {
	dir := t.TempDir()

	cs := nfpm.Contents{
		{Destination: "/var/log/example.log", Type: nfpm.TypeRPMGhost},
	}

	out, err := cs.PrepareForPackager(dir, nfpm.PackagerRPM, 0o022, false, time.Now())
	require.NoError(t, err)
	require.Len(t, out, 1)
	assert.Equal(t, os.FileMode(0o644), out[0].FileInfo.Mode,
		"ghost entries are file-ish with no disk backing")
}

func TestContentPrepareForPackager_PackagerFiltering(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "bin/tool", 0o755)

	cs := nfpm.Contents{
		{
			Source: "bin/tool", Destination: "/usr/bin/tool-deb",
			Type: nfpm.TypeFile, Packager: nfpm.PackagerDeb,
		},
		{
			Source: "bin/tool", Destination: "/usr/bin/tool-rpm",
			Type: nfpm.TypeFile, Packager: nfpm.PackagerRPM,
		},
		{Source: "bin/tool", Destination: "/usr/bin/tool-any", Type: nfpm.TypeFile},
	}

	debOut, err := cs.PrepareForPackager(dir, nfpm.PackagerDeb, 0o022, false, time.Now())
	require.NoError(t, err)
	require.Len(t, debOut, 2)
	assert.Equal(t, "/usr/bin/tool-any", debOut[0].Destination)
	assert.Equal(t, "/usr/bin/tool-deb", debOut[1].Destination)

	rpmOut, err := cs.PrepareForPackager(dir, nfpm.PackagerRPM, 0o022, false, time.Now())
	require.NoError(t, err)
	require.Len(t, rpmOut, 2)
	assert.Equal(t, "/usr/bin/tool-any", rpmOut[0].Destination)
	assert.Equal(t, "/usr/bin/tool-rpm", rpmOut[1].Destination)
}

func TestContentPrepareForPackager_GlobExpandsMultipleMatches(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "bin/a", 0o755)
	writeFile(t, dir, "bin/b", 0o755)

	cs := nfpm.Contents{
		{Source: "bin/*", Destination: "/usr/bin", Type: nfpm.TypeFile},
	}

	out, err := cs.PrepareForPackager(dir, nfpm.PackagerDeb, 0o022, false, time.Now())
	require.NoError(t, err)
	require.Len(t, out, 2)
	assert.Equal(t, "/usr/bin/a", out[0].Destination)
	assert.Equal(t, "/usr/bin/b", out[1].Destination)
}

func TestContentPrepareForPackager_GlobSingleMatchUsesDestinationVerbatim(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "bin/only", 0o755)

	cs := nfpm.Contents{
		{Source: "bin/on*", Destination: "/usr/bin/renamed", Type: nfpm.TypeFile},
	}

	out, err := cs.PrepareForPackager(dir, nfpm.PackagerDeb, 0o022, false, time.Now())
	require.NoError(t, err)
	require.Len(t, out, 1)
	assert.Equal(t, "/usr/bin/renamed", out[0].Destination)
}

func TestContentPrepareForPackager_GlobMatchesNothingIsAnError(t *testing.T) {
	dir := t.TempDir()

	cs := nfpm.Contents{
		{Source: "bin/does-not-exist-*", Destination: "/usr/bin", Type: nfpm.TypeFile},
	}

	_, err := cs.PrepareForPackager(dir, nfpm.PackagerDeb, 0o022, false, time.Now())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "bin/does-not-exist-*")
}

func TestContentPrepareForPackager_DisableGlobbingPassesThroughLiterally(t *testing.T) {
	dir := t.TempDir()
	// Deliberately do NOT create bin/missing on disk: disableGlobbing must
	// never touch the filesystem.

	cs := nfpm.Contents{
		{Source: "bin/missing", Destination: "/usr/bin/missing", Type: nfpm.TypeFile},
		{Source: "sometree", Destination: "/usr/share/example", Type: nfpm.TypeTree},
	}

	out, err := cs.PrepareForPackager(dir, nfpm.PackagerDeb, 0o022, true, time.Now())
	require.NoError(t, err)
	require.Len(t, out, 2)

	fileEntry := out[0]
	assert.Equal(t, "/usr/bin/missing", fileEntry.Destination)
	assert.Equal(t, "bin/missing", fileEntry.Source,
		"source left exactly as declared, no baseDir join")
	assert.Equal(t, os.FileMode(0o644), fileEntry.FileInfo.Mode)

	treeEntry := out[1]
	assert.Equal(t, nfpm.TypeTree, treeEntry.Type,
		"type preserved so callers can still dispatch on IsTree()")
	assert.Equal(t, "sometree", treeEntry.Source)
	assert.Equal(t, os.FileMode(0o755), treeEntry.FileInfo.Mode,
		"tree passthrough defaults to the dir literal")
}

func TestContentPrepareForPackager_TreeExpansion(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "tree/bin/tool", 0o755)
	writeFile(t, dir, "tree/share/doc/notes.txt", 0o644)
	require.NoError(t, os.Symlink("bin/tool", filepath.Join(dir, "tree", "link")))

	mtime := time.Date(2025, 2, 2, 0, 0, 0, 0, time.UTC)

	cs := nfpm.Contents{
		{Source: "tree", Destination: "/usr/share/example", Type: nfpm.TypeTree},
	}

	out, err := cs.PrepareForPackager(dir, nfpm.PackagerDeb, 0o022, false, mtime)
	require.NoError(t, err)

	byDest := make(map[string]*nfpm.Content, len(out))
	for _, e := range out {
		byDest[e.Destination] = e
	}

	require.Contains(t, byDest, "/usr/share/example")
	assert.Equal(t, nfpm.TypeImplicitDir, byDest["/usr/share/example"].Type,
		"tree root becomes an implicit dir")

	require.Contains(t, byDest, "/usr/share/example/bin")
	assert.Equal(t, nfpm.TypeImplicitDir, byDest["/usr/share/example/bin"].Type)

	require.Contains(t, byDest, "/usr/share/example/bin/tool")
	assert.Equal(t, nfpm.TypeFile, byDest["/usr/share/example/bin/tool"].Type)
	assert.Equal(t, os.FileMode(0o755), byDest["/usr/share/example/bin/tool"].FileInfo.Mode)

	require.Contains(t, byDest, "/usr/share/example/share/doc")
	assert.Equal(t, nfpm.TypeImplicitDir, byDest["/usr/share/example/share/doc"].Type)

	require.Contains(t, byDest, "/usr/share/example/share/doc/notes.txt")
	assert.Equal(t, nfpm.TypeFile, byDest["/usr/share/example/share/doc/notes.txt"].Type)

	require.Contains(t, byDest, "/usr/share/example/link")
	link := byDest["/usr/share/example/link"]
	assert.Equal(t, nfpm.TypeSymlink, link.Type)
	assert.Equal(t, "bin/tool", link.Source)

	// Deterministic, sorted-by-destination order.
	for i := 1; i < len(out); i++ {
		assert.Less(t, out[i-1].Destination, out[i].Destination)
	}
}

func TestContentPrepareForPackager_ConfigTreeLeafTypeSubstitution(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "conftree/app.conf", 0o644)

	cs := nfpm.Contents{
		{Source: "conftree", Destination: "/etc/example", Type: nfpm.TypeConfigNoReplaceTree},
	}

	out, err := cs.PrepareForPackager(dir, nfpm.PackagerDeb, 0o022, false, time.Now())
	require.NoError(t, err)

	for _, e := range out {
		if e.Destination == "/etc/example/app.conf" {
			assert.Equal(t, nfpm.TypeConfigNoReplace, e.Type,
				"leaf files inherit the tree's config flavour, not TypeFile")

			return
		}
	}

	t.Fatal("expected /etc/example/app.conf in output")
}

func TestContentPrepareForPackager_TreeSourceMissing(t *testing.T) {
	dir := t.TempDir()

	cs := nfpm.Contents{{Source: "does-not-exist", Destination: "/x", Type: nfpm.TypeTree}}

	_, err := cs.PrepareForPackager(dir, nfpm.PackagerDeb, 0o022, false, time.Now())
	require.Error(t, err)
}

func TestContentPrepareForPackager_TreeSourceNotADirectory(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "notadir", 0o644)

	cs := nfpm.Contents{{Source: "notadir", Destination: "/x", Type: nfpm.TypeTree}}

	_, err := cs.PrepareForPackager(dir, nfpm.PackagerDeb, 0o022, false, time.Now())
	require.Error(t, err)
}

func TestContentPrepareForPackager_DestinationEscapeRejected(t *testing.T) {
	dir := t.TempDir()

	cs := nfpm.Contents{{Destination: "../../etc/passwd", Type: nfpm.TypeDir}}

	_, err := cs.PrepareForPackager(dir, nfpm.PackagerDeb, 0o022, false, time.Now())
	require.Error(t, err)
}

func TestContentPrepareForPackager_DestinationNormalisedToLeadingSlash(t *testing.T) {
	dir := t.TempDir()

	cs := nfpm.Contents{{Destination: "usr/bin/tool", Type: nfpm.TypeDir}}

	out, err := cs.PrepareForPackager(dir, nfpm.PackagerDeb, 0o022, false, time.Now())
	require.NoError(t, err)
	require.Len(t, out, 1)
	assert.Equal(t, "/usr/bin/tool", out[0].Destination)
}

func TestContentPrepareForPackager_SortedByDestination(t *testing.T) {
	dir := t.TempDir()

	cs := nfpm.Contents{
		{Destination: "/z", Type: nfpm.TypeDir},
		{Destination: "/a", Type: nfpm.TypeDir},
		{Destination: "/m", Type: nfpm.TypeDir},
	}

	out, err := cs.PrepareForPackager(dir, nfpm.PackagerDeb, 0o022, false, time.Now())
	require.NoError(t, err)
	require.Len(t, out, 3)
	got := []string{out[0].Destination, out[1].Destination, out[2].Destination}
	assert.Equal(t, []string{"/a", "/m", "/z"}, got)
}

func TestContentPrepareForPackager_DoesNotMutateOriginalEntries(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "bin/tool", 0o765)

	original := &nfpm.Content{Source: "bin/tool", Destination: "usr/bin/tool", Type: nfpm.TypeFile}
	cs := nfpm.Contents{original}

	out, err := cs.PrepareForPackager(dir, nfpm.PackagerDeb, 0o022, false, time.Now())
	require.NoError(t, err)
	require.Len(t, out, 1)

	assert.Equal(t, "/usr/bin/tool", out[0].Destination)
	assert.Equal(t, "usr/bin/tool", original.Destination,
		"PrepareForPackager must not mutate its input")
	assert.Nil(t, original.FileInfo, "PrepareForPackager must not mutate its input")
}
