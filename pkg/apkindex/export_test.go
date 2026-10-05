// SPDX-FileCopyrightText: 2021-2026 Gianluca Boiano
// SPDX-License-Identifier: GPL-3.0-only

package apkindex

import (
	"archive/tar"
	"bufio"
	"context"
	"crypto/sha1" //nolint:gosec
	"io"
	"path/filepath"
)

// This file exports internal functions and variables for testing purposes.

// NewIndexForTesting creates a new Index for testing.
func NewIndexForTesting() *Index {
	return NewIndex()
}

// ParseReposContent exposes the internal parseReposContent function for testing.
var ParseReposContent = parseReposContent

// SetGlobalIndex sets the global index cache for testing purposes.
// Call with nil to reset the cache.
func SetGlobalIndex(idx *Index) {
	globalIndex.Store(idx)
}

// ExportSafeAPKPath exposes safeAPKPath for testing.
func ExportSafeAPKPath(entryName string) (string, bool) {
	return safeAPKPath(entryName)
}

// ExportSafeAPKSymlinkTarget exposes safeAPKSymlinkTarget for testing.
func ExportSafeAPKSymlinkTarget(linkPath, target string) error {
	return safeAPKSymlinkTarget(linkPath, target)
}

// ExportExtractAPKEntry exposes extractAPKEntry for testing.
func ExportExtractAPKEntry(tr *tar.Reader, hdr *tar.Header) error {
	return extractAPKEntry(tr, hdr)
}

// ExportExtractAPKData exposes extractAPKData for testing.
func ExportExtractAPKData(r io.Reader) error {
	return extractAPKData(r)
}

// ExportSha1Hex exposes sha1Hex for testing.
func ExportSha1Hex(s string) string {
	return sha1Hex(s)
}

// ExportBuildAPKDownloadRequests exposes buildAPKDownloadRequests for testing.
func ExportBuildAPKDownloadRequests(
	_ context.Context, idx *Index, destDir string, names []string,
) ([]apkDownload, map[string]string, error) {
	return idx.buildAPKDownloadRequests(destDir, names)
}

// ExportExtractAPKDataTo exposes extractAPKDataTo for testing.
func ExportExtractAPKDataTo(r io.Reader, root string) error {
	return extractAPKDataTo(r, root)
}

// ExportReadInstalledDB exposes readInstalledDB for testing.
func ExportReadInstalledDB() map[string]bool {
	return readInstalledDB()
}

// ExportReadInstalledStanzas exposes readInstalledStanzas for testing.
func ExportReadInstalledStanzas() map[string]string {
	return readInstalledStanzas()
}

// ExportWriteInstalledStanzas exposes writeInstalledStanzasAt for testing with a custom path.
func ExportWriteInstalledStanzas(dbPath string, stanzas map[string]string) error {
	return writeInstalledStanzasAt(dbPath, stanzas)
}

// ExportTryReadPkgInfoFromNextStream exposes tryReadPkgInfoFromNextStream for testing.
func ExportTryReadPkgInfoFromNextStream(br *bufio.Reader) string {
	pkgInfo, _ := tryReadPkgInfoFromNextStream(br)
	return pkgInfo
}

// ExportRegisterInstalled exposes registerInstalledAt for testing. The dbPath
// mirrors the on-disk layout (<tmpDir>/lib/apk/db/installed) used by the
// production constant so existing test expectations still hold.
func ExportRegisterInstalled(tmpDir string, pkg *Package, pkgInfo string) error {
	dbPath := filepath.Join(tmpDir, "lib", "apk", "db", "installed")
	return registerInstalledAt(dbPath, pkg, pkgInfo)
}

// ExportReadControlVerified exposes readControlVerified for testing.
func ExportReadControlVerified(r io.Reader, pkg *Package) (string, error) {
	hr := &hashingReader{br: bufio.NewReader(r), h: sha1.New()} //nolint:gosec

	return readControlVerified(hr, pkg)
}
