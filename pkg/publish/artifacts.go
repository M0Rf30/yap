// SPDX-FileCopyrightText: 2021-2026 Gianluca Boiano
// SPDX-License-Identifier: GPL-3.0-only

package publish

import (
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/M0Rf30/yap/v2/pkg/errors"
)

// Package formats understood by the publisher.
const (
	FormatDeb    = "deb"
	FormatRPM    = "rpm"
	FormatAPK    = "apk"
	FormatPacman = "pacman"
)

// DetectFormat classifies an artifact by file name. Signature sidecars
// (.asc, .sig) and SBOM files are not packages and report ok=false.
func DetectFormat(path string) (string, bool) {
	name := strings.ToLower(filepath.Base(path))

	switch {
	case strings.HasSuffix(name, ".deb"):
		return FormatDeb, true
	case strings.HasSuffix(name, ".rpm"):
		return FormatRPM, true
	case strings.HasSuffix(name, ".apk"):
		return FormatAPK, true
	case isPacmanName(name):
		return FormatPacman, true
	default:
		return "", false
	}
}

// isPacmanName matches NAME.pkg.tar.<compression> but not its .sig sidecar.
func isPacmanName(name string) bool {
	_, ext, ok := strings.Cut(name, ".pkg.tar")
	if !ok {
		return false
	}

	return ext == "" || (strings.HasPrefix(ext, ".") && !strings.Contains(ext[1:], "."))
}

// CollectArtifacts expands paths into package files. A file is kept as is
// (explicit); a directory contributes the package files directly inside it,
// in name order. dirMembers reports which results came from a directory scan.
func CollectArtifacts(paths []string) (files []string, dirMembers map[string]bool, err error) {
	dirMembers = make(map[string]bool)
	seen := make(map[string]bool)

	add := func(p string, fromDir bool) {
		abs, absErr := filepath.Abs(p)
		if absErr != nil {
			abs = p
		}

		if seen[abs] {
			return
		}

		seen[abs] = true

		files = append(files, abs)
		dirMembers[abs] = fromDir
	}

	for _, p := range paths {
		info, statErr := os.Stat(p)
		if statErr != nil {
			return nil, nil, errors.Wrap(statErr, errors.ErrTypeFileSystem, "cannot access artifact").
				WithOperation("CollectArtifacts").
				WithContext("path", p)
		}

		if !info.IsDir() {
			if _, ok := DetectFormat(p); !ok {
				return nil, nil, errors.New(errors.ErrTypeValidation,
					"unsupported package file (expected .deb, .rpm, .apk or .pkg.tar.*)").
					WithOperation("CollectArtifacts").
					WithContext("path", p)
			}

			add(p, false)

			continue
		}

		entries, readErr := os.ReadDir(p)
		if readErr != nil {
			return nil, nil, errors.Wrap(readErr, errors.ErrTypeFileSystem, "cannot read directory").
				WithOperation("CollectArtifacts").
				WithContext("path", p)
		}

		var found []string

		for _, e := range entries {
			if e.IsDir() {
				continue
			}

			if _, ok := DetectFormat(e.Name()); ok {
				found = append(found, filepath.Join(p, e.Name()))
			}
		}

		sort.Strings(found)

		for _, f := range found {
			add(f, true)
		}
	}

	return files, dirMembers, nil
}

// guessPackageName extracts the package name from a conventional file name
// (name_version_arch.deb, name-version-release.arch.rpm). It is only a hint to
// narrow server-side listings; "" means unknown.
func guessPackageName(format, path string) string {
	base := filepath.Base(path)

	switch format {
	case FormatDeb:
		name, _, ok := strings.Cut(base, "_")
		if ok {
			return name
		}
	case FormatRPM:
		base = strings.TrimSuffix(base, ".rpm")
		if i := strings.LastIndex(base, "."); i > 0 {
			base = base[:i] // drop .arch
		}

		for range 2 { // drop -release and -version
			i := strings.LastIndex(base, "-")
			if i <= 0 {
				return ""
			}

			base = base[:i]
		}

		return base
	}

	return ""
}
