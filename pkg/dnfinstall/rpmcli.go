// SPDX-FileCopyrightText: 2021-2026 Gianluca Boiano
// SPDX-License-Identifier: GPL-3.0-only

package dnfinstall

import (
	"context"
	"os"
	"os/exec"

	"github.com/M0Rf30/yap/v2/pkg/errors"
	"github.com/M0Rf30/yap/v2/pkg/i18n"
	"github.com/M0Rf30/yap/v2/pkg/logger"
)

// InstallWithRPM resolves the transitive closure of names against the cached
// repository metadata, downloads the verified .rpm files and installs them
// with "rpm --install --nodeps" (the dependency closure is already resolved).
// It replaces "dnf install" on hosts where the rpm tool must own the
// transaction; Install is the in-process variant.
func InstallWithRPM(ctx context.Context, names []string) error {
	client, index, err := hostIndex(ctx)
	if err != nil {
		return errors.Wrap(err, errors.ErrTypeBuild, "failed to load repository metadata").
			WithOperation("InstallWithRPM")
	}

	resolution, err := index.Resolve(ctx, names)
	if err != nil {
		return errors.Wrap(err, errors.ErrTypeBuild, "failed to resolve dependencies").
			WithOperation("InstallWithRPM")
	}

	// Non-fatal: log and continue — rpm will surface hard errors.
	for _, name := range resolution.Unresolved {
		logger.Warn(i18n.T("logger.dnfinstall.warn.unresolvable_dependency"), "package", name)
	}

	if len(resolution.Packages) == 0 {
		return nil
	}

	tmpDir, err := os.MkdirTemp("", "dnfinstall-rpm-*")
	if err != nil {
		return errors.Wrap(err, errors.ErrTypeFileSystem, "failed to create temporary directory").
			WithOperation("InstallWithRPM")
	}

	defer func() { _ = os.RemoveAll(tmpDir) }()

	var totalBytes int64
	for _, p := range resolution.Packages {
		totalBytes += p.Size
	}

	logger.Info(i18n.T("logger.dnfinstall.info.downloading_packages"), "count", len(resolution.Packages),
		"total_bytes", totalBytes)

	paths, err := client.DownloadAll(ctx, resolution.Packages, tmpDir)
	if err != nil {
		return errors.Wrap(err, errors.ErrTypeNetwork, "failed to download packages").
			WithOperation("InstallWithRPM")
	}

	return rpmInstall(ctx, paths)
}

// rpmInstall installs the given .rpm files via "rpm --install --nodeps".
func rpmInstall(ctx context.Context, paths []string) error {
	if len(paths) == 0 {
		return nil
	}

	cmd := exec.CommandContext(ctx, "rpm", append([]string{"--install", "--nodeps"}, paths...)...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	logger.Info(i18n.T("logger.dnfinstall.info.installing_rpm_packages"), "count", len(paths))

	if err := cmd.Run(); err != nil {
		return errors.Wrap(err, errors.ErrTypeBuild, "failed to install RPM packages").
			WithOperation("rpmInstall")
	}

	return nil
}
