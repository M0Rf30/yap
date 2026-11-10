// SPDX-FileCopyrightText: 2021-2026 Gianluca Boiano
// SPDX-License-Identifier: GPL-3.0-only

// Package rpmdb answers installed-package queries from the host's RPM
// database and, optionally, registers packages in the SQLite one.
//
// Reading is delegated to github.com/M0Rf30/pkgformat/rpm/rpmdb, which handles
// every on-disk format rpm has used: SQLite (Fedora 33+, RHEL 9+, openSUSE
// and Arch Linux), ndb (openSUSE/SLE Packages.db) and BerkeleyDB (RHEL 8 and
// earlier). The database is found under the host root and is only ever read.
// When the host has none, the functions return ErrNoDatabase and the caller
// falls back to the rpm command.
//
// The Writer (OpenWriter) is the opt-in counterpart that appends packages to a
// SQLite rpmdb.
package rpmdb

import (
	"context"
	"errors"

	pkgrpmdb "github.com/M0Rf30/pkgformat/rpm/rpmdb"

	yapErrors "github.com/M0Rf30/yap/v2/pkg/errors"
	"github.com/M0Rf30/yap/v2/pkg/logger"
)

// ErrNoDatabase is returned when the host has no RPM database under its root.
// The caller should fall back to the rpm command.
var ErrNoDatabase = errors.New("rpmdb: no RPM database found")

// hostRoot is the directory the host's RPM database is looked up under.
var hostRoot = "/"

// withDB runs fn on the host database, opened read-only for the call.
func withDB[T any](fn func(*pkgrpmdb.DB) (T, error)) (T, error) {
	var zero T

	path, _, err := pkgrpmdb.Find(hostRoot)
	if errors.Is(err, pkgrpmdb.ErrNotFound) {
		return zero, ErrNoDatabase
	}

	if err != nil {
		return zero, yapErrors.Wrap(err, yapErrors.ErrTypeFileSystem, "failed to find rpm database").
			WithContext("root", hostRoot).
			WithOperation("withDB")
	}

	db, err := pkgrpmdb.Open(path, pkgrpmdb.Options{Logger: logger.Slog()})
	if err != nil {
		return zero, yapErrors.Wrap(err, yapErrors.ErrTypeFileSystem, "failed to open rpm database").
			WithContext("path", path).
			WithOperation("withDB")
	}

	defer func() { _ = db.Close() }()

	return fn(db)
}

// ListInstalled returns the sorted names of the installed packages.
func ListInstalled(ctx context.Context) ([]string, error) {
	return withDB(func(db *pkgrpmdb.DB) ([]string, error) { return db.InstalledNames(ctx) })
}

// ListInstalledProvides returns the sorted capabilities (Provides) satisfied
// by the installed packages, including each package's own name and the virtual
// names such as "coreutils" provided by "coreutils-single" on minimal images.
func ListInstalledProvides(ctx context.Context) ([]string, error) {
	return withDB(func(db *pkgrpmdb.DB) ([]string, error) { return db.InstalledProvides(ctx) })
}

// FilterInstalled returns the subset of names that are NOT installed, in the
// order given. A package counts as installed when its name is registered; the
// database is read once however many names are asked about.
func FilterInstalled(ctx context.Context, names []string) ([]string, error) {
	installed, err := ListInstalled(ctx)
	if err != nil {
		return nil, err
	}

	have := make(map[string]struct{}, len(installed))
	for _, name := range installed {
		have[name] = struct{}{}
	}

	missing := make([]string, 0, len(names))

	for _, name := range names {
		if _, ok := have[name]; !ok {
			missing = append(missing, name)
		}
	}

	return missing, nil
}
