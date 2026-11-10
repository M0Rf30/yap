// SPDX-FileCopyrightText: 2021-2026 Gianluca Boiano
// SPDX-License-Identifier: GPL-3.0-only

// Package apk provides functionality for building Alpine Linux APK packages.
package apk

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"time"

	pkgapk "github.com/M0Rf30/pkgformat/apk"

	"github.com/M0Rf30/yap/v2/pkg/builders/common"
	"github.com/M0Rf30/yap/v2/pkg/constants"
	"github.com/M0Rf30/yap/v2/pkg/errors"
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

// BuildPackage creates an APK package without external dependencies: the
// staged package tree and the PKGBUILD metadata are written as an abuild-style
// .apk (control segment with .PKGINFO and install scripts, data segment with
// per-file SHA-1 records). Returns the path to the created APK file.
func (a *Apk) BuildPackage(ctx context.Context, artifactsPath string, targetArch string) (string, error) {
	a.SetTargetArchitecture(targetArch)

	pkgFilePath := filepath.Join(artifactsPath, a.BuildPackageName(".apk"))

	err := a.writePackage(ctx, pkgFilePath)
	if err != nil {
		return "", err
	}

	a.LogPackageCreated(pkgFilePath)

	return pkgFilePath, nil
}

// PrepareFakeroot sets up the APK package metadata: it applies the PKGBUILD
// options to the staged tree and records the installed size, build date and
// origin that BuildPackage writes into .PKGINFO.
func (a *Apk) PrepareFakeroot(ctx context.Context, artifactsPath string, targetArch string) error {
	// Resolve the cross strip env before ArchComputed is overwritten:
	// BuildCrossStripEnvSlice is a no-op when targetArch == ArchComputed.
	stripEnv := a.CrossStripEnvMap(targetArch)
	a.SetTargetArchitecture(targetArch)

	// Apply PKGBUILD options (strip/docs/libtool/purge/zipman/emptydirs) before
	// measuring the payload so that the installed size reflects the final tree.
	err := a.ApplyOptionsWithEnv(stripEnv)
	if err != nil {
		return err
	}

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

	// Note: APK format does not have a native changelog convention.
	// Changelog support is skipped for APK packages.

	return nil
}

// spec maps the PKGBUILD onto the library's package description: the .PKGINFO
// fields, the install scripts and the staged files.
func (a *Apk) spec() (*pkgapk.Spec, error) {
	pb := a.PKGBUILD

	if pb.Epoch != "" {
		// Alpine versions have no epoch; the library refuses one.
		logger.Warn(i18n.T("logger.apk.warn.epoch_ignored"),
			"package", pb.PkgName, "epoch", pb.Epoch)
	}

	staged, err := pkgapk.FilesFromDir(pb.PackageDir)
	if err != nil {
		return nil, errors.Wrap(err, errors.ErrTypeFileSystem, "failed to list package files").
			WithOperation("spec").
			WithContext("package_dir", pb.PackageDir)
	}

	return &pkgapk.Spec{
		Info: pkgapk.Info{
			Generator:     "yap " + pb.YAPVersion,
			Name:          pb.PkgName,
			Version:       pb.PkgVer + "-r" + pb.PkgRel,
			Arch:          pb.ArchComputed,
			Description:   pb.PkgDesc,
			URL:           pb.URL,
			License:       strings.Join(pb.License, " "),
			Origin:        pb.Origin,
			Packager:      pb.Maintainer,
			Commit:        pb.Commit,
			BuildDate:     time.Unix(pb.BuildDate, 0),
			InstalledSize: pb.InstalledSize,
			Depends:       pb.Depends,
			Provides:      pb.Provides,
			Replaces:      pb.Replaces,
		},
		Scripts: a.scripts(),
		Files:   staged,
	}, nil
}

// scripts renders one shell script per lifecycle hook. The install hooks double
// as upgrade hooks, matching the other builders.
func (a *Apk) scripts() pkgapk.Scripts {
	script := func(body string) string {
		if body == "" {
			return ""
		}

		content := "#!/bin/sh\n" + a.PrepareScriptletWithHelpers(body)
		if !strings.HasSuffix(content, "\n") {
			content += "\n"
		}

		return content
	}

	pre := script(a.PKGBUILD.PreInst)
	post := script(a.PKGBUILD.PostInst)

	return pkgapk.Scripts{
		PreInstall:    pre,
		PreUpgrade:    pre,
		PostInstall:   post,
		PostUpgrade:   post,
		PreDeinstall:  script(a.PKGBUILD.PreRm),
		PostDeinstall: script(a.PKGBUILD.PostRm),
	}
}

// writePackage writes the .apk to outputFile, removing a partial file on error.
func (a *Apk) writePackage(ctx context.Context, outputFile string) error {
	err := ctx.Err()
	if err != nil {
		return err
	}

	spec, err := a.spec()
	if err != nil {
		return err
	}

	out, err := os.Create(filepath.Clean(outputFile))
	if err != nil {
		return errors.Wrap(err, errors.ErrTypeFileSystem, "failed to create package file").
			WithOperation("writePackage").
			WithContext("output_file", outputFile)
	}

	_, err = pkgapk.WritePackage(out, spec, pkgapk.WriteOptions{})
	if closeErr := out.Close(); err == nil && closeErr != nil {
		err = closeErr
	}

	if err != nil {
		_ = os.Remove(outputFile)

		return errors.Wrap(err, errors.ErrTypePackaging, "failed to write apk package").
			WithOperation("writePackage").
			WithContext("output_file", outputFile)
	}

	return nil
}
