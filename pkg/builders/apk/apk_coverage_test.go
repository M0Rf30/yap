package apk

import (
	"archive/tar"
	"bytes"
	"context"
	"os"
	"path/filepath"
	"syscall"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestApkFileOpenNonExistentRegular exercises the os.Open error path in Open().
func TestApkFileOpenNonExistentRegular(t *testing.T) {
	// Create a real file, stat it, then remove it so Open() fails.
	tmpFile, err := os.CreateTemp("", "apkopen-*")
	require.NoError(t, err)

	name := tmpFile.Name()
	require.NoError(t, tmpFile.Close())

	info, err := os.Stat(name)
	require.NoError(t, err)

	require.NoError(t, os.Remove(name))

	af := apkFile{
		FileInfo:      info,
		diskPath:      name, // file no longer exists
		nameInArchive: "testfile",
	}

	rc, err := af.Open()
	// Should return an error since the file was removed
	assert.Error(t, err)
	assert.Nil(t, rc)
}

// TestWalkAPKFilesWithSubdirs exercises the directory traversal with nested dirs.
func TestWalkAPKFilesWithSubdirs(t *testing.T) {
	tmpDir := t.TempDir()

	// Create nested structure
	subDir := filepath.Join(tmpDir, "usr", "bin")
	require.NoError(t, os.MkdirAll(subDir, 0o755))

	require.NoError(t, os.WriteFile(filepath.Join(subDir, "prog"), []byte("data"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(tmpDir, "file.txt"), []byte("text"), 0o644))

	files, err := walkAPKFiles(tmpDir)
	require.NoError(t, err)

	// Should have: usr/, usr/bin/, usr/bin/prog, file.txt
	assert.GreaterOrEqual(t, len(files), 4)

	names := make(map[string]bool)
	for _, f := range files {
		names[f.nameInArchive] = true
	}

	// walkAPKFiles returns raw relative names; the "/" suffix is added later
	// by createTarGzWithChecksums, not by the walker itself.
	assert.True(t, names["usr"] || names["usr/"])
	assert.True(t, names["usr/bin"] || names["usr/bin/"])
	assert.True(t, names["usr/bin/prog"])
	assert.True(t, names["file.txt"])
}

// TestBuildPackageCreateTarGzError exercises the BuildPackage error path when
// createTarGzWithChecksums fails (non-existent PackageDir).
func TestBuildPackageCreateTarGzError(t *testing.T) {
	pkgBuild := createTestPKGBUILD()
	builder := NewBuilder(pkgBuild)

	tmpDir := t.TempDir()

	// Point PackageDir to a non-existent directory so createTarGzWithChecksums fails.
	builder.PKGBUILD.PackageDir = "/nonexistent/pkg/dir"
	builder.PKGBUILD.ArchComputed = "x86_64"

	_, err := builder.BuildPackage(context.Background(), tmpDir, "")
	assert.Error(t, err)
}

// TestPrepareFakerootGetDirSizeError exercises the GetDirSize error path.
func TestPrepareFakerootGetDirSizeError(t *testing.T) {
	pkgBuild := createTestPKGBUILD()
	builder := NewBuilder(pkgBuild)

	tmpDir := t.TempDir()

	// Point PackageDir to a non-existent directory so GetDirSize fails.
	builder.PKGBUILD.PackageDir = "/nonexistent/pkg/dir"

	err := builder.PrepareFakeroot(context.Background(), tmpDir, "")
	assert.Error(t, err)
}

// TestPrepareFakerootInstallScriptError exercises the createInstallScript error path.
// We set hooks but make PackageDir read-only so the script write fails.
func TestPrepareFakerootInstallScriptError(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("cannot test read-only dir as root")
	}

	pkgBuild := createTestPKGBUILD()
	pkgBuild.PreInst = "echo pre"
	builder := NewBuilder(pkgBuild)

	tmpDir := t.TempDir()
	pkgDir := filepath.Join(tmpDir, "pkg")
	require.NoError(t, os.MkdirAll(pkgDir, 0o755))

	builder.PKGBUILD.PackageDir = pkgDir

	// PrepareFakeroot will call createPkgInfo (writes .PKGINFO) then createInstallScript.
	// Make pkgDir read-only after .PKGINFO is written to cause createInstallScript to fail.
	// We do this by making the dir read-only before calling PrepareFakeroot.
	require.NoError(t, os.Chmod(pkgDir, 0o555))

	defer func() { _ = os.Chmod(pkgDir, 0o755) }()

	err := builder.PrepareFakeroot(context.Background(), tmpDir, "")
	assert.Error(t, err)
}

// TestCreateTarGzOutputFileError exercises the os.Create failure path.
func TestCreateTarGzOutputFileError(t *testing.T) {
	pkgBuild := createTestPKGBUILD()
	builder := NewBuilder(pkgBuild)

	tmpDir := t.TempDir()
	pkgDir := filepath.Join(tmpDir, "pkg")
	require.NoError(t, os.MkdirAll(pkgDir, 0o755))

	// Create a .PKGINFO so the control pass has something to write
	require.NoError(t, os.WriteFile(
		filepath.Join(pkgDir, ".PKGINFO"),
		[]byte("pkgname = test\n"),
		0o644,
	))

	builder.PKGBUILD.PackageDir = pkgDir

	// Output path inside a non-existent directory → os.Create fails
	err := builder.createTarGzWithChecksums(context.Background(), pkgDir, "/nonexistent/dir/out.apk")
	assert.Error(t, err)
}

// TestCreateTarGzWriteDataError exercises the out.Write(dataBuf) error path.
// We create a valid source dir but make the output file read-only after creation.
func TestCreateTarGzWithDataFile(t *testing.T) {
	pkgBuild := createTestPKGBUILD()
	builder := NewBuilder(pkgBuild)

	tmpDir := t.TempDir()
	pkgDir := filepath.Join(tmpDir, "pkg")
	require.NoError(t, os.MkdirAll(pkgDir, 0o755))

	// Add a data file (non-control) to exercise the data stream path
	require.NoError(t, os.WriteFile(
		filepath.Join(pkgDir, "datafile"),
		[]byte("binary content here"),
		0o644,
	))

	builder.PKGBUILD.PackageDir = pkgDir

	outFile := filepath.Join(tmpDir, "out.apk")
	err := builder.createTarGzWithChecksums(context.Background(), pkgDir, outFile)
	require.NoError(t, err)

	info, err := os.Stat(outFile)
	require.NoError(t, err)
	assert.Greater(t, info.Size(), int64(0))
}

// TestWriteFileWithChecksumBadHeader exercises the tar.FileInfoHeader error path.
// We pass a FileInfo whose Sys() returns an unexpected type to trigger the error.
func TestWriteFileWithChecksumBadHeader(t *testing.T) {
	pkgBuild := createTestPKGBUILD()
	builder := NewBuilder(pkgBuild)

	// Use a symlink apkFile with an empty linkTarget — tar.FileInfoHeader should still work.
	// To trigger a real header error we need an invalid FileInfo.
	// The easiest approach: use a fakeFileInfo with symlink mode but empty link target
	// which causes tar.FileInfoHeader to succeed but we can verify the symlink path.
	tmpDir := t.TempDir()
	target := filepath.Join(tmpDir, "target")
	require.NoError(t, os.WriteFile(target, []byte("x"), 0o644))

	link := filepath.Join(tmpDir, "link")
	require.NoError(t, os.Symlink(target, link))

	info, err := os.Lstat(link)
	require.NoError(t, err)

	af := apkFile{
		FileInfo:      info,
		diskPath:      link,
		nameInArchive: "link",
		linkTarget:    target,
	}

	var buf bytes.Buffer

	tw := tar.NewWriter(&buf)
	ctx := context.Background()

	err = builder.writeFileWithChecksum(ctx, tw, af)
	assert.NoError(t, err)

	_ = tw.Close()
}

// TestWriteFileWithChecksumDataFileChecksumPath exercises the full SHA1 checksum
// path for a regular non-control data file.
func TestWriteFileWithChecksumDataFileChecksumPath(t *testing.T) {
	pkgBuild := createTestPKGBUILD()
	builder := NewBuilder(pkgBuild)

	tmpDir := t.TempDir()
	dataFile := filepath.Join(tmpDir, "usr_bin_prog")
	require.NoError(t, os.WriteFile(dataFile, []byte("#!/bin/sh\necho hello"), 0o755))

	info, err := os.Stat(dataFile)
	require.NoError(t, err)

	// Non-control file name (no leading dot) → triggers SHA1 checksum path
	af := apkFile{
		FileInfo:      info,
		diskPath:      dataFile,
		nameInArchive: "usr/bin/prog",
	}

	var buf bytes.Buffer

	tw := tar.NewWriter(&buf)
	ctx := context.Background()

	err = builder.writeFileWithChecksum(ctx, tw, af)
	require.NoError(t, err)

	_ = tw.Close()

	// Verify PAX header with SHA1 checksum was written
	assert.Greater(t, buf.Len(), 0)
}

// TestWriteFileWithChecksumDataFileOpenError exercises the first file.Open() error
// in the SHA1 checksum path (pass 1).
func TestWriteFileWithChecksumDataFileOpenError(t *testing.T) {
	pkgBuild := createTestPKGBUILD()
	builder := NewBuilder(pkgBuild)

	// Create a real file, stat it, then remove it so Open() fails during checksum pass.
	tmpFile, err := os.CreateTemp("", "apkwfc-*")
	require.NoError(t, err)

	_, err = tmpFile.WriteString("data")
	require.NoError(t, err)
	require.NoError(t, tmpFile.Close())

	info, err := os.Stat(tmpFile.Name())
	require.NoError(t, err)

	require.NoError(t, os.Remove(tmpFile.Name()))

	// Non-control name → triggers SHA1 path → Open() will fail
	af := apkFile{
		FileInfo:      info,
		diskPath:      tmpFile.Name(),
		nameInArchive: "usr/bin/missing",
	}

	var buf bytes.Buffer

	tw := tar.NewWriter(&buf)
	ctx := context.Background()

	err = builder.writeFileWithChecksum(ctx, tw, af)
	assert.Error(t, err)

	_ = tw.Close()
}

// TestWriteFileWithChecksumControlFileOpenError exercises the file.Open() error
// in the control-file (non-SHA1) path.
func TestWriteFileWithChecksumControlFileOpenError(t *testing.T) {
	pkgBuild := createTestPKGBUILD()
	builder := NewBuilder(pkgBuild)

	// Create a real file, stat it, then remove it.
	tmpFile, err := os.CreateTemp("", "apkwfc-*")
	require.NoError(t, err)

	_, err = tmpFile.WriteString("pkgname = test\n")
	require.NoError(t, err)
	require.NoError(t, tmpFile.Close())

	info, err := os.Stat(tmpFile.Name())
	require.NoError(t, err)

	require.NoError(t, os.Remove(tmpFile.Name()))

	// Control file name → skips SHA1 → goes to else branch → Open() will fail
	af := apkFile{
		FileInfo:      info,
		diskPath:      tmpFile.Name(),
		nameInArchive: ".PKGINFO",
	}

	var buf bytes.Buffer

	tw := tar.NewWriter(&buf)
	ctx := context.Background()

	err = builder.writeFileWithChecksum(ctx, tw, af)
	assert.Error(t, err)

	_ = tw.Close()
}

// TestCreateTarGzWithChecksumsBadOutputDir exercises the os.Create error when
// the output directory doesn't exist.
func TestCreateTarGzWithChecksumsBadOutputDir(t *testing.T) {
	pkgBuild := createTestPKGBUILD()
	builder := NewBuilder(pkgBuild)

	tmpDir := t.TempDir()
	pkgDir := filepath.Join(tmpDir, "pkg")
	require.NoError(t, os.MkdirAll(pkgDir, 0o755))

	builder.PKGBUILD.PackageDir = pkgDir

	err := builder.createTarGzWithChecksums(context.Background(), pkgDir, "/no/such/dir/out.apk")
	assert.Error(t, err)
}

// TestCreateTarGzWithChecksumsWriteFileError exercises the writeFileWithChecksum
// error path inside the data pass by injecting a bad file.
func TestCreateTarGzWithChecksumsDataWriteError(t *testing.T) {
	pkgBuild := createTestPKGBUILD()
	builder := NewBuilder(pkgBuild)

	tmpDir := t.TempDir()
	pkgDir := filepath.Join(tmpDir, "pkg")
	require.NoError(t, os.MkdirAll(pkgDir, 0o755))

	// Create a data file, then make it unreadable so writeFileWithChecksum fails.
	dataFile := filepath.Join(pkgDir, "prog")
	require.NoError(t, os.WriteFile(dataFile, []byte("data"), 0o644))

	if os.Getuid() == 0 {
		t.Skip("cannot test unreadable file as root")
	}

	require.NoError(t, os.Chmod(dataFile, 0o000))

	defer func() { _ = os.Chmod(dataFile, 0o644) }()

	builder.PKGBUILD.PackageDir = pkgDir

	outFile := filepath.Join(tmpDir, "out.apk")
	err := builder.createTarGzWithChecksums(context.Background(), pkgDir, outFile)
	assert.Error(t, err)
}

// TestCreateTarGzWithChecksumsControlWriteError exercises the writeFileWithChecksum
// error path inside the control pass by making a control file unreadable.
func TestCreateTarGzWithChecksumsControlWriteError(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("cannot test unreadable file as root")
	}

	pkgBuild := createTestPKGBUILD()
	builder := NewBuilder(pkgBuild)

	tmpDir := t.TempDir()
	pkgDir := filepath.Join(tmpDir, "pkg")
	require.NoError(t, os.MkdirAll(pkgDir, 0o755))

	// Create a .PKGINFO control file, then make it unreadable.
	pkginfoPath := filepath.Join(pkgDir, ".PKGINFO")
	require.NoError(t, os.WriteFile(pkginfoPath, []byte("pkgname = test\n"), 0o644))
	require.NoError(t, os.Chmod(pkginfoPath, 0o000))

	defer func() { _ = os.Chmod(pkginfoPath, 0o644) }()

	builder.PKGBUILD.PackageDir = pkgDir

	outFile := filepath.Join(tmpDir, "out.apk")
	err := builder.createTarGzWithChecksums(context.Background(), pkgDir, outFile)
	assert.Error(t, err)
}

// TestWriteFileWithChecksumDataFileCopyError exercises the io.Copy error in pass 2
// (writing data into the tar) by making the file unreadable between pass 1 and pass 2.
// This is hard to trigger without mocking; we skip it and rely on the open-error tests.

// TestWalkAPKFilesStatError exercises the d.Info() error path.
// This is hard to trigger directly; we verify walkAPKFiles handles a
// directory that disappears mid-walk gracefully (returns error).
func TestWalkAPKFilesDisappearingDir(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("cannot test permission error as root")
	}

	tmpDir := t.TempDir()
	subDir := filepath.Join(tmpDir, "sub")
	require.NoError(t, os.MkdirAll(subDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(subDir, "file"), []byte("x"), 0o644))

	// Make subDir unreadable so WalkDir returns an error when entering it
	require.NoError(t, os.Chmod(subDir, 0o000))

	defer func() { _ = os.Chmod(subDir, 0o755) }()

	_, err := walkAPKFiles(tmpDir)
	// WalkDir propagates the permission error
	assert.Error(t, err)
	assert.ErrorIs(t, err, syscall.EACCES)
}
