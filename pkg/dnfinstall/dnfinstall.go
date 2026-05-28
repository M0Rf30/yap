package dnfinstall

import (
	"context"
	"os"

	"github.com/M0Rf30/yap/v2/pkg/dnfcache"
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
//   - Any other value → install into that directory (fakeroot use).
//
// AllowUnverifiedRPMs: RPM packages ship with GPG signatures. Verification
// of those signatures is not yet wired into this package. Callers that
// accept this gap (e.g. inside a trusted CI container) must set this flag
// explicitly.
//
// RunLDConfig defaults to true; set false in fakeroot scenarios where the
// ld.so.cache would be meaningless.
//
// SkipScriptlets: if true, pre/post install scriptlets are not executed.
// Useful for fakeroot scenarios where scriptlets may fail or be unnecessary.
//
// KeyringPath: optional override for the GPG keyring directory. Defaults to
// /etc/pki/rpm-gpg/. Used for signature verification.
//
// WriteSystemRpmdb: if true, ALSO write to /var/lib/rpm/rpmdb.sqlite (SQLite
// hosts only — Fedora 33+, RHEL 9+, Rocky 9+). Default is false; YAP uses
// yapdb for state tracking instead. On BDB hosts or when the SQLite rpmdb is
// absent, enabling this fails the install rather than silently diverging
// from the on-disk system state.
//
// StrictScriptlets: if true, %pretrans/%post/%posttrans failures are treated
// as fatal. RPM convention is non-fatal; this flag is intended for build-time
// makedepends provisioning where a broken scriptlet poisons later builds.
type Options struct {
	RootDir             string
	AllowRootInstall    bool
	AllowUnverifiedRPMs bool
	RunLDConfig         bool
	SkipScriptlets      bool
	StrictScriptlets    bool
	KeyringPath         string
	WriteSystemRpmdb    bool
}

// Install performs a full dnf install equivalent with default options.
//
// Default policy: when running on a privileged host (uid 0 — the expected
// case inside a yap build container) `AllowRootInstall` is implicitly
// enabled. On a developer workstation running as a regular user, the
// safety guard still trips and refuses to clobber the host filesystem.
//
// Callers needing explicit control (sandboxed RootDir, suppressed ldconfig,
// etc.) should call InstallWithOptions directly.
func Install(ctx context.Context, names []string) error {
	privileged := platform.IsPrivilegedHost()

	return InstallWithOptions(ctx, names, Options{
		RunLDConfig:      true,
		AllowRootInstall: privileged,
		// Inside a yap build container repo metadata trust is established at
		// fetch time (Release/repomd verified). Package-level GPG keyrings
		// for third-party repos (EPEL, RPM Fusion, modules) are frequently
		// missing in minimal Rocky/Alma/RHEL images and would block legitimate
		// builds. Privileged-host = trusted-CI context → allow.
		AllowUnverifiedRPMs: privileged,
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

	// Load the dnf cache and resolve the transitive closure of dependencies.
	cache := dnfcache.Load()

	resolved, unresolved, err := cache.ResolveDeps(ctx, names)
	if err != nil {
		return errors.Wrap(err, errors.ErrTypeBuild, "failed to resolve dependencies").
			WithOperation("InstallWithOptions")
	}

	if len(unresolved) > 0 {
		// Non-fatal: log and continue — installation will surface hard errors.
		logger.Warn("some packages could not be resolved", "count", len(unresolved))

		for _, name := range unresolved {
			logger.Debug("unresolved package", "name", name)
		}
	}

	if len(resolved) == 0 {
		return nil
	}

	// Download and install packages in dependency order.
	if err := installPackages(ctx, cache, resolved, rootDir, opts); err != nil {
		return err
	}

	// Refresh dynamic linker cache exactly once per transaction (vs once
	// per package), iff requested.
	if opts.RunLDConfig {
		if err := runLDConfig(ctx, rootDir); err != nil {
			// A failed cache refresh should not abort an otherwise-successful
			// install: log and continue, mirroring rpm's tolerance of a
			// missing or non-fatal ldconfig.
			logger.Warn("ldconfig refresh failed (continuing)", "error", err.Error())
		}
	}

	logger.Info("installation complete", "count", len(resolved))

	return nil
}

// InstallFile installs a single local RPM file to the target filesystem.
// Useful for installing pre-downloaded or custom-built packages.
func InstallFile(ctx context.Context, rpmPath string, opts Options) error {
	rootDir, err := resolveRootDir(opts)
	if err != nil {
		return err
	}

	if _, err := os.Stat(rpmPath); err != nil {
		return errors.Wrap(err, errors.ErrTypeFileSystem, "RPM file not found").
			WithOperation("InstallFile").
			WithContext("path", rpmPath)
	}

	// Extract the RPM to rootDir
	if err := installPackage(ctx, rpmPath, rootDir, opts); err != nil {
		return errors.Wrap(err, errors.ErrTypeBuild, "failed to install RPM").
			WithOperation("InstallFile").
			WithContext("path", rpmPath)
	}

	logger.Info("installed RPM file", "path", rpmPath)

	return nil
}

// resolveRootDir returns the destDir for the install transaction. "/"
// requires Options.AllowRootInstall — otherwise the caller almost
// certainly meant to pass a fakeroot directory.
func resolveRootDir(opts Options) (string, error) {
	rootDir := opts.RootDir
	if rootDir == "" {
		rootDir = "/"
	}

	if rootDir == "/" && !opts.AllowRootInstall {
		return "", errors.New(errors.ErrTypeValidation,
			"refusing to install to / without AllowRootInstall").
			WithOperation("resolveRootDir")
	}

	if rootDir != "/" {
		if _, err := os.Stat(rootDir); err != nil {
			return "", errors.Wrap(err, errors.ErrTypeFileSystem,
				"RootDir does not exist").
				WithOperation("resolveRootDir").
				WithContext("rootDir", rootDir)
		}
	}

	return rootDir, nil
}

// installPackages downloads and installs each package in the resolved closure.
func installPackages(ctx context.Context, cache *dnfcache.Cache, resolved []*dnfcache.PackageInfo, rootDir string, opts Options) error { //nolint:lll
	return downloadAndInstall(ctx, cache, resolved, rootDir, opts)
}
