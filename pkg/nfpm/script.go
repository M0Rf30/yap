package nfpm

import (
	"fmt"
	"os"
	"path"
	"strings"

	"github.com/M0Rf30/yap/v2/pkg/i18n"
	"github.com/M0Rf30/yap/v2/pkg/shell"
)

// Default file/dir modes and ownership applied when a Content entry carries
// no FileInfo (or a zero Mode) — mirrors PrepareForPackager's own fallback so
// script.go behaves identically whether or not contents were prepared.
const (
	defaultFileMode os.FileMode = 0o644
	defaultDirMode  os.FileMode = 0o755
	defaultOwner                = "root"
	defaultGroup                = "root"
)

// globMetaChars are the shell pathname-expansion metacharacters used to
// detect an unexpanded glob pattern in a Content.Source.
const globMetaChars = "*?["

// BuildPackageScript synthesizes the bash package() function body that
// installs contents, and returns one dropped-feature message per content
// entry that has no PKGBUILD equivalent (ghost, debian changelog, or a set
// file_info.lang), in Contents order.
//
// expandGlobs mirrors ConvertOptions.ExpandGlobs: when true every entry is
// assumed already resolved by PrepareForPackager (globs expanded, trees
// flattened) and is emitted as a direct install/ln command; when false, a
// Source containing shell glob metacharacters is emitted as a runtime `for`
// loop and `tree`-typed entries are emitted as a directory copy.
//
// The body never stats the filesystem (safe to call on raw, un-prepared
// contents), never emits `set -e` (the builder prologue already does), and
// is deterministic: one command per line, LF-terminated, in Contents order.
func BuildPackageScript(contents Contents, expandGlobs bool) (script string, dropped []string) {
	var lines []string

	for _, entry := range contents {
		cmds, msgs := renderContent(entry, expandGlobs)
		lines = append(lines, cmds...)
		dropped = append(dropped, msgs...)
	}

	if len(lines) == 0 {
		return "", dropped
	}

	return strings.Join(lines, "\n") + "\n", dropped
}

// renderContent dispatches a single Content entry to its command builder,
// or reports it as dropped when its type has no PKGBUILD equivalent.
func renderContent(entry *Content, expandGlobs bool) (commands, dropped []string) {
	if entry.FileInfo != nil && entry.FileInfo.Lang != "" {
		dropped = append(dropped, langDroppedMessage(entry))
	}

	switch entry.Type {
	case TypeRPMGhost:
		return nil, append(dropped, ghostDroppedMessage(entry))
	case TypeDebChangelog:
		return nil, append(dropped, changelogDroppedMessage(entry))
	}

	switch {
	case entry.Type == TypeDir || entry.Type == TypeImplicitDir:
		return dirCommands(entry), dropped
	case entry.Type == TypeSymlink:
		return symlinkCommands(entry), dropped
	case entry.IsTree():
		return treeCommands(entry), dropped
	default:
		return fileCommands(entry, expandGlobs), dropped
	}
}

// dirCommands renders a `dir`/`implicit dir` entry.
func dirCommands(entry *Content) []string {
	owner, group := contentOwnerGroup(entry.FileInfo)
	mode := modeFor(entry.FileInfo, true)
	dst := destRef(entry.Destination)

	return []string{
		fmt.Sprintf("install -d %s -o %s -g %s %s", modeFlag(mode), owner, group, dst),
	}
}

// symlinkCommands renders a `symlink` entry: ensure the parent directory
// exists, then (re)create the link. Source is the literal link target, not
// a file to read — it is quoted verbatim, never ${startdir}-prefixed.
func symlinkCommands(entry *Content) []string {
	dstDir := destRef(path.Dir(normalizeDest(entry.Destination)))
	dst := destRef(entry.Destination)
	src := shell.SingleQuote(entry.Source)

	return []string{
		"install -d " + dstDir,
		"ln -sfn " + src + " " + dst,
	}
}

// treeCommands renders a `tree` (or `config|*|tree`) entry as a directory
// copy: create the destination directory, then recursively copy the source
// tree's contents into it.
func treeCommands(entry *Content) []string {
	owner, group := contentOwnerGroup(entry.FileInfo)
	mode := modeFor(entry.FileInfo, true)
	dst := destRef(entry.Destination)
	srcTree := sourceRefSuffixed(entry.Source, "/.")
	dstTree := destRefSuffixed(entry.Destination, "/")

	return []string{
		fmt.Sprintf("install -d %s -o %s -g %s %s", modeFlag(mode), owner, group, dst),
		"cp -a " + srcTree + " " + dstTree,
	}
}

// fileCommands renders a file-ish entry ("", file, config*, doc, license,
// licence, readme). When expandGlobs is false and Source still contains
// shell glob metacharacters, it defers expansion to a runtime `for` loop.
func fileCommands(entry *Content, expandGlobs bool) []string {
	owner, group := contentOwnerGroup(entry.FileInfo)
	mode := modeFor(entry.FileInfo, false)

	if !expandGlobs && strings.ContainsAny(entry.Source, globMetaChars) {
		return globLoopCommands(entry, owner, group, mode)
	}

	src := sourceRef(entry.Source)
	dst := destRef(entry.Destination)

	return []string{
		fmt.Sprintf("install -D %s -o %s -g %s %s %s", modeFlag(mode), owner, group, src, dst),
	}
}

// globLoopCommands renders a runtime shell loop that installs every file
// matching entry.Source into the entry.Destination directory, preserving
// each match's basename.
func globLoopCommands(entry *Content, owner, group string, mode os.FileMode) []string {
	pattern := globSourceRef(entry.Source)
	dstDir := destRef(entry.Destination)

	return []string{
		"for f in " + pattern + "; do",
		fmt.Sprintf("  install -D %s -o %s -g %s \"$f\" %s\"/$(basename \"$f\")\"",
			modeFlag(mode), owner, group, dstDir),
		"done",
	}
}

// contentOwnerGroup resolves the owner/group for a content entry, defaulting
// to "root"/"root" when unset.
func contentOwnerGroup(fi *ContentFileInfo) (owner, group string) {
	owner, group = defaultOwner, defaultGroup

	if fi != nil {
		if fi.Owner != "" {
			owner = fi.Owner
		}

		if fi.Group != "" {
			group = fi.Group
		}
	}

	return owner, group
}

// modeFor resolves the mode for a content entry, defaulting to 0755 for
// directory-ish entries and 0644 otherwise when unset.
func modeFor(fi *ContentFileInfo, isDir bool) os.FileMode {
	if fi != nil && fi.Mode != 0 {
		return fi.Mode.Perm()
	}

	if isDir {
		return defaultDirMode
	}

	return defaultFileMode
}

// modeFlag renders mode as an `install -m` flag using 4-digit octal.
func modeFlag(mode os.FileMode) string {
	return fmt.Sprintf("-m%04o", mode.Perm())
}

// sourceRef quotes src for use as a real filesystem path to read from,
// prefixing a relative path with the unquoted, expanding "${startdir}/".
func sourceRef(src string) string {
	if strings.HasPrefix(src, "/") {
		return shell.SingleQuote(src)
	}

	return `"${startdir}/"` + shell.SingleQuote(src)
}

// sourceRefSuffixed is sourceRef with suffix appended to src before quoting.
func sourceRefSuffixed(src, suffix string) string {
	if strings.HasPrefix(src, "/") {
		return shell.SingleQuote(src + suffix)
	}

	return `"${startdir}/"` + shell.SingleQuote(src+suffix)
}

// globSourceRef mirrors sourceRef but leaves src unquoted (and the
// "${startdir}/" prefix, if any, adjacent-concatenated) so shell pathname
// expansion still applies to any glob metacharacters it contains.
func globSourceRef(src string) string {
	if strings.HasPrefix(src, "/") {
		return src
	}

	return `"${startdir}/"` + src
}

// destRef quotes dst (normalized to an absolute, cleaned path) for use
// against "${pkgdir}".
func destRef(dst string) string {
	return `"${pkgdir}"` + shell.SingleQuote(normalizeDest(dst))
}

// destRefSuffixed is destRef with suffix appended to dst after
// normalization but before quoting.
func destRefSuffixed(dst, suffix string) string {
	return `"${pkgdir}"` + shell.SingleQuote(normalizeDest(dst)+suffix)
}

// normalizeDest defensively ensures dst is an absolute, cleaned POSIX path,
// matching the normalization PrepareForPackager already applies (raw specs
// fed to this function without going through PrepareForPackager may not
// have it applied yet).
func normalizeDest(dst string) string {
	if dst == "" {
		return "/"
	}

	if !strings.HasPrefix(dst, "/") {
		dst = "/" + dst
	}

	return path.Clean(dst)
}

// i18nArgDestination is the i18n.T template-argument key every dropped
// content-feature message renders entry.Destination under.
const i18nArgDestination = "Destination"

// ghostDroppedMessage reports a `ghost` content entry as dropped.
func ghostDroppedMessage(entry *Content) string {
	return i18n.T("messages.nfpm.convert.dropped_content_ghost",
		map[string]any{i18nArgDestination: entry.Destination})
}

// changelogDroppedMessage reports a `debian changelog` content entry as
// dropped.
func changelogDroppedMessage(entry *Content) string {
	return i18n.T("messages.nfpm.convert.dropped_content_changelog",
		map[string]any{i18nArgDestination: entry.Destination})
}

// langDroppedMessage reports a content entry's file_info.lang as dropped.
func langDroppedMessage(entry *Content) string {
	return i18n.T("messages.nfpm.convert.dropped_content_lang",
		map[string]any{i18nArgDestination: entry.Destination})
}
