// SPDX-FileCopyrightText: 2021-2026 Gianluca Boiano
// SPDX-License-Identifier: GPL-3.0-only

package apkindex

import (
	"archive/tar"
	"bufio"
	"context"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	pkgapk "github.com/M0Rf30/pkgformat/apk"
	"github.com/M0Rf30/pkgformat/safepath"
	"github.com/klauspost/compress/gzip"

	"github.com/M0Rf30/yap/v2/pkg/errors"
	"github.com/M0Rf30/yap/v2/pkg/i18n"
	"github.com/M0Rf30/yap/v2/pkg/logger"
)

const apkInstalledDB = "/lib/apk/db/installed"

// InstallPackages resolves the requested packages and their transitive deps,
// downloads each one (checked against its index entry), extracts it to /, and
// updates /lib/apk/db/installed. Replaces "apk add".
func (r *Repos) InstallPackages(ctx context.Context, names []string) error {
	// 1. Resolve transitive deps.
	resolution, err := r.Index.ResolveDeps(names)
	if err != nil {
		return errors.Wrap(err, errors.ErrTypeValidation, "failed to resolve dependencies").
			WithOperation("InstallPackages")
	}

	for _, capability := range resolution.Virtual {
		logger.Debug(i18n.T("logger.apkindex.debug.resolved_virtual"), "capability", capability)
	}

	for _, name := range resolution.Unresolved {
		// Skipped, like apk does: it rejects them at install time if they matter.
		logger.Debug(i18n.T("logger.apkindex.debug.unresolved"), "package", name)
	}

	logger.Info(i18n.T("logger.apkindex.info.resolved_transitive_deps"), "seeds", len(names),
		"to_install", len(resolution.Packages),
		"via_virtual", len(resolution.Virtual),
		"unresolved", len(resolution.Unresolved))

	if len(resolution.Packages) == 0 {
		return nil
	}

	// 2. Filter out already-installed packages.
	installed := readInstalledDB()

	var toInstall []*pkgapk.RepoPackage

	for _, p := range resolution.Packages {
		if !installed[p.Name] {
			toInstall = append(toInstall, p)
		}
	}

	if len(toInstall) == 0 {
		logger.Info(i18n.T("logger.apkindex.info.all_packages_already_installed"), "requested", len(names),
			"resolved", len(resolution.Packages))

		return nil
	}

	var totalBytes int64
	for _, p := range toInstall {
		totalBytes += p.Size
	}

	logger.Info(i18n.T("logger.apkindex.info.installing_apk_packages"), "to_install", len(toInstall),
		"resolved", len(resolution.Packages),
		"skipped_installed", len(resolution.Packages)-len(toInstall),
		"total_bytes", totalBytes)

	// 3. Download all .apk files to a temp dir.
	tmpDir, err := os.MkdirTemp("", "yap-apk-*")
	if err != nil {
		return errors.Wrap(err, errors.ErrTypeFileSystem, "failed to create temporary directory").
			WithOperation("InstallPackages")
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	paths, err := r.client.Download(ctx, tmpDir, toInstall...)
	if err != nil {
		return errors.Wrap(err, errors.ErrTypeNetwork, "failed to download packages").
			WithOperation("InstallPackages")
	}

	// 4. Extract each .apk to / and register in installed DB.
	for i, p := range toInstall {
		if err := extractAndRegister(paths[i], p); err != nil {
			return errors.Wrap(err, errors.ErrTypePackaging, "failed to install package").
				WithOperation("InstallPackages").
				WithContext("package", p.Name)
		}

		logger.Debug(i18n.T("logger.apkindex.debug.installed"), "package", p.Name, "version", p.Version)
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
// The file was checked against its index entry by the client, so it is a valid
// package of the size, checksum, name and version the index lists.
func extractAndRegister(apkPath string, pkg *pkgapk.RepoPackage) error {
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

	if err := skipControl(br, pkg); err != nil {
		return err
	}

	// Now br is positioned at the data.tar.gz stream.
	if err := extractAPKData(br); err != nil {
		return err
	}

	// Register in /lib/apk/db/installed.
	if err := registerInstalled(pkg); err != nil {
		return errors.Wrap(err, errors.ErrTypeFileSystem, "failed to register installed package").
			WithOperation("extractAndRegister").
			WithContext("package", pkg.Name)
	}

	return nil
}

// byteReader is the reader shape gzip needs to avoid read-ahead buffering.
type byteReader interface {
	io.Reader
	io.ByteReader
}

// skipControl consumes the optional signature stream and the control stream
// (the one holding .PKGINFO), leaving r at the start of the data stream.
func skipControl(r byteReader, pkg *pkgapk.RepoPackage) error {
	for range 2 {
		found, err := skipStream(r)
		if err != nil {
			return err
		}

		if found {
			return nil
		}
	}

	return errors.New(errors.ErrTypeParser, "APK control stream with .PKGINFO not found").
		WithOperation("skipControl").
		WithContext("package", pkg.Name)
}

// skipStream reads the next gzip stream from br and reports whether its tar
// archive holds .PKGINFO (false for the signature stream). It drains the stream
// so br is positioned at the next member's magic bytes.
func skipStream(br byteReader) (bool, error) {
	gz, err := gzip.NewReader(br)
	if err != nil {
		return false, errors.Wrap(err, errors.ErrTypeParser, "failed to create gzip reader").
			WithOperation("skipStream")
	}
	defer func() { _ = gz.Close() }()

	// One gzip member per stream — don't auto-advance into the next stream.
	gz.Multistream(false)

	tr := tar.NewReader(gz)

	found := false

	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}

		if err != nil {
			return false, errors.Wrap(err, errors.ErrTypeParser, "failed to read tar entry").
				WithOperation("skipStream")
		}

		if hdr.Name == ".PKGINFO" {
			found = true
		}
	}

	// CRITICAL: drain remaining bytes of this gzip member so br is
	// positioned at the start of the next member's magic. Cap at 16 MiB
	// to defend against decompression bombs.
	_, _ = io.Copy(io.Discard, io.LimitReader(gz, 16<<20))

	return found, nil
}

// extractAPKData reads the data.tar.gz stream from an APK file and extracts files to /.
func extractAPKData(r io.Reader) error {
	return extractAPKDataTo(r, "/")
}

// extractAPKDataTo is extractAPKData with an explicit destination root.
func extractAPKDataTo(r io.Reader, root string) error {
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

		if err := extractAPKEntryAt(tr2, hdr, root); err != nil {
			return err
		}
	}

	return nil
}

// safeAPKPath joins "/" with a sanitised tar-entry name, rejecting
// traversal attempts and absolute entry names (APK tarballs always use
// relative member names). Containment logic lives in github.com/M0Rf30/pkgformat/safepath.
func safeAPKPath(entryName string) (string, bool) {
	if filepath.IsAbs(entryName) {
		return "", false
	}

	p, err := safepath.JoinStrict("/", entryName)
	if err != nil {
		return "", false
	}

	return p, true
}

// resolveAPKEntry maps an archive member name to a host path under root.
// With root "/" this is safeAPKPath. Otherwise every component of the
// directory part (or of the whole name when full is set) is resolved with
// chroot semantics so pre-existing symlinks cannot redirect the write.
func resolveAPKEntry(root, name string, full bool) (string, bool) {
	abs, ok := safeAPKPath(name)
	if !ok {
		return "", false
	}

	if root == "/" {
		return abs, true
	}

	if full {
		p, err := safepath.ResolveInRoot(root, abs)
		return p, err == nil
	}

	parent, err := safepath.ResolveInRoot(root, filepath.Dir(abs))
	if err != nil {
		return "", false
	}

	return filepath.Join(parent, filepath.Base(abs)), true
}

// safeAPKSymlinkTarget rejects symlink targets that would escape the
// filesystem root via "..". Absolute targets are permitted because APK
// packages commonly ship absolute symlinks (/usr/bin/foo → /usr/bin/bar).
// Containment logic lives in github.com/M0Rf30/pkgformat/safepath.
func safeAPKSymlinkTarget(linkPath, target string) error {
	return safepath.SymlinkTarget("/", linkPath, target)
}

// extractAPKEntryAt extracts a single tar entry below root.
// Handles regular files, directories, and symlinks with proper sanitization.
func extractAPKEntryAt(tr *tar.Reader, hdr *tar.Header, root string) error {
	targetPath, ok := resolveAPKEntry(root, hdr.Name, hdr.Typeflag == tar.TypeDir)
	if !ok {
		logger.Warn(i18n.T("logger.apkindex.warn.skipping_unsafe_path_apk"), "path", hdr.Name)

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
		return extractAPKRegular(tr, hdr, targetPath)

	case tar.TypeDir:
		return extractAPKDir(hdr, targetPath)

	case tar.TypeSymlink:
		if err := safeAPKSymlinkTarget(targetPath, hdr.Linkname); err != nil {
			logger.Warn(i18n.T("logger.apkindex.warn.skipping_unsafe_apk_symlink"),
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

	case tar.TypeLink:
		// Hardlink. APK uses these for multi-call binaries: the g++
		// package ships usr/bin/c++ as the regular file and usr/bin/g++,
		// usr/bin/x86_64-alpine-linux-musl-g++ as hardlinks to it.
		// Dropping them left build-base "installed" without a g++.
		linkSrc, ok := resolveAPKEntry(root, hdr.Linkname, true)
		if !ok {
			logger.Warn(i18n.T("logger.apkindex.warn.skipping_unsafe_path_apk"), "path", hdr.Linkname)

			return nil
		}

		return hardlinkOrCopy(linkSrc, targetPath, hdr.FileInfo().Mode())
	}

	return nil
}

// extractAPKDir creates a directory entry, preserving special mode bits.
func extractAPKDir(hdr *tar.Header, targetPath string) error {
	mode := hdr.FileInfo().Mode()

	if err := os.MkdirAll(targetPath, mode.Perm()); err != nil {
		return errors.Wrap(err, errors.ErrTypeFileSystem, "failed to create directory").
			WithOperation("extractAPKEntry").
			WithContext("path", targetPath)
	}

	if mode&(os.ModeSetuid|os.ModeSetgid|os.ModeSticky) != 0 {
		if err := os.Chmod(targetPath, mode); err != nil {
			return errors.Wrap(err, errors.ErrTypeFileSystem, "failed to set directory permissions").
				WithOperation("extractAPKEntry").
				WithContext("path", targetPath)
		}
	}

	return nil
}

// extractAPKRegular writes a regular file entry to targetPath.
//
// The payload goes to a sibling temp file that is then atomically renamed
// over the target. This avoids ETXTBSY ("text file busy") when overwriting
// a binary that is currently being executed — the kernel allows unlinking
// a running binary but rejects truncate/write on it. Busybox-style images
// symlink many tools to /bin/busybox, and /bin/tar (the very tool the
// install pipeline shells out to elsewhere) is one of them, so this
// matters in practice.
func extractAPKRegular(tr *tar.Reader, hdr *tar.Header, targetPath string) error {
	// Cap per-file size at 2 GiB.
	const maxFileSize = 2 << 30

	tmpPath := targetPath + ".apk-new"

	f, err := os.Create(tmpPath) //nolint:gosec
	if err != nil {
		return errors.Wrap(err, errors.ErrTypeFileSystem, "failed to create temporary file").
			WithOperation("extractAPKRegular").
			WithContext("path", tmpPath)
	}

	if _, err := io.Copy(f, io.LimitReader(tr, maxFileSize)); err != nil {
		_ = f.Close()
		_ = os.Remove(tmpPath)

		return errors.Wrap(err, errors.ErrTypeFileSystem, "failed to copy file contents").
			WithOperation("extractAPKRegular").
			WithContext("path", tmpPath)
	}

	if err := f.Close(); err != nil {
		_ = os.Remove(tmpPath)

		return errors.Wrap(err, errors.ErrTypeFileSystem, "failed to close file").
			WithOperation("extractAPKRegular").
			WithContext("path", tmpPath)
	}

	// Preserve permissions before the rename so the file is in its final
	// state when it becomes visible at targetPath.
	if err := os.Chmod(tmpPath, hdr.FileInfo().Mode()); err != nil {
		_ = os.Remove(tmpPath)

		return errors.Wrap(err, errors.ErrTypeFileSystem, "failed to set file permissions").
			WithOperation("extractAPKRegular").
			WithContext("path", tmpPath)
	}

	if err := os.Rename(tmpPath, targetPath); err != nil {
		_ = os.Remove(tmpPath)

		return errors.Wrap(err, errors.ErrTypeFileSystem, "failed to rename file").
			WithOperation("extractAPKRegular").
			WithContext("from", tmpPath).
			WithContext("to", targetPath)
	}

	return nil
}

// hardlinkOrCopy links linkSrc to targetPath, falling back to a copy.
func hardlinkOrCopy(linkSrc, targetPath string, mode os.FileMode) error {
	_ = os.Remove(targetPath)

	if err := os.Link(linkSrc, targetPath); err == nil {
		return nil
	}

	in, err := os.Open(linkSrc) //nolint:gosec // path validated by caller
	if err != nil {
		return errors.Wrap(err, errors.ErrTypeFileSystem, "failed to open hardlink target").
			WithOperation("extractAPKHardlink").
			WithContext("target", linkSrc)
	}

	defer func() { _ = in.Close() }()

	out, err := os.OpenFile(targetPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, mode.Perm()) //nolint:gosec
	if err != nil {
		return errors.Wrap(err, errors.ErrTypeFileSystem, "failed to create hardlink copy").
			WithOperation("extractAPKHardlink").
			WithContext("path", targetPath)
	}

	if _, err := io.Copy(out, in); err != nil {
		_ = out.Close()

		return errors.Wrap(err, errors.ErrTypeFileSystem, "failed to copy hardlink target").
			WithOperation("extractAPKHardlink").
			WithContext("path", targetPath)
	}

	if err := out.Close(); err != nil {
		return err //nolint:wrapcheck
	}

	// OpenFile applies the umask and drops setuid/setgid/sticky.
	if err := os.Chmod(targetPath, mode); err != nil {
		return errors.Wrap(err, errors.ErrTypeFileSystem, "failed to set file permissions").
			WithOperation("extractAPKHardlink").
			WithContext("path", targetPath)
	}

	return nil
}

// registerInstalled writes a package stanza into /lib/apk/db/installed.
// On reinstall/upgrade the existing stanza for the same package name is
// replaced rather than appended (which would leak duplicate entries).
func registerInstalled(pkg *pkgapk.RepoPackage) error {
	return registerInstalledAt(apkInstalledDB, pkg)
}

// registerInstalledAt is registerInstalled parameterized by the installed-db
// path. The exported behaviour uses the package-level apkInstalledDB constant;
// tests drive it against a temp path.
func registerInstalledAt(dbPath string, pkg *pkgapk.RepoPackage) error {
	if err := os.MkdirAll(filepath.Dir(dbPath), 0o755); err != nil {
		return errors.Wrap(err, errors.ErrTypeFileSystem, "failed to create database directory").
			WithOperation("registerInstalled").
			WithContext("path", filepath.Dir(dbPath))
	}

	// Read existing DB → stanza map keyed by package name.
	existing := readInstalledStanzasAt(dbPath)

	existing[pkg.Name] = buildInstalledStanza(pkg)

	return writeInstalledStanzasAt(dbPath, existing)
}

// readInstalledStanzasAt parses the installed db at dbPath into a map of
// package-name → raw stanza text (newline-terminated, no trailing blank).
func readInstalledStanzasAt(dbPath string) map[string]string {
	f, err := os.Open(dbPath) //nolint:gosec // caller-controlled db path (constant in prod, temp in tests)
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

// writeInstalledStanzasAt writes the stanza map to the installed-db at dbPath
// atomically. Stanzas are written in sorted name order so the output is
// reproducible. Production uses the package-level constant; tests pass a
// temp path.
func writeInstalledStanzasAt(dbPath string, stanzas map[string]string) error {
	tmpPath := dbPath + ".tmp"

	//nolint:gosec // G304: caller-controlled db path (constant in prod, temp in tests)
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

	if err := os.Rename(tmpPath, dbPath); err != nil {
		_ = os.Remove(tmpPath)

		return errors.Wrap(err, errors.ErrTypeFileSystem, "failed to rename database file").
			WithOperation("writeInstalledStanzas").
			WithContext("from", tmpPath).
			WithContext("to", dbPath)
	}

	return nil
}

// buildInstalledStanza synthesises an installed-db stanza (single-letter
// tags, as read by readInstalledStanzasAt) from the index entry.
func buildInstalledStanza(pkg *pkgapk.RepoPackage) string {
	var sb strings.Builder

	add := func(tag, val string) {
		if val != "" {
			sb.WriteString(tag + ":" + val + "\n")
		}
	}

	add("P", pkg.Name)
	add("V", pkg.Version)
	add("A", pkg.Arch)
	add("S", strconv.FormatInt(pkg.Size, 10))
	add("I", strconv.FormatInt(pkg.InstalledSize, 10))
	add("T", pkg.Description)
	add("U", pkg.Package.URL)
	add("L", pkg.License)
	add("o", pkg.Origin)
	add("m", pkg.Maintainer)
	add("D", strings.Join(pkg.Depends, " "))
	add("p", strings.Join(pkg.Provides, " "))

	return sb.String()
}
