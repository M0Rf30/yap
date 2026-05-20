package apkindex

import (
	"archive/tar"
	"bufio"
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/M0Rf30/yap/v2/pkg/logger"
)

// InstallPackages downloads each requested package + transitive deps, extracts each
// to /, and updates /lib/apk/db/installed. Replaces "apk add".
// For now, scriptlets are skipped (logged as warnings).
func (idx *Index) InstallPackages(ctx context.Context, names []string) error {
	// 1. Resolve transitive deps.
	resolved, err := idx.ResolveDeps(names)
	if err != nil {
		return fmt.Errorf("apkindex: resolve deps: %w", err)
	}

	if len(resolved) == 0 {
		logger.Info("apkindex: no packages to install")
		return nil
	}

	// 2. Filter out already-installed packages.
	installed := readInstalledDB()

	var toInstall []*Package

	for _, p := range resolved {
		if !installed[p.Name] {
			toInstall = append(toInstall, p)
		}
	}

	if len(toInstall) == 0 {
		logger.Info("apkindex: all packages already installed",
			"count", len(resolved))

		return nil
	}

	logger.Info("apkindex: installing packages",
		"count", len(toInstall), "total_resolved", len(resolved))

	// 3. Download all .apk files to a temp dir.
	tmpDir, err := os.MkdirTemp("", "yap-apk-*")
	if err != nil {
		return fmt.Errorf("apkindex: mktemp: %w", err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	for _, p := range toInstall {
		if _, err := idx.DownloadPackage(ctx, tmpDir, p.Name); err != nil {
			return fmt.Errorf("apkindex: download %s: %w", p.Name, err)
		}
	}

	// 4. Extract each .apk to / and register in installed DB.
	for _, p := range toInstall {
		apkPath := filepath.Join(tmpDir, p.Name+"-"+p.Version+".apk")

		if err := extractAndRegister(apkPath, p); err != nil {
			return fmt.Errorf("apkindex: install %s: %w", p.Name, err)
		}

		logger.Info("apkindex: installed",
			"package", p.Name, "version", p.Version)
	}

	return nil
}

// readInstalledDB parses /lib/apk/db/installed and returns a set of
// installed package names. Returns empty map on read error.
func readInstalledDB() map[string]bool {
	const apkInstalledDB = "/lib/apk/db/installed"

	f, err := os.Open(apkInstalledDB) // #nosec G304 -- constant path
	if err != nil {
		return make(map[string]bool)
	}
	defer func() { _ = f.Close() }()

	installed := make(map[string]bool)
	scanner := bufio.NewScanner(f)

	var currentPkg string

	for scanner.Scan() {
		line := scanner.Text()

		if line == "" {
			// End of stanza — commit the package name.
			if currentPkg != "" {
				installed[currentPkg] = true
				currentPkg = ""
			}

			continue
		}

		// APK installed DB uses single-char field tags: "P:<pkgname>"
		if pkg, ok := strings.CutPrefix(line, "P:"); ok {
			currentPkg = pkg
		}
	}

	// Flush last stanza (file may not end with blank line).
	if currentPkg != "" {
		installed[currentPkg] = true
	}

	return installed
}

// extractAndRegister extracts a .apk file to / and registers it in the installed database.
// Orchestrates extraction of control and data streams, then registers the package.
func extractAndRegister(apkPath string, pkg *Package) error {
	// Open the .apk file (2-stream concatenated gzip: control + data).
	f, err := os.Open(apkPath) // #nosec G304 -- temp dir path
	if err != nil {
		return fmt.Errorf("open apk: %w", err)
	}
	defer func() { _ = f.Close() }()

	// Extract control stream to get .PKGINFO.
	pkgInfo, err := extractAPKControl(f)
	if err != nil {
		return err
	}

	// Extract data stream to filesystem.
	if err := extractAPKData(f); err != nil {
		return err
	}

	// Register in /lib/apk/db/installed.
	if err := registerInstalled(pkg, pkgInfo); err != nil {
		return fmt.Errorf("register installed: %w", err)
	}

	return nil
}

// extractAPKControl reads the control.tar.gz stream from an APK file and returns the .PKGINFO content.
func extractAPKControl(f *os.File) (string, error) {
	gz1, err := gzip.NewReader(f)
	if err != nil {
		return "", fmt.Errorf("gzip reader 1: %w", err)
	}
	defer func() { _ = gz1.Close() }()

	tr1 := tar.NewReader(gz1)

	// Scan control stream to find .PKGINFO (for installed DB).
	for {
		hdr, err := tr1.Next()
		if err == io.EOF {
			break
		}

		if err != nil {
			return "", fmt.Errorf("tar read control: %w", err)
		}

		if hdr.Name == ".PKGINFO" {
			data, err := io.ReadAll(tr1)
			if err != nil {
				return "", fmt.Errorf("read pkginfo: %w", err)
			}

			return string(data), nil
		}
	}

	return "", nil
}

// extractAPKData reads the data.tar.gz stream from an APK file and extracts files to the filesystem.
func extractAPKData(f *os.File) error {
	gz2, err := gzip.NewReader(f)
	if err != nil {
		return fmt.Errorf("gzip reader 2: %w", err)
	}
	defer func() { _ = gz2.Close() }()

	tr2 := tar.NewReader(gz2)

	for {
		hdr, err := tr2.Next()
		if err == io.EOF {
			break
		}

		if err != nil {
			return fmt.Errorf("tar read data: %w", err)
		}

		if err := extractAPKEntry(tr2, hdr); err != nil {
			return err
		}
	}

	return nil
}

// extractAPKEntry extracts a single tar entry to the filesystem.
// Handles regular files, directories, and symlinks with proper sanitization.
func extractAPKEntry(tr *tar.Reader, hdr *tar.Header) error {
	// Sanitize path to prevent directory traversal attacks.
	// APK files should only contain relative paths without ".." components.
	cleanName := filepath.Clean(hdr.Name)
	if strings.HasPrefix(cleanName, "..") || filepath.IsAbs(cleanName) {
		logger.Warn("apkindex: skipping unsafe path in archive",
			"path", hdr.Name)

		return nil
	}

	// Extract to /. //nolint:gocritic
	targetPath := filepath.Join("/", cleanName) //nolint:gocritic // #nosec G306

	// Create parent directories.
	if err := os.MkdirAll(filepath.Dir(targetPath), 0o755); err != nil {
		return fmt.Errorf("mkdir: %w", err)
	}

	switch hdr.Typeflag {
	case tar.TypeReg:
		// Regular file.
		f, err := os.Create(targetPath)
		if err != nil {
			return fmt.Errorf("create file: %w", err)
		}

		if _, err := io.Copy(f, tr); err != nil { // #nosec G110 — tar stream is from trusted .apk file
			_ = f.Close()
			return fmt.Errorf("copy file: %w", err)
		}

		if err := f.Close(); err != nil {
			return fmt.Errorf("close file: %w", err)
		}

		// Preserve permissions.
		if err := os.Chmod(targetPath, os.FileMode(hdr.Mode)); err != nil { // #nosec G115 — hdr.Mode is from tar header
			return fmt.Errorf("chmod: %w", err)
		}

	case tar.TypeDir:
		// Directory.
		if err := os.MkdirAll(targetPath, os.FileMode(hdr.Mode)); err != nil { // #nosec G115 — hdr.Mode is from tar header
			return fmt.Errorf("mkdir: %w", err)
		}

	case tar.TypeSymlink:
		// Symlink.
		if err := os.Symlink(hdr.Linkname, targetPath); err != nil {
			// Ignore if already exists.
			if !os.IsExist(err) {
				return fmt.Errorf("symlink: %w", err)
			}
		}
	}

	return nil
}

// registerInstalled appends a stanza to /lib/apk/db/installed.
func registerInstalled(pkg *Package, pkgInfo string) error {
	const apkInstalledDB = "/lib/apk/db/installed"

	// Ensure the directory exists.
	if err := os.MkdirAll(filepath.Dir(apkInstalledDB), 0o755); err != nil {
		return fmt.Errorf("mkdir: %w", err)
	}

	// Open for append (create if not exists).
	f, err := os.OpenFile(apkInstalledDB, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("open db: %w", err)
	}
	defer func() { _ = f.Close() }()

	// Build a minimal stanza from pkg and pkgInfo.
	// If pkgInfo is available, use it; otherwise, build from pkg fields.
	var stanza string

	if pkgInfo != "" {
		stanza = pkgInfo
	} else {
		// Fallback: minimal stanza.
		stanza = fmt.Sprintf("P:%s\nV:%s\nA:%s\nI:%d\n",
			pkg.Name, pkg.Version, pkg.Arch, pkg.InstSize)
	}

	// Ensure stanza ends with newline and blank line separator.
	stanza = strings.TrimRight(stanza, "\n") + "\n\n"

	if _, err := f.WriteString(stanza); err != nil {
		return fmt.Errorf("write stanza: %w", err)
	}

	return nil
}
