// Package pkgbuild provides PKGBUILD structure and manipulation functionality.
package pkgbuild

import (
	"os"
	"path/filepath"

	"github.com/M0Rf30/yap/v2/pkg/errors"
)

// ReadChangelog returns the changelog file contents, resolved relative to the
// PKGBUILD directory. Returns ChangelogData verbatim when it is non-empty,
// without touching the Changelog path. Returns empty bytes and nil error if
// both ChangelogData and Changelog are empty. Returns wrapped error if
// Changelog is set but the file is unreadable.
func (pkgBuild *PKGBUILD) ReadChangelog() ([]byte, error) {
	if len(pkgBuild.ChangelogData) > 0 {
		return pkgBuild.ChangelogData, nil
	}

	if pkgBuild.Changelog == "" {
		return nil, nil
	}

	// Resolve path relative to StartDir (where PKGBUILD is located)
	changelogPath := pkgBuild.Changelog
	if !filepath.IsAbs(changelogPath) {
		changelogPath = filepath.Join(pkgBuild.StartDir, pkgBuild.Changelog)
	}

	changelogPath = filepath.Clean(changelogPath)

	data, err := os.ReadFile(changelogPath) //nolint:gosec // path resolved from trusted PKGBUILD
	if err != nil {
		return nil, errors.Wrap(err, errors.ErrTypeFileSystem,
			"failed to read changelog file").
			WithOperation("ReadChangelog").
			WithContext("path", changelogPath)
	}

	return data, nil
}
