// SPDX-FileCopyrightText: 2021-2026 Gianluca Boiano
// SPDX-License-Identifier: GPL-3.0-only

// Package deb provides functionality for building Debian (.deb) packages from PKGBUILD specifications.
package deb

import (
	"context"
	"os"
	"path/filepath"
	"time"

	pkgdeb "github.com/M0Rf30/pkgformat/deb"

	"github.com/M0Rf30/yap/v2/pkg/builders/common"
	"github.com/M0Rf30/yap/v2/pkg/constants"
	"github.com/M0Rf30/yap/v2/pkg/errors"
	"github.com/M0Rf30/yap/v2/pkg/files"
	"github.com/M0Rf30/yap/v2/pkg/i18n"
	"github.com/M0Rf30/yap/v2/pkg/logger"
	"github.com/M0Rf30/yap/v2/pkg/pkgbuild"
)

// Package represents a Deb package.
//
// It contains the directory path of the package and the PKGBUILD struct, which
// contains the metadata and build instructions for the package.
type Package struct {
	*common.BaseBuilder
	compression string
}

// NewBuilder creates a new Debian package manager with optional compression setting.
// If compression is empty, defaults to "zstd".
func NewBuilder(pkgBuild *pkgbuild.PKGBUILD, compression string) *Package {
	if compression == "" {
		compression = "zstd"
	}

	return &Package{
		BaseBuilder: common.NewBaseBuilder(pkgBuild, "deb"),
		compression: compression,
	}
}

// BuildPackage writes the Debian package into artifactsPath and removes the
// package directory, returning the path to the created DEB file.
func (d *Package) BuildPackage(_ context.Context, artifactsPath string, _ string) (string, error) {
	spec, err := d.spec()
	if err != nil {
		return "", err
	}

	payload, err := pkgdeb.FSEntries(os.DirFS(d.PKGBUILD.PackageDir), time.Time{})
	if err != nil {
		return "", errors.Wrap(err, errors.ErrTypeFileSystem, "failed to list package payload").
			WithOperation("BuildPackage")
	}

	debTemp, err := os.MkdirTemp(d.PKGBUILD.SourceDir, "tmp")
	if err != nil {
		return "", err
	}

	defer func() {
		err := os.RemoveAll(debTemp)
		if err != nil {
			logger.Warn(i18n.T("logger.deb.warn.failed_to_remove_temporary"),
				"path", debTemp, "error", err)
		}
	}()

	debPath := filepath.Join(artifactsPath, d.BuildPackageName(constants.ExtDEB))

	err = pkgdeb.WriteFile(debPath, spec, payload, pkgdeb.WriteOptions{TempDir: debTemp})
	if err != nil {
		return "", errors.Wrap(err, errors.ErrTypePackaging, "failed to write deb package").
			WithOperation("BuildPackage").
			WithContext("path", debPath)
	}

	d.LogPackageCreated(debPath)

	err = os.RemoveAll(d.PKGBUILD.PackageDir)
	if err != nil {
		return "", err
	}

	return debPath, nil
}

// PrepareFakeroot sets up the environment for building a Debian package in a fakeroot context.
// It retrieves architecture and release information, applies the PKGBUILD options to the
// payload and normalises the dependency fields. The method returns an error if any step fails.
func (d *Package) PrepareFakeroot(_ context.Context, _ string, targetArch string) error {
	d.FormatRelease(map[string]string{})
	d.LogCrossCompilation(targetArch)
	// Resolve the cross strip env before ArchComputed is overwritten:
	// BuildCrossStripEnvSlice is a no-op when targetArch == ArchComputed.
	stripEnv := d.CrossStripEnvMap(targetArch)
	d.SetTargetArchitecture(targetArch)

	// Apply PKGBUILD options (strip, docs, libtool, static, zipman, emptydirs)
	// before the package is written so Installed-Size and md5sums describe the
	// final payload rather than the pre-option tree.
	err := d.ApplyOptionsWithEnv(stripEnv)
	if err != nil {
		return err
	}

	d.PKGBUILD.Depends = d.ProcessDependencies(d.PKGBUILD.Depends)
	d.PKGBUILD.PreDepends = d.ProcessDependencies(d.PKGBUILD.PreDepends)
	d.PKGBUILD.MakeDepends = d.ProcessDependencies(d.PKGBUILD.MakeDepends)
	d.PKGBUILD.OptDepends = d.ProcessDependencies(d.PKGBUILD.OptDepends)
	d.PKGBUILD.Conflicts = d.ProcessDependencies(d.PKGBUILD.Conflicts)
	d.PKGBUILD.Breaks = d.ProcessDependencies(d.PKGBUILD.Breaks)
	d.PKGBUILD.Replaces = d.ProcessDependencies(d.PKGBUILD.Replaces)
	d.PKGBUILD.Provides = d.ProcessDependencies(d.PKGBUILD.Provides)

	return nil
}

// spec maps the PKGBUILD onto the package description the deb writer takes.
// Installed-Size, md5sums, the copyright file, the remove-script header and the
// changelog placement are all the writer's business.
func (d *Package) spec() (*pkgdeb.Spec, error) {
	pb := d.PKGBUILD

	changelog, err := pb.ReadChangelog()
	if err != nil {
		return nil, err
	}

	config, err := d.readDebconf(pb.DebConfig)
	if err != nil {
		return nil, err
	}

	templates, err := d.readDebconf(pb.DebTemplate)
	if err != nil {
		return nil, err
	}

	return &pkgdeb.Spec{
		Name:         pb.PkgName,
		Epoch:        pb.Epoch,
		Version:      pb.PkgVer,
		Release:      pb.PkgRel,
		Architecture: pb.ArchComputed,
		MultiArch:    pb.MultiArch,
		Section:      pb.Section,
		Priority:     pb.Priority,
		Maintainer:   pb.Maintainer,
		SourcePkg:    pb.SourcePkg,
		URL:          pb.URL,
		Bugs:         pb.Bugs,
		Description:  pb.PkgDesc,
		Provides:     pb.Provides,
		PreDepends:   pb.PreDepends,
		Depends:      pb.Depends,
		Conflicts:    pb.Conflicts,
		Breaks:       pb.Breaks,
		Replaces:     pb.Replaces,
		Recommends:   pb.OptDepends,
		Suggests:     pb.Suggests,
		Enhances:     pb.Enhances,
		BuiltUsing:   pb.BuiltUsing,
		Conffiles:    pb.Backup,
		Licenses:     pb.License,
		Copyrights:   pb.Copyright,
		Scripts: pkgdeb.Scripts{
			Preinst:   d.scriptlet(pb.PreInst),
			Postinst:  d.scriptlet(pb.PostInst),
			Prerm:     d.scriptlet(pb.PreRm),
			Postrm:    d.scriptlet(pb.PostRm),
			Config:    config,
			Templates: templates,
		},
		Changelog:   changelog,
		Compression: d.compression,
		ModTime:     files.SourceDateEpochFromEnv(),
	}, nil
}

// scriptlet prepends only the helper function definitions that are actually
// called within the scriptlet, so that helpers like _postinst() or
// _postinst_legacy() are available at install time without injecting
// unrelated build-time helpers (_package, _package_systemd, etc.).
func (d *Package) scriptlet(script string) string {
	if script == "" {
		return ""
	}

	return d.PrepareScriptletWithHelpers(script)
}

// readDebconf returns the content of a debconf asset named relative to the
// PKGBUILD directory, or "" when the PKGBUILD names none.
func (d *Package) readDebconf(name string) (string, error) {
	if name == "" {
		return "", nil
	}

	data, err := os.ReadFile(filepath.Join(d.PKGBUILD.Home, name)) //nolint:gosec // path named by the PKGBUILD
	if err != nil {
		return "", errors.Wrap(err, errors.ErrTypeFileSystem, "failed to read debconf file").
			WithOperation("readDebconf").
			WithContext("path", name)
	}

	return string(data), nil
}
