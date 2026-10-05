// SPDX-FileCopyrightText: 2021-2026 Gianluca Boiano
// SPDX-License-Identifier: GPL-3.0-only

// Package pacman provides functionality for building Arch Linux (.pkg.tar.zst) packages from PKGBUILD specifications.
package pacman

import (
	"bytes"
	"context"
	"encoding/hex"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"text/template"

	"github.com/klauspost/pgzip"

	"github.com/M0Rf30/yap/v2/pkg/archive"
	"github.com/M0Rf30/yap/v2/pkg/builders/common"
	"github.com/M0Rf30/yap/v2/pkg/constants"
	"github.com/M0Rf30/yap/v2/pkg/crypto"
	"github.com/M0Rf30/yap/v2/pkg/errors"
	"github.com/M0Rf30/yap/v2/pkg/files"
	"github.com/M0Rf30/yap/v2/pkg/i18n"
	"github.com/M0Rf30/yap/v2/pkg/logger"
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
func (m *Pkg) BuildPackage(ctx context.Context, artifactsPath string, targetArch string) (string, error) {
	m.SetTargetArchitecture(targetArch)

	pkgName := m.BuildPackageName(constants.ExtPacmanZst)
	pkgFilePath := filepath.Join(artifactsPath, pkgName)

	err := archive.CreateTarZst(ctx, m.PKGBUILD.PackageDir, pkgFilePath, false)
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
func (m *Pkg) PrepareFakeroot(ctx context.Context, artifactsPath string, targetArch string) error {
	m.pacmanDir = m.PKGBUILD.StartDir

	// Resolve the cross strip environment before the target arch is stored in
	// ArchComputed: BuildCrossStripEnvSlice is a no-op when both are equal.
	stripEnv := m.CrossStripEnvMap(targetArch)

	// Render .PKGINFO/.BUILDINFO/PKGBUILD with the target architecture so the
	// metadata matches the artifact file name produced by BuildPackage.
	m.SetTargetArchitecture(targetArch)

	// Apply options first so the computed installed size reflects the
	// stripped/purged payload.
	if err := m.ApplyOptionsWithEnv(stripEnv); err != nil {
		return err
	}

	if err := m.computeBuildMetadata(artifactsPath); err != nil {
		return err
	}

	if err := m.renderPKGBUILDFile(); err != nil {
		return err
	}

	if err := m.writePackageMetadata(); err != nil {
		return err
	}

	if err := m.writeInstallScriptIfNeeded(); err != nil {
		return err
	}

	// .CHANGELOG and .INSTALL are part of the payload, so they must exist
	// before .MTREE is generated.
	if err := m.writeChangelogIfPresent(); err != nil {
		return err
	}

	return m.writeMTREE()
}

// computeBuildMetadata computes installed size, source date epoch, and other
// PKGBUILD metadata fields needed for spec rendering.
func (m *Pkg) computeBuildMetadata(artifactsPath string) error {
	installedSize, err := files.GetDirSize(m.PKGBUILD.PackageDir)
	if err != nil {
		return errors.Wrap(err, errors.ErrTypeFileSystem, "failed to get package dir size").
			WithOperation("computeBuildMetadata")
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
// Dotfiles are included (.PKGINFO, .BUILDINFO, .INSTALL and legitimately
// packaged ones such as /etc/skel/.bashrc); only .MTREE itself is skipped.
func (m *Pkg) writeMTREE() error {
	walker := files.NewWalker(m.PKGBUILD.PackageDir, files.WalkOptions{
		BackupFiles:  m.PKGBUILD.Backup,
		SkipPatterns: []string{".MTREE"},
	})

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

// writeInstallScriptIfNeeded renders the pacman install scriptlet when the
// PKGBUILD declares any of the six hooks. It is shipped inside the package
// payload as .INSTALL (the only name pacman honours), prefixed with the
// PKGBUILD helper functions the hooks call. A <pkgname>.install copy is also
// kept next to the generated PKGBUILD, which references it via install=.
func (m *Pkg) writeInstallScriptIfNeeded() error {
	if m.PKGBUILD.PreInst == "" && m.PKGBUILD.PostInst == "" &&
		m.PKGBUILD.PreRm == "" && m.PKGBUILD.PostRm == "" &&
		m.PKGBUILD.PreUpgrade == "" && m.PKGBUILD.PostUpgrade == "" {
		return nil
	}

	var buf bytes.Buffer

	if err := m.PKGBUILD.RenderSpec(postInstall).Execute(&buf, m.PKGBUILD); err != nil {
		return errors.Wrap(err, errors.ErrTypeFileSystem, "failed to render .INSTALL").
			WithOperation("writeInstallScriptIfNeeded")
	}

	script := []byte(m.PrepareScriptletWithHelpers(buf.String()))

	targets := []string{
		filepath.Join(m.PKGBUILD.PackageDir, ".INSTALL"),
		filepath.Join(m.pacmanDir, m.PKGBUILD.PkgName+".install"),
	}

	for _, target := range targets {
		if err := os.WriteFile(filepath.Clean(target), script, 0o644); err != nil { //nolint:gosec
			return errors.Wrap(err, errors.ErrTypeFileSystem, "failed to write install scriptlet").
				WithContext("path", target).
				WithOperation("writeInstallScriptIfNeeded")
		}
	}

	return nil
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

	return os.WriteFile(filepath.Clean(changelogPath), changelogData, 0o644) //nolint:gosec
}

// mtreeMode converts a Go os.FileMode into the POSIX octal permission string
// used by mtree: permission bits plus setuid (4000), setgid (2000) and
// sticky (1000). Go's type bits (ModeDir, ModeSymlink, ...) are dropped.
func mtreeMode(mode os.FileMode) string {
	posix := uint32(mode.Perm())

	if mode&os.ModeSetuid != 0 {
		posix |= 0o4000
	}

	if mode&os.ModeSetgid != 0 {
		posix |= 0o2000
	}

	if mode&os.ModeSticky != 0 {
		posix |= 0o1000
	}

	return strconv.FormatUint(uint64(posix), 8)
}

func renderMtree(entries []*files.Entry) (string, error) {
	tmpl, err := template.New("mtree").
		Funcs(template.FuncMap{"mode": mtreeMode}).
		Parse(dotMtree)
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
			logger.Warn(i18n.T("logger.pacman.warn.failed_to_close_output"), "error", err)
		}
	}()

	// Create a gzip writer
	gzipWriter := pgzip.NewWriter(out)

	defer func() {
		err := gzipWriter.Close()
		if err != nil {
			logger.Warn(i18n.T("logger.pacman.warn.failed_to_close_gzip"), "error", err)
		}
	}()

	// Copy the source file to the gzip writer
	_, err = io.Copy(gzipWriter, strings.NewReader(mtreeContent))
	if err != nil {
		return err
	}

	return nil
}
