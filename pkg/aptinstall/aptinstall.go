// Package aptinstall provides a pure-Go replacement for "apt-get install".
//
// It performs full package installation including:
//   - Transitive dependency resolution
//   - .deb download and extraction
//   - Maintainer scriptlet execution (preinst, postinst, prerm, postrm)
//   - /var/lib/dpkg/status database updates
//   - /var/lib/dpkg/info/<pkg>.* metadata files
//   - Conffile collision handling
//   - Dynamic linker cache refresh (ldconfig)
//
// No subprocess fallback: if a package cannot be installed, an error is returned.
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
)

// Install performs a full pure-Go apt-get install equivalent.
// It resolves transitive dependencies, downloads .debs, extracts them,
// runs maintainer scriptlets, and updates the dpkg database.
// Returns an error if any package cannot be resolved, downloaded, or installed.
func Install(ctx context.Context, names []string) error {
	if len(names) == 0 {
		return nil
	}

	// Ensure dpkg directories exist.
	if err := ensureDpkgDirs(); err != nil {
		return errors.Wrap(err, errors.ErrTypeFileSystem, "ensure dpkg dirs").
			WithOperation("Install")
	}

	cache := aptcache.Load()

	// Resolve transitive dependencies.
	pkgs, unresolved, err := cache.ResolveDeps(names)
	if err != nil {
		return errors.Wrap(err, errors.ErrTypeBuild, "resolve dependencies").
			WithOperation("Install")
	}

	if len(unresolved) > 0 {
		return errors.New(errors.ErrTypeBuild,
			fmt.Sprintf("unresolvable packages: %v", unresolved)).
			WithOperation("Install")
	}

	if len(pkgs) == 0 {
		return nil
	}

	logger.Info("aptinstall: resolved dependencies",
		"count", len(pkgs))

	// Download all .debs to a temporary directory.
	tmpDir, err := os.MkdirTemp("", "yap-aptinstall-*")
	if err != nil {
		return errors.Wrap(err, errors.ErrTypeFileSystem, "create temp dir").
			WithOperation("Install")
	}

	defer func() { _ = os.RemoveAll(tmpDir) }()

	// Extract package names from resolved PackageInfo slice.
	pkgNames := make([]string, 0, len(pkgs))
	for _, p := range pkgs {
		pkgNames = append(pkgNames, p.Name)
	}

	// Download all packages.
	if err := cache.Download(ctx, tmpDir, pkgNames); err != nil {
		return errors.Wrap(err, errors.ErrTypeBuild, "download packages").
			WithOperation("Install")
	}

	logger.Info("aptinstall: downloaded packages", "count", len(pkgs))

	// Parse all .debs to extract metadata.
	debMetadata := make(map[string]*debContents)

	for _, p := range pkgs {
		debPath := filepath.Join(tmpDir, filepath.Base(p.Filename))

		contents, err := parseDEB(debPath)
		if err != nil {
			return errors.Wrap(err, errors.ErrTypeBuild, "parse DEB").
				WithContext("package", p.Name).
				WithOperation("Install")
		}

		debMetadata[p.Name] = contents
	}

	// Install packages in dependency order.
	for _, p := range pkgs {
		contents := debMetadata[p.Name]

		if err := installPackage(ctx, p, contents, tmpDir); err != nil {
			return errors.Wrap(err, errors.ErrTypeBuild, "install package").
				WithContext("package", p.Name).
				WithOperation("Install")
		}
	}

	// Refresh dynamic linker cache.
	RefreshLDCache()

	logger.Info("aptinstall: installation complete", "count", len(pkgs))

	return nil
}

// installPackage installs a single package.
func installPackage(ctx context.Context, pkg *aptcache.PackageInfo, contents *debContents, tmpDir string) error {
	pkgName := pkg.Name
	arch := pkg.Architecture

	logger.Info("Installing package", "package", pkgName, "arch", arch)

	// Determine old version (if upgrading).
	oldVersion := ""

	if pkg.Installed {
		// Parse control to get version.
		controlFields := parseControl(contents.Control)
		if v, ok := controlFields["Version"]; ok {
			oldVersion = v
		}
	}

	// Run preinst scriptlet.
	if preinst, ok := contents.Scriptlets["preinst"]; ok {
		action := "install"
		args := []string{}

		if oldVersion != "" {
			action = "upgrade"
			args = []string{oldVersion}
		}

		if err := runScriptlet(ctx, "preinst", preinst, pkgName, action, args...); err != nil {
			return fmt.Errorf("preinst failed: %w", err)
		}
	}

	// Parse conffiles.
	conffiles := strings.Split(strings.TrimSpace(contents.Conffiles), "\n")

	// Extract data.tar to /.
	debPath := filepath.Join(tmpDir, filepath.Base(pkg.Filename))

	if err := extractDataTar(debPath, "/", conffiles); err != nil {
		return fmt.Errorf("extract data.tar: %w", err)
	}

	logger.Info("Extracted package files", "package", pkgName)

	// Update dpkg status (unpacked state).
	if err := updateDpkgStatusForPackage(pkgName, arch, contents.Control, "install ok unpacked"); err != nil {
		return fmt.Errorf("update dpkg status (unpacked): %w", err)
	}

	// Write dpkg info files.
	if err := writeDpkgInfoFiles(pkgName, arch, contents); err != nil {
		return fmt.Errorf("write dpkg info files: %w", err)
	}

	// Run postinst scriptlet.
	if postinst, ok := contents.Scriptlets["postinst"]; ok {
		action := "configure"
		args := []string{}

		if oldVersion != "" {
			args = []string{oldVersion}
		}

		if err := runScriptlet(ctx, "postinst", postinst, pkgName, action, args...); err != nil {
			return fmt.Errorf("postinst failed: %w", err)
		}
	}

	// Update dpkg status (fully installed).
	if err := updateDpkgStatusForPackage(pkgName, arch, contents.Control, "install ok installed"); err != nil {
		return fmt.Errorf("update dpkg status (installed): %w", err)
	}

	logger.Info("Package installed successfully", "package", pkgName)

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
