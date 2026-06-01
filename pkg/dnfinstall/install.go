package dnfinstall

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"time"

	rpmutils "github.com/sassoftware/go-rpmutils"

	"github.com/M0Rf30/yap/v2/pkg/rpmdb"

	"github.com/M0Rf30/yap/v2/pkg/dnfcache"
	"github.com/M0Rf30/yap/v2/pkg/errors"
	"github.com/M0Rf30/yap/v2/pkg/i18n"
	"github.com/M0Rf30/yap/v2/pkg/logger"
	"github.com/M0Rf30/yap/v2/pkg/yapdb"
)

// formatRPM is the constant identifier for the RPM package format.
const formatRPM = "rpm"

// downloadAndInstall downloads all packages in the resolved closure and
// installs them in dependency order.
func downloadAndInstall(
	ctx context.Context,
	cache *dnfcache.Cache,
	resolved []*dnfcache.PackageInfo,
	rootDir string,
	opts Options,
) error {
	// Create a temporary directory for downloaded RPMs.
	tmpDir, err := os.MkdirTemp("", "dnfinstall-*")
	if err != nil {
		return errors.Wrap(err, errors.ErrTypeFileSystem, "failed to create temp directory").
			WithOperation("downloadAndInstall")
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	// Download all packages.
	rpmPaths := make(map[string]string) // package name -> local path

	var mu sync.Mutex

	for _, pkg := range resolved {
		// Check context cancellation.
		if err := ctx.Err(); err != nil {
			return errors.Wrap(err, errors.ErrTypeFileSystem, "context cancelled").
				WithOperation("downloadAndInstall")
		}

		// Download the RPM.
		path, err := downloadRPM(ctx, cache, pkg, tmpDir)
		if err != nil {
			return errors.Wrap(err, errors.ErrTypeBuild, "failed to download package").
				WithOperation("downloadAndInstall").
				WithContext("package", pkg.Name)
		}

		mu.Lock()
		rpmPaths[pkg.Name] = path
		mu.Unlock()

		logger.Debug(i18n.T("logger.dnfinstall.debug.downloaded_rpm"), "package", pkg.Name, "path", path)
	}

	// Install packages in dependency order.
	for _, pkg := range resolved {
		rpmPath := rpmPaths[pkg.Name]

		if err := installPackage(ctx, rpmPath, rootDir, opts); err != nil {
			return errors.Wrap(err, errors.ErrTypeBuild, "failed to install package").
				WithOperation("downloadAndInstall").
				WithContext("package", pkg.Name)
		}
	}

	return nil
}

// downloadRPM downloads a single .rpm from the dnfcache to destDir.
// Returns the local path to the downloaded file.
func downloadRPM(ctx context.Context, _ *dnfcache.Cache, pkg *dnfcache.PackageInfo, destDir string) (string, error) {
	if pkg == nil {
		return "", errors.New(errors.ErrTypeValidation, "nil package").
			WithOperation("downloadRPM")
	}

	path, err := dnfcache.DownloadRPM(ctx, pkg, destDir)
	if err != nil {
		return "", errors.Wrap(err, errors.ErrTypeNetwork, "failed to download package").
			WithOperation("downloadRPM").
			WithContext("package", pkg.Name)
	}

	return path, nil
}

// installPackage extracts a single RPM file to rootDir.
// Implements the full install sequence: verify → %pretrans → %pre → extract → %post → yapdb → rpmdb → %posttrans.
//
//nolint:gocyclo,cyclop // sequential install phases each guarded by a strict/non-strict scriptlet branch
func installPackage(ctx context.Context, rpmPath, rootDir string, opts Options) (retErr error) {
	// Acquire install lock to prevent concurrent modifications.
	release, err := acquireLock(ctx, rootDir)
	if err != nil {
		return errors.Wrap(err, errors.ErrTypeFileSystem, "failed to acquire install lock").
			WithOperation("installPackage").
			WithContext("path", rpmPath)
	}
	defer func() {
		if e := release(); e != nil && retErr == nil {
			retErr = e
		}
	}()

	// Verify GPG signature before proceeding.
	if err := verifyRPMSignature(ctx, rpmPath, opts); err != nil {
		return errors.Wrap(err, errors.ErrTypeValidation, "RPM signature verification failed").
			WithOperation("installPackage").
			WithContext("path", rpmPath)
	}

	// Open and parse the RPM once for all phases.
	f, err := os.Open(rpmPath) //nolint:gosec
	if err != nil {
		return errors.Wrap(err, errors.ErrTypeFileSystem, "failed to open RPM file").
			WithOperation("installPackage").
			WithContext("path", rpmPath)
	}
	defer func() { _ = f.Close() }()

	rpm, err := rpmutils.ReadRpm(f)
	if err != nil {
		return errors.Wrap(err, errors.ErrTypeValidation, "failed to parse RPM").
			WithOperation("installPackage").
			WithContext("path", rpmPath)
	}

	pkgName, _ := rpm.Header.GetString(rpmutils.NAME)

	// Run %pretrans scriptlet.
	// %pretrans is typically optional setup (filesystem layout, SELinux
	// labels). Per the same lenient policy as %post/%posttrans, log and
	// continue on failure so a quirky third-party package doesn't abort
	// the build environment provisioning.
	if err := runScriptlet(ctx, scriptletPreTrans, rpm, rootDir, opts); err != nil {
		if opts.StrictScriptlets {
			return errors.Wrap(err, errors.ErrTypeBuild, "pretrans scriptlet failed").
				WithOperation("installPackage").
				WithContext("package", pkgName)
		}

		logger.Warn(i18n.T("logger.dnfinstall.warn.pretrans_scriptlet_failed_continuing"),
			"package", pkgName, "error", err.Error())
	}

	// Run %pre scriptlet (before file extraction).
	if err := runScriptlet(ctx, scriptletPreIn, rpm, rootDir, opts); err != nil {
		return errors.Wrap(err, errors.ErrTypeBuild, "prein scriptlet failed").
			WithOperation("installPackage").
			WithContext("package", pkgName)
	}

	// Extract the RPM to rootDir.
	entry, err := extractRPMWithHeader(ctx, rpmPath, rootDir, rpm, opts)
	if err != nil {
		return errors.Wrap(err, errors.ErrTypeBuild, "failed to extract RPM").
			WithOperation("installPackage").
			WithContext("path", rpmPath)
	}

	// Run %post scriptlet (after file extraction).
	// Per RPM convention, %post failures are non-fatal: log and continue.
	if err := runScriptlet(ctx, scriptletPostIn, rpm, rootDir, opts); err != nil {
		if opts.StrictScriptlets {
			return errors.Wrap(err, errors.ErrTypeBuild, "postin scriptlet failed").
				WithOperation("installPackage").
				WithContext("package", pkgName)
		}

		logger.Warn(i18n.T("logger.dnfinstall.warn.postin_scriptlet_failed_continuing"),
			"package", pkgName, "error", err.Error())
	}

	logger.Info(i18n.T("logger.dnfinstall.info.installed_rpm_package"),
		"path", filepath.Base(rpmPath), "files", len(entry.Files))

	// Write YAP state database (mandatory).
	if err := writeYapdb(ctx, rpm, entry, rootDir); err != nil {
		return errors.Wrap(err, errors.ErrTypeBuild, "failed to write yapdb").
			WithOperation("installPackage").
			WithContext("package", pkgName)
	}

	// Optionally write the system rpmdb (SQLite only). On BDB hosts or when the
	// SQLite rpmdb is absent this fails; opting in treats that as a hard error
	// rather than silently diverging from the on-disk system state.
	if opts.WriteSystemRpmdb {
		if err := writeSystemRpmdb(ctx, rpm, entry, rootDir); err != nil {
			return errors.Wrap(err, errors.ErrTypeInternal,
				"failed to write system rpmdb").
				WithOperation("installPackage").
				WithContext("package", pkgName)
		}
	}

	// Run %posttrans scriptlet (after everything).
	// Per RPM convention, %posttrans failures are non-fatal: log and continue.
	if err := runScriptlet(ctx, scriptletPostTrans, rpm, rootDir, opts); err != nil {
		if opts.StrictScriptlets {
			return errors.Wrap(err, errors.ErrTypeBuild, "posttrans scriptlet failed").
				WithOperation("installPackage").
				WithContext("package", pkgName)
		}

		logger.Warn(i18n.T("logger.dnfinstall.warn.posttrans_scriptlet_failed_continuing"),
			"package", pkgName, "error", err.Error())
	}

	return nil
}

// writeYapdb writes the installed package metadata to the YAP state database.
func writeYapdb(ctx context.Context, rpm *rpmutils.Rpm, entry *rpmEntry, rootDir string) error {
	// Extract package metadata from RPM header.
	name, _ := rpm.Header.GetString(rpmutils.NAME)
	version, _ := rpm.Header.GetString(rpmutils.VERSION)
	release, _ := rpm.Header.GetString(rpmutils.RELEASE)
	arch, _ := rpm.Header.GetString(rpmutils.ARCH)
	epoch, _ := rpm.Header.GetString(rpmutils.EPOCH)
	summary, _ := rpm.Header.GetString(rpmutils.SUMMARY)

	// Convert installedFile to yapdb.File.
	var files []yapdb.File
	for _, f := range entry.Files {
		files = append(files, yapdb.File{
			Path:       f.Path,
			Mode:       f.Mode,
			IsDir:      f.IsDir,
			IsSymlink:  f.IsSymlink,
			LinkTarget: f.LinkTarget,
			SHA256:     f.SHA256,
		})
	}

	// Extract capabilities from RPM header (Provides, Requires, Conflicts, Obsoletes).
	caps := extractCapabilities(rpm)

	// Open yapdb and insert the package record.
	db, err := yapdb.Open(ctx, yapdb.DefaultPath(rootDir))
	if err != nil {
		return errors.Wrap(err, errors.ErrTypeFileSystem, "failed to open yapdb").
			WithOperation("writeYapdb").
			WithContext("package", name)
	}
	defer func() { _ = db.Close() }()

	pkg := &yapdb.Package{
		Name:        name,
		Epoch:       epoch,
		Version:     version,
		Release:     release,
		Arch:        arch,
		Format:      formatRPM,
		Summary:     summary,
		InstallTime: time.Now(),
		Files:       files,
		Caps:        caps,
	}

	if err := db.Insert(ctx, pkg); err != nil {
		return errors.Wrap(err, errors.ErrTypeFileSystem, "failed to insert package into yapdb").
			WithOperation("writeYapdb").
			WithContext("package", name)
	}

	return nil
}

// writeSystemRpmdb optionally writes to the system rpmdb.sqlite (SQLite only).
// On BDB systems or if the file doesn't exist, returns an error that is logged
// as a warning by the caller (opting in is treated as a hard failure).
func writeSystemRpmdb(ctx context.Context, rpm *rpmutils.Rpm, entry *rpmEntry, rootDir string) error {
	rpmdbPath := filepath.Join(rootDir, "var", "lib", "rpm", "rpmdb.sqlite")

	// Check if the SQLite rpmdb exists.
	if _, err := os.Stat(rpmdbPath); err != nil {
		return errors.Wrap(err, errors.ErrTypeFileSystem, "system rpmdb not found (BDB system?)").
			WithContext("path", rpmdbPath)
	}

	writer, err := rpmdb.OpenWriter(ctx, rpmdbPath)
	if err != nil {
		return errors.Wrap(err, errors.ErrTypeFileSystem, "failed to open system rpmdb").
			WithOperation("writeSystemRpmdb").
			WithContext("path", rpmdbPath)
	}
	defer func() { _ = writer.Close() }()

	if err := writer.Install(ctx, rpm, toRPMDBFiles(entry.Files)); err != nil {
		return errors.Wrap(err, errors.ErrTypeFileSystem, "failed to write package to system rpmdb").
			WithOperation("writeSystemRpmdb").
			WithContext("path", rpmdbPath)
	}

	return nil
}

// toRPMDBFiles converts the dnfinstall installedFile slice into the
// rpmdb.InstalledFile shape expected by rpmdb.Writer.Install.
func toRPMDBFiles(files []installedFile) []rpmdb.InstalledFile {
	out := make([]rpmdb.InstalledFile, 0, len(files))
	for i := range files {
		f := &files[i]
		out = append(out, rpmdb.InstalledFile{
			Path:       f.Path,
			Size:       f.Size,
			Mode:       uint32(f.Mode), //nolint:gosec // os.FileMode bits fit uint32
			SHA256:     f.SHA256,
			LinkTarget: f.LinkTarget,
		})
	}

	return out
}
