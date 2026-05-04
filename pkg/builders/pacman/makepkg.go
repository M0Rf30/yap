// Package pacman provides functionality for building Arch Linux (.pkg.tar.zst) packages from PKGBUILD specifications.
package pacman

import (
	"bytes"
	"context"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"text/template"

	"github.com/klauspost/pgzip"

	"github.com/M0Rf30/yap/v2/pkg/archive"
	"github.com/M0Rf30/yap/v2/pkg/builders/common"
	"github.com/M0Rf30/yap/v2/pkg/constants"
	"github.com/M0Rf30/yap/v2/pkg/crypto"
	"github.com/M0Rf30/yap/v2/pkg/files"
	"github.com/M0Rf30/yap/v2/pkg/i18n"
	"github.com/M0Rf30/yap/v2/pkg/logger"
	"github.com/M0Rf30/yap/v2/pkg/options"
	"github.com/M0Rf30/yap/v2/pkg/pkgbuild"
)

// Pkg represents a package manager for the Pkg distribution.
//
// It contains methods for building, installing, and updating packages.
type Pkg struct {
	*common.BaseBuilder
	pacmanDir string
}

// NewBuilder creates a new Pacman package builder.
func NewBuilder(pkgBuild *pkgbuild.PKGBUILD) *Pkg {
	return &Pkg{
		BaseBuilder: common.NewBaseBuilder(pkgBuild, "pacman"),
	}
}

// BuildPackage initiates the package building process for the Makepkg instance.
//
// It takes a single parameter:
// - artifactsPath: a string representing the path where the build artifacts will be stored.
//
// The method calls the internal pacmanBuild function to perform the actual build process.
// Returns the path to the created package file.
func (m *Pkg) BuildPackage(artifactsPath string, targetArch string) (string, error) {
	m.SetTargetArchitecture(targetArch)

	completeVersion := m.PKGBUILD.PkgVer

	if m.PKGBUILD.Epoch != "" {
		completeVersion = fmt.Sprintf("%s:%s", m.PKGBUILD.Epoch, m.PKGBUILD.PkgVer)
	}

	// Build package name with the complete version for Pacman format
	pkgName := fmt.Sprintf("%s-%s-%s-%s.pkg.tar.zst",
		m.PKGBUILD.PkgName,
		completeVersion,
		m.PKGBUILD.PkgRel,
		m.PKGBUILD.ArchComputed)
	pkgFilePath := filepath.Join(artifactsPath, pkgName)

	err := archive.CreateTarZst(context.Background(), m.PKGBUILD.PackageDir, pkgFilePath, false)
	if err != nil {
		return "", err
	}

	// Log package creation using common functionality
	m.LogPackageCreated(pkgFilePath)

	return pkgFilePath, nil
}

// PrepareFakeroot sets um the environment for building a package in a fakeroot context.
//
// It takes an artifactsPath parameter, which specifies where to store build artifacts.
// The method initializes the pacmanDir, resolves the package destination, and creates
// the PKGBUILD and post-installation script files if necessary. It returns an error
// if any stem fails.
func (m *Pkg) PrepareFakeroot(artifactsPath string, targetArch string) error {
	m.pacmanDir = m.PKGBUILD.StartDir
	// Note: Don't override ArchComputed here - it should remain the native architecture
	// The targetArch is used for package naming in BuildPackage method

	if err := m.computeBuildMetadata(artifactsPath); err != nil {
		return err
	}

	if err := m.renderPKGBUILDFile(); err != nil {
		return err
	}

	if err := m.writePackageMetadata(); err != nil {
		return err
	}

	if m.PKGBUILD.StripEnabled {
		if err := options.Strip(m.PKGBUILD.PackageDir); err != nil {
			return err
		}
	}

	if err := m.writeMTREE(); err != nil {
		return err
	}

	if err := m.writeInstallScriptIfNeeded(); err != nil {
		return err
	}

	return m.writeChangelogIfPresent()
}

// computeBuildMetadata computes installed size, source date epoch, and other
// PKGBUILD metadata fields needed for spec rendering.
func (m *Pkg) computeBuildMetadata(artifactsPath string) error {
	installedSize, err := files.GetDirSize(m.PKGBUILD.PackageDir)
	if err != nil {
		return fmt.Errorf("failed to get package dir size: %w", err)
	}

	m.PKGBUILD.InstalledSize = installedSize

	sourceDateEpoch, err := files.ResolveSourceDateEpoch(m.PKGBUILD.Home)
	if err != nil {
		return err
	}

	const pkgTypeDefault = "pkg"

	m.PKGBUILD.BuildDate = sourceDateEpoch.Unix()
	m.PKGBUILD.PkgDest, _ = filepath.Abs(artifactsPath)
	m.PKGBUILD.PkgType = pkgTypeDefault // can be pkg, split, debug, src
	m.PKGBUILD.YAPVersion = constants.YAPVersion

	return nil
}

// renderPKGBUILDFile renders and writes the PKGBUILD spec, then computes its
// SHA256 checksum for inclusion in .BUILDINFO.
func (m *Pkg) renderPKGBUILDFile() error {
	tmpl := m.PKGBUILD.RenderSpec(specFile)
	pkgBuildFile := filepath.Join(m.pacmanDir, "PKGBUILD")

	if m.PKGBUILD.Home != m.PKGBUILD.StartDir {
		if err := m.PKGBUILD.CreateSpec(pkgBuildFile, tmpl); err != nil {
			return err
		}
	}

	checksumBytes, err := crypto.CalculateSHA256(pkgBuildFile)
	if err != nil {
		return err
	}

	m.PKGBUILD.Checksum = hex.EncodeToString(checksumBytes)

	return nil
}

// writePackageMetadata renders and writes the .PKGINFO and .BUILDINFO files.
func (m *Pkg) writePackageMetadata() error {
	pkgInfoTmpl := m.PKGBUILD.RenderSpec(dotPkginfo)
	if err := m.PKGBUILD.CreateSpec(
		filepath.Join(m.PKGBUILD.PackageDir, ".PKGINFO"), pkgInfoTmpl); err != nil {
		return err
	}

	buildInfoTmpl := m.PKGBUILD.RenderSpec(dotBuildinfo)

	return m.PKGBUILD.CreateSpec(
		filepath.Join(m.PKGBUILD.PackageDir, ".BUILDINFO"), buildInfoTmpl)
}

// writeMTREE walks the package directory and writes a gzip-compressed .MTREE.
func (m *Pkg) writeMTREE() error {
	walker := m.CreateFileWalker()

	entries, err := walker.Walk()
	if err != nil {
		return err
	}

	mtreeFile, err := renderMtree(entries)
	if err != nil {
		return err
	}

	return createMTREEGzip(mtreeFile, filepath.Join(m.PKGBUILD.PackageDir, ".MTREE"))
}

// writeInstallScriptIfNeeded writes the <pkgname>.install file when the
// PKGBUILD declares any of the six scriptlet hooks.
func (m *Pkg) writeInstallScriptIfNeeded() error {
	if m.PKGBUILD.PreInst == "" && m.PKGBUILD.PostInst == "" &&
		m.PKGBUILD.PreRm == "" && m.PKGBUILD.PostRm == "" &&
		m.PKGBUILD.PreUpgrade == "" && m.PKGBUILD.PostUpgrade == "" {
		return nil
	}

	tmpl := m.PKGBUILD.RenderSpec(postInstall)

	return m.PKGBUILD.CreateSpec(
		filepath.Join(m.pacmanDir, m.PKGBUILD.PkgName+".install"), tmpl)
}

// writeChangelogIfPresent writes a .CHANGELOG file in the package root when
// the PKGBUILD declares a changelog source.
func (m *Pkg) writeChangelogIfPresent() error {
	changelogData, err := m.PKGBUILD.ReadChangelog()
	if err != nil {
		return err
	}

	if changelogData == nil {
		return nil
	}

	changelogPath := filepath.Join(m.PKGBUILD.PackageDir, ".CHANGELOG")

	// #nosec G306 -- .CHANGELOG matches Pacman world-readable convention
	return os.WriteFile(filepath.Clean(changelogPath), changelogData, 0o644)
}

func renderMtree(entries []*files.Entry) (string, error) {
	tmpl, err := template.New("mtree").Parse(dotMtree)
	if err != nil {
		return "", err
	}

	var buf bytes.Buffer

	err = tmpl.Execute(&buf, entries)
	if err != nil {
		return "", err
	}

	return buf.String(), nil
}

// createMTREEGzip creates a compressed tar.zst archive from the specified source
// directory. It takes the source directory and the output file path as
// arguments and returns an error if any occurs.
func createMTREEGzip(mtreeContent, outputFile string) error {
	cleanFilePath := filepath.Clean(outputFile)

	out, err := os.Create(cleanFilePath)
	if err != nil {
		return err
	}

	defer func() {
		err := out.Close()
		if err != nil {
			logger.Warn(i18n.T("logger.createmtreegzip.warn.failed_to_close_output_1"), "error", err)
		}
	}()

	// Create a gzip writer
	gzipWriter := pgzip.NewWriter(out)

	defer func() {
		err := gzipWriter.Close()
		if err != nil {
			logger.Warn(i18n.T("logger.createmtreegzip.warn.failed_to_close_gzip_1"), "error", err)
		}
	}()

	// Copy the source file to the gzip writer
	_, err = io.Copy(gzipWriter, strings.NewReader(mtreeContent))
	if err != nil {
		return err
	}

	return nil
}
