// SPDX-FileCopyrightText: 2021-2026 Gianluca Boiano
// SPDX-License-Identifier: GPL-3.0-only

// Package pacmandb refreshes the host's Arch repository sync databases.
//
// The pacman.conf and mirrorlist parsing, the multi-mirror failover and the
// atomic <repo>.db / <repo>.db.sig writes are done by
// github.com/M0Rf30/pkgformat/pacman; this package only supplies the host
// paths (/etc/pacman.conf, /var/lib/pacman/sync) and yap's logging.
package pacmandb

import (
	"context"
	"os"

	"github.com/M0Rf30/pkgformat/httpclient"
	"github.com/M0Rf30/pkgformat/pacman"

	"github.com/M0Rf30/yap/v2/pkg/errors"
	"github.com/M0Rf30/yap/v2/pkg/i18n"
	"github.com/M0Rf30/yap/v2/pkg/logger"
)

const (
	pacmanConfPath = "/etc/pacman.conf"
	pacmanSyncDir  = "/var/lib/pacman/sync"
)

// Sync downloads <repo>.db for every enabled repo in pacman.conf.
// Returns the number of repos successfully synced and the failures joined.
func Sync(ctx context.Context) (succeeded int, err error) {
	return syncTo(ctx, pacmanConfPath, pacmanSyncDir, nil)
}

// syncTo is Sync with explicit paths and HTTP client (nil selects the default).
func syncTo(
	ctx context.Context, confPath, syncDir string, httpc *httpclient.Client,
) (int, error) {
	cfg, err := pacman.ParseConfigFile(confPath)
	if err != nil {
		return 0, errors.Wrap(err, errors.ErrTypeConfiguration, "failed to parse pacman.conf").
			WithOperation("Sync").
			WithContext("path", confPath)
	}

	if err := os.MkdirAll(syncDir, 0o755); err != nil { //nolint:gosec // pacman's sync dir is world-readable
		return 0, errors.Wrap(err, errors.ErrTypeFileSystem, "failed to create sync directory").
			WithOperation("Sync").
			WithContext("dir", syncDir)
	}

	arch := cfg.Architecture
	if arch == "" || arch == "auto" {
		arch = "" // the client maps the running GOARCH
	}

	client, err := pacman.NewClient(pacman.ClientOptions{HTTP: httpc, Arch: arch, Logger: logger.Slog()})
	if err != nil {
		return 0, errors.Wrap(err, errors.ErrTypeConfiguration, "failed to create pacman client").
			WithOperation("Sync")
	}

	logger.Info(i18n.T("logger.pacmandb.info.syncing_repos"), "repos", len(cfg.Repos), "arch", arch)

	succeeded, err := client.Sync(ctx, cfg.Repos, syncDir)
	if err != nil {
		// Per-repo failures are warnings; the others were still synced.
		logger.Warn(i18n.T("logger.pacmandb.warn.repo_sync_failed"), "error", err)

		err = errors.Wrap(err, errors.ErrTypeNetwork, "failed to sync repositories").
			WithOperation("Sync")
	}

	logger.Info(i18n.T("logger.pacmandb.info.sync_complete"), "succeeded", succeeded, "total", len(cfg.Repos))

	return succeeded, err
}
