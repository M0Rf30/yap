// SPDX-FileCopyrightText: 2021-2026 Gianluca Boiano
// SPDX-License-Identifier: GPL-3.0-only

// Package aptinstall installs Debian packages into a target root.
//
// It runs the dpkg install pipeline: transitive dep resolution against
// pkg/apthost, .deb download and parsing through pkgformat/deb,
// conffile-aware data-archive extraction, maintainer scriptlets via /bin/sh, atomic
// /var/lib/dpkg/status + /var/lib/dpkg/info/<pkg>.* updates,
// conffile-preserving extraction, and a single ldconfig at the end of
// the transaction. Failures surface as errors; nothing falls back to a
// distro install subprocess.
//
// Scriptlets run as real /bin/sh child processes (not in-process via
// mvdan.cc/sh) because debconf's confmodule re-execs the script via
// Perl IPC::Open2, and update-alternatives/dpkg-trigger/etc. expect
// real exec semantics.
//
// The caller is responsible for refreshing apt indexes (pkg/apthost)
// before Install — trust of every downloaded .deb is bounded by the
// SHA-256 in the apt-index manifest, which itself is only as trustworthy
// as the InRelease PGP verification that produced it.
package aptinstall

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/M0Rf30/pkgformat/deb/apt"

	"github.com/M0Rf30/yap/v2/pkg/apthost"
	"github.com/M0Rf30/yap/v2/pkg/errors"
	"github.com/M0Rf30/yap/v2/pkg/i18n"
	"github.com/M0Rf30/yap/v2/pkg/logger"
	"github.com/M0Rf30/yap/v2/pkg/platform"
)

// Options controls Install's runtime behaviour.
//
// RootDir is the filesystem root the installation writes into. dpkg state
// (status, info/, locks, yapdb) is re-rooted under it; maintainer scripts and
// ldconfig run chrooted into it when privileged and are skipped otherwise.
//
//   - "" / "/" → install into the live system root. Refused unless
//     AllowRootInstall is true: the typical caller is yap running inside a
//     build container, but on a developer workstation accidentally invoking
//     Install would clobber the host filesystem.
//   - Any other value → install into that directory (fakeroot use). dpkg
//     status / lock files are still rooted at /var/lib/dpkg under it.
//
// RunLDConfig defaults to true; set false in fakeroot scenarios where the
// ld.so.cache would be meaningless.
//
// WriteDpkgStatus defaults to false. When false, installed packages are
// recorded in YAP's internal state database (pkg/yapdb) only. When true,
// packages are ALSO written to /var/lib/dpkg/status (legacy behavior).
// Rationale: YAP runs in ephemeral build containers where dpkg is not the
// source of truth.
type Options struct {
	RootDir          string
	AllowRootInstall bool
	RunLDConfig      bool
	WriteDpkgStatus  bool
}

// Install performs a full apt-get install equivalent with default options.
//
// Default policy: when running on a privileged host (uid 0 — the expected
// case inside a yap build container) `AllowRootInstall` is implicitly
// enabled. On a developer workstation running as a regular user, the
// safety guard still trips and refuses to clobber the host filesystem.
//
// Callers needing explicit control (sandboxed RootDir, suppressed
// ldconfig, etc.) should call InstallWithOptions directly.
func Install(ctx context.Context, names []string) error {
	return InstallWithOptions(ctx, names, Options{
		RunLDConfig:      true,
		AllowRootInstall: platform.IsPrivilegedHost(),
	})
}

// InstallWithOptions is the explicit-options variant of Install.
func InstallWithOptions(ctx context.Context, names []string, opts Options) error {
	if len(names) == 0 {
		return nil
	}

	rootDir, err := resolveRootDir(opts)
	if err != nil {
		return err
	}

	// Ensure dpkg directories exist before locking inside them.
	if err := ensureDpkgDirs(rootDir); err != nil {
		return errors.Wrap(err, errors.ErrTypeFileSystem, "ensure dpkg dirs").
			WithOperation("Install")
	}

	// Take the dpkg lock for the duration of the transaction so concurrent
	// dpkg/apt processes (or accidental re-entry) can't race the status
	// file read-modify-write cycle.
	lock, err := acquireDpkgLock(rootDir)
	if err != nil {
		return errors.Wrap(err, errors.ErrTypeFileSystem, "acquire dpkg lock").
			WithOperation("Install")
	}

	defer lock.Release()

	pkgs, oldVersions, tmpDir, debMetadata, err := resolveAndPrepare(ctx, names)
	if err != nil {
		return err
	}

	if tmpDir == "" {
		// resolveAndPrepare returns no error and no tmpDir when the closure
		// is empty (nothing to install).
		return nil
	}

	defer func() { _ = os.RemoveAll(tmpDir) }()

	// Install packages in dependency order.
	for _, p := range pkgs {
		contents := debMetadata[p.Filename]

		err := installPackage(ctx, p, oldVersions[p.Filename], contents, tmpDir, rootDir, opts)
		if err != nil {
			return errors.Wrap(err, errors.ErrTypeBuild, "install package").
				WithContext("package", p.Name).
				WithOperation("Install")
		}
	}

	// Refresh dynamic linker cache exactly once per transaction (vs once
	// per package), iff requested.
	if opts.RunLDConfig {
		refreshLDCacheAt(rootDir)
	}

	logger.Info(i18n.T("logger.aptinstall.info.installation_complete"), "count", len(pkgs))

	return nil
}

// resolveRootDir returns the destDir for the install transaction. "/"
// requires Options.AllowRootInstall — otherwise the caller almost
// certainly meant a fakeroot path and is about to clobber the live host.
func resolveRootDir(opts Options) (string, error) {
	rootDir := opts.RootDir
	if rootDir == "" {
		rootDir = "/"
	}

	if rootDir == "/" && !opts.AllowRootInstall {
		return "", errors.New(errors.ErrTypeConfiguration,
			"aptinstall: refusing to install into / without Options.AllowRootInstall").
			WithOperation("Install")
	}

	return rootDir, nil
}

// planInstall works out what installing names takes, the way apt-get install
// does: new packages and their dependencies, and upgrades of installed
// packages whose new dependents need a newer version (an exact
// "binutils-common (= X)" against an older installed one). Packages come in
// apt's unpack order; oldVersions maps each upgraded package's Filename to
// the installed version it replaces.
//
// A plan that removes or downgrades an installed package is refused: Install
// has no removal step, and a build must not take packages away from the host.
// Recommends are not installed, as before.
func planInstall(
	cache *apt.Cache, names []string,
) (pkgs []*apt.PackageInfo, oldVersions map[string]string, err error) {
	noRecommends := false

	// SIMPLIFIED: the resolver spends one step per cache slot on each of its
	// up to 20 passes, so apt.DefaultPlanSteps (200000) is used up by one
	// pass over an Ubuntu cache with a foreign architecture (~210000 slots).
	// The budget grows with the cache here until pkgformat scales its own.
	plan, err := cache.Plan(apt.PlanRequest{Install: names}, apt.PlanOptions{
		InstallRecommends: &noRecommends,
		MaxSteps:          apt.DefaultPlanSteps + 20*cache.PackageCount(),
	})
	if err != nil {
		return nil, nil, errors.Wrap(err, errors.ErrTypeBuild, "resolve dependencies").
			WithOperation("Install")
	}

	if len(plan.Unresolved) > 0 {
		return nil, nil, errors.New(errors.ErrTypeBuild,
			fmt.Sprintf("unresolvable packages: %v", plan.Unresolved)).
			WithOperation("Install")
	}

	if len(plan.Remove) > 0 || len(plan.Downgrade) > 0 {
		return nil, nil, errors.New(errors.ErrTypeBuild,
			fmt.Sprintf("installing %v would remove %v and downgrade %v",
				names, labels(plan.Remove), labels(plan.Downgrade))).
			WithOperation("Install")
	}

	oldVersions = make(map[string]string)

	for _, ch := range plan.Changes {
		pkgs = append(pkgs, ch.Package)

		if ch.From != nil {
			oldVersions[ch.Package.Filename] = ch.From.Version
		}
	}

	// Filter out foreign-arch packages that are not Multi-Arch: same: they
	// are not executable on the host. Multi-Arch: same packages (dev
	// libraries) are intentionally kept because they are legitimately
	// installed for the foreign arch.
	return filterForeignArchPackages(pkgs), oldVersions, nil
}

// labels names packages as name:arch version for an error message.
func labels(pkgs []*apt.PackageInfo) []string {
	out := make([]string, 0, len(pkgs))
	for _, p := range pkgs {
		out = append(out, p.Name+":"+p.Architecture+" "+p.Version)
	}

	return out
}

// resolveAndPrepare plans the install, downloads every .deb, and pre-parses
// each .deb's control metadata. The caller takes ownership of tmpDir and
// must os.RemoveAll it. Returns an empty tmpDir when the plan is empty (no
// work to do).
func resolveAndPrepare(
	ctx context.Context, names []string,
) (
	pkgs []*apt.PackageInfo, oldVersions map[string]string,
	tmpDir string, debMetadata map[string]*debContents, err error,
) {
	cache := apthost.Cache()

	pkgs, oldVersions, err = planInstall(cache, names)
	if err != nil {
		return nil, nil, "", nil, err
	}

	if len(pkgs) == 0 {
		return nil, nil, "", nil, nil
	}

	var totalBytes int64
	for _, p := range pkgs {
		totalBytes += p.Size
	}

	logger.Info(i18n.T("logger.aptinstall.info.resolved_dependencies"), "seeds", len(names),
		"to_install", len(pkgs),
		"total_bytes", totalBytes)

	tmpDir, err = os.MkdirTemp("", "yap-aptinstall-*")
	if err != nil {
		return nil, nil, "", nil, errors.Wrap(err, errors.ErrTypeFileSystem, "create temp dir").
			WithOperation("Install")
	}

	pkgNames := make([]string, 0, len(pkgs))
	for _, p := range pkgs {
		if p.Architecture == "" || p.Architecture == "all" {
			pkgNames = append(pkgNames, p.Name)
		} else {
			pkgNames = append(pkgNames, p.Name+":"+p.Architecture)
		}
	}

	if err := cache.Download(ctx, tmpDir, pkgNames, apthost.DownloadOptions()); err != nil {
		_ = os.RemoveAll(tmpDir)

		return nil, nil, "", nil, errors.Wrap(err, errors.ErrTypeBuild, "download packages").
			WithOperation("Install")
	}

	logger.Info(i18n.T("logger.aptinstall.info.downloaded_packages"), "count", len(pkgs))

	debMetadata = make(map[string]*debContents, len(pkgs))

	for _, p := range pkgs {
		debPath := filepath.Join(tmpDir, filepath.Base(p.Filename))

		contents, err := parseDEB(debPath)
		if err != nil {
			_ = os.RemoveAll(tmpDir)

			return nil, nil, "", nil, errors.Wrap(err, errors.ErrTypeBuild, "parse DEB").
				WithContext("package", p.Name).
				WithOperation("Install")
		}

		debMetadata[p.Filename] = contents
	}

	return pkgs, oldVersions, tmpDir, debMetadata, nil
}

// installPackage installs a single package. oldVersion is the installed
// version an upgrade replaces, "" for a fresh install; it selects the dpkg
// maintainer script arguments:
//
//	preinst install                 — fresh install
//	preinst upgrade <old-version>   — upgrading an existing install
//	postinst configure [<old-version>] — configure; old-version empty on
//	                                     first install
func installPackage(
	ctx context.Context,
	pkg *apt.PackageInfo,
	oldVersion string,
	contents *debContents,
	tmpDir, rootDir string,
	opts Options,
) error {
	pkgName := pkg.Name
	arch := pkg.Architecture

	logger.Debug(i18n.T("logger.aptinstall.debug.installing"), "package", pkgName, "arch", arch)

	// Sequence mirrors dpkg's own unpack flow:
	//
	//   1. Write /var/lib/dpkg/info/<pkg>.{preinst,postinst,...} so the
	//      maintainer scripts exist on disk before any of them runs.
	//      This matters for debconf's confmodule, which re-execs the
	//      script through Perl IPC::Open2 — that exec needs a real
	//      on-disk path.
	//   2. Run preinst (install | upgrade <old-version>).
	//   3. Extract data.tar to rootDir.
	//   4. Mark "install ok unpacked" in /var/lib/dpkg/status.
	//   5. Run postinst configure [<old-version>].
	//   6. Mark "install ok installed".
	//
	// The previous ordering ran preinst before writing the info files,
	// which meant `source /usr/share/debconf/confmodule` inside the
	// script blew up with "exec of postinst configure failed: No such
	// file or directory" because $0 didn't refer to any real file.

	if err := writeDpkgInfoFiles(rootDir, pkgName, arch, contents); err != nil {
		return errors.Wrap(err, errors.ErrTypeFileSystem, "write dpkg info files").
			WithContext("package", pkgName).
			WithOperation("installPackage")
	}

	if err := runMaintainerScript(
		ctx, rootDir, "preinst", pkgName, arch, contents, oldVersion,
	); err != nil {
		return err
	}

	// Parse conffiles.
	conffiles := strings.Split(strings.TrimSpace(contents.Conffiles), "\n")

	// Extract data.tar to the configured root.
	debPath := filepath.Join(tmpDir, filepath.Base(pkg.Filename))

	if err := extractDataTar(debPath, rootDir, conffiles); err != nil {
		return errors.Wrap(err, errors.ErrTypeFileSystem, "extract data.tar").
			WithContext("package", pkgName).
			WithOperation("installPackage")
	}

	// Update dpkg status (unpacked state).
	if err := updateDpkgStatusForPackage(ctx, pkgName, arch, contents.Control, "install ok unpacked", rootDir, opts, contents.Files); err != nil { //nolint:lll
		return errors.Wrap(err, errors.ErrTypeFileSystem, "update dpkg status (unpacked)").
			WithContext("package", pkgName).
			WithOperation("installPackage")
	}

	// postinst failures are non-fatal: match dpkg's behaviour where a
	// configure failure leaves the package in "install ok unpacked" state
	// (files extracted, but not marked configured). The build can still
	// proceed and a user can run `dpkg --configure -a` later to retry.
	// Real-world offenders: man-db (update-mandb wants a usable /proc),
	// debconf-driven scripts that hit unconfigured tty, packages that
	// shell out to systemctl in a container without systemd.
	postinstErr := runMaintainerScript(
		ctx, rootDir, "postinst", pkgName, arch, contents, oldVersion,
	)

	finalState := "install ok installed"

	if postinstErr != nil {
		logger.Warn(i18n.T("logger.aptinstall.warn.postinst_failed_leaving_package"), "package", pkgName,
			"arch", arch,
			"error", postinstErr,
			"hint", "run `dpkg --configure -a` after install to retry")

		finalState = "install ok unpacked"
	}

	if err := updateDpkgStatusForPackage(ctx, pkgName, arch, contents.Control, finalState, rootDir, opts, contents.Files); err != nil { //nolint:lll
		return errors.Wrap(err, errors.ErrTypeFileSystem, "update dpkg status (final)").
			WithContext("package", pkgName).
			WithOperation("installPackage")
	}

	if postinstErr == nil {
		logger.Info(i18n.T("logger.aptinstall.info.installed"), "package", pkgName, "arch", arch)
	} else {
		logger.Info(i18n.T("logger.aptinstall.info.installed_unconfigured"), "package", pkgName, "arch", arch)
	}

	return nil
}

// runMaintainerScript invokes a single maintainer scriptlet (preinst or
// postinst) with the correct dpkg-conformant action + args. Returns nil
// when the package ships no script for the given phase.
func runMaintainerScript(
	ctx context.Context,
	rootDir, phase, pkgName, arch string,
	contents *debContents,
	oldVersion string,
) error {
	body, ok := contents.Scriptlets[phase]
	if !ok || body == "" {
		return nil
	}

	var (
		action string
		args   []string
	)

	switch phase {
	case "preinst":
		action = "install"
		if oldVersion != "" {
			action = "upgrade"
			args = []string{oldVersion}
		}
	case "postinst":
		action = "configure"

		if oldVersion != "" {
			args = []string{oldVersion}
		}
	default:
		return errors.New(errors.ErrTypeInternal, "unknown maintainer phase").
			WithOperation("runMaintainerScript").WithContext("phase", phase)
	}

	if !canRunScripts(rootDir) {
		logger.Warn(i18n.T("logger.aptinstall.warn.skipping_maintainer_script_requires_root"),
			"package", pkgName, "script", phase, "root_dir", rootDir)

		return nil
	}

	scriptPath := scriptletPathForPackage(pkgName, arch, contents.Control, phase)
	if err := runScriptlet(ctx, rootDir, scriptPath, phase, pkgName, action, args...); err != nil {
		return errors.Wrap(err, errors.ErrTypeBuild, phase+" failed").
			WithContext("package", pkgName).
			WithOperation("installPackage")
	}

	return nil
}

// filterForeignArchPackages removes packages from the list that are foreign-arch
// and not Multi-Arch: same.
//
// WITH the arch-aware cache (name:arch keys + Cache.Plan multi-arch rules),
// this function now acts as a safety net rather than the primary fix. It
// catches any remaining foreign-arch packages that slipped through (e.g.
// packages where the seed explicitly requested the foreign arch).
func filterForeignArchPackages(pkgs []*apt.PackageInfo) []*apt.PackageInfo {
	hostArch := apthost.HostArch()

	filtered := pkgs[:0]

	for _, p := range pkgs {
		arch := p.Architecture
		// "all" packages are arch-independent — always keep.
		if arch == "all" || arch == "" || arch == hostArch {
			filtered = append(filtered, p)
			continue
		}

		// Foreign arch: keep only Multi-Arch: same (co-installable dev libs).
		if p.MultiArchSame() {
			filtered = append(filtered, p)
			continue
		}

		logger.Debug(i18n.T("logger.aptinstall.debug.skipping_foreign_arch_non"), "package", p.Name,
			"arch", arch,
			"multi_arch", p.MultiArch,
			"host_arch", hostArch)
	}

	return filtered
}

// canRunScripts reports whether maintainer scripts may run for rootDir:
// always for the host root, and for other roots only when privileged
// (chroot(2) requires root).
func canRunScripts(rootDir string) bool {
	return rootDir == "" || filepath.Clean(rootDir) == "/" || os.Geteuid() == 0
}

// RefreshLDCache runs ldconfig to refresh the dynamic linker cache.
// Non-fatal: if ldconfig is not found or fails, we log a warning but continue.
func RefreshLDCache() {
	refreshLDCacheAt("/")
}

// refreshLDCacheAt runs ldconfig against rootDir (ldconfig -r for non-host
// roots; skipped with a warning when unprivileged).
func refreshLDCacheAt(rootDir string) {
	bin, err := exec.LookPath("ldconfig")
	if err != nil {
		logger.Debug(i18n.T("logger.aptinstall.debug.ldconfig_not_found_skipping"))

		return
	}

	var args []string

	if rootDir != "" && filepath.Clean(rootDir) != "/" {
		if !canRunScripts(rootDir) {
			logger.Warn(i18n.T("logger.aptinstall.warn.skipping_ldconfig_requires_root"),
				"root_dir", rootDir)

			return
		}

		args = []string{"-r", rootDir}
	}

	// nolint:noctx // ldconfig is a system utility, not a network call
	cmd := exec.Command(bin, args...)

	if err := cmd.Run(); err != nil {
		logger.Warn(i18n.T("logger.aptinstall.warn.ldconfig_failed"), "error", err)
	}
}
