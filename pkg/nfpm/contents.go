package nfpm

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/M0Rf30/yap/v2/pkg/errors"
	"github.com/M0Rf30/yap/v2/pkg/safepath"
)

// Content declares a single file, directory, symlink, or directory tree
// shipped by the package.
type Content struct {
	// Source is the on-disk path (or glob pattern) providing the payload.
	// Meaningless for TypeDir/TypeImplicitDir/TypeRPMGhost; for TypeSymlink
	// it is the link target rather than an on-disk path.
	Source string `yaml:"src,omitempty"`
	// Destination is the absolute path the content is installed at.
	Destination string `yaml:"dst"`
	// Type selects the packaging semantics; see the Type* constants. Empty
	// is equivalent to TypeFile.
	Type string `yaml:"type,omitempty"`
	// Packager restricts this entry to a single packager (see Packagers);
	// empty applies to every packager.
	Packager string `yaml:"packager,omitempty"`
	// FileInfo overrides the resolved owner/group/mode/mtime metadata.
	FileInfo *ContentFileInfo `yaml:"file_info,omitempty"`
	// Expand enables ${VAR}/$VAR expansion of Source and Destination during
	// Parse/Load.
	Expand bool `yaml:"expand,omitempty"`
}

// ContentFileInfo overrides the metadata PrepareForPackager would otherwise
// default for a Content entry.
type ContentFileInfo struct {
	// Owner is the packaged file's owning user name. Defaults to "root".
	Owner string `yaml:"owner,omitempty"`
	// Group is the packaged file's owning group name. Defaults to "root".
	Group string `yaml:"group,omitempty"`
	// Mode is the packaged file's permission bits. Defaults to the on-disk
	// mode masked by Info.Umask, or 0o644/0o755 when there is no on-disk
	// source to inspect.
	Mode os.FileMode `yaml:"mode,omitempty"`
	// MTime is the packaged file's modification time. Defaults to Info.MTime.
	MTime time.Time `yaml:"mtime,omitempty"`
	// Lang is the RPM %lang tag for locale-specific files.
	Lang string `yaml:"lang,omitempty"`
	// Size is the on-disk file size in bytes, computed by
	// PrepareForPackager; it is never read from YAML.
	Size int64 `yaml:"-"`
}

// Contents is an ordered list of package content entries.
type Contents []*Content

// Content type constants — the exact strings nfpm uses in Content.Type.
const (
	// TypeFile is a plain regular file. The zero value ("") is equivalent.
	TypeFile = "file"
	// TypeDir is a directory the packager creates explicitly.
	TypeDir = "dir"
	// TypeImplicitDir is a directory created as a side effect of packaging
	// one of its descendants (e.g. tree expansion); never written by hand.
	TypeImplicitDir = "implicit dir"
	// TypeTree copies an entire on-disk directory tree, expanded by
	// PrepareForPackager into per-file/per-dir/per-symlink entries.
	TypeTree = "tree"
	// TypeSymlink is a symbolic link; Source holds the link target.
	TypeSymlink = "symlink"
	// TypeConfig is a configuration file preserved across upgrades/removals.
	TypeConfig = "config"
	// TypeConfigNoReplace is a config file dpkg/rpm never overwrites once
	// modified by the admin.
	TypeConfigNoReplace = "config|noreplace"
	// TypeConfigMissingOK is a config file whose absence at removal time is
	// not an error.
	TypeConfigMissingOK = "config|missingok"
	// TypeConfigTree is TypeConfig applied to every file in a TypeTree.
	TypeConfigTree = "config|tree"
	// TypeConfigNoReplaceTree is TypeConfigNoReplace applied to every file
	// in a TypeTree.
	TypeConfigNoReplaceTree = "config|noreplace|tree"
	// TypeConfigMissingOKTree is TypeConfigMissingOK applied to every file
	// in a TypeTree.
	TypeConfigMissingOKTree = "config|missingok|tree"
	// TypeRPMGhost is an RPM %ghost entry: metadata only, no payload.
	TypeRPMGhost = "ghost"
	// TypeRPMDoc is an RPM %doc entry.
	TypeRPMDoc = "doc"
	// TypeRPMLicence is an RPM %license entry (British spelling accepted by
	// nfpm as an alias).
	TypeRPMLicence = "licence"
	// TypeRPMLicense is an RPM %license entry.
	TypeRPMLicense = "license"
	// TypeRPMReadme is an RPM %readme entry.
	TypeRPMReadme = "readme"
	// TypeDebChangelog is the Debian changelog.gz entry.
	TypeDebChangelog = "debian changelog"
)

// noDiskSourceTypes lists content types that never require (or read) an
// on-disk Source, matching real packaging semantics: directories are pure
// metadata, symlinks store their target rather than a path, and RPM ghost
// entries are deliberately absent from the payload.
var noDiskSourceTypes = map[string]bool{
	TypeDir:         true,
	TypeImplicitDir: true,
	TypeSymlink:     true,
	TypeRPMGhost:    true,
}

// knownContentTypes lists every Content.Type value Validate accepts.
var knownContentTypes = map[string]bool{
	"":                      true,
	TypeFile:                true,
	TypeDir:                 true,
	TypeImplicitDir:         true,
	TypeTree:                true,
	TypeSymlink:             true,
	TypeConfig:              true,
	TypeConfigNoReplace:     true,
	TypeConfigMissingOK:     true,
	TypeConfigTree:          true,
	TypeConfigNoReplaceTree: true,
	TypeConfigMissingOKTree: true,
	TypeRPMGhost:            true,
	TypeRPMDoc:              true,
	TypeRPMLicence:          true,
	TypeRPMLicense:          true,
	TypeRPMReadme:           true,
	TypeDebChangelog:        true,
}

// IsConfig reports whether the type marks a config file (any config|*
// variant, including the plain "config").
func (c *Content) IsConfig() bool {
	return strings.HasPrefix(c.Type, TypeConfig)
}

// IsTree reports whether the type is tree or any config|*|tree variant.
func (c *Content) IsTree() bool {
	return c.Type == TypeTree || strings.HasSuffix(c.Type, "|"+TypeTree)
}

// cleanDestination normalises a Content.Destination to a leading-slash
// clean path for comparison purposes (duplicate detection in Validate).
// Unlike PrepareForPackager's safepath-based normalisation, this never
// errors on ".." escapes — filepath.Clean simply clamps them at the root —
// since Validate must stay disk-access-free and side-effect-free.
func cleanDestination(dest string) string {
	return filepath.Clean("/" + dest)
}

// PrepareForPackager resolves a Contents list for one packager:
//   - drops entries whose Packager is set and != packager
//   - expands globs in Source relative to baseDir, and tree entries into
//     per-file/per-dir/per-symlink entries, unless disableGlobbing is set —
//     in which case every non dir/symlink/ghost entry is passed through
//     untouched (Source left exactly as declared, no disk access at all;
//     used for pure spec<->spec conversion where the caller re-resolves
//     Source itself)
//   - applies FileInfo defaults (owner/group "root", mode from disk & ~umask
//     else 0o644/0o755, mtime falling back to Info.MTime)
//   - normalises Destination to a leading-slash clean path, rejecting ".."
//     escapes
//   - returns a deterministic order (sorted by Destination)
//
// baseDir is the directory containing the nfpm spec; relative Source paths
// resolve against it. Non-existent Source for a file-ish type is an error.
func (cs Contents) PrepareForPackager(
	baseDir, packager string, umask os.FileMode, disableGlobbing bool, mtime time.Time,
) (Contents, error) {
	var out Contents

	for _, c := range cs {
		if c.Packager != "" && c.Packager != packager {
			continue
		}

		entries, err := prepareOneContent(c, baseDir, umask, disableGlobbing, mtime)
		if err != nil {
			return nil, err
		}

		out = append(out, entries...)
	}

	for _, c := range out {
		resolved, err := safepath.Join("/", c.Destination)
		if err != nil {
			return nil, errors.Wrap(err, errors.ErrTypeValidation, "invalid content destination").
				WithOperation("PrepareForPackager").
				WithContext("destination", c.Destination)
		}

		c.Destination = resolved
	}

	sort.Slice(out, func(i, j int) bool {
		return out[i].Destination < out[j].Destination
	})

	return out, nil
}

// prepareOneContent dispatches a single (pre packager-filter) Content entry
// to the right expansion strategy.
func prepareOneContent(
	c *Content, baseDir string, umask os.FileMode, disableGlobbing bool, mtime time.Time,
) (Contents, error) {
	switch {
	case noDiskSourceTypes[c.Type]:
		entry := cloneContent(c)
		isDir := c.Type == TypeDir || c.Type == TypeImplicitDir
		applyContentDefaults(entry, nil, isDir, c.Type != TypeSymlink, mtime, umask)

		return Contents{entry}, nil
	case disableGlobbing:
		entry := cloneContent(c)
		applyContentDefaults(entry, nil, c.IsTree(), true, mtime, umask)

		return Contents{entry}, nil
	case c.IsTree():
		return expandTreeContent(c, baseDir, umask, mtime)
	default:
		return expandFileContent(c, baseDir, umask, mtime)
	}
}

// cloneContent deep-copies a Content (and its FileInfo, if any) so callers
// can mutate the result without aliasing the original entry.
func cloneContent(c *Content) *Content {
	clone := *c

	if c.FileInfo != nil {
		fi := *c.FileInfo
		clone.FileInfo = &fi
	}

	return &clone
}

// applyContentDefaults fills FileInfo owner/group/mode/mtime/size for a
// single resolved content entry, without overwriting anything the spec
// already declared.
//
// diskInfo is the on-disk stat result backing this entry, or nil when none
// exists (symlink, plain dir, ghost, and disableGlobbing passthrough
// entries carry no payload to inspect). isDir selects the literal mode
// fallback (0o755) used when diskInfo is nil; the literal file fallback is
// 0o644. applyModeDefault is false only for symlinks, whose permission bits
// are meaningless to every consumer and are left unset unless declared.
func applyContentDefaults(
	c *Content, diskInfo fs.FileInfo, isDir, applyModeDefault bool, mtime time.Time, umask os.FileMode,
) {
	if c.FileInfo == nil {
		c.FileInfo = &ContentFileInfo{}
	}

	if c.FileInfo.Owner == "" {
		c.FileInfo.Owner = defaultOwner
	}

	if c.FileInfo.Group == "" {
		c.FileInfo.Group = defaultGroup
	}

	if c.FileInfo.Mode == 0 && applyModeDefault {
		switch {
		case diskInfo != nil:
			c.FileInfo.Mode = diskInfo.Mode().Perm() &^ umask
		case isDir:
			c.FileInfo.Mode = 0o755
		default:
			c.FileInfo.Mode = 0o644
		}
	}

	if c.FileInfo.MTime.IsZero() {
		c.FileInfo.MTime = mtime
	}

	if diskInfo != nil && !diskInfo.IsDir() {
		c.FileInfo.Size = diskInfo.Size()
	}
}

// expandFileContent resolves a file-ish (non tree/dir/symlink/ghost) entry:
// its Source is glob-expanded relative to baseDir, and every match becomes
// its own Content (sharing Destination as a directory prefix when there is
// more than one match, used verbatim when there is exactly one).
func expandFileContent(
	c *Content, baseDir string, umask os.FileMode, mtime time.Time,
) (Contents, error) {
	pattern := c.Source

	resolvedPattern := pattern
	if !filepath.IsAbs(resolvedPattern) {
		resolvedPattern = filepath.Join(baseDir, resolvedPattern)
	}

	matches, err := filepath.Glob(resolvedPattern)
	if err != nil {
		return nil, errors.Wrap(err, errors.ErrTypeConfiguration, "invalid content glob pattern").
			WithOperation("PrepareForPackager").
			WithContext("pattern", pattern)
	}

	if len(matches) == 0 {
		return nil, errors.New(errors.ErrTypeFileSystem,
			fmt.Sprintf("content source %q matched no files", pattern)).
			WithOperation("PrepareForPackager").
			WithContext("pattern", pattern)
	}

	out := make(Contents, 0, len(matches))

	for _, m := range matches {
		info, statErr := os.Stat(m)
		if statErr != nil {
			return nil, errors.Wrap(statErr, errors.ErrTypeFileSystem, "content source not found").
				WithOperation("PrepareForPackager").
				WithContext("source", m)
		}

		entry := cloneContent(c)
		entry.Source = m

		if len(matches) > 1 {
			entry.Destination = filepath.Join(c.Destination, filepath.Base(m))
		}

		applyContentDefaults(entry, info, info.IsDir(), true, mtime, umask)
		out = append(out, entry)
	}

	return out, nil
}

// expandTreeContent walks an on-disk directory tree, producing one Content
// per regular file, one TypeImplicitDir per directory (including the tree
// root itself), and one TypeSymlink per symbolic link encountered.
func expandTreeContent(
	c *Content, baseDir string, umask os.FileMode, mtime time.Time,
) (Contents, error) {
	leafFileType := TypeFile
	if c.Type != TypeTree {
		leafFileType = strings.TrimSuffix(c.Type, "|"+TypeTree)
	}

	root := c.Source
	if !filepath.IsAbs(root) {
		root = filepath.Join(baseDir, root)
	}

	rootInfo, err := os.Stat(root)
	if err != nil {
		return nil, errors.Wrap(err, errors.ErrTypeFileSystem, "tree content source not found").
			WithOperation("PrepareForPackager").
			WithContext("source", c.Source)
	}

	if !rootInfo.IsDir() {
		return nil, errors.New(errors.ErrTypeConfiguration, "tree content source is not a directory").
			WithOperation("PrepareForPackager").
			WithContext("source", c.Source)
	}

	var out Contents

	walkErr := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		rel, relErr := filepath.Rel(root, path)
		if relErr != nil {
			return relErr
		}

		dest := c.Destination
		if rel != "." {
			dest = filepath.Join(c.Destination, rel)
		}

		return appendTreeEntry(&out, c, path, dest, d, leafFileType, umask, mtime)
	})
	if walkErr != nil {
		return nil, errors.Wrap(walkErr, errors.ErrTypeFileSystem, "failed to walk tree content").
			WithOperation("PrepareForPackager").
			WithContext("source", c.Source)
	}

	return out, nil
}

// appendTreeEntry converts a single filepath.WalkDir callback invocation
// into the right Content entry (symlink, implicit dir, or leaf file) and
// appends it to out.
func appendTreeEntry(
	out *Contents, c *Content, path, dest string, d fs.DirEntry, leafFileType string,
	umask os.FileMode, mtime time.Time,
) error {
	switch {
	case d.Type()&fs.ModeSymlink != 0:
		target, readErr := os.Readlink(path)
		if readErr != nil {
			return readErr
		}

		entry := cloneContent(c)
		entry.Type = TypeSymlink
		entry.Source = target
		entry.Destination = dest
		applyContentDefaults(entry, nil, false, false, mtime, umask)
		*out = append(*out, entry)
	case d.IsDir():
		dirInfo, infoErr := d.Info()
		if infoErr != nil {
			return infoErr
		}

		entry := cloneContent(c)
		entry.Type = TypeImplicitDir
		entry.Source = ""
		entry.Destination = dest
		applyContentDefaults(entry, dirInfo, true, true, mtime, umask)
		*out = append(*out, entry)
	default:
		fileInfo, infoErr := d.Info()
		if infoErr != nil {
			return infoErr
		}

		entry := cloneContent(c)
		entry.Type = leafFileType
		entry.Source = path
		entry.Destination = dest
		applyContentDefaults(entry, fileInfo, false, true, mtime, umask)
		*out = append(*out, entry)
	}

	return nil
}
