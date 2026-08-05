// Package nfpm implements in-tree support for the goreleaser/nfpm spec-file
// dialect (nfpm.yaml), including bidirectional conversion with YAP's
// extended-PKGBUILD specfile.
package nfpm

import (
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/M0Rf30/yap/v2/pkg/constants"
	"github.com/M0Rf30/yap/v2/pkg/errors"
	"github.com/M0Rf30/yap/v2/pkg/i18n"
	"github.com/M0Rf30/yap/v2/pkg/pkgbuild"
)

// alnumChars is the set of ASCII letters and digits shared by every
// packager's legal version-string charset.
const alnumChars = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789"

// Debian control field name constants with a direct PKGBUILD equivalent;
// shared by mapDebFields (below) and fromPKGBUILDDebFields (convert_from.go).
const (
	debFieldBugs       = "Bugs"
	debFieldMultiArch  = "Multi-Arch"
	debFieldSource     = "Source"
	debFieldBuiltUsing = "Built-Using"
)

// knownDebFieldKeys are the deb.fields entries with a direct PKGBUILD
// equivalent; anything else is reported as dropped.
var knownDebFieldKeys = map[string]struct{}{
	debFieldBugs:       {},
	debFieldMultiArch:  {},
	debFieldSource:     {},
	debFieldBuiltUsing: {},
}

// ConvertOptions controls nfpm -> PKGBUILD conversion behavior.
type ConvertOptions struct {
	// Packager selects which override set and which format conventions apply.
	// Required. One of Packagers.
	Packager string
	// Distro / Codename / StartDir / Home / TargetArch mirror parser.ParseFile.
	Distro     string
	Codename   string
	StartDir   string
	Home       string
	TargetArch string
	// ExpandGlobs resolves Content.Source globs and tree entries against the
	// filesystem. true for builds, false for pure spec->spec conversion.
	ExpandGlobs bool
}

// ToPKGBUILD converts the spec into a PKGBUILD ready for pkg/builder.
// The returned []string carries one human-readable message per nfpm feature
// that has no YAP equivalent and was dropped (caller logs them).
func (c *Config) ToPKGBUILD(opts *ConvertOptions) (*pkgbuild.PKGBUILD, []string, error) {
	if FormatForPackager(opts.Packager) == "" {
		return nil, nil, errors.New(errors.ErrTypeValidation,
			i18n.T("errors.nfpm.convert.unsupported_packager")).
			WithOperation("ToPKGBUILD").
			WithContext("packager", opts.Packager)
	}

	cfg := c
	if !opts.ExpandGlobs && !c.DisableGlobbing {
		clone := *c
		clone.DisableGlobbing = true
		cfg = &clone
	}

	info, err := cfg.ForPackager(opts.Packager)
	if err != nil {
		return nil, nil, errors.Wrap(err, errors.ErrTypeConfiguration,
			i18n.T("errors.nfpm.convert.resolve_overrides_failed")).
			WithOperation("ToPKGBUILD").
			WithContext("packager", opts.Packager)
	}

	pkg := &pkgbuild.PKGBUILD{}

	applyIdentity(pkg, info, opts)
	applyDependencies(pkg, info)
	mapDebFields(pkg, info.Deb.Fields)

	pkg.Backup = collectBackup(info.Contents)

	folded, versionMsg := FoldVersion(
		opts.Packager, info.Version, info.Prerelease, info.VersionMetadata)
	pkg.PkgVer = folded

	var messages []string
	if versionMsg != "" {
		messages = append(messages, versionMsg)
	}

	baseDir := c.BaseDir()

	if err := mapScriptlets(pkg, info, opts.Packager, baseDir); err != nil {
		return nil, nil, err
	}

	if err := applyChangelog(pkg, info, opts, baseDir); err != nil {
		return nil, nil, err
	}

	script, contentMsgs := BuildPackageScript(info.Contents, opts.ExpandGlobs)
	pkg.Package = script

	messages = append(messages, contentMsgs...)
	messages = append(messages, droppedFeatureMessages(info)...)

	pkg.Distro = opts.Distro
	pkg.Codename = opts.Codename
	pkg.StartDir = opts.StartDir
	pkg.Home = opts.Home
	pkg.TargetArch = opts.TargetArch
	// SourceDir mirrors the PKGBUILD path in pkg/parser: the builder's
	// initDirs() creates it and every synthesized script runs with
	// `cd "${srcdir}"`, so it must be populated even though nfpm specs never
	// fetch upstream sources into it.
	pkg.SourceDir = filepath.Join(opts.StartDir, "src")

	pkg.Init()

	// nfpm contents are prebuilt artifacts: ship them byte-identical, never
	// let makepkg-style post-processing touch them. This MUST run AFTER
	// Init(): Init() -> processOptions() unconditionally resets these flags
	// (plus Docs/Libtool/Purge/Static, which we intentionally leave at their
	// makepkg defaults) to makepkg's hardcoded defaults whenever Options is
	// empty — setting them before Init() gets silently clobbered.
	pkg.StripEnabled = false
	pkg.ZipManEnabled = false
	pkg.DebugEnabled = false
	// EmptyDirsEnabled follows makepkg's polarity: enabled means "LEAVE empty
	// directories in the package", and options.Apply only runs
	// RemoveEmptyDirs when it is false. An nfpm `type: dir` entry is an
	// explicit request for an empty directory (a log or spool dir), so it
	// must stay enabled or those entries would be silently deleted after the
	// synthesized package() created them.
	pkg.EmptyDirsEnabled = true

	return pkg, messages, nil
}

// applyIdentity maps the scalar identity fields (name, description, arch,
// licensing, ...) from info onto pkg.
func applyIdentity(pkg *pkgbuild.PKGBUILD, info *Info, opts *ConvertOptions) {
	pkg.PkgName = info.Name
	pkg.PkgBase = info.Name

	if opts.Packager == PackagerArchLinux && info.ArchLinux.Pkgbase != "" {
		pkg.PkgBase = info.ArchLinux.Pkgbase
	}

	// TrimSpace matters: a YAML folded scalar (`description: >`) keeps a
	// trailing newline, which would otherwise leak a blank line into every
	// format's metadata (deb control, .PKGINFO, RPM Summary).
	pkg.PkgDesc = strings.TrimSpace(info.Description)
	if opts.Packager == PackagerRPM && info.RPM.Summary != "" {
		pkg.PkgDesc = strings.TrimSpace(info.RPM.Summary)
	}

	pkg.PkgRel = info.Release
	pkg.Epoch = info.Epoch
	pkg.Section = info.Section
	pkg.Priority = info.Priority
	pkg.Maintainer = info.Maintainer
	pkg.URL = info.Homepage
	pkg.Group = info.RPM.Group

	if info.License != "" {
		pkg.License = []string{info.License}
	}

	pkg.Arch = []string{resolveArch(info, opts.Packager)}
}

// applyDependencies maps the dependency-relation fields.
func applyDependencies(pkg *pkgbuild.PKGBUILD, info *Info) {
	pkg.Depends = info.Depends
	pkg.OptDepends = info.Recommends
	pkg.Suggests = info.Suggests
	pkg.Conflicts = info.Conflicts
	pkg.Replaces = info.Replaces
	pkg.Provides = info.Provides
	pkg.Breaks = info.Deb.Breaks
	pkg.PreDepends = info.Deb.Predepends
}

// resolveArch resolves the PKGBUILD Arch value for one packager: "all" folds
// to "any"; otherwise the packager-specific arch (when set) or Info.Arch is
// run through constants.NormalizeArchitecture.
func resolveArch(info *Info, packager string) string {
	arch := packagerArch(info, packager)
	if arch == "" {
		arch = info.Arch
	}

	if arch == constants.ArchNfpmAll {
		return constants.ArchAny
	}

	return constants.NormalizeArchitecture(arch)
}

// packagerArch returns the packager-specific arch override field, or "" when
// packager is unrecognized or unset.
func packagerArch(info *Info, packager string) string {
	switch packager {
	case PackagerRPM:
		return info.RPM.Arch
	case PackagerDeb:
		return info.Deb.Arch
	case PackagerAPK:
		return info.APK.Arch
	case PackagerArchLinux:
		return info.ArchLinux.Arch
	case PackagerIPK:
		return info.IPK.Arch
	default:
		return ""
	}
}

// FoldVersion computes the PKGBUILD PkgVer for the given packager from an
// nfpm version triple, applying packager-specific prerelease/metadata
// separators and sanitizing the result to characters legal for the target
// version field. The second return value is a non-empty dropped-feature
// message when version_metadata was supplied but had to be discarded (the
// apk/archlinux version fields have no metadata separator).
func FoldVersion(packager, version, prerelease, metadata string) (pkgVer, dropped string) {
	var raw string

	switch packager {
	case PackagerDeb, PackagerRPM:
		raw = version
		if prerelease != "" {
			raw += "~" + prerelease
		}

		if metadata != "" {
			raw += "+" + metadata
		}
	default:
		raw = version + strings.ReplaceAll(prerelease, "-", "_")

		if metadata != "" {
			dropped = i18n.T("messages.nfpm.convert.dropped_version_metadata")
		}
	}

	return sanitizeVersion(raw, legalVersionChars(packager)), dropped
}

// legalVersionChars returns the charset legal in the target packager's
// version field: deb allows "~-+", rpm disallows the hyphen (illegal in an
// RPM %version), apk/archlinux disallow both "~" and "+".
func legalVersionChars(packager string) string {
	switch packager {
	case PackagerDeb:
		return alnumChars + ".+~-"
	case PackagerRPM:
		return alnumChars + ".+~"
	default:
		return alnumChars + "._"
	}
}

// sanitizeVersion drops every rune of raw that is not in legal.
func sanitizeVersion(raw, legal string) string {
	return strings.Map(func(r rune) rune {
		if strings.ContainsRune(legal, r) {
			return r
		}

		return -1
	}, raw)
}

// mapDebFields copies the recognized deb.fields entries onto pkg. Unknown
// keys are reported separately by droppedFeatureMessages.
func mapDebFields(pkg *pkgbuild.PKGBUILD, fields map[string]string) {
	if v, ok := fields[debFieldBugs]; ok {
		pkg.Bugs = v
	}

	if v, ok := fields[debFieldMultiArch]; ok {
		pkg.MultiArch = v
	}

	if v, ok := fields[debFieldSource]; ok {
		pkg.SourcePkg = v
	}

	if v, ok := fields[debFieldBuiltUsing]; ok {
		pkg.BuiltUsing = splitCommaTrim(v)
	}
}

// splitCommaTrim splits s on commas, trimming surrounding whitespace and
// dropping empty elements.
func splitCommaTrim(s string) []string {
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))

	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}

	return out
}

// collectBackup returns the Backup list: the Destination (leading slash
// stripped) of every config-marked content entry, in Contents order.
func collectBackup(contents Contents) []string {
	var backup []string

	for _, entry := range contents {
		if entry.IsConfig() {
			backup = append(backup, strings.TrimPrefix(entry.Destination, "/"))
		}
	}

	return backup
}

// scriptletMapping pairs an nfpm scriptlet source path with the PKGBUILD
// field it should populate.
type scriptletMapping struct {
	src string
	dst *string
}

// mapScriptlets reads every nfpm scriptlet file that applies to packager and
// stores its indented body in the matching PKGBUILD field. It also copies
// the deb.scripts.templates/config paths verbatim (they are not read).
func mapScriptlets(pkg *pkgbuild.PKGBUILD, info *Info, packager, baseDir string) error {
	mappings := []scriptletMapping{
		{info.Scripts.PreInstall, &pkg.PreInst},
		{info.Scripts.PostInstall, &pkg.PostInst},
		{info.Scripts.PreRemove, &pkg.PreRm},
		{info.Scripts.PostRemove, &pkg.PostRm},
		{info.RPM.Scripts.PreTrans, &pkg.PreTrans},
		{info.RPM.Scripts.PostTrans, &pkg.PostTrans},
	}

	switch packager {
	case PackagerAPK:
		mappings = append(mappings,
			scriptletMapping{info.APK.Scripts.PreUpgrade, &pkg.PreUpgrade},
			scriptletMapping{info.APK.Scripts.PostUpgrade, &pkg.PostUpgrade},
		)
	case PackagerArchLinux:
		mappings = append(mappings,
			scriptletMapping{info.ArchLinux.Scripts.PreUpgrade, &pkg.PreUpgrade},
			scriptletMapping{info.ArchLinux.Scripts.PostUpgrade, &pkg.PostUpgrade},
		)
	}

	for _, m := range mappings {
		if m.src == "" {
			continue
		}

		body, err := readScriptlet(baseDir, m.src)
		if err != nil {
			return err
		}

		*m.dst = body
	}

	pkg.DebTemplate = info.Deb.Scripts.Templates
	pkg.DebConfig = info.Deb.Scripts.Config

	return nil
}

// readScriptlet reads the scriptlet file at relPath (resolved against
// baseDir when relative), strips a leading shebang line and any leading
// blank lines, and indents every remaining line by two spaces so it drops
// cleanly into a builder's function template. A missing file is a hard
// error.
func readScriptlet(baseDir, relPath string) (string, error) {
	fullPath := relPath
	if !filepath.IsAbs(relPath) {
		fullPath = filepath.Join(baseDir, relPath)
	}

	//nolint:gosec // path comes from an already-loaded, trusted nfpm spec
	data, err := os.ReadFile(fullPath)
	if err != nil {
		return "", errors.Wrap(err, errors.ErrTypeFileSystem,
			i18n.T("errors.nfpm.convert.scriptlet_read_failed")).
			WithOperation("ToPKGBUILD").
			WithContext("path", fullPath)
	}

	return IndentScriptBody(data), nil
}

// IndentScriptBody strips a leading "#!" line and any leading blank lines
// from data, then indents every remaining line by two spaces so it drops
// cleanly into a builder's function template. Exported so render_pkgbuild.go
// can apply the identical transform when embedding scriptlet bodies.
func IndentScriptBody(data []byte) string {
	lines := strings.Split(strings.ReplaceAll(string(data), "\r\n", "\n"), "\n")

	idx := 0
	if idx < len(lines) && strings.HasPrefix(lines[idx], "#!") {
		idx++
	}

	for idx < len(lines) && lines[idx] == "" {
		idx++
	}

	body := lines[idx:]

	// Drop the single trailing empty element produced by a final newline in
	// the source file so we don't emit a dangling blank last line.
	if n := len(body); n > 0 && body[n-1] == "" {
		body = body[:n-1]
	}

	var b strings.Builder

	for _, line := range body {
		b.WriteString("  ")
		b.WriteString(line)
		b.WriteByte('\n')
	}

	return b.String()
}

// applyChangelog renders info.Changelog (a goreleaser/chglog file path) into
// pkg.ChangelogData using the dialect appropriate for packager. It is a
// no-op when Info.Changelog is empty.
func applyChangelog(pkg *pkgbuild.PKGBUILD, info *Info, opts *ConvertOptions, baseDir string) error {
	if info.Changelog == "" {
		return nil
	}

	clPath := info.Changelog
	if !filepath.IsAbs(clPath) {
		clPath = filepath.Join(baseDir, clPath)
	}

	changelog, err := LoadChangelog(clPath)
	if err != nil {
		return errors.Wrap(err, errors.ErrTypeFileSystem,
			i18n.T("errors.nfpm.convert.changelog_load_failed")).
			WithOperation("ToPKGBUILD").
			WithContext("path", clPath)
	}

	if opts.Packager == PackagerDeb {
		codename := opts.Codename
		if codename == "" {
			codename = "unstable"
		}

		pkg.ChangelogData = changelog.RenderDebian(info.Name, codename)

		return nil
	}

	pkg.ChangelogData = changelog.RenderRPM(info.Name)

	return nil
}

// Dropped-feature name constants for droppedFeatureMessages; convert_to_test.go
// references the same identifiers so a typo fails compilation, not just the
// assertion.
const (
	featVendor            = "vendor"
	featRPMBuildHost      = "rpm.buildhost"
	featRPMPackager       = "rpm.packager"
	featArchLinuxPackager = "archlinux.packager"
	featRPMPrefixes       = "rpm.prefixes"
	featRPMRequiresPost   = "rpm.requires.post"
	featRPMGhostFiles     = "rpm.ghost_files"
	featRPMScriptsVerify  = "rpm.scripts.verify"
	featRPMCompression    = "rpm.compression"
	featRPMSignature      = "rpm.signature"
	featDebCompression    = "deb.compression"
	featDebArchVariant    = "deb.arch_variant"
	featDebTriggers       = "deb.triggers"
	featDebScriptsRules   = "deb.scripts.rules"
	featDebSignature      = "deb.signature"
	featAPKSignature      = "apk.signature"
)

// droppedFeatureMessages returns one human-readable message per nfpm feature
// present in info that has no PKGBUILD equivalent.
func droppedFeatureMessages(info *Info) []string {
	features := []struct {
		name    string
		present bool
	}{
		{featVendor, info.Vendor != ""},
		{featRPMBuildHost, info.RPM.BuildHost != ""},
		{featRPMPackager, info.RPM.Packager != ""},
		{featArchLinuxPackager, info.ArchLinux.Packager != ""},
		{featRPMPrefixes, len(info.RPM.Prefixes) > 0},
		{featRPMRequiresPost, len(info.RPM.Requires.Post) > 0},
		{featRPMGhostFiles, len(info.RPM.Ghosts) > 0},
		{featRPMScriptsVerify, info.RPM.Scripts.Verify != ""},
		{featRPMCompression, info.RPM.Compression != ""},
		{featRPMSignature, !isZeroRPMSignature(info.RPM.Signature)},
		{featDebCompression, info.Deb.Compression != ""},
		{featDebArchVariant, info.Deb.ArchVariant != ""},
		{featDebTriggers, hasDebTriggers(&info.Deb.Triggers)},
		{featDebScriptsRules, info.Deb.Scripts.Rules != ""},
		{featDebSignature, !isZeroDebSignature(&info.Deb.Signature)},
		{featAPKSignature, !isZeroAPKSignature(info.APK.Signature)},
		{PackagerIPK, !isZeroIPK(&info.IPK)},
	}

	var msgs []string

	for _, f := range features {
		if f.present {
			msgs = append(msgs, droppedFeatureMessage(f.name))
		}
	}

	return append(msgs, droppedDebFieldMessages(info.Deb.Fields)...)
}

// droppedFeatureMessage renders the generic dropped-feature message for a
// named nfpm field.
func droppedFeatureMessage(name string) string {
	return i18n.T("messages.nfpm.convert.dropped_feature", map[string]any{"Feature": name})
}

// droppedDebFieldMessages returns one dropped-feature message per
// unrecognized deb.fields key, sorted for determinism.
func droppedDebFieldMessages(fields map[string]string) []string {
	if len(fields) == 0 {
		return nil
	}

	keys := make([]string, 0, len(fields))

	for k := range fields {
		if _, known := knownDebFieldKeys[k]; !known {
			keys = append(keys, k)
		}
	}

	sort.Strings(keys)

	msgs := make([]string, 0, len(keys))
	for _, k := range keys {
		msgs = append(msgs, droppedFeatureMessage("deb.fields["+k+"]"))
	}

	return msgs
}

// isZeroRPMSignature reports whether s carries no configuration.
func isZeroRPMSignature(s RPMSignature) bool {
	return s.KeyFile == "" && s.KeyID == ""
}

// isZeroDebSignature reports whether s carries no configuration.
func isZeroDebSignature(s *DebSignature) bool {
	return s.KeyFile == "" && s.KeyID == "" && s.Method == "" && s.Type == "" && s.Signer == ""
}

// isZeroAPKSignature reports whether s carries no configuration.
func isZeroAPKSignature(s APKSignature) bool {
	return s.KeyFile == "" && s.KeyID == "" && s.KeyName == ""
}

// isZeroIPK reports whether ipk carries no configuration.
func isZeroIPK(ipk *IPK) bool {
	return ipk.ABIVersion == "" && len(ipk.Alternatives) == 0 && ipk.Arch == "" &&
		!ipk.AutoInstalled && !ipk.Essential && len(ipk.Fields) == 0 &&
		len(ipk.Predepends) == 0 && len(ipk.Tags) == 0
}

// hasDebTriggers reports whether t carries any trigger declaration.
func hasDebTriggers(t *DebTriggers) bool {
	return len(t.Interest) > 0 || len(t.InterestAwait) > 0 || len(t.InterestNoAwait) > 0 ||
		len(t.Activate) > 0 || len(t.ActivateAwait) > 0 || len(t.ActivateNoAwait) > 0
}
