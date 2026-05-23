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

// controlEntry describes a single file to add to control.tar.gz.
type controlEntry struct {
	name    string
	content string
	mode    int64
}

// dataEntry describes a single file to add to data.tar.gz.
type dataEntry struct {
	name    string
	content string
}

// createMinimalDEB builds a minimal but structurally valid .deb AR archive.
//
// controlEntries are written into control.tar.gz; dataEntries into data.tar.gz.
// Returns the path to the created .deb file inside a test-managed temp dir.
func createMinimalDEB(t *testing.T, controlEntries []controlEntry, dataEntries []dataEntry) string {
	t.Helper()

	tmpDir := t.TempDir()

	// ── control.tar.gz ──────────────────────────────────────────────────────
	controlBuf := new(bytes.Buffer)
	controlGz := gzip.NewWriter(controlBuf)
	controlTar := tar.NewWriter(controlGz)

	for _, e := range controlEntries {
		mode := e.mode
		if mode == 0 {
			mode = 0o644
		}

		hdr := &tar.Header{
			Name: "./" + e.name,
			Mode: mode,
			Size: int64(len(e.content)),
		}

		require.NoError(t, controlTar.WriteHeader(hdr))
		_, err := controlTar.Write([]byte(e.content))
		require.NoError(t, err)
	}

	require.NoError(t, controlTar.Close())
	require.NoError(t, controlGz.Close())

	// ── data.tar.gz ─────────────────────────────────────────────────────────
	dataBuf := new(bytes.Buffer)
	dataGz := gzip.NewWriter(dataBuf)
	dataTar := tar.NewWriter(dataGz)

	for _, e := range dataEntries {
		hdr := &tar.Header{
			Name:     "./" + e.name,
			Mode:     0o755,
			Size:     int64(len(e.content)),
			Typeflag: tar.TypeReg,
		}

		require.NoError(t, dataTar.WriteHeader(hdr))
		_, err := dataTar.Write([]byte(e.content))
		require.NoError(t, err)
	}

	require.NoError(t, dataTar.Close())
	require.NoError(t, dataGz.Close())

	// ── AR archive (.deb) ───────────────────────────────────────────────────
	debPath := filepath.Join(tmpDir, "test.deb")
	debFile, err := os.Create(debPath)
	require.NoError(t, err)

	defer func() { _ = debFile.Close() }()

	arWriter := ar.NewWriter(debFile)
	require.NoError(t, arWriter.WriteGlobalHeader())

	// debian-binary member.
	debianBinary := "2.0\n"
	require.NoError(t, arWriter.WriteHeader(&ar.Header{
		Name: "debian-binary",
		Size: int64(len(debianBinary)),
		Mode: 0o644,
	}))
	_, err = arWriter.Write([]byte(debianBinary))
	require.NoError(t, err)

	// control.tar.gz member.
	require.NoError(t, arWriter.WriteHeader(&ar.Header{
		Name: "control.tar.gz",
		Size: int64(controlBuf.Len()),
		Mode: 0o644,
	}))
	_, err = arWriter.Write(controlBuf.Bytes())
	require.NoError(t, err)

	// data.tar.gz member.
	require.NoError(t, arWriter.WriteHeader(&ar.Header{
		Name: "data.tar.gz",
		Size: int64(dataBuf.Len()),
		Mode: 0o644,
	}))
	_, err = arWriter.Write(dataBuf.Bytes())
	require.NoError(t, err)

	return debPath
}

// minimalControl returns a well-formed control file string.
func minimalControl(pkg, version string) string {
	return "Package: " + pkg + "\n" +
		"Version: " + version + "\n" +
		"Architecture: amd64\n" +
		"Maintainer: Test <test@example.com>\n" +
		"Description: Test package\n"
}

// TestParseDEB_ValidMinimal verifies that a minimal .deb with only a control
// file is parsed successfully and the Control field is populated.
func TestParseDEB_ValidMinimal(t *testing.T) {
	ctrl := minimalControl("mypkg", "1.0-1")
	debPath := createMinimalDEB(t,
		[]controlEntry{{name: "control", content: ctrl}},
		nil,
	)

	contents, err := aptinstall.ParseDEBForTesting(debPath)
	require.NoError(t, err)
	require.NotNil(t, contents)

	assert.Equal(t, ctrl, contents.Control)
	assert.Empty(t, contents.Scriptlets)
	assert.Empty(t, contents.Md5sums)
	assert.Empty(t, contents.Conffiles)
	assert.Empty(t, contents.Files)
}

// TestParseDEB_WithScriptlets verifies that preinst and postinst scriptlets
// are extracted into the Scriptlets map.
func TestParseDEB_WithScriptlets(t *testing.T) {
	preinst := "#!/bin/sh\necho pre-install\n"
	postinst := "#!/bin/sh\necho post-install\n"

	debPath := createMinimalDEB(t,
		[]controlEntry{
			{name: "control", content: minimalControl("mypkg", "1.0-1")},
			{name: "preinst", content: preinst, mode: 0o755},
			{name: "postinst", content: postinst, mode: 0o755},
		},
		nil,
	)

	contents, err := aptinstall.ParseDEBForTesting(debPath)
	require.NoError(t, err)
	require.NotNil(t, contents)

	assert.Equal(t, preinst, contents.Scriptlets["preinst"])
	assert.Equal(t, postinst, contents.Scriptlets["postinst"])
	assert.NotContains(t, contents.Scriptlets, "prerm")
	assert.NotContains(t, contents.Scriptlets, "postrm")
}

// TestParseDEB_WithAllScriptlets verifies all four scriptlet types.
func TestParseDEB_WithAllScriptlets(t *testing.T) {
	scripts := map[string]string{
		"preinst":  "#!/bin/sh\necho preinst\n",
		"postinst": "#!/bin/sh\necho postinst\n",
		"prerm":    "#!/bin/sh\necho prerm\n",
		"postrm":   "#!/bin/sh\necho postrm\n",
	}

	entries := []controlEntry{
		{name: "control", content: minimalControl("mypkg", "1.0-1")},
	}
	for name, body := range scripts {
		entries = append(entries, controlEntry{name: name, content: body, mode: 0o755})
	}

	debPath := createMinimalDEB(t, entries, nil)

	contents, err := aptinstall.ParseDEBForTesting(debPath)
	require.NoError(t, err)
	require.NotNil(t, contents)

	for name, body := range scripts {
		assert.Equal(t, body, contents.Scriptlets[name], "scriptlet %q mismatch", name)
	}
}

// TestParseDEB_WithMd5sums verifies that the md5sums file is captured.
func TestParseDEB_WithMd5sums(t *testing.T) {
	md5sums := "d41d8cd98f00b204e9800998ecf8427e  usr/bin/mypkg\n"

	debPath := createMinimalDEB(t,
		[]controlEntry{
			{name: "control", content: minimalControl("mypkg", "1.0-1")},
			{name: "md5sums", content: md5sums},
		},
		nil,
	)

	contents, err := aptinstall.ParseDEBForTesting(debPath)
	require.NoError(t, err)
	require.NotNil(t, contents)

	assert.Equal(t, md5sums, contents.Md5sums)
}

// TestParseDEB_WithConffiles verifies that the conffiles list is captured.
func TestParseDEB_WithConffiles(t *testing.T) {
	conffiles := "/etc/mypkg/mypkg.conf\n/etc/mypkg/extra.conf\n"

	debPath := createMinimalDEB(t,
		[]controlEntry{
			{name: "control", content: minimalControl("mypkg", "1.0-1")},
			{name: "conffiles", content: conffiles},
		},
		nil,
	)

	contents, err := aptinstall.ParseDEBForTesting(debPath)
	require.NoError(t, err)
	require.NotNil(t, contents)

	assert.Equal(t, conffiles, contents.Conffiles)
}

// TestParseDEB_WithDataFiles verifies that regular files in data.tar.gz are
// collected into the Files slice (directories are skipped).
func TestParseDEB_WithDataFiles(t *testing.T) {
	debPath := createMinimalDEB(t,
		[]controlEntry{
			{name: "control", content: minimalControl("mypkg", "1.0-1")},
		},
		[]dataEntry{
			{name: "usr/bin/mypkg", content: "binary"},
			{name: "usr/share/doc/mypkg/README", content: "readme"},
		},
	)

	contents, err := aptinstall.ParseDEBForTesting(debPath)
	require.NoError(t, err)
	require.NotNil(t, contents)

	assert.Contains(t, contents.Files, "usr/bin/mypkg")
	assert.Contains(t, contents.Files, "usr/share/doc/mypkg/README")
	assert.Len(t, contents.Files, 2)
}

// TestParseDEB_DataDirectoriesSkipped verifies that directory entries in
// data.tar.gz are not included in the Files slice.
func TestParseDEB_DataDirectoriesSkipped(t *testing.T) {
	ctrl := minimalControl("mypkg", "1.0-1")

	// Build a data.tar.gz that contains a directory entry followed by a file.
	dataBuf := new(bytes.Buffer)
	dataGz := gzip.NewWriter(dataBuf)
	dataTar := tar.NewWriter(dataGz)

	// Directory entry.
	require.NoError(t, dataTar.WriteHeader(&tar.Header{
		Name:     "./usr/bin/",
		Mode:     0o755,
		Typeflag: tar.TypeDir,
	}))
	// Regular file entry.
	content := "binary"
	require.NoError(t, dataTar.WriteHeader(&tar.Header{
		Name:     "./usr/bin/mypkg",
		Mode:     0o755,
		Size:     int64(len(content)),
		Typeflag: tar.TypeReg,
	}))
	_, err := dataTar.Write([]byte(content))
	require.NoError(t, err)

	require.NoError(t, dataTar.Close())
	require.NoError(t, dataGz.Close())

	// Assemble the .deb manually so we can inject the custom data.tar.gz.
	tmpDir := t.TempDir()
	debPath := filepath.Join(tmpDir, "test.deb")
	debFile, err := os.Create(debPath)
	require.NoError(t, err)

	defer func() { _ = debFile.Close() }()

	arWriter := ar.NewWriter(debFile)
	require.NoError(t, arWriter.WriteGlobalHeader())

	debianBinary := "2.0\n"
	require.NoError(t, arWriter.WriteHeader(&ar.Header{Name: "debian-binary", Size: int64(len(debianBinary)), Mode: 0o644}))
	_, err = arWriter.Write([]byte(debianBinary))
	require.NoError(t, err)

	// control.tar.gz with just the control file.
	controlBuf := new(bytes.Buffer)
	controlGz := gzip.NewWriter(controlBuf)
	controlTar := tar.NewWriter(controlGz)
	require.NoError(t, controlTar.WriteHeader(&tar.Header{Name: "./control", Mode: 0o644, Size: int64(len(ctrl))}))
	_, err = controlTar.Write([]byte(ctrl))
	require.NoError(t, err)
	require.NoError(t, controlTar.Close())
	require.NoError(t, controlGz.Close())

	require.NoError(t, arWriter.WriteHeader(&ar.Header{Name: "control.tar.gz", Size: int64(controlBuf.Len()), Mode: 0o644}))
	_, err = arWriter.Write(controlBuf.Bytes())
	require.NoError(t, err)

	require.NoError(t, arWriter.WriteHeader(&ar.Header{Name: "data.tar.gz", Size: int64(dataBuf.Len()), Mode: 0o644}))
	_, err = arWriter.Write(dataBuf.Bytes())
	require.NoError(t, err)

	contents, err := aptinstall.ParseDEBForTesting(debPath)
	require.NoError(t, err)
	require.NotNil(t, contents)

	// Only the regular file should appear; the directory must be absent.
	assert.Equal(t, []string{"usr/bin/mypkg"}, contents.Files)
}

// TestParseDEB_NonExistentFile verifies that opening a missing path returns an error.
func TestParseDEB_NonExistentFile(t *testing.T) {
	_, err := aptinstall.ParseDEBForTesting("/nonexistent/path/to/package.deb")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "open DEB")
}

// TestParseDEB_InvalidARArchive verifies that a file that is not an AR archive
// returns a parse error.
func TestParseDEB_InvalidARArchive(t *testing.T) {
	tmpDir := t.TempDir()
	badPath := filepath.Join(tmpDir, "bad.deb")

	require.NoError(t, os.WriteFile(badPath, []byte("this is not an AR archive"), 0o644))

	_, err := aptinstall.ParseDEBForTesting(badPath)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "parse AR archive")
}

// TestParseDEB_MissingControlFile verifies that a .deb whose control.tar.gz
// contains no "control" entry is rejected with a descriptive error.
func TestParseDEB_MissingControlFile(t *testing.T) {
	// Build a .deb with a control.tar.gz that has no "control" entry.
	debPath := createMinimalDEB(t,
		[]controlEntry{
			// Only a scriptlet — no "control" file.
			{name: "postinst", content: "#!/bin/sh\necho hi\n", mode: 0o755},
		},
		nil,
	)

	_, err := aptinstall.ParseDEBForTesting(debPath)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "control file not found")
}

// TestParseDEB_FullPackage exercises all fields together: control, scriptlets,
// md5sums, conffiles, and data files in a single .deb.
func TestParseDEB_FullPackage(t *testing.T) {
	ctrl := minimalControl("fullpkg", "2.3-4")
	preinst := "#!/bin/sh\necho pre\n"
	postinst := "#!/bin/sh\necho post\n"
	md5sums := "abc123  usr/bin/fullpkg\n"
	conffiles := "/etc/fullpkg/fullpkg.conf\n"

	debPath := createMinimalDEB(t,
		[]controlEntry{
			{name: "control", content: ctrl},
			{name: "preinst", content: preinst, mode: 0o755},
			{name: "postinst", content: postinst, mode: 0o755},
			{name: "md5sums", content: md5sums},
			{name: "conffiles", content: conffiles},
		},
		[]dataEntry{
			{name: "usr/bin/fullpkg", content: "binary"},
			{name: "etc/fullpkg/fullpkg.conf", content: "config"},
		},
	)

	contents, err := aptinstall.ParseDEBForTesting(debPath)
	require.NoError(t, err)
	require.NotNil(t, contents)

	assert.Equal(t, ctrl, contents.Control)
	assert.Equal(t, preinst, contents.Scriptlets["preinst"])
	assert.Equal(t, postinst, contents.Scriptlets["postinst"])
	assert.Equal(t, md5sums, contents.Md5sums)
	assert.Equal(t, conffiles, contents.Conffiles)
	assert.Contains(t, contents.Files, "usr/bin/fullpkg")
	assert.Contains(t, contents.Files, "etc/fullpkg/fullpkg.conf")
}
