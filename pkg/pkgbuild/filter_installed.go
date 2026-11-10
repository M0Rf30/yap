// SPDX-FileCopyrightText: 2021-2026 Gianluca Boiano
// SPDX-License-Identifier: GPL-3.0-only

// filter_installed.go: host package-database probes used to skip already-
// installed makedepends. Backed by the per-format readers (aptcache, apk
// installed DB, pacman local DB, rpmdb SQLite/ndb/BDB); subprocess fallbacks
// remain only for legacy RPM hosts.

package pkgbuild

import (
	"bytes"
	"context"
	stderrors "errors"
	"runtime"
	"strings"

	pkgapk "github.com/M0Rf30/pkgformat/apk"
	"github.com/M0Rf30/pkgformat/pacman"

	"github.com/M0Rf30/yap/v2/pkg/apthost"
	"github.com/M0Rf30/yap/v2/pkg/constants"
	"github.com/M0Rf30/yap/v2/pkg/i18n"
	"github.com/M0Rf30/yap/v2/pkg/logger"
	"github.com/M0Rf30/yap/v2/pkg/rpmdb"
	"github.com/M0Rf30/yap/v2/pkg/shell"
)

// filterInstalledPackages checks which packages are not installed and returns only those.
// Backed by direct dpkg/apk/pacman/rpmdb readers; falls back to the
// distro's `rpm -q` subprocess only on legacy BerkeleyDB RPM hosts.
func filterInstalledPackages(packageManager string, packages []string) []string {
	if len(packages) == 0 {
		return nil
	}

	switch packageManager {
	case aptGetPM, aptPM, dpkgPM:
		return filterInstalledDEB(packages)
	case apkPM:
		return filterInstalledAPK(packages)
	case "pacman":
		return filterInstalledPacman(packages)
	case "rpm", yumPM, dnfPM, "zypper":
		return filterInstalledRPM(packages)
	default:
		return packages
	}
}

// filterInstalledDEB uses the in-process aptcache (reads /var/lib/dpkg/status)
// to check which DEB packages are already installed — no subprocess needed.
//
// Handles :arch qualifiers correctly: a foreign-arch qualifier (e.g. "libssl-dev:arm64"
// on an amd64 host) cannot be answered from a single Installed bit, so those are
// reported as missing and let apt-get install decide (it is idempotent).
func filterInstalledDEB(packages []string) []string {
	cache := apthost.Cache()
	hostDebArch := constants.GetArchMapping().TranslateArch(constants.FormatDEB, runtime.GOARCH)

	missing := make([]string, 0, len(packages))

	for _, pkg := range packages {
		// Strip deb version constraint syntax "name (>= 1.0)" first.
		name, _, _ := strings.Cut(pkg, " (")
		// Separate arch qualifier if present.
		bareName, qualifier, hasQualifier := strings.Cut(name, ":")
		bareName = strings.TrimSpace(bareName)
		qualifier = strings.TrimSpace(qualifier)

		// Foreign-arch qualifier cannot be answered reliably; treat as missing.
		if hasQualifier && qualifier != "" && qualifier != hostDebArch {
			missing = append(missing, pkg)
			continue
		}

		info, ok := cache.Lookup(bareName)
		if !ok || !info.Installed {
			missing = append(missing, pkg)
		}
	}

	return missing
}

// hostRoot is the directory the host's apk and pacman databases are looked up
// under.
const hostRoot = "/"

// filterInstalledAPK reads the apk installed database (/lib/apk/db/installed)
// to check which APK packages are installed.
func filterInstalledAPK(packages []string) []string {
	installed, err := apkInstalledSet(hostRoot)
	if err != nil {
		// DB unreadable — assume nothing is installed so we don't skip deps.
		logger.Debug(i18n.T("logger.pkgbuild.debug.installed_db_unreadable"), "format", "apk", "error", err)

		return packages
	}

	return filterMissing(packages, installed)
}

// installedNamer is what the apk and pacman database readers have in common.
type installedNamer interface {
	InstalledNames(ctx context.Context) ([]string, error)
	Close() error
}

// installedSet finds the database under root, opens it and returns the set of
// the names of its packages.
func installedSet(
	root string,
	find func(root string) (string, error),
	open func(path string) (installedNamer, error),
) (map[string]bool, error) {
	path, err := find(root)
	if err != nil {
		return nil, err
	}

	db, err := open(path)
	if err != nil {
		return nil, err
	}

	defer func() { _ = db.Close() }()

	names, err := db.InstalledNames(context.Background())
	if err != nil {
		return nil, err
	}

	return nameSet(names), nil
}

// apkInstalledSet returns the set of installed package names of the apk
// database under root.
func apkInstalledSet(root string) (map[string]bool, error) {
	return installedSet(root, pkgapk.FindInstalled, func(path string) (installedNamer, error) {
		return pkgapk.OpenInstalled(path, pkgapk.InstalledOptions{Logger: logger.Slog(), SkipFiles: true})
	})
}

// filterInstalledPacman reads the pacman local database
// (/var/lib/pacman/local) to check which packages are installed. A package is
// named by the %NAME% of its desc, not by its directory.
func filterInstalledPacman(packages []string) []string {
	installed, err := pacmanInstalledSet(hostRoot)
	if err != nil {
		logger.Debug(i18n.T("logger.pkgbuild.debug.installed_db_unreadable"), "format", "pacman", "error", err)

		return packages
	}

	return filterMissing(packages, installed)
}

// pacmanInstalledSet returns the set of installed package names of the pacman
// local database under root.
func pacmanInstalledSet(root string) (map[string]bool, error) {
	return installedSet(root, pacman.FindLocalDB, func(path string) (installedNamer, error) {
		return pacman.OpenLocalDB(path, pacman.LocalDBOptions{Logger: logger.Slog(), SkipFiles: true})
	})
}

func nameSet(names []string) map[string]bool {
	set := make(map[string]bool, len(names))
	for _, n := range names {
		set[n] = true
	}

	return set
}

// filterMissing returns the packages whose name, without version constraint,
// is not in installed.
func filterMissing(packages []string, installed map[string]bool) []string {
	missing := make([]string, 0, len(packages))

	for _, pkg := range packages {
		if !installed[stripVersionConstraint(pkg)] {
			missing = append(missing, pkg)
		}
	}

	return missing
}

// filterInstalledRPM reads the host's RPM database directly, whichever format
// it is in (SQLite, ndb, BerkeleyDB). A batched `rpm -q` subprocess is the
// fallback for hosts with no database file, or one that cannot be read.
func filterInstalledRPM(packages []string) []string {
	missing, err := rpmdb.FilterInstalled(context.Background(), packages)
	if err == nil {
		return missing
	}

	if stderrors.Is(err, rpmdb.ErrNoDatabase) {
		logger.Debug(i18n.T("logger.pkgbuild.debug.no_rpmdb_host_falling"))
	} else {
		logger.Warn(i18n.T("logger.pkgbuild.warn.open_failed_falling_back"), "error", err)
	}

	return filterInstalledRPMSubprocess(packages)
}

// filterInstalledRPMSubprocess is the subprocess fallback. It runs a
// single batched `rpm -q pkg1 pkg2 …` (one line of output per argument, in
// order) instead of one subprocess per package.
func filterInstalledRPMSubprocess(packages []string) []string {
	var out bytes.Buffer

	args := append([]string{"-q"}, packages...)

	err := shell.ExecCapture(context.Background(), &out, "", "rpm", args...)
	if err == nil {
		return nil // every queried package is installed
	}

	// rpm exits non-zero when ANY queried package is missing; map the
	// output lines back to the inputs positionally.
	lines := strings.Split(strings.TrimRight(out.String(), "\n"), "\n")
	if len(lines) != len(packages) {
		// Unexpected output shape (rpm missing, usage error, stderr noise)
		// — be conservative and report everything as missing, matching the
		// old per-package loop when every `rpm -q` invocation failed.
		return packages
	}

	missing := make([]string, 0, len(packages))

	for i, line := range lines {
		if strings.Contains(line, "is not installed") {
			missing = append(missing, packages[i])
		}
	}

	return missing
}
