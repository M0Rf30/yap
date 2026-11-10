// SPDX-FileCopyrightText: 2021-2026 Gianluca Boiano
// SPDX-License-Identifier: GPL-3.0-only

package apkindex

import (
	"io"
	"path/filepath"

	pkgapk "github.com/M0Rf30/pkgformat/apk"
)

// This file exports internal functions and variables for testing purposes.

// SetGlobalRepos sets the global repository cache for testing purposes.
// Call with nil to reset the cache.
func SetGlobalRepos(r *Repos) {
	globalRepos.Store(r)
}

// ExportSafeAPKPath exposes safeAPKPath for testing.
func ExportSafeAPKPath(entryName string) (string, bool) {
	return safeAPKPath(entryName)
}

// ExportSafeAPKSymlinkTarget exposes safeAPKSymlinkTarget for testing.
func ExportSafeAPKSymlinkTarget(linkPath, target string) error {
	return safeAPKSymlinkTarget(linkPath, target)
}

// ExportExtractAPKDataTo exposes extractAPKDataTo for testing.
func ExportExtractAPKDataTo(r io.Reader, root string) error {
	return extractAPKDataTo(r, root)
}

// ExportReadInstalledDB exposes readInstalledDB for testing.
func ExportReadInstalledDB() map[string]bool {
	return readInstalledDB()
}

// ExportWriteInstalledStanzas exposes writeInstalledStanzasAt for testing with a custom path.
func ExportWriteInstalledStanzas(dbPath string, stanzas map[string]string) error {
	return writeInstalledStanzasAt(dbPath, stanzas)
}

// ExportRegisterInstalled exposes registerInstalledAt for testing. The dbPath
// mirrors the on-disk layout (<tmpDir>/lib/apk/db/installed) used by the
// production constant so existing test expectations still hold.
func ExportRegisterInstalled(tmpDir string, pkg *pkgapk.RepoPackage) error {
	dbPath := filepath.Join(tmpDir, "lib", "apk", "db", "installed")
	return registerInstalledAt(dbPath, pkg)
}

// ExportBuildInstalledStanza exposes buildInstalledStanza for testing.
func ExportBuildInstalledStanza(pkg *pkgapk.RepoPackage) string {
	return buildInstalledStanza(pkg)
}
