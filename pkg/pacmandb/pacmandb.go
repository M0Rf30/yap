// SPDX-FileCopyrightText: 2021-2026 Gianluca Boiano
// SPDX-License-Identifier: GPL-3.0-only

// Package pacmandb refreshes the host's Arch repository sync databases.
//
// The pacman.conf and mirrorlist parsing, the multi-mirror failover and the
// atomic <repo>.db / <repo>.db.sig writes are done by
// github.com/M0Rf30/pkgformat/pacman; this package only supplies the host
// paths (/etc/pacman.conf, /var/lib/pacman/sync, the archlinux-keyring files)
// and yap's logging. Each repository's SigLevel and Usage come from the parsed
// pacman.conf and are enforced by the client.
package pacmandb

import (
	"context"
	"os"
	"runtime"

	"github.com/M0Rf30/pkgformat/gpg"
	"github.com/M0Rf30/pkgformat/httpclient"
	"github.com/M0Rf30/pkgformat/pacman"

	"github.com/M0Rf30/yap/v2/pkg/errors"
	"github.com/M0Rf30/yap/v2/pkg/i18n"
	"github.com/M0Rf30/yap/v2/pkg/logger"
)

const (
	pacmanConfPath = "/etc/pacman.conf"
	pacmanSyncDir  = "/var/lib/pacman/sync"
	// pacmanKeyringDir holds the public keys of the distribution keyring
	// packages (archlinux-keyring), the ones "pacman-key --populate" trusts.
	pacmanKeyringDir = "/usr/share/pacman/keyrings"
)

// Sync downloads <repo>.db for every enabled repo in pacman.conf.
// Returns the number of repos successfully synced and the failures joined.
func Sync(ctx context.Context) (succeeded int, err error) {
	return syncTo(ctx, pacmanConfPath, pacmanSyncDir, pacmanKeyringDir, nil)
}

// syncTo is Sync with explicit paths and HTTP client (nil selects the default).
func syncTo(
	ctx context.Context, confPath, syncDir, keyringDir string, httpc *httpclient.Client,
) (int, error) {
	cfg, err := pacman.ParseConfigFile(confPath, pacman.ParseOptions{})
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

	// The client resolves $arch itself when the option is absent.
	arch := cfg.Arch(runtime.GOARCH)
	repos := cfg.ReposFor(pacman.UsageSync)
	keyring := loadKeyring(keyringDir)

	if keyring.Len() == 0 {
		repos = withoutDBSignatures(repos)
	}

	client, err := pacman.NewClient(pacman.ClientOptions{
		HTTP: httpc, Arch: arch, Keyring: keyring, Logger: logger.Slog(),
	})
	if err != nil {
		return 0, errors.Wrap(err, errors.ErrTypeConfiguration, "failed to create pacman client").
			WithOperation("Sync")
	}

	logger.Info(i18n.T("logger.pacmandb.info.syncing_repos"), "repos", len(repos), "arch", arch)

	succeeded, err := client.Sync(ctx, repos, syncDir)
	if err != nil {
		// Per-repo failures are warnings; the others were still synced.
		logger.Warn(i18n.T("logger.pacmandb.warn.repo_sync_failed"), "error", err)

		err = errors.Wrap(err, errors.ErrTypeNetwork, "failed to sync repositories").
			WithOperation("Sync")
	}

	logger.Info(i18n.T("logger.pacmandb.info.sync_complete"), "succeeded", succeeded, "total", len(repos))

	return succeeded, err
}

// loadKeyring reads the distribution keyrings in dir. A missing or empty
// directory gives an empty keyring.
func loadKeyring(dir string) *gpg.Keyring {
	keyring, err := gpg.LoadKeyringDir(dir)
	if err != nil {
		logger.Debug(i18n.T("logger.pacmandb.debug.keyring_unreadable"), "dir", dir, "error", err)

		return nil
	}

	return keyring
}

// withoutDBSignatures returns repos with database signature checking turned
// off. Without any trusted key a SigLevel that asks for verification can only
// fail; sync keeps working as it did before signatures were checked, and says so.
func withoutDBSignatures(repos []pacman.Repo) []pacman.Repo {
	logger.Warn(i18n.T("logger.pacmandb.warn.syncing_without_verification"))

	out := make([]pacman.Repo, len(repos))
	for i, repo := range repos {
		repo.SigLevel.Database = pacman.SigPolicy{Check: pacman.SigNever}
		out[i] = repo
	}

	return out
}
