// SPDX-FileCopyrightText: 2021-2026 Gianluca Boiano
// SPDX-License-Identifier: GPL-3.0-only

// Package pacman provides functionality for building Arch Linux (.pkg.tar.zst) packages from PKGBUILD specifications.
package pacman

import (
	"context"
	"encoding/hex"
	"os"
	"path/filepath"

	"github.com/M0Rf30/pkgformat/pacman"

	"github.com/M0Rf30/yap/v2/pkg/builders/common"
	"github.com/M0Rf30/yap/v2/pkg/constants"
	"github.com/M0Rf30/yap/v2/pkg/crypto"
	"github.com/M0Rf30/yap/v2/pkg/errors"
	"github.com/M0Rf30/yap/v2/pkg/files"
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

	if err := ctx.Err(); err != nil {
		return "", err
	}

	spec, err := m.spec()
	if err != nil {
		return "", err
	}

	pkgFilePath := filepath.Join(artifactsPath, m.BuildPackageName(constants.ExtPacmanZst))

	err = spec.WriteFile(pkgFilePath, pacman.WriteOptions{
		Compression: pacman.CompressionZstd,
		Generator:   "yap " + m.PKGBUILD.YAPVersion,
	})
	if err != nil {
		return "", errors.Wrap(err, errors.ErrTypePackaging, "failed to write pacman package").
			WithOperation("BuildPackage").
			WithContext("path", pkgFilePath)
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

	return m.writeInstallCopy()
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

// spec maps the PKGBUILD to the package the pacman library writes: .PKGINFO,
// .BUILDINFO, .INSTALL, .CHANGELOG and the payload staged in PackageDir (the
// library generates .MTREE).
func (m *Pkg) spec() (*pacman.Spec, error) {
	pb := m.PKGBUILD

	changelog, err := pb.ReadChangelog()
	if err != nil {
		return nil, err
	}

	payload, err := pacman.ScanDir(pb.PackageDir)
	if err != nil {
		return nil, errors.Wrap(err, errors.ErrTypeFileSystem, "failed to list package payload").
			WithOperation("spec").
			WithContext("package_dir", pb.PackageDir)
	}

	version := pb.PkgVer + "-" + pb.PkgRel
	if pb.Epoch != "" {
		version = pb.Epoch + ":" + version
	}

	var groups []string
	if pb.Group != "" {
		groups = []string{pb.Group}
	}

	buildOptions := pb.Options
	if len(buildOptions) == 0 {
		buildOptions = []string{"strip", "staticlibs"}
	}

	return &pacman.Spec{
		Info: pacman.PkgInfo{
			Name:        pb.PkgName,
			Base:        pb.EffectivePkgBase(),
			Version:     version,
			Description: pb.PkgDesc,
			URL:         pb.URL,
			Packager:    pb.Maintainer,
			Arch:        pb.ArchComputed,
			BuildDate:   pb.BuildDate,
			InstalledSz: pb.InstalledSize,
			Licenses:    pb.License,
			Groups:      groups,
			Replaces:    pb.Replaces,
			Conflicts:   pb.Conflicts,
			Provides:    pb.Provides,
			Backup:      pb.Backup,
			Depends:     pb.Depends,
			OptDepends:  pb.OptDepends,
			MakeDepends: pb.MakeDepends,
			XData:       []string{"pkgtype=" + pb.PkgType},
		},
		BuildInfo: &pacman.BuildInfo{
			PkgName:        pb.PkgName,
			PkgBase:        pb.EffectivePkgBase(),
			PkgVer:         version,
			PkgArch:        pb.ArchComputed,
			PkgbuildSHA256: pb.Checksum,
			Packager:       pb.Maintainer,
			BuildDate:      pb.BuildDate,
			BuildDir:       pb.PackageDir,
			StartDir:       pb.Home,
			BuildTool:      "yap",
			BuildToolVer:   pb.YAPVersion,
			Options:        buildOptions,
		},
		Scriptlets: m.scriptlets(),
		Changelog:  changelog,
		Files:      payload,
	}, nil
}

// scriptlets maps the six PKGBUILD hooks to the .INSTALL file, preceded by the
// PKGBUILD helper functions the hooks call. Hooks that only declare install
// steps also run them on upgrade. It returns nil when no hook is declared.
func (m *Pkg) scriptlets() *pacman.Scriptlets {
	pb := m.PKGBUILD

	sc := &pacman.Scriptlets{
		PreInstall:      pb.PreInst,
		PostInstall:     pb.PostInst,
		PreUpgrade:      pb.PreUpgrade,
		PostUpgrade:     pb.PostUpgrade,
		PreRemove:       pb.PreRm,
		PostRemove:      pb.PostRm,
		UpgradeFallback: true,
	}

	if sc.Empty() {
		return nil
	}

	sc.Preamble = pb.HelperFunctionsPreamble(sc.Hooks())

	return sc
}

// writeInstallCopy keeps a <pkgname>.install copy of the scriptlet next to the
// generated PKGBUILD, which references it via install=. The package itself
// carries .INSTALL, written by the library.
func (m *Pkg) writeInstallCopy() error {
	sc := m.scriptlets()
	if sc == nil {
		return nil
	}

	target := filepath.Join(m.pacmanDir, m.PKGBUILD.PkgName+".install")
	if err := os.WriteFile(filepath.Clean(target), sc.Render(), 0o644); err != nil { //nolint:gosec
		return errors.Wrap(err, errors.ErrTypeFileSystem, "failed to write install scriptlet").
			WithContext("path", target).
			WithOperation("writeInstallCopy")
	}

	return nil
}
