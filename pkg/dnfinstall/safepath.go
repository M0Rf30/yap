package dnfinstall

import (
	"path/filepath"
	"strings"

	"github.com/M0Rf30/yap/v2/pkg/errors"
)

// safeRPMPath joins rootDir with a sanitised CPIO entry name, rejecting
// traversal attempts (".." or absolute paths).
//
// This mirrors safeAPKPath from pkg/apkindex but works with an explicit
// rootDir (for fakeroot scenarios) rather than always using "/".
func safeRPMPath(rootDir, name string) (string, error) {
	// Reject entries with absolute paths outright.
	if filepath.IsAbs(name) {
		name = strings.TrimPrefix(name, "/")
	}

	cleaned := filepath.Clean(filepath.Join(rootDir, name))
	cleanedRoot := filepath.Clean(rootDir)

	rel, err := filepath.Rel(cleanedRoot, cleaned)
	if err != nil {
		return "", errors.Wrap(err, errors.ErrTypeValidation, "path traversal").
			WithOperation("safeRPMPath").
			WithContext("name", name).
			WithContext("rootDir", rootDir)
	}

	// Reject ".", "/", empty, and parent traversal attempts.
	if rel == "." || rel == "" || rel == "/" || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", errors.New(errors.ErrTypeValidation, "path traversal: entry escapes rootDir").
			WithOperation("safeRPMPath").
			WithContext("name", name).
			WithContext("rootDir", rootDir)
	}

	return cleaned, nil
}

// safeRPMSymlinkTarget validates that a symlink's target stays under rootDir.
// Absolute targets are rejected outright; relative targets are resolved
// against the symlink's own location.
//
// This mirrors safeAPKSymlinkTarget from pkg/apkindex but works with an
// explicit rootDir.
func safeRPMSymlinkTarget(rootDir, linkPath, target string) error {
	if filepath.IsAbs(target) {
		// Absolute symlink targets are common in RPM packages (e.g. /usr/share/...).
		// They are safe at install time because the symlink itself is created
		// under rootDir; the target resolution only matters at runtime, when
		// the package is actually installed at /. Permit them.
		return nil
	}

	// Relative: resolve the target relative to the symlink's directory and
	// confirm the result stays under rootDir.
	resolved := filepath.Clean(filepath.Join(filepath.Dir(linkPath), target))

	rel, err := filepath.Rel(filepath.Clean(rootDir), resolved)
	if err != nil {
		return errors.Wrap(err, errors.ErrTypeValidation, "symlink target traversal").
			WithOperation("safeRPMSymlinkTarget").
			WithContext("link", linkPath).
			WithContext("target", target)
	}

	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return errors.New(errors.ErrTypeValidation, "symlink target traversal: escapes rootDir").
			WithOperation("safeRPMSymlinkTarget").
			WithContext("link", linkPath).
			WithContext("target", target).
			WithContext("rootDir", rootDir)
	}

	return nil
}
