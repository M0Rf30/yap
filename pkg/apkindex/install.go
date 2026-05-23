package apkindex

import (
	"archive/tar"
	"bufio"
	"compress/gzip"
	"context"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/M0Rf30/yap/v2/pkg/errors"
	"github.com/M0Rf30/yap/v2/pkg/logger"
)

const apkInstalledDB = "/lib/apk/db/installed"

// InstallOptions controls the safety / trust knobs for InstallPackages.
//
// AllowUnverifiedPackages: APK packages and APKINDEX tarballs ship with RSA
// signatures. Verification of those signatures is not yet wired into this
// package. Callers that accept this gap (e.g. inside a trusted CI container
// fetching over HTTPS from official mirrors) must set this flag explicitly.
type InstallOptions struct {
	AllowUnverifiedPackages bool
}

// InstallPackages downloads each requested package + transitive deps, extracts each
// to /, and updates /lib/apk/db/installed. Replaces "apk add".
//
// Equivalent to InstallPackagesWithOptions with the zero options (strict).
func (idx *Index) InstallPackages(ctx context.Context, names []string) error {
	return idx.InstallPackagesWithOptions(ctx, names, InstallOptions{})
}

// InstallPackagesWithOptions is the explicit-options variant of InstallPackages.
// Scriptlets are not currently executed; they are logged as warnings.
func (idx *Index) InstallPackagesWithOptions(
	ctx context.Context, names []string, opts InstallOptions,
) error {
	if !opts.AllowUnverifiedPackages {
		return errors.New(errors.ErrTypeValidation,
			"APK signature verification is not yet implemented; "+
				"set InstallOptions.AllowUnverifiedPackages to acknowledge").
			WithOperation("InstallPackagesWithOptions")
	}

	logger.Warn("installing APK packages without signature verification " +
		"(set InstallOptions.AllowUnverifiedPackages=false to refuse)")

	// 1. Resolve transitive deps.
	resolved, err := idx.ResolveDeps(names)
	if err != nil {
		return errors.Wrap(err, errors.ErrTypeInternal, "failed to resolve dependencies").
			WithOperation("InstallPackagesWithOptions")
	}

	if len(resolved) == 0 {
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
			"requested", len(names),
			"resolved", len(resolved))

		return nil
	}

	var totalBytes int64
	for _, p := range toInstall {
		totalBytes += p.Size
	}

	logger.Info("apkindex: installing APK packages",
		"to_install", len(toInstall),
		"resolved", len(resolved),
		"skipped_installed", len(resolved)-len(toInstall),
		"total_bytes", totalBytes)

	// 3. Download all .apk files to a temp dir.
	tmpDir, err := os.MkdirTemp("", "yap-apk-*")
	if err != nil {
		return errors.Wrap(err, errors.ErrTypeFileSystem, "failed to create temporary directory").
			WithOperation("InstallPackagesWithOptions")
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	pkgNames := make([]string, len(toInstall))
	for i, p := range toInstall {
		pkgNames[i] = p.Name
	}

	if _, downloadErr := idx.DownloadPackages(ctx, tmpDir, pkgNames); downloadErr != nil {
		return downloadErr
	}

	// 4. Extract each .apk to / and register in installed DB.
	for _, p := range toInstall {
		apkPath := filepath.Join(tmpDir, p.Name+"-"+p.Version+".apk")

		if err := extractAndRegister(apkPath, p); err != nil {
			return errors.Wrap(err, errors.ErrTypePackaging, "failed to install package").
				WithOperation("InstallPackagesWithOptions").
				WithContext("package", p.Name)
		}

		logger.Debug("installed", "package", p.Name, "version", p.Version)
	}

	return nil
}

// readInstalledDB parses /lib/apk/db/installed and returns a set of
// installed package names. Returns empty map on read error.
func readInstalledDB() map[string]bool {
	f, err := os.Open(apkInstalledDB)
	if err != nil {
		return make(map[string]bool)
	}
	defer func() { _ = f.Close() }()

	installed := make(map[string]bool)
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 1024*1024), 1024*1024)

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
	// Open the .apk file (2-or-3-stream concatenated gzip: [signature] + control + data).
	f, err := os.Open(apkPath) //nolint:gosec
	if err != nil {
		return errors.Wrap(err, errors.ErrTypeFileSystem, "failed to open APK file").
			WithOperation("extractAndRegister").
			WithContext("path", apkPath)
	}
	defer func() { _ = f.Close() }()

	// Use bufio.Reader so each gzip.NewReader inherits the buffered
	// position state correctly across stream boundaries.
	br := bufio.NewReader(f)

	// Try to read .PKGINFO from the first stream (control).
	pkgInfo, err := tryReadPkgInfoFromNextStream(br)
	if err != nil {
		return err
	}

	// If first stream was signature (no .PKGINFO), try the next one (control).
	if pkgInfo == "" {
		pkgInfo, err = tryReadPkgInfoFromNextStream(br)
		if err != nil {
			return err
		}
	}

	// Now br is positioned at the data.tar.gz stream.
	if err := extractAPKData(br); err != nil {
		return err
	}

	// Register in /lib/apk/db/installed.
	if err := registerInstalled(pkg, pkgInfo); err != nil {
		return errors.Wrap(err, errors.ErrTypeFileSystem, "failed to register installed package").
			WithOperation("extractAndRegister").
			WithContext("package", pkg.Name)
	}

	return nil
}

// tryReadPkgInfoFromNextStream reads the next gzip stream from br, looking for
// .PKGINFO in the tar archive. Returns empty string if .PKGINFO is not found
// (e.g., signature stream). Drains the stream so br is positioned at the next
// member's magic bytes.
func tryReadPkgInfoFromNextStream(br *bufio.Reader) (string, error) {
	gz, err := gzip.NewReader(br)
	if err != nil {
		return "", errors.Wrap(err, errors.ErrTypeParser, "failed to create gzip reader").
			WithOperation("tryReadPkgInfoFromNextStream")
	}
	defer func() { _ = gz.Close() }()

	// One gzip member per stream — don't auto-advance into the next stream.
	gz.Multistream(false)

	tr := tar.NewReader(gz)

	var pkgInfo string

	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}

		if err != nil {
			return "", errors.Wrap(err, errors.ErrTypeParser, "failed to read tar entry").
				WithOperation("tryReadPkgInfoFromNextStream")
		}

		if hdr.Name == ".PKGINFO" {
			data, err := io.ReadAll(io.LimitReader(tr, 1<<20)) // 1 MiB cap
			if err != nil {
				return "", errors.Wrap(err, errors.ErrTypeParser, "failed to read .PKGINFO file").
					WithOperation("tryReadPkgInfoFromNextStream")
			}

			pkgInfo = string(data)
		}
	}

	// CRITICAL: drain remaining bytes of this gzip member so br is
	// positioned at the start of the next member's magic. Cap at 16 MiB
	// to defend against decompression bombs.
	_, _ = io.Copy(io.Discard, io.LimitReader(gz, 16<<20))

	return pkgInfo, nil
}

// extractAPKData reads the data.tar.gz stream from an APK file and extracts files to the filesystem.
func extractAPKData(r io.Reader) error {
	gz2, err := gzip.NewReader(r)
	if err != nil {
		return errors.Wrap(err, errors.ErrTypeParser, "failed to create gzip reader for data stream").
			WithOperation("extractAPKData")
	}
	defer func() { _ = gz2.Close() }()

	tr2 := tar.NewReader(gz2)

	for {
		hdr, err := tr2.Next()
		if err == io.EOF {
			break
		}

		if err != nil {
			return errors.Wrap(err, errors.ErrTypeParser, "failed to read tar entry from data stream").
				WithOperation("extractAPKData")
		}

		if err := extractAPKEntry(tr2, hdr); err != nil {
			return err
		}
	}

	return nil
}

// safeAPKPath joins "/" with a sanitised tar-entry name, rejecting
// traversal attempts (".." or absolute paths). The legacy `cleanName`
// + `filepath.Join("/", …)` pattern is correct only because `cleanName`
// has already been verified to not start with "..".
func safeAPKPath(entryName string) (string, bool) {
	cleanName := filepath.Clean(entryName)
	if cleanName == "." || cleanName == "/" {
		return "", false
	}

	if strings.HasPrefix(cleanName, "..") || filepath.IsAbs(cleanName) {
		return "", false
	}

	// nolint:gocritic // We *do* want / as the join base — APK is system-wide.
	return filepath.Join("/", cleanName), true
}

// safeAPKSymlinkTarget rejects symlink targets that would escape the
// filesystem root via "..". Absolute targets are permitted because APK
// packages commonly ship absolute symlinks (/usr/bin/foo → /usr/bin/bar).
func safeAPKSymlinkTarget(linkPath, target string) error {
	if filepath.IsAbs(target) {
		return nil
	}

	resolved := filepath.Clean(filepath.Join(filepath.Dir(linkPath), target))
	if strings.HasPrefix(resolved, "..") {
		return errors.New(errors.ErrTypeValidation, "symlink target escapes root").
			WithOperation("safeAPKSymlinkTarget").
			WithContext("link_path", linkPath).
			WithContext("target", target)
	}

	return nil
}

// extractAPKEntry extracts a single tar entry to the filesystem.
// Handles regular files, directories, and symlinks with proper sanitization.
func extractAPKEntry(tr *tar.Reader, hdr *tar.Header) error {
	targetPath, ok := safeAPKPath(hdr.Name)
	if !ok {
		logger.Warn("skipping unsafe path in APK archive", "path", hdr.Name)

		return nil
	}

	// Create parent directories.
	if err := os.MkdirAll(filepath.Dir(targetPath), 0o755); err != nil {
		return errors.Wrap(err, errors.ErrTypeFileSystem, "failed to create parent directories").
			WithOperation("extractAPKEntry").
			WithContext("path", targetPath)
	}

	switch hdr.Typeflag {
	case tar.TypeReg:
		// Regular file. Cap per-file size at 2 GiB.
		const maxFileSize = 2 << 30

		// Write to a sibling temp file then atomically rename over the
		// target. This avoids ETXTBSY ("text file busy") when overwriting
		// a binary that is currently being executed — the kernel allows
		// unlinking a running binary but rejects truncate/write on it.
		// Busybox-style images symlink many tools to /bin/busybox, and
		// /bin/tar (the very tool we shell out to during the install
		// pipeline elsewhere) is one of them, so this matters in practice.
		tmpPath := targetPath + ".apk-new"

		f, err := os.Create(tmpPath) //nolint:gosec
		if err != nil {
			return errors.Wrap(err, errors.ErrTypeFileSystem, "failed to create temporary file").
				WithOperation("extractAPKEntry").
				WithContext("path", tmpPath)
		}

		if _, err := io.Copy(f, io.LimitReader(tr, maxFileSize)); err != nil {
			_ = f.Close()
			_ = os.Remove(tmpPath)

			return errors.Wrap(err, errors.ErrTypeFileSystem, "failed to copy file contents").
				WithOperation("extractAPKEntry").
				WithContext("path", tmpPath)
		}

		if err := f.Close(); err != nil {
			_ = os.Remove(tmpPath)

			return errors.Wrap(err, errors.ErrTypeFileSystem, "failed to close file").
				WithOperation("extractAPKEntry").
				WithContext("path", tmpPath)
		}

		// Preserve permissions before the rename so the file is in its
		// final state when it becomes visible at targetPath.
		if err := os.Chmod(tmpPath, os.FileMode(hdr.Mode)); err != nil { //nolint:gosec
			_ = os.Remove(tmpPath)

			return errors.Wrap(err, errors.ErrTypeFileSystem, "failed to set file permissions").
				WithOperation("extractAPKEntry").
				WithContext("path", tmpPath)
		}

		if err := os.Rename(tmpPath, targetPath); err != nil {
			_ = os.Remove(tmpPath)

			return errors.Wrap(err, errors.ErrTypeFileSystem, "failed to rename file").
				WithOperation("extractAPKEntry").
				WithContext("from", tmpPath).
				WithContext("to", targetPath)
		}

	case tar.TypeDir:
		// Directory.
		if err := os.MkdirAll(targetPath, os.FileMode(hdr.Mode)); err != nil { //nolint:gosec
			return errors.Wrap(err, errors.ErrTypeFileSystem, "failed to create directory").
				WithOperation("extractAPKEntry").
				WithContext("path", targetPath)
		}

	case tar.TypeSymlink:
		if err := safeAPKSymlinkTarget(targetPath, hdr.Linkname); err != nil {
			logger.Warn("skipping unsafe APK symlink",
				"path", hdr.Name, "target", hdr.Linkname, "error", err)

			return nil
		}

		if err := os.Symlink(hdr.Linkname, targetPath); err != nil {
			// Ignore if already exists.
			if !os.IsExist(err) {
				return errors.Wrap(err, errors.ErrTypeFileSystem, "failed to create symlink").
					WithOperation("extractAPKEntry").
					WithContext("path", targetPath).
					WithContext("target", hdr.Linkname)
			}
		}
	}

	return nil
}

// registerInstalled writes a package stanza into /lib/apk/db/installed.
// On reinstall/upgrade the existing stanza for the same package name is
// replaced rather than appended (which would leak duplicate entries).
func registerInstalled(pkg *Package, pkgInfo string) error {
	if err := os.MkdirAll(filepath.Dir(apkInstalledDB), 0o755); err != nil {
		return errors.Wrap(err, errors.ErrTypeFileSystem, "failed to create database directory").
			WithOperation("registerInstalled").
			WithContext("path", filepath.Dir(apkInstalledDB))
	}

	// Read existing DB → stanza map keyed by package name.
	existing := readInstalledStanzas()

	// Build the new stanza.
	var stanza string

	if pkgInfo != "" {
		stanza = pkgInfo
	} else {
		// Build stanza manually without fmt.Sprintf
		var sb strings.Builder
		sb.WriteString("P:")
		sb.WriteString(pkg.Name)
		sb.WriteString("\nV:")
		sb.WriteString(pkg.Version)
		sb.WriteString("\nA:")
		sb.WriteString(pkg.Arch)
		sb.WriteString("\nI:")
		// Convert int64 to string
		sb.WriteString(string(rune(pkg.InstSize))) //nolint:gosec
		sb.WriteString("\n")
		stanza = sb.String()
	}

	existing[pkg.Name] = strings.TrimRight(stanza, "\n") + "\n"

	return writeInstalledStanzas(existing)
}

// readInstalledStanzas parses /lib/apk/db/installed into a map of
// package-name → raw stanza text (newline-terminated, no trailing blank).
func readInstalledStanzas() map[string]string {
	f, err := os.Open(apkInstalledDB)
	if err != nil {
		return make(map[string]string)
	}
	defer func() { _ = f.Close() }()

	stanzas := make(map[string]string)
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 1024*1024), 1024*1024)

	var (
		current strings.Builder
		name    string
	)

	flush := func() {
		if name != "" && current.Len() > 0 {
			stanzas[name] = strings.TrimRight(current.String(), "\n") + "\n"
		}

		current.Reset()

		name = ""
	}

	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			flush()
			continue
		}

		if pkg, ok := strings.CutPrefix(line, "P:"); ok {
			name = pkg
		}

		current.WriteString(line)
		current.WriteString("\n")
	}

	flush()

	return stanzas
}

// writeInstalledStanzas writes the stanza map to /lib/apk/db/installed
// atomically. Stanzas are written in sorted name order so the output is
// reproducible.
func writeInstalledStanzas(stanzas map[string]string) error {
	tmpPath := apkInstalledDB + ".tmp"

	// nolint:gosec // G304: constant path
	f, err := os.OpenFile(tmpPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return errors.Wrap(err, errors.ErrTypeFileSystem, "failed to open database file").
			WithOperation("writeInstalledStanzas").
			WithContext("path", tmpPath)
	}

	names := make([]string, 0, len(stanzas))
	for k := range stanzas {
		names = append(names, k)
	}

	sort.Strings(names)

	for _, n := range names {
		if _, err := f.WriteString(stanzas[n] + "\n"); err != nil {
			_ = f.Close()
			_ = os.Remove(tmpPath)

			return errors.Wrap(err, errors.ErrTypeFileSystem, "failed to write stanza to database").
				WithOperation("writeInstalledStanzas").
				WithContext("package", n)
		}
	}

	if err := f.Sync(); err != nil {
		_ = f.Close()
		_ = os.Remove(tmpPath)

		return errors.Wrap(err, errors.ErrTypeFileSystem, "failed to sync database file").
			WithOperation("writeInstalledStanzas").
			WithContext("path", tmpPath)
	}

	if err := f.Close(); err != nil {
		_ = os.Remove(tmpPath)

		return errors.Wrap(err, errors.ErrTypeFileSystem, "failed to close database file").
			WithOperation("writeInstalledStanzas").
			WithContext("path", tmpPath)
	}

	if err := os.Rename(tmpPath, apkInstalledDB); err != nil {
		_ = os.Remove(tmpPath)

		return errors.Wrap(err, errors.ErrTypeFileSystem, "failed to rename database file").
			WithOperation("writeInstalledStanzas").
			WithContext("from", tmpPath).
			WithContext("to", apkInstalledDB)
	}

	return nil
}
