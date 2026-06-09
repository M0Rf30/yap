// filter_installed.go: host package-database probes used to skip already-
// installed makedepends. Backed by the per-format readers (aptcache, apk
// installed DB, pacman local DB, rpmdb SQLite/BDB); subprocess fallbacks
// remain only for legacy RPM hosts.

package pkgbuild

import (
	"bufio"
	"bytes"
	"context"
	stderrors "errors"
	"os"
	"runtime"
	"strings"

	"github.com/M0Rf30/yap/v2/pkg/aptcache"
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
	cache := aptcache.Load()
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

// filterInstalledAPK reads /lib/apk/db/installed to check which APK packages are installed.
func filterInstalledAPK(packages []string) []string {
	const apkInstalledDB = "/lib/apk/db/installed"

	installed := apkInstalledSet(apkInstalledDB)
	if installed == nil {
		// DB unreadable — assume nothing is installed so we don't skip deps.
		return packages
	}

	missing := make([]string, 0, len(packages))

	for _, pkg := range packages {
		name := stripVersionConstraint(pkg)
		if !installed[name] {
			missing = append(missing, pkg)
		}
	}

	return missing
}

// apkInstalledSet parses the APK installed database and returns a set of
// installed package names. Returns nil on read error.
func apkInstalledSet(path string) map[string]bool {
	f, err := os.Open(path) //nolint:gosec
	if err != nil {
		return nil
	}

	defer func() { _ = f.Close() }()

	installed := make(map[string]bool)
	scanner := bufio.NewScanner(f)

	var currentPkg string

	for scanner.Scan() {
		line := scanner.Text()

		if line == "" {
			if currentPkg != "" {
				installed[currentPkg] = true
				currentPkg = ""
			}

			continue
		}

		if pkg, ok := strings.CutPrefix(line, "P:"); ok {
			currentPkg = pkg
		}
	}

	if currentPkg != "" {
		installed[currentPkg] = true
	}

	return installed
}

// filterInstalledPacman checks /var/lib/pacman/local/ for installed packages.
// Each installed package has a subdirectory named "<pkgname>-<version>-<pkgrel>/".
func filterInstalledPacman(packages []string) []string {
	const pacmanLocalDB = "/var/lib/pacman/local"

	entries, err := os.ReadDir(pacmanLocalDB)
	if err != nil {
		return packages
	}

	installed := make(map[string]bool, len(entries))

	for _, e := range entries {
		if !e.IsDir() || e.Name() == "ALPM_DB_VERSION" {
			continue
		}

		name := pacmanDirToName(e.Name())
		if name != "" {
			installed[name] = true
		}
	}

	missing := make([]string, 0, len(packages))

	for _, pkg := range packages {
		name := stripVersionConstraint(pkg)
		if !installed[name] {
			missing = append(missing, pkg)
		}
	}

	return missing
}

// pacmanDirToName extracts the package name from a pacman local DB directory
// name of the form "<name>-<version>-<pkgrel>". pkgver cannot contain hyphens
// (per Arch wiki), so stripping the last two hyphen-delimited segments is safe.
func pacmanDirToName(dir string) string {
	idx := strings.LastIndex(dir, "-")
	if idx < 0 {
		return ""
	}

	withoutPkgrel := dir[:idx]

	idx = strings.LastIndex(withoutPkgrel, "-")
	if idx < 0 {
		return ""
	}

	return withoutPkgrel[:idx]
}

// filterInstalledRPM queries the SQLite rpmdb directly on modern hosts
// (Fedora 33+, RHEL 9+, Rocky 9+, Alma 9+, openSUSE 15.5+). On legacy
// BerkeleyDB hosts it reads the BDB Packages database natively; a batched
// `rpm -q` subprocess remains the last-resort fallback.
func filterInstalledRPM(packages []string) []string {
	db, err := rpmdb.Open()
	if err == nil {
		return db.FilterInstalled(context.Background(), packages)
	}

	if stderrors.Is(err, rpmdb.ErrLegacyDB) {
		logger.Debug(i18n.T("logger.pkgbuild.debug.legacy_bdb_host_falling"))

		if legacy, err := rpmdb.OpenLegacy(); err == nil {
			if missing, err := legacy.FilterInstalled(context.Background(), packages); err == nil {
				return missing
			}
		}
	} else {
		logger.Warn(i18n.T("logger.pkgbuild.warn.open_failed_falling_back"), "error", err)
	}

	return filterInstalledRPMSubprocess(packages)
}

// filterInstalledRPMSubprocess is the legacy BerkeleyDB fallback. It runs a
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
