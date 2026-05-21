// Package aptinstall installs Debian packages into a target root.
//
// It runs the dpkg install pipeline: transitive dep resolution against
// pkg/aptcache, .deb download via grab, ar/control.tar/data.tar
// extraction, maintainer scriptlets via /bin/sh, atomic
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
// The caller is responsible for refreshing apt indexes (pkg/aptrepo)
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

	"github.com/M0Rf30/yap/v2/pkg/aptcache"
	"github.com/M0Rf30/yap/v2/pkg/errors"
	"github.com/M0Rf30/yap/v2/pkg/logger"
	"github.com/M0Rf30/yap/v2/pkg/platform"
)

// Options controls Install's runtime behaviour.
//
// RootDir is the filesystem root the installation writes into.
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
type Options struct {
	RootDir          string
	AllowRootInstall bool
	RunLDConfig      bool
}

// Install performs a full pure-Go apt-get install equivalent with default
// options.
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

	// Take the dpkg lock for the duration of the transaction so concurrent
	// dpkg/apt processes (or accidental re-entry) can't race the status
	// file read-modify-write cycle.
	lock, err := acquireDpkgLock()
	if err != nil {
		return errors.Wrap(err, errors.ErrTypeFileSystem, "acquire dpkg lock").
			WithOperation("Install")
	}

	defer lock.Release()

	// Ensure dpkg directories exist.
	if err := ensureDpkgDirs(); err != nil {
		return errors.Wrap(err, errors.ErrTypeFileSystem, "ensure dpkg dirs").
			WithOperation("Install")
	}

	pkgs, tmpDir, debMetadata, err := resolveAndPrepare(ctx, names)
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
		contents := debMetadata[p.Name]

		if err := installPackage(ctx, p, contents, tmpDir, rootDir); err != nil {
			return errors.Wrap(err, errors.ErrTypeBuild, "install package").
				WithContext("package", p.Name).
				WithOperation("Install")
		}
	}

	// Refresh dynamic linker cache exactly once per transaction (vs once
	// per package), iff requested.
	if opts.RunLDConfig {
		RefreshLDCache()
	}

	logger.Info("installation complete", "count", len(pkgs))

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

// resolveAndPrepare resolves the transitive closure, downloads every .deb,
// and pre-parses each .deb's control metadata. The caller takes ownership
// of tmpDir and must os.RemoveAll it. Returns ("", nil, nil) when the
// closure is empty (no work to do).
func resolveAndPrepare(
	ctx context.Context, names []string,
) (pkgs []*aptcache.PackageInfo, tmpDir string, debMetadata map[string]*debContents, err error) {
	cache := aptcache.Load()

	var unresolved []string

	pkgs, unresolved, err = cache.ResolveDeps(names)
	if err != nil {
		return nil, "", nil, errors.Wrap(err, errors.ErrTypeBuild, "resolve dependencies").
			WithOperation("Install")
	}

	if len(unresolved) > 0 {
		return nil, "", nil, errors.New(errors.ErrTypeBuild,
			fmt.Sprintf("unresolvable packages: %v", unresolved)).
			WithOperation("Install")
	}

	if len(pkgs) == 0 {
		return nil, "", nil, nil
	}

	logger.Info("resolved dependencies", "count", len(pkgs))

	tmpDir, err = os.MkdirTemp("", "yap-aptinstall-*")
	if err != nil {
		return nil, "", nil, errors.Wrap(err, errors.ErrTypeFileSystem, "create temp dir").
			WithOperation("Install")
	}

	pkgNames := make([]string, 0, len(pkgs))
	for _, p := range pkgs {
		pkgNames = append(pkgNames, p.Name)
	}

	if err := cache.Download(ctx, tmpDir, pkgNames); err != nil {
		_ = os.RemoveAll(tmpDir)

		return nil, "", nil, errors.Wrap(err, errors.ErrTypeBuild, "download packages").
			WithOperation("Install")
	}

	logger.Info("downloaded packages", "count", len(pkgs))

	debMetadata = make(map[string]*debContents, len(pkgs))

	for _, p := range pkgs {
		debPath := filepath.Join(tmpDir, filepath.Base(p.Filename))

		contents, err := parseDEB(debPath)
		if err != nil {
			_ = os.RemoveAll(tmpDir)

			return nil, "", nil, errors.Wrap(err, errors.ErrTypeBuild, "parse DEB").
				WithContext("package", p.Name).
				WithOperation("Install")
		}

		debMetadata[p.Name] = contents
	}

	return pkgs, tmpDir, debMetadata, nil
}

// currentInstalledVersion returns the version currently recorded in
// /var/lib/dpkg/status for pkg, or "" when the package is not installed.
//
// dpkg semantics for maintainer scripts:
//
//	preinst install                 — fresh install
//	preinst upgrade <old-version>   — upgrading an existing install
//	postinst configure [<old-version>] — configure; old-version empty on
//	                                     first install
//
// The OLD version must come from /var/lib/dpkg/status, NOT from the
// newly-downloaded .deb's control file (which carries the NEW version
// we're about to install).
func currentInstalledVersion(pkg *aptcache.PackageInfo) string {
	if !pkg.Installed {
		return ""
	}

	entries, err := readDpkgStatus()
	if err != nil {
		return ""
	}

	key := pkg.Name
	if pkg.Architecture != "" {
		key = pkg.Name + ":" + pkg.Architecture
	}

	if e, ok := entries[key]; ok {
		return e.fields["Version"]
	}
	// Fallback for entries without arch qualifier.
	if e, ok := entries[pkg.Name]; ok {
		return e.fields["Version"]
	}

	return ""
}

// installPackage installs a single package.
func installPackage(
	ctx context.Context,
	pkg *aptcache.PackageInfo,
	contents *debContents,
	tmpDir, rootDir string,
) error {
	pkgName := pkg.Name
	arch := pkg.Architecture

	logger.Debug("installing", "package", pkgName, "arch", arch)

	oldVersion := currentInstalledVersion(pkg)

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

	if err := writeDpkgInfoFiles(pkgName, arch, contents); err != nil {
		return errors.Wrap(err, errors.ErrTypeFileSystem, "write dpkg info files").
			WithContext("package", pkgName).
			WithOperation("installPackage")
	}

	if err := runMaintainerScript(
		ctx, "preinst", pkgName, arch, contents, oldVersion,
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
	if err := updateDpkgStatusForPackage(pkgName, arch, contents.Control, "install ok unpacked"); err != nil {
		return errors.Wrap(err, errors.ErrTypeFileSystem, "update dpkg status (unpacked)").
			WithContext("package", pkgName).
			WithOperation("installPackage")
	}

	if err := runMaintainerScript(
		ctx, "postinst", pkgName, arch, contents, oldVersion,
	); err != nil {
		return err
	}

	// Update dpkg status (fully installed).
	if err := updateDpkgStatusForPackage(pkgName, arch, contents.Control, "install ok installed"); err != nil {
		return errors.Wrap(err, errors.ErrTypeFileSystem, "update dpkg status (installed)").
			WithContext("package", pkgName).
			WithOperation("installPackage")
	}

	logger.Info("installed", "package", pkgName, "arch", arch)

	return nil
}

// runMaintainerScript invokes a single maintainer scriptlet (preinst or
// postinst) with the correct dpkg-conformant action + args. Returns nil
// when the package ships no script for the given phase.
func runMaintainerScript(
	ctx context.Context,
	phase, pkgName, arch string,
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
		return fmt.Errorf("unknown maintainer phase %q", phase)
	}

	scriptPath := scriptletPathForPackage(pkgName, arch, contents.Control, phase)
	if err := runScriptlet(ctx, scriptPath, phase, pkgName, action, args...); err != nil {
		return errors.Wrap(err, errors.ErrTypeBuild, phase+" failed").
			WithContext("package", pkgName).
			WithOperation("installPackage")
	}

	return nil
}

// RefreshLDCache runs ldconfig to refresh the dynamic linker cache.
// Non-fatal: if ldconfig is not found or fails, we log a warning but continue.
func RefreshLDCache() {
	bin, err := exec.LookPath("ldconfig")
	if err != nil {
		logger.Debug("ldconfig not found, skipping linker cache refresh")

		return
	}

	// nolint:noctx // ldconfig is a system utility, not a network call
	cmd := exec.Command(bin) // #nosec G204 - LookPath returns absolute path

	if err := cmd.Run(); err != nil {
		logger.Warn("ldconfig failed", "error", err)
	}
}
