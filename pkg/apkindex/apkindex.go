// SPDX-FileCopyrightText: 2021-2026 Gianluca Boiano
// SPDX-License-Identifier: GPL-3.0-only

// Package apkindex installs .apk packages onto the running Alpine system. It
// replaces "apk update" and "apk add" subprocess calls.
//
// The repository client, the APKINDEX parser and the dependency resolver are
// github.com/M0Rf30/pkgformat/apk; this package adds what is specific to yap:
// the host configuration (/etc/apk/repositories, /etc/apk/keys), the trust
// decision, the extraction onto "/" and the installed database.
//
// Typical usage:
//
//	repos, err := apkindex.Update(ctx)
//	if err != nil {
//		return err
//	}
//	if err := repos.InstallPackages(ctx, []string{"gcc", "musl-dev"}); err != nil {
//		return err
//	}
package apkindex

import (
	"context"
	"os"
	"sync/atomic"

	pkgapk "github.com/M0Rf30/pkgformat/apk"
	"github.com/M0Rf30/pkgformat/httpclient"

	"github.com/M0Rf30/yap/v2/pkg/errors"
	"github.com/M0Rf30/yap/v2/pkg/i18n"
	"github.com/M0Rf30/yap/v2/pkg/logger"
	"github.com/M0Rf30/yap/v2/pkg/platform"
)

const (
	apkCacheDir = "/var/cache/apk"
	apkKeysDir  = "/etc/apk/keys"
)

// globalRepos caches the most recent Repos from Update so Install can reuse it
// without re-fetching.
var globalRepos atomic.Pointer[Repos]

// Repos is the loaded state of the configured repositories: the merged index
// and the client that downloads the packages the index lists.
type Repos struct {
	Index  *pkgapk.Index
	client *pkgapk.Client
}

// UpdateOptions configures UpdateWithOptions.
type UpdateOptions struct {
	// Repos are the repositories, in order of preference.
	Repos []pkgapk.Repo
	// Arch is the architecture directory of every repository.
	Arch string
	// KeysDir is the directory of trusted public keys. An index is only
	// accepted with a valid signature by one of them.
	KeysDir string
	// CacheDir receives a copy of each verified index.
	CacheDir string
	// AllowUnverified accepts indexes without checking their signature when
	// KeysDir holds no keys at all. Downloaded packages are still checked
	// against the index entries.
	AllowUnverified bool
	// HTTP fetches indexes and packages; nil selects the library default.
	HTTP *httpclient.Client
}

// Update fetches APKINDEX.tar.gz from every repo in /etc/apk/repositories,
// verifies it against the keys in /etc/apk/keys, and returns the loaded
// repositories. Replaces "apk update". The result is cached globally so
// Install can reuse it without re-fetching.
//
// Without any trusted key the indexes are accepted unverified only on a
// privileged host (uid 0, the expected case inside a yap build container) and
// refused on a developer workstation.
func Update(ctx context.Context) (*Repos, error) {
	repos, err := LoadRepos()
	if err != nil {
		return nil, errors.Wrap(err, errors.ErrTypeConfiguration, "load repos").
			WithOperation("Update")
	}

	arch := DetectArch()
	if arch == "" {
		return nil, errors.New(errors.ErrTypeConfiguration, "could not detect APK architecture").
			WithOperation("Update")
	}

	return UpdateWithOptions(ctx, &UpdateOptions{
		Repos:           repos,
		Arch:            arch,
		KeysDir:         apkKeysDir,
		CacheDir:        apkCacheDir,
		AllowUnverified: platform.IsPrivilegedHost(),
	})
}

// UpdateWithOptions is Update with explicit repositories, trust and HTTP client.
func UpdateWithOptions(ctx context.Context, opts *UpdateOptions) (*Repos, error) {
	copts := pkgapk.ClientOptions{
		Repos:    opts.Repos,
		Arch:     opts.Arch,
		CacheDir: opts.CacheDir,
		HTTP:     opts.HTTP,
		Logger:   logger.Slog(),
	}

	trusted, err := hasTrustedKeys(opts.KeysDir)
	if err != nil {
		return nil, err
	}

	switch {
	case trusted:
		copts.KeysDir = opts.KeysDir
		// apk-tools verifies SHA-1 signatures and Alpine still signs its
		// indexes with them, so both types are accepted.
		copts.Algorithms = []pkgapk.SigAlgorithm{pkgapk.SigRSASHA1, pkgapk.SigRSASHA256}
	case opts.AllowUnverified:
		logger.Warn(i18n.T("logger.apkindex.warn.installing_without_verification"), "keys_dir", opts.KeysDir)

		copts.InsecureSkipVerify = true
	default:
		return nil, errors.New(errors.ErrTypeValidation,
			"no trusted APK keys; install keys or run on a privileged host to accept unverified indexes").
			WithOperation("UpdateWithOptions").
			WithContext("keys_dir", opts.KeysDir)
	}

	client, err := pkgapk.NewClient(copts)
	if err != nil {
		return nil, errors.Wrap(err, errors.ErrTypeConfiguration, "failed to create APK client").
			WithOperation("UpdateWithOptions")
	}

	logger.Info(i18n.T("logger.apkindex.info.updating_indexes"), "repos", len(opts.Repos), "arch", opts.Arch)

	updated, err := client.Update(ctx)
	if err != nil {
		return nil, errors.Wrap(err, errors.ErrTypeNetwork, "no APK repository index could be loaded").
			WithOperation("UpdateWithOptions").
			WithContext("repos", len(opts.Repos))
	}

	for _, failure := range updated.Failures {
		logger.Warn(i18n.T("logger.apkindex.warn.fetch_failed"), "url", failure.Repo.URL, "error", failure.Err)
	}

	pkgs, caps := updated.Stats()
	logger.Info(i18n.T("logger.apkindex.info.indexes_loaded"),
		"repos_succeeded", len(opts.Repos)-len(updated.Failures),
		"repos_total", len(opts.Repos),
		"packages", pkgs,
		"capabilities", caps)

	loaded := &Repos{Index: updated.Index, client: client}
	globalRepos.Store(loaded)

	return loaded, nil
}

// hasTrustedKeys reports whether dir holds at least one trusted public key. A
// missing directory holds none; a damaged key is an error, never silently
// dropped from the trust set.
func hasTrustedKeys(dir string) (bool, error) {
	ring, err := pkgapk.LoadKeyring(dir)
	if os.IsNotExist(err) {
		return false, nil
	}

	if err != nil {
		return false, errors.Wrap(err, errors.ErrTypeConfiguration, "failed to load APK keys").
			WithOperation("hasTrustedKeys").
			WithContext("keys_dir", dir)
	}

	return ring.Len() > 0, nil
}

// Load returns the cached Repos from the most recent Update call, or nil if
// Update has not been called yet.
func Load() *Repos {
	return globalRepos.Load()
}

// Install is a convenience function that calls Update to fetch the indexes,
// then installs the requested packages. This is the main entry point for
// replacing "apk update && apk add <pkgs>" subprocess calls.
func Install(ctx context.Context, names []string) error {
	repos := Load()
	if repos == nil {
		var err error

		repos, err = Update(ctx)
		if err != nil {
			return err
		}
	}

	return repos.InstallPackages(ctx, names)
}
