// Package apk provides functionality for building Alpine Linux APK packages.
package apk

import (
	"archive/tar"
	"bytes"
	"context"
	"crypto/sha1" //nolint:gosec // SHA1 required by APK format for checksum headers
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/klauspost/compress/gzip"
	"github.com/mholt/archives"

	"github.com/M0Rf30/yap/v2/pkg/builders/common"
	"github.com/M0Rf30/yap/v2/pkg/constants"
	"github.com/M0Rf30/yap/v2/pkg/files"
	"github.com/M0Rf30/yap/v2/pkg/git"
	"github.com/M0Rf30/yap/v2/pkg/i18n"
	"github.com/M0Rf30/yap/v2/pkg/logger"
	"github.com/M0Rf30/yap/v2/pkg/pkgbuild"
)

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

// BuildPackage creates an APK package in pure Go without external dependencies.
// The package is created as a gzip-compressed tar archive containing the package
// files and metadata (.PKGINFO and optional .install script).
func (a *Apk) BuildPackage(artifactsPath string, targetArch string) error {
	// If target architecture is specified for cross-compilation, use it
	if targetArch != "" {
		a.PKGBUILD.ArchComputed = targetArch
	}

	a.TranslateArchitecture()

	pkgName := a.BuildPackageName(".apk")
	pkgFilePath := filepath.Join(artifactsPath, pkgName)

	err := a.createAPKPackage(pkgFilePath, artifactsPath)
	if err != nil {
		return err
	}

	a.LogPackageCreated(pkgFilePath)

	return nil
}

// PrepareFakeroot sets up the APK package metadata.
// It generates the .PKGINFO file and optional install scripts for lifecycle hooks.
func (a *Apk) PrepareFakeroot(artifactsPath string, targetArch string) error {
	// If target architecture is specified for cross-compilation, use it
	if targetArch != "" {
		a.PKGBUILD.ArchComputed = targetArch
	}

	a.TranslateArchitecture()

	a.PKGBUILD.InstalledSize, _ = files.GetDirSize(a.PKGBUILD.PackageDir)
	a.PKGBUILD.BuildDate = time.Now().Unix()
	a.PKGBUILD.PkgDest = artifactsPath
	a.PKGBUILD.YAPVersion = constants.YAPVersion

	if a.PKGBUILD.Origin == "" {
		a.PKGBUILD.Origin = a.PKGBUILD.PkgName
	}

	if a.PKGBUILD.Commit == "" {
		a.PKGBUILD.Commit = git.GetCommitHash(a.PKGBUILD.StartDir)
	}

	err := a.createPkgInfo()
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

	return nil
}

// createAPKPackage creates an APK package archive.
// APK packages are gzip-compressed tar archives containing the package files
// and metadata (.PKGINFO).
func (a *Apk) createAPKPackage(pkgFilePath, _ string) error {
	return a.createTarGzWithChecksums(a.PKGBUILD.PackageDir, pkgFilePath)
}

// createPkgInfo generates the .PKGINFO metadata file for APK packages.
// This file contains package information like name, version, dependencies, etc.
func (a *Apk) createPkgInfo() error {
	tmpl := a.PKGBUILD.RenderSpec(dotPkginfo)

	pkginfoPath := filepath.Join(a.PKGBUILD.PackageDir, ".PKGINFO")

	return a.PKGBUILD.CreateSpec(pkginfoPath, tmpl)
}

// createInstallScript generates the install script for APK packages.
// This script contains pre/post install/remove hooks.
func (a *Apk) createInstallScript() error {
	tmpl := a.PKGBUILD.RenderSpec(installScript)

	scriptPath := filepath.Join(a.PKGBUILD.PackageDir, ".install")

	return a.PKGBUILD.CreateSpec(scriptPath, tmpl)
}

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

// createTarGzWithChecksums creates an APK package following Alpine's format:
// The APK is a concatenation of control.tar.gz and data.tar.gz.
// The datahash in .PKGINFO is the SHA256 of data.tar.gz.
//
// This implements the Alpine APK format with APK-TOOLS.checksum.SHA1 headers.
//
//nolint:gocyclo,cyclop // APK format requires specific two-stream construction
func (a *Apk) createTarGzWithChecksums(sourceDir, outputFile string) error {
	ctx := context.Background()
	options := &archives.FromDiskOptions{
		FollowSymlinks: false,
	}

	fileList, err := archives.FilesFromDisk(ctx, options, map[string]string{
		sourceDir + string(os.PathSeparator): "",
	})
	if err != nil {
		return err
	}

	for i := range fileList {
		if fileList[i].IsDir() && !strings.HasSuffix(fileList[i].NameInArchive, "/") {
			fileList[i].NameInArchive += "/"
		}
	}

	// Step 1: Create data.tar.gz with only non-control files
	var dataBuf bytes.Buffer

	gzData := gzip.NewWriter(&dataBuf)
	gzData.ModTime = time.Unix(0, 0)
	twData := tar.NewWriter(gzData)

	dataFileCount := 0

	for _, file := range fileList {
		if isControlFile(file.NameInArchive) {
			continue
		}

		dataFileCount++

		if err := a.writeFileWithChecksum(ctx, twData, file); err != nil {
			_ = twData.Close()
			_ = gzData.Close()

			return err
		}
	}

	if err := twData.Close(); err != nil {
		return err
	}

	if err := gzData.Close(); err != nil {
		return err
	}

	// Step 2: Compute datahash = SHA256(data.tar.gz)
	dataHasher := sha256.New()
	dataHasher.Write(dataBuf.Bytes())
	dataHash := hex.EncodeToString(dataHasher.Sum(nil))
	a.PKGBUILD.DataHash = dataHash

	// Step 3: Regenerate .PKGINFO with datahash field
	if err := a.createPkgInfo(); err != nil {
		return err
	}

	// Step 4: Reload file list to get updated .PKGINFO
	fileList2, err := archives.FilesFromDisk(ctx, options, map[string]string{
		sourceDir + string(os.PathSeparator): "",
	})
	if err != nil {
		return err
	}

	for i := range fileList2 {
		if fileList2[i].IsDir() && !strings.HasSuffix(fileList2[i].NameInArchive, "/") {
			fileList2[i].NameInArchive += "/"
		}
	}

	// Step 5: Create control.tar.gz with control files only
	var controlBuf bytes.Buffer

	gzControl := gzip.NewWriter(&controlBuf)
	gzControl.ModTime = time.Unix(0, 0)
	twControl := tar.NewWriter(gzControl)

	controlFileCount := 0

	for _, file := range fileList2 {
		if !isControlFile(file.NameInArchive) {
			continue
		}

		controlFileCount++

		if err := a.writeFileWithChecksum(ctx, twControl, file); err != nil {
			_ = twControl.Close()
			_ = gzControl.Close()

			return err
		}
	}

	if err := twControl.Close(); err != nil {
		return err
	}

	if err := gzControl.Close(); err != nil {
		return err
	}

	// Step 6: Write final APK = control.tar.gz + data.tar.gz
	cleanFilePath := filepath.Clean(outputFile)

	out, err := os.Create(cleanFilePath)
	if err != nil {
		return err
	}

	defer func() {
		if closeErr := out.Close(); closeErr != nil {
			logger.Warn(i18n.T("logger.createtargz.warn.failed_to_close_output_1"),
				"path", cleanFilePath,
				"error", closeErr)
		}
	}()

	// Write control.tar.gz
	if _, err := out.Write(controlBuf.Bytes()); err != nil {
		return err
	}

	// Write data.tar.gz
	if _, err := out.Write(dataBuf.Bytes()); err != nil {
		return err
	}

	return nil
}

// writeFileWithChecksum writes a file to the tar archive with optional PAX checksum.
// Control files use USTAR format, data files use PAX format with SHA1 checksums.
// nolint:gocyclo,nestif
func (a *Apk) writeFileWithChecksum(
	ctx context.Context, tw *tar.Writer, file archives.FileInfo,
) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	hdr, err := tar.FileInfoHeader(file, file.LinkTarget)
	if err != nil {
		return fmt.Errorf("file %s: creating header: %w", file.NameInArchive, err)
	}

	hdr.Name = file.NameInArchive
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
		f, err := file.Open()
		if err != nil {
			return fmt.Errorf("file %s: opening for checksum: %w", file.NameInArchive, err)
		}

		defer func() {
			_ = f.Close()
		}()

		// nolint:gosec
		hasher := sha1.New()

		content, err := io.ReadAll(f)
		if err != nil {
			return fmt.Errorf("file %s: reading for checksum: %w", file.NameInArchive, err)
		}

		if _, err := hasher.Write(content); err != nil {
			return fmt.Errorf("file %s: computing checksum: %w", file.NameInArchive, err)
		}

		hdr.PAXRecords["APK-TOOLS.checksum.SHA1"] = hex.EncodeToString(hasher.Sum(nil))

		if err := tw.WriteHeader(hdr); err != nil {
			return fmt.Errorf("file %s: writing header: %w", file.NameInArchive, err)
		}

		if _, err := tw.Write(content); err != nil {
			return fmt.Errorf("file %s: writing data: %w", file.NameInArchive, err)
		}

		return nil
	}

	if err := tw.WriteHeader(hdr); err != nil {
		return fmt.Errorf("file %s: writing header: %w", file.NameInArchive, err)
	}

	if hdr.Typeflag != tar.TypeReg {
		return nil
	}

	f, err := file.Open()
	if err != nil {
		return fmt.Errorf("file %s: opening: %w", file.NameInArchive, err)
	}

	defer func() {
		_ = f.Close()
	}()

	if _, err := io.Copy(tw, f); err != nil {
		return fmt.Errorf("file %s: writing data: %w", file.NameInArchive, err)
	}

	return nil
}
