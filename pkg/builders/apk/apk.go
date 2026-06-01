// Package apk provides functionality for building Alpine Linux APK packages.
package apk

import (
	"archive/tar"
	"context"
	"crypto/sha1" //nolint:gosec
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/klauspost/compress/gzip"

	"github.com/M0Rf30/yap/v2/pkg/builders/common"
	"github.com/M0Rf30/yap/v2/pkg/constants"
	"github.com/M0Rf30/yap/v2/pkg/errors"
	"github.com/M0Rf30/yap/v2/pkg/files"
	"github.com/M0Rf30/yap/v2/pkg/git"
	"github.com/M0Rf30/yap/v2/pkg/i18n"
	"github.com/M0Rf30/yap/v2/pkg/logger"
	"github.com/M0Rf30/yap/v2/pkg/pkgbuild"
)

// apkFile is a tiny replacement for archives.FileInfo, carrying only the
// fields writeFileWithChecksum needs. It avoids pulling in mholt/archives just
// for a directory walker.
type apkFile struct {
	fs.FileInfo

	diskPath      string // absolute path on disk (empty for synthetic entries)
	nameInArchive string // canonicalised name (forward slashes, no leading sep)
	linkTarget    string // populated for symlinks
}

func (a apkFile) NameInArchive() string { return a.nameInArchive }
func (a apkFile) LinkTarget() string    { return a.linkTarget }

// Open returns a reader for the file's contents. Symlinks and directories
// have no body and return an empty reader.
func (a apkFile) Open() (io.ReadCloser, error) {
	if !a.Mode().IsRegular() {
		return io.NopCloser(strings.NewReader("")), nil
	}

	return os.Open(filepath.Clean(a.diskPath))
}

// walkAPKFiles walks sourceDir and emits an apkFile for each entry with names
// relative to sourceDir. Mirrors archives.FilesFromDisk(FollowSymlinks: false).
func walkAPKFiles(sourceDir string) ([]apkFile, error) {
	sourceDir = filepath.Clean(sourceDir)

	var result []apkFile

	err := filepath.WalkDir(sourceDir, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}

		if path == sourceDir {
			return nil
		}

		info, err := d.Info()
		if err != nil {
			return err
		}

		rel, err := filepath.Rel(sourceDir, path)
		if err != nil {
			return err
		}

		name := filepath.ToSlash(rel)

		entry := apkFile{
			FileInfo:      info,
			diskPath:      path,
			nameInArchive: name,
		}

		if info.Mode()&os.ModeSymlink != 0 {
			t, err := os.Readlink(path)
			if err != nil {
				return err
			}

			entry.linkTarget = t
		}

		result = append(result, entry)

		return nil
	})
	if err != nil {
		return nil, err
	}

	return result, nil
}

// Apk represents the APK package builder.
// It embeds the common.BaseBuilder to inherit shared functionality.
type Apk struct {
	*common.BaseBuilder
}

// NewBuilder creates a new APK package builder.
func NewBuilder(pkgBuild *pkgbuild.PKGBUILD) *Apk {
	return &Apk{
		BaseBuilder: common.NewBaseBuilder(pkgBuild, "apk"),
	}
}

// BuildPackage creates an APK package without external dependencies.
// The package is created as a gzip-compressed tar archive containing the package
// files and metadata (.PKGINFO and optional .install script).
// Returns the path to the created APK file.
func (a *Apk) BuildPackage(ctx context.Context, artifactsPath string, targetArch string) (string, error) {
	a.SetTargetArchitecture(targetArch)

	pkgName := a.BuildPackageName(".apk")
	pkgFilePath := filepath.Join(artifactsPath, pkgName)

	err := a.createTarGzWithChecksums(ctx, a.PKGBUILD.PackageDir, pkgFilePath)
	if err != nil {
		return "", err
	}

	a.LogPackageCreated(pkgFilePath)

	return pkgFilePath, nil
}

// PrepareFakeroot sets up the APK package metadata.
// It generates the .PKGINFO file and optional install scripts for lifecycle hooks.
func (a *Apk) PrepareFakeroot(ctx context.Context, artifactsPath string, targetArch string) error {
	a.SetTargetArchitecture(targetArch)

	installedSize, err := files.GetDirSize(a.PKGBUILD.PackageDir)
	if err != nil {
		return errors.Wrap(err, errors.ErrTypeFileSystem, "failed to get package dir size").
			WithOperation("PrepareFakeroot")
	}

	a.PKGBUILD.InstalledSize = installedSize
	a.PKGBUILD.BuildDate = time.Now().Unix()
	a.PKGBUILD.PkgDest = artifactsPath
	a.PKGBUILD.YAPVersion = constants.YAPVersion

	if a.PKGBUILD.Origin == "" {
		a.PKGBUILD.Origin = a.PKGBUILD.PkgName
	}

	if a.PKGBUILD.Commit == "" {
		a.PKGBUILD.Commit = git.GetCommitHash(a.PKGBUILD.StartDir)
	}

	err = a.createPkgInfo()
	if err != nil {
		return err
	}

	if a.PKGBUILD.PreInst != "" || a.PKGBUILD.PostInst != "" ||
		a.PKGBUILD.PreRm != "" || a.PKGBUILD.PostRm != "" {
		err = a.createInstallScript()
		if err != nil {
			return err
		}
	}

	// Note: APK format does not have a native changelog convention.
	// Changelog support is skipped for APK packages.

	return nil
}

// createPkgInfo generates the .PKGINFO metadata file for APK packages.
// This file contains package information like name, version, dependencies, etc.
func (a *Apk) createPkgInfo() error {
	tmpl := a.PKGBUILD.RenderSpec(dotPkginfo)

	pkginfoPath := filepath.Join(a.PKGBUILD.PackageDir, pkginfoFileName)

	return a.PKGBUILD.CreateSpec(pkginfoPath, tmpl)
}

// createInstallScript generates the install script for APK packages.
// This script contains pre/post install/remove hooks.
func (a *Apk) createInstallScript() error {
	tmpl := a.PKGBUILD.RenderSpec(installScript)

	scriptPath := filepath.Join(a.PKGBUILD.PackageDir, ".install")

	return a.PKGBUILD.CreateSpec(scriptPath, tmpl)
}

// pkginfoFileName is the canonical archive name for the APK metadata file.
const pkginfoFileName = ".PKGINFO"

// isControlFile determines if a file is a control/metadata file.
// Control files use USTAR format, data files use PAX with checksums.
func isControlFile(name string) bool {
	return strings.HasPrefix(name, ".PKGINFO") ||
		strings.HasPrefix(name, ".SIGN") ||
		strings.HasPrefix(name, ".pre-") ||
		strings.HasPrefix(name, ".post-") ||
		strings.HasPrefix(name, ".install") ||
		strings.HasPrefix(name, ".trigger")
}

// writeFilteredMembers writes tar members matching a predicate to the tar writer.
// It's used to write either control or data files to their respective streams.
func (a *Apk) writeFilteredMembers(
	ctx context.Context,
	tw *tar.Writer,
	fileList []apkFile,
	predicate func(apkFile) bool,
) error {
	for _, file := range fileList {
		if !predicate(file) {
			continue
		}

		if err := a.writeFileWithChecksum(ctx, tw, file); err != nil {
			return err
		}
	}

	return nil
}

// createTarGzWithChecksums creates an APK package following Alpine's format:
// The APK is a concatenation of control.tar.gz and data.tar.gz.
// The datahash in .PKGINFO is the SHA256 of data.tar.gz.
//
// This implements the Alpine APK format with APK-TOOLS.checksum.SHA1 headers.
// Streams tar.gz archives directly to temp files to minimize memory usage.
//
//nolint:gocyclo,cyclop // dual-stream orchestration with two temp-file flushes is inherently sequential
func (a *Apk) createTarGzWithChecksums(ctx context.Context, sourceDir, outputFile string) error {
	// Walk source directory once
	fileList, err := walkAPKFiles(sourceDir)
	if err != nil {
		return err
	}

	// Pacman-style: append "/" to directory entry names.
	for i := range fileList {
		if fileList[i].IsDir() && !strings.HasSuffix(fileList[i].nameInArchive, "/") {
			fileList[i].nameInArchive += "/"
		}
	}

	// Create data.tar.gz with only non-control files, streaming to temp file
	dataTempFile, err := os.CreateTemp("", "apk-data-*.tar.gz")
	if err != nil {
		return errors.Wrap(err, errors.ErrTypeFileSystem,
			"failed to create temp file for data.tar.gz").
			WithOperation("createTarGzWithChecksums").
			WithContext("sourceDir", sourceDir)
	}

	dataTempPath := dataTempFile.Name()

	defer func() {
		_ = dataTempFile.Close()
		_ = os.Remove(dataTempPath)
	}()

	dataHasher := sha256.New()
	dataWriter := io.MultiWriter(dataTempFile, dataHasher)

	gzData := gzip.NewWriter(dataWriter)
	gzData.ModTime = time.Unix(0, 0)
	twData := tar.NewWriter(gzData)

	if err := a.writeFilteredMembers(ctx, twData, fileList,
		func(f apkFile) bool { return !isControlFile(f.nameInArchive) }); err != nil {
		_ = twData.Close()
		_ = gzData.Close()

		return err
	}

	if err := twData.Close(); err != nil {
		return err
	}

	if err := gzData.Close(); err != nil {
		return err
	}

	// Compute datahash = SHA256(data.tar.gz)
	dataHash := hex.EncodeToString(dataHasher.Sum(nil))
	a.PKGBUILD.DataHash = dataHash

	// Regenerate .PKGINFO with datahash field
	if err := a.createPkgInfo(); err != nil {
		return err
	}

	// Update .PKGINFO entry in fileList with new stat info from disk
	pkginfoPath := filepath.Join(sourceDir, pkginfoFileName)

	pkginfoInfo, err := os.Stat(pkginfoPath)
	if err == nil {
		// Find and update the .PKGINFO entry in fileList
		for i := range fileList {
			if fileList[i].nameInArchive == pkginfoFileName {
				fileList[i].FileInfo = pkginfoInfo
				fileList[i].diskPath = pkginfoPath

				break
			}
		}
	}

	// Create control.tar.gz with control files only, streaming to temp file
	controlTempFile, err := os.CreateTemp("", "apk-control-*.tar.gz")
	if err != nil {
		return errors.Wrap(err, errors.ErrTypeFileSystem,
			"failed to create temp file for control.tar.gz").
			WithOperation("createTarGzWithChecksums").
			WithContext("sourceDir", sourceDir)
	}

	controlTempPath := controlTempFile.Name()

	defer func() {
		_ = controlTempFile.Close()
		_ = os.Remove(controlTempPath)
	}()

	gzControl := gzip.NewWriter(controlTempFile)
	gzControl.ModTime = time.Unix(0, 0)
	twControl := tar.NewWriter(gzControl)

	if err := a.writeFilteredMembers(ctx, twControl, fileList,
		func(f apkFile) bool { return isControlFile(f.nameInArchive) }); err != nil {
		_ = twControl.Close()
		_ = gzControl.Close()

		return err
	}

	if err := twControl.Close(); err != nil {
		return err
	}

	if err := gzControl.Close(); err != nil {
		return err
	}

	// Write final APK = control.tar.gz + data.tar.gz
	cleanFilePath := filepath.Clean(outputFile)

	out, err := os.Create(cleanFilePath)
	if err != nil {
		return err
	}

	defer func() {
		if closeErr := out.Close(); closeErr != nil {
			logger.Warn(i18n.T("logger.apk.warn.failed_to_close_output"),
				"path", cleanFilePath,
				"error", closeErr)
		}
	}()

	// Seek to beginning of control temp file and copy to output
	if _, err := controlTempFile.Seek(0, 0); err != nil {
		return errors.Wrap(err, errors.ErrTypeFileSystem,
			"failed to seek control temp file").
			WithOperation("createTarGzWithChecksums").
			WithContext("tempFile", controlTempPath)
	}

	if _, err := io.Copy(out, controlTempFile); err != nil {
		return errors.Wrap(err, errors.ErrTypeFileSystem,
			"failed to write control.tar.gz to output").
			WithOperation("createTarGzWithChecksums").
			WithContext("outputFile", cleanFilePath)
	}

	// Seek to beginning of data temp file and copy to output
	if _, err := dataTempFile.Seek(0, 0); err != nil {
		return errors.Wrap(err, errors.ErrTypeFileSystem,
			"failed to seek data temp file").
			WithOperation("createTarGzWithChecksums").
			WithContext("tempFile", dataTempPath)
	}

	if _, err := io.Copy(out, dataTempFile); err != nil {
		return errors.Wrap(err, errors.ErrTypeFileSystem,
			"failed to write data.tar.gz to output").
			WithOperation("createTarGzWithChecksums").
			WithContext("outputFile", cleanFilePath)
	}

	return nil
}

// writeFileWithChecksum writes a file to the tar archive with optional PAX checksum.
// Control files use USTAR format, data files use PAX format with SHA1 checksums.
//
//nolint:gocyclo,nestif // writeFileWithChecksum handles multiple PAX/USTAR format branches inline
func (a *Apk) writeFileWithChecksum(
	ctx context.Context, tw *tar.Writer, file apkFile,
) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	hdr, err := tar.FileInfoHeader(file.FileInfo, file.linkTarget)
	if err != nil {
		return errors.Wrap(err, errors.ErrTypePackaging,
			fmt.Sprintf("file %s: creating header", file.nameInArchive)).
			WithOperation("writeFileWithChecksum").
			WithContext("file", file.nameInArchive)
	}

	hdr.Name = file.nameInArchive
	if hdr.Name == "" {
		hdr.Name = file.Name()
	}

	hdr.Uid = 0
	hdr.Gid = 0
	hdr.Uname = "root"
	hdr.Gname = "root"

	hdr.ModTime = time.Unix(0, 0)
	hdr.ChangeTime = time.Time{}
	hdr.AccessTime = time.Time{}

	// All files use PAX format to match Alpine's abuild-tar behavior
	hdr.Format = tar.FormatPAX
	hdr.PAXRecords = make(map[string]string)

	// Add standard PAX records for reproducibility
	hdr.PAXRecords["mtime"] = "0"
	hdr.PAXRecords["atime"] = "0"
	hdr.PAXRecords["ctime"] = "0"

	// Data files (non-control) get SHA1 checksums
	if !isControlFile(hdr.Name) && hdr.Typeflag == tar.TypeReg {
		// Pass 1: compute SHA1 by streaming (no full buffer)
		hasher := sha1.New() //nolint:gosec

		f1, err := file.Open()
		if err != nil {
			return errors.Wrap(err, errors.ErrTypePackaging,
				fmt.Sprintf("file %s: opening for checksum", file.nameInArchive)).
				WithOperation("writeFileWithChecksum").
				WithContext("file", file.nameInArchive)
		}

		if _, err := io.Copy(hasher, f1); err != nil {
			_ = f1.Close()

			return errors.Wrap(err, errors.ErrTypePackaging,
				fmt.Sprintf("file %s: computing checksum", file.nameInArchive)).
				WithOperation("writeFileWithChecksum").
				WithContext("file", file.nameInArchive)
		}

		_ = f1.Close()

		hdr.PAXRecords["APK-TOOLS.checksum.SHA1"] = hex.EncodeToString(hasher.Sum(nil))

		if err := tw.WriteHeader(hdr); err != nil {
			return errors.Wrap(err, errors.ErrTypePackaging,
				fmt.Sprintf("file %s: writing header", file.nameInArchive)).
				WithOperation("writeFileWithChecksum").
				WithContext("file", file.nameInArchive)
		}

		// Pass 2: stream file data into tar writer
		f2, err := file.Open()
		if err != nil {
			return errors.Wrap(err, errors.ErrTypePackaging,
				fmt.Sprintf("file %s: opening for write", file.nameInArchive)).
				WithOperation("writeFileWithChecksum").
				WithContext("file", file.nameInArchive)
		}
		defer func() { _ = f2.Close() }()

		if _, err := io.Copy(tw, f2); err != nil {
			return errors.Wrap(err, errors.ErrTypePackaging,
				fmt.Sprintf("file %s: writing data", file.nameInArchive)).
				WithOperation("writeFileWithChecksum").
				WithContext("file", file.nameInArchive)
		}

		return nil
	}

	if err := tw.WriteHeader(hdr); err != nil {
		return errors.Wrap(err, errors.ErrTypePackaging,
			fmt.Sprintf("file %s: writing header", file.nameInArchive)).
			WithOperation("writeFileWithChecksum").
			WithContext("file", file.nameInArchive)
	}

	if hdr.Typeflag != tar.TypeReg {
		return nil
	}

	f, err := file.Open()
	if err != nil {
		return errors.Wrap(err, errors.ErrTypePackaging,
			fmt.Sprintf("file %s: opening", file.nameInArchive)).
			WithOperation("writeFileWithChecksum").
			WithContext("file", file.nameInArchive)
	}

	defer func() {
		_ = f.Close()
	}()

	if _, err := io.Copy(tw, f); err != nil {
		return errors.Wrap(err, errors.ErrTypePackaging,
			fmt.Sprintf("file %s: writing data", file.nameInArchive)).
			WithOperation("writeFileWithChecksum").
			WithContext("file", file.nameInArchive)
	}

	return nil
}
