package aptinstall_test

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"os"
	"path/filepath"
	"testing"

	"github.com/m0rf30/ar"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/M0Rf30/yap/v2/pkg/aptinstall"
)

// ── helpers ──────────────────────────────────────────────────────────────────

// buildDataTarGz returns the bytes of a data.tar.gz containing the supplied
// entries. Each entry is a (name, content, typeflag, linkname, mode) tuple.
type tarEntry struct {
	name     string
	content  string
	typeflag byte
	linkname string
	mode     int64
}

func buildDataTarGzEntries(t *testing.T, entries []tarEntry) []byte {
	t.Helper()

	buf := new(bytes.Buffer)
	gw := gzip.NewWriter(buf)
	tw := tar.NewWriter(gw)

	for _, e := range entries {
		mode := e.mode
		if mode == 0 {
			mode = 0o644
		}

		typeflag := e.typeflag
		if typeflag == 0 {
			typeflag = tar.TypeReg
		}

		hdr := &tar.Header{
			Name:     e.name,
			Mode:     mode,
			Size:     int64(len(e.content)),
			Typeflag: typeflag,
			Linkname: e.linkname,
		}

		require.NoError(t, tw.WriteHeader(hdr))

		if e.content != "" {
			_, err := tw.Write([]byte(e.content))
			require.NoError(t, err)
		}
	}

	require.NoError(t, tw.Close())
	require.NoError(t, gw.Close())

	return buf.Bytes()
}

// buildDEB assembles a minimal .deb AR archive with the given data.tar.gz bytes.
// The control.tar.gz contains a single minimal control file.
func buildDEB(t *testing.T, dataTarGzBytes []byte) string {
	t.Helper()

	tmpDir := t.TempDir()

	// Minimal control.tar.gz.
	controlBuf := new(bytes.Buffer)
	cgz := gzip.NewWriter(controlBuf)
	ctw := tar.NewWriter(cgz)
	ctrl := "Package: testpkg\nVersion: 1.0\nArchitecture: amd64\nMaintainer: Test\nDescription: test\n"
	require.NoError(t, ctw.WriteHeader(&tar.Header{
		Name: "./control",
		Mode: 0o644,
		Size: int64(len(ctrl)),
	}))
	_, err := ctw.Write([]byte(ctrl))
	require.NoError(t, err)
	require.NoError(t, ctw.Close())
	require.NoError(t, cgz.Close())

	debPath := filepath.Join(tmpDir, "test.deb")
	debFile, err := os.Create(debPath)
	require.NoError(t, err)

	defer func() { _ = debFile.Close() }()

	arw := ar.NewWriter(debFile)
	require.NoError(t, arw.WriteGlobalHeader())

	debBin := "2.0\n"
	require.NoError(t, arw.WriteHeader(&ar.Header{Name: "debian-binary", Size: int64(len(debBin)), Mode: 0o644}))
	_, err = arw.Write([]byte(debBin))
	require.NoError(t, err)

	require.NoError(t, arw.WriteHeader(&ar.Header{Name: "control.tar.gz", Size: int64(controlBuf.Len()), Mode: 0o644}))
	_, err = arw.Write(controlBuf.Bytes())
	require.NoError(t, err)

	require.NoError(t, arw.WriteHeader(&ar.Header{Name: "data.tar.gz", Size: int64(len(dataTarGzBytes)), Mode: 0o644}))
	_, err = arw.Write(dataTarGzBytes)
	require.NoError(t, err)

	return debPath
}

// buildDEBNoData assembles a .deb with no data.tar member at all.
func buildDEBNoData(t *testing.T) string {
	t.Helper()

	tmpDir := t.TempDir()

	controlBuf := new(bytes.Buffer)
	cgz := gzip.NewWriter(controlBuf)
	ctw := tar.NewWriter(cgz)
	ctrl := "Package: testpkg\nVersion: 1.0\nArchitecture: amd64\nMaintainer: Test\nDescription: test\n"
	require.NoError(t, ctw.WriteHeader(&tar.Header{Name: "./control", Mode: 0o644, Size: int64(len(ctrl))}))
	_, err := ctw.Write([]byte(ctrl))
	require.NoError(t, err)
	require.NoError(t, ctw.Close())
	require.NoError(t, cgz.Close())

	debPath := filepath.Join(tmpDir, "nodata.deb")
	debFile, err := os.Create(debPath)
	require.NoError(t, err)

	defer func() { _ = debFile.Close() }()

	arw := ar.NewWriter(debFile)
	require.NoError(t, arw.WriteGlobalHeader())

	debBin := "2.0\n"
	require.NoError(t, arw.WriteHeader(&ar.Header{Name: "debian-binary", Size: int64(len(debBin)), Mode: 0o644}))
	_, err = arw.Write([]byte(debBin))
	require.NoError(t, err)

	require.NoError(t, arw.WriteHeader(&ar.Header{Name: "control.tar.gz", Size: int64(controlBuf.Len()), Mode: 0o644}))
	_, err = arw.Write(controlBuf.Bytes())
	require.NoError(t, err)

	// Deliberately omit data.tar.gz.
	return debPath
}

// ── TestExtractDataTar ────────────────────────────────────────────────────────

// TestExtractDataTar_ValidDEB verifies that a well-formed .deb extracts its
// data files into destDir.
func TestExtractDataTar_ValidDEB(t *testing.T) {
	t.Parallel()

	dataTarGz := buildDataTarGzEntries(t, []tarEntry{
		{name: "./usr/bin/hello", content: "#!/bin/sh\necho hello\n", mode: 0o755},
		{name: "./etc/hello.conf", content: "key=value\n", mode: 0o644},
	})
	debPath := buildDEB(t, dataTarGz)
	destDir := t.TempDir()

	err := aptinstall.ExtractDataTarForTesting(debPath, destDir, nil)
	require.NoError(t, err)

	// Both files must exist under destDir.
	got, err := os.ReadFile(filepath.Join(destDir, "usr", "bin", "hello"))
	require.NoError(t, err)
	assert.Equal(t, "#!/bin/sh\necho hello\n", string(got))

	got, err = os.ReadFile(filepath.Join(destDir, "etc", "hello.conf"))
	require.NoError(t, err)
	assert.Equal(t, "key=value\n", string(got))
}

// TestExtractDataTar_NonExistentDEB verifies that a missing .deb path returns
// an error containing "open DEB".
func TestExtractDataTar_NonExistentDEB(t *testing.T) {
	t.Parallel()

	err := aptinstall.ExtractDataTarForTesting("/nonexistent/path/pkg.deb", t.TempDir(), nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "open DEB")
}

// TestExtractDataTar_InvalidARArchive verifies that a file that is not an AR
// archive returns an error containing "parse AR archive".
func TestExtractDataTar_InvalidARArchive(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	badPath := filepath.Join(tmpDir, "bad.deb")
	require.NoError(t, os.WriteFile(badPath, []byte("not an AR archive at all"), 0o644))

	err := aptinstall.ExtractDataTarForTesting(badPath, t.TempDir(), nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "parse AR archive")
}

// TestExtractDataTar_NoDataTar verifies that a .deb without a data.tar member
// returns an error containing "data.tar not found".
func TestExtractDataTar_NoDataTar(t *testing.T) {
	t.Parallel()

	debPath := buildDEBNoData(t)

	err := aptinstall.ExtractDataTarForTesting(debPath, t.TempDir(), nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "data.tar not found")
}

// TestExtractDataTar_ConffileNotOverwritten verifies that a conffile already
// present on disk is NOT overwritten (dpkg non-interactive upgrade semantics).
func TestExtractDataTar_ConffileNotOverwritten(t *testing.T) {
	t.Parallel()

	const (
		localEdit = "USER_EDITED=keep-me\n"
		upstream  = "UPSTREAM=overwrite-me\n"
	)

	dataTarGz := buildDataTarGzEntries(t, []tarEntry{
		{name: "./etc/myapp.conf", content: upstream, mode: 0o644},
	})
	debPath := buildDEB(t, dataTarGz)
	destDir := t.TempDir()

	// Pre-create the conffile with a local edit.
	confPath := filepath.Join(destDir, "etc", "myapp.conf")
	require.NoError(t, os.MkdirAll(filepath.Dir(confPath), 0o755))
	require.NoError(t, os.WriteFile(confPath, []byte(localEdit), 0o644))

	err := aptinstall.ExtractDataTarForTesting(debPath, destDir, []string{"/etc/myapp.conf"})
	require.NoError(t, err)

	got, err := os.ReadFile(confPath)
	require.NoError(t, err)
	assert.Equal(t, localEdit, string(got), "conffile must not be overwritten on upgrade")
}

// ── TestExtractTarDir ─────────────────────────────────────────────────────────

// TestExtractTarDir_CreatesDirectory verifies that extractTarDir creates the
// target directory with the mode from the tar header.
func TestExtractTarDir_CreatesDirectory(t *testing.T) {
	t.Parallel()

	tmp := t.TempDir()
	fullPath := filepath.Join(tmp, "newdir")

	hdr := &tar.Header{
		Name:     "./newdir/",
		Mode:     0o750,
		Typeflag: tar.TypeDir,
	}

	err := aptinstall.ExtractTarDirForTesting(hdr, fullPath)
	require.NoError(t, err)

	info, err := os.Stat(fullPath)
	require.NoError(t, err)
	assert.True(t, info.IsDir(), "expected a directory")
}

// TestExtractTarDir_IdempotentOnExistingDir verifies that calling extractTarDir
// on an already-existing directory does not return an error (MkdirAll is
// idempotent).
func TestExtractTarDir_IdempotentOnExistingDir(t *testing.T) {
	t.Parallel()

	tmp := t.TempDir()
	existing := filepath.Join(tmp, "already")
	require.NoError(t, os.Mkdir(existing, 0o755))

	hdr := &tar.Header{
		Name:     "./already/",
		Mode:     0o755,
		Typeflag: tar.TypeDir,
	}

	err := aptinstall.ExtractTarDirForTesting(hdr, existing)
	require.NoError(t, err)
}

// TestExtractTarDir_NestedPath verifies that extractTarDir creates all
// intermediate directories (MkdirAll semantics).
func TestExtractTarDir_NestedPath(t *testing.T) {
	t.Parallel()

	tmp := t.TempDir()
	nested := filepath.Join(tmp, "a", "b", "c")

	hdr := &tar.Header{
		Name:     "./a/b/c/",
		Mode:     0o755,
		Typeflag: tar.TypeDir,
	}

	err := aptinstall.ExtractTarDirForTesting(hdr, nested)
	require.NoError(t, err)

	info, err := os.Stat(nested)
	require.NoError(t, err)
	assert.True(t, info.IsDir())
}

// ── TestExtractTarSymlink ─────────────────────────────────────────────────────

// TestExtractTarSymlink_RelativeTarget verifies that a symlink with a relative
// target that stays within destDir is created successfully.
func TestExtractTarSymlink_RelativeTarget(t *testing.T) {
	t.Parallel()

	destDir := t.TempDir()
	// Create the target file so the symlink has something to point at.
	targetFile := filepath.Join(destDir, "usr", "lib", "libfoo.so.1")
	require.NoError(t, os.MkdirAll(filepath.Dir(targetFile), 0o755))
	require.NoError(t, os.WriteFile(targetFile, []byte("lib"), 0o644))

	linkPath := filepath.Join(destDir, "usr", "lib", "libfoo.so")

	hdr := &tar.Header{
		Name:     "./usr/lib/libfoo.so",
		Typeflag: tar.TypeSymlink,
		Linkname: "libfoo.so.1", // relative, stays inside destDir
	}

	err := aptinstall.ExtractTarSymlinkForTesting(hdr, destDir, linkPath)
	require.NoError(t, err)

	// Verify the symlink exists and points to the right target.
	target, err := os.Readlink(linkPath)
	require.NoError(t, err)
	assert.Equal(t, "libfoo.so.1", target)
}

// TestExtractTarSymlink_AbsoluteTarget verifies that a symlink with an absolute
// target is permitted (absolute targets are safe at install time because the
// symlink itself lives under destDir; the target is only resolved at runtime
// when the package is installed at /).
func TestExtractTarSymlink_AbsoluteTarget(t *testing.T) {
	t.Parallel()

	destDir := t.TempDir()
	linkPath := filepath.Join(destDir, "usr", "bin", "python")
	require.NoError(t, os.MkdirAll(filepath.Dir(linkPath), 0o755))

	hdr := &tar.Header{
		Name:     "./usr/bin/python",
		Typeflag: tar.TypeSymlink,
		Linkname: "/usr/bin/python3", // absolute — permitted
	}

	err := aptinstall.ExtractTarSymlinkForTesting(hdr, destDir, linkPath)
	require.NoError(t, err)

	target, err := os.Readlink(linkPath)
	require.NoError(t, err)
	assert.Equal(t, "/usr/bin/python3", target)
}

// TestExtractTarSymlink_PathTraversalSkipped verifies that a symlink whose
// relative target resolves outside destDir is silently skipped (not an error —
// the function logs a warning and returns nil).
func TestExtractTarSymlink_PathTraversalSkipped(t *testing.T) {
	t.Parallel()

	destDir := t.TempDir()
	linkPath := filepath.Join(destDir, "usr", "bin", "evil")
	require.NoError(t, os.MkdirAll(filepath.Dir(linkPath), 0o755))

	hdr := &tar.Header{
		Name:     "./usr/bin/evil",
		Typeflag: tar.TypeSymlink,
		Linkname: "../../../../etc/passwd", // escapes destDir
	}

	// extractTarSymlink logs a warning and returns nil for unsafe targets.
	err := aptinstall.ExtractTarSymlinkForTesting(hdr, destDir, linkPath)
	require.NoError(t, err)

	// The symlink must NOT have been created.
	_, statErr := os.Lstat(linkPath)
	assert.True(t, os.IsNotExist(statErr), "unsafe symlink must not be created")
}

// TestExtractTarSymlink_ReplacesExistingSymlink verifies that an existing
// symlink at the same path is replaced by the new one.
func TestExtractTarSymlink_ReplacesExistingSymlink(t *testing.T) {
	t.Parallel()

	destDir := t.TempDir()
	linkPath := filepath.Join(destDir, "lib", "libfoo.so")
	require.NoError(t, os.MkdirAll(filepath.Dir(linkPath), 0o755))

	// Create an old symlink first.
	require.NoError(t, os.Symlink("libfoo.so.0", linkPath))

	hdr := &tar.Header{
		Name:     "./lib/libfoo.so",
		Typeflag: tar.TypeSymlink,
		Linkname: "libfoo.so.1",
	}

	err := aptinstall.ExtractTarSymlinkForTesting(hdr, destDir, linkPath)
	require.NoError(t, err)

	target, err := os.Readlink(linkPath)
	require.NoError(t, err)
	assert.Equal(t, "libfoo.so.1", target)
}
