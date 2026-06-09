// Package safepath centralises the path-containment checks used by every
// archive/package extractor in YAP (deb data.tar, RPM CPIO payloads, APK
// tarballs, container rootfs layers, source archives). It defends against
// zip-slip path traversal and symlink-escape attacks.
//
// All joins use filepath.Rel-based containment: immune to the
// prefix-aliasing flaw of naive strings.HasPrefix checks (root "/tmp/foo"
// must not admit "/tmp/foobar/evil"), and still meaningful when root is
// "/" (where every absolute path trivially passes a prefix check).
package safepath

import (
	"path/filepath"
	"strings"

	"github.com/M0Rf30/yap/v2/pkg/errors"
)

// Join joins root and an archive entry name and verifies the result stays
// inside root. Absolute entry names are reinterpreted as root-relative
// (tar/cpio writers disagree on leading slashes). Entries that resolve to
// root itself (e.g. tar's "./" directory entry) are allowed; use
// JoinStrict to reject them.
func Join(root, name string) (string, error) {
	cleaned, rel, err := relativize(root, name)
	if err != nil {
		return "", err
	}

	if escapes(rel) {
		return "", escapeError("Join", root, name, rel)
	}

	return cleaned, nil
}

// JoinStrict is Join but additionally rejects entries that resolve to the
// root itself ("", ".", "/"). CPIO/RPM extraction uses this: a payload
// entry must always name something *inside* the install root.
func JoinStrict(root, name string) (string, error) {
	cleaned, rel, err := relativize(root, name)
	if err != nil {
		return "", err
	}

	if rel == "." || rel == "" || rel == "/" || escapes(rel) {
		return "", escapeError("JoinStrict", root, name, rel)
	}

	return cleaned, nil
}

// SymlinkTarget validates the target of a symlink being created at
// linkPath (an on-disk path under root, as returned by Join/JoinStrict).
//
// Absolute targets are accepted: real packages routinely ship absolute
// symlinks (e.g. /usr/lib64/libfoo.so.1 → /usr/lib/libfoo.so.1) and
// dpkg/rpm/apk accept them — they only resolve at runtime, when the
// package content lives at /.
//
// Relative targets are resolved against the symlink's own parent
// directory and must stay inside root. This is defense-in-depth against
// symlink+write attacks where a malicious archive first plants a symlink
// pointing outside the root and then writes a regular file through it.
func SymlinkTarget(root, linkPath, target string) error {
	if target == "" || filepath.IsAbs(target) {
		return nil
	}

	resolved := filepath.Clean(filepath.Join(filepath.Dir(linkPath), target))

	rel, err := filepath.Rel(filepath.Clean(root), resolved)
	if err != nil {
		return errors.Wrap(err, errors.ErrTypeValidation, "symlink target traversal").
			WithOperation("SymlinkTarget").
			WithContext("link", linkPath).
			WithContext("target", target).
			WithContext("root", root)
	}

	if escapes(rel) {
		return errors.New(errors.ErrTypeValidation, "symlink target escapes root").
			WithOperation("SymlinkTarget").
			WithContext("link", linkPath).
			WithContext("target", target).
			WithContext("root", root)
	}

	return nil
}

// EntrySymlinkTarget validates a symlink target for an archive entry whose
// paths are still archive-relative (no on-disk root yet). The target is
// resolved against the entry's parent directory inside the archive; an
// escape manifests as a leading "..".
//
// Absolute targets are accepted for the same reason as SymlinkTarget.
func EntrySymlinkTarget(entryName, target string) error {
	if target == "" || filepath.IsAbs(target) || strings.HasPrefix(target, "/") {
		return nil
	}

	parent := filepath.ToSlash(filepath.Dir(filepath.ToSlash(entryName)))
	if parent == "." || parent == "/" {
		parent = ""
	}

	joined := filepath.ToSlash(filepath.Join(parent, filepath.ToSlash(target)))

	if joined == ".." || strings.HasPrefix(joined, "../") {
		return errors.New(errors.ErrTypeValidation, "symlink target escapes archive root").
			WithOperation("EntrySymlinkTarget").
			WithContext("entry", entryName).
			WithContext("target", target).
			WithContext("resolved", joined)
	}

	return nil
}

// relativize joins root+name (treating absolute names as root-relative)
// and returns the cleaned join plus its path relative to root.
//
// Names that lexically climb above the root ("../x", "a/../../x") are
// rejected by inspecting the cleaned NAME, not just the join result: when
// root is "/" filepath.Clean clamps ".." at the root ("/../etc" → "/etc"),
// which would silently normalize a hostile entry instead of rejecting it.
func relativize(root, name string) (cleaned, rel string, err error) {
	if filepath.IsAbs(name) {
		name = strings.TrimPrefix(name, "/")
	}

	if cleanName := filepath.Clean(name); escapes(cleanName) {
		return "", "", errors.New(errors.ErrTypeValidation, "path traversal: entry climbs above root").
			WithOperation("safepath.relativize").
			WithContext("entry", name).
			WithContext("root", root)
	}

	cleanRoot := filepath.Clean(root)
	cleaned = filepath.Clean(filepath.Join(cleanRoot, name))

	rel, err = filepath.Rel(cleanRoot, cleaned)
	if err != nil {
		return "", "", errors.Wrap(err, errors.ErrTypeValidation, "path traversal").
			WithOperation("safepath.relativize").
			WithContext("entry", name).
			WithContext("root", root)
	}

	return cleaned, rel, nil
}

// escapes reports whether a root-relative path points outside the root.
func escapes(rel string) bool {
	return rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

// escapeError builds the standard traversal rejection error.
func escapeError(op, root, name, rel string) error {
	return errors.New(errors.ErrTypeValidation, "path traversal: entry escapes root").
		WithOperation(op).
		WithContext("entry", name).
		WithContext("root", root).
		WithContext("resolved_rel", rel)
}
