// Package pacmandb refreshes Arch repository sync databases.
//
// It parses /etc/pacman.conf (with Include + mirrorlist expansion),
// resolves $repo/$arch placeholders, fetches each <repo>.db file with
// multi-mirror failover, and writes the result atomically to
// /var/lib/pacman/sync/.
package pacmandb

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"

	"golang.org/x/sync/errgroup"

	"github.com/M0Rf30/yap/v2/pkg/errors"
	"github.com/M0Rf30/yap/v2/pkg/i18n"
	"github.com/M0Rf30/yap/v2/pkg/logger"
)

const pacmanSyncDir = "/var/lib/pacman/sync"

// Sync downloads <repo>.db for every enabled repo in pacman.conf.
// Returns the number of repos successfully synced and the first error.
func Sync(ctx context.Context) (succeeded int, err error) {
	cfg, err := ParseConfig("/etc/pacman.conf")
	if err != nil {
		return 0, err
	}

	if err := os.MkdirAll(pacmanSyncDir, 0o755); err != nil {
		return 0, err
	}

	arch := cfg.Architecture
	if arch == "" || arch == "auto" {
		arch = detectArch()
	}

	logger.Info(i18n.T("logger.pacmandb.info.syncing_repos"), "repos", len(cfg.Repos),
		"arch", arch)

	// Repos are independent (separate .db destinations) — sync them in
	// parallel, bounded like the other repo fetchers. Failures are
	// per-repo warnings; the first error is reported to the caller.
	var (
		mu       sync.Mutex
		firstErr error
	)

	g := new(errgroup.Group)
	g.SetLimit(4)

	for _, repo := range cfg.Repos {
		g.Go(func() error {
			if err := syncRepo(ctx, repo, arch); err != nil {
				mu.Lock()
				if firstErr == nil {
					firstErr = err
				}
				mu.Unlock()

				logger.Warn(i18n.T("logger.pacmandb.warn.repo_sync_failed"), "repo", repo.Name, "error", err)

				return nil // best-effort: don't abort sibling repos
			}

			mu.Lock()
			succeeded++
			mu.Unlock()

			return nil
		})
	}

	_ = g.Wait()

	logger.Info(i18n.T("logger.pacmandb.info.sync_complete"), "succeeded", succeeded,
		"total", len(cfg.Repos))

	return succeeded, firstErr
}

func syncRepo(ctx context.Context, repo Repo, arch string) error {
	for _, server := range repo.Servers {
		url := substituteVars(server, repo.Name, arch) + "/" + repo.Name + ".db"
		dest := filepath.Join(pacmanSyncDir, repo.Name+".db")

		logger.Debug(i18n.T("logger.pacmandb.debug.trying_mirror"), "repo", repo.Name, "url", url)

		err := downloadFile(ctx, url, dest)
		if err != nil {
			logger.Debug(i18n.T("logger.pacmandb.debug.mirror_failed"), "repo", repo.Name, "url", url, "error", err)

			continue
		}

		// Also try to fetch the .sig (optional, used for signature checking).
		sigDest := dest + ".sig"
		_ = downloadFile(ctx, url+".sig", sigDest) // best-effort

		var sizeBytes int64
		if fi, statErr := os.Stat(dest); statErr == nil {
			sizeBytes = fi.Size()
		}

		logger.Info(i18n.T("logger.pacmandb.info.repo_synced"), "repo", repo.Name, "url", url, "bytes", sizeBytes)

		return nil
	}

	return errors.New(errors.ErrTypeNetwork, "all mirrors failed for repository").
		WithOperation("syncRepo").
		WithContext("repo", repo.Name)
}

func substituteVars(server, repo, arch string) string {
	s := strings.ReplaceAll(server, "$repo", repo)
	s = strings.ReplaceAll(s, "$arch", arch)

	return strings.TrimRight(s, "/")
}

const (
	x86_64Arch  = "x86_64"
	aarch64Arch = "aarch64"
	armv7hArch  = "armv7h"
	i686Arch    = "i686"
)

// detectArch maps runtime.GOARCH to Arch Linux pkg-arch name.
func detectArch() string {
	switch runtime.GOARCH {
	case "amd64":
		return x86_64Arch
	case "arm64":
		return aarch64Arch
	case "arm":
		return armv7hArch
	case "386":
		return i686Arch
	default:
		return runtime.GOARCH
	}
}
