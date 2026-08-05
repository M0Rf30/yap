package nfpm

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"

	"github.com/M0Rf30/yap/v2/pkg/constants"
	"github.com/M0Rf30/yap/v2/pkg/errors"
	"github.com/M0Rf30/yap/v2/pkg/files"
	"github.com/M0Rf30/yap/v2/pkg/i18n"
	"github.com/M0Rf30/yap/v2/pkg/shell"
)

// maxDirectiveLineLength is the line budget an array directive tries to fit
// on a single line before falling back to one element per line.
const maxDirectiveLineLength = 100

// RenderOptions controls auxiliary file emission while rendering a PKGBUILD.
type RenderOptions struct {
	// OutputDir is the directory sidecar files are written into — currently
	// the converted changelog. When empty, the changelog= directive is
	// still emitted but the sidecar is not written, and RenderPKGBUILD
	// reports that through its returned messages.
	OutputDir string
}

// RenderPKGBUILD emits YAP specfile text for c: scalar and array directives
// in a stable, human-friendly order, per-packager overrides as YAP's
// suffixed directives (depends__apt, ...), scriptlet functions read from the
// files scripts.* reference (resolved against c.BaseDir()), a changelog=
// directive with its RPM-dialect sidecar written under opts.OutputDir when
// c.Changelog is set, and a package() function built by the same
// synthesizer ToPKGBUILD uses (BuildPackageScript, ExpandGlobs=false). A nil
// opts behaves as a zero RenderOptions.
func (c *Config) RenderPKGBUILD(opts *RenderOptions) (rendered []byte, messages []string, err error) {
	if opts == nil {
		opts = &RenderOptions{}
	}

	lines := renderScalars(c)

	changelogLines, changelogMessages, err := renderChangelogDirective(c, opts)
	if err != nil {
		return nil, nil, err
	}

	lines = append(lines, changelogLines...)
	lines = append(lines, renderArchDirectives(c)...)
	lines = append(lines, renderArrayDirective("license", licenseArray(c))...)
	lines = append(lines, renderDependencyArrays(c)...)

	scriptLines, err := renderScriptlets(c)
	if err != nil {
		return nil, nil, err
	}

	lines = append(lines, scriptLines...)
	lines = append(lines, renderPackageFunc(c)...)

	return []byte(strings.Join(lines, "\n") + "\n"), changelogMessages, nil
}

// renderChangelogDirective renders changelog="<name>.changelog" from
// c.Changelog (a goreleaser/chglog YAML path, resolved against c.BaseDir()).
// YAP's PKGBUILD changelog= file is always parsed in the RPM %changelog
// dialect (pkg/builders/rpm.parseRPMChangelog) regardless of target
// packager — the per-format Debian rendering only applies on the in-memory
// ToPKGBUILD build path — so the sidecar is rendered with Changelog.RenderRPM
// unconditionally. When opts.OutputDir is empty the sidecar is not written
// and one message reports where the directive points instead; c.Changelog
// unset is a silent no-op returning no lines and no message. A changelog
// path that fails to load is a hard error, matching ToPKGBUILD's
// applyChangelog.
func renderChangelogDirective(c *Config, opts *RenderOptions) (lines, messages []string, err error) {
	if c.Changelog == "" {
		return nil, nil, nil
	}

	clPath := c.Changelog
	if !filepath.IsAbs(clPath) {
		clPath = filepath.Join(c.BaseDir(), clPath)
	}

	changelog, err := LoadChangelog(clPath)
	if err != nil {
		return nil, nil, errors.Wrap(err, errors.ErrTypeFileSystem,
			i18n.T("errors.nfpm.convert.changelog_load_failed")).
			WithOperation("RenderPKGBUILD").
			WithContext("path", clPath)
	}

	rel := c.Name + ".changelog"
	lines = renderScalarDirective("changelog", rel)

	if opts.OutputDir == "" {
		messages = []string{
			fmt.Sprintf(i18n.T("messages.nfpm.convert.changelog_not_written"), rel),
		}

		return lines, messages, nil
	}

	if err := writeChangelogFile(opts.OutputDir, rel, changelog.RenderRPM(c.Name)); err != nil {
		return nil, nil, err
	}

	return lines, nil, nil
}

// writeChangelogFile writes the rendered RPM-%changelog-dialect content to
// dir/name.
func writeChangelogFile(dir, name string, content []byte) error {
	if err := files.ExistsMakeDir(dir); err != nil {
		return errors.Wrap(err, errors.ErrTypeFileSystem,
			i18n.T("errors.nfpm.changelog_write_failed")).
			WithOperation("writeChangelogFile").
			WithContext("dir", dir)
	}

	path := filepath.Join(dir, name)

	if err := os.WriteFile(path, content, 0o644); err != nil { //nolint:gosec // sidecar text file
		return errors.Wrap(err, errors.ErrTypeFileSystem,
			i18n.T("errors.nfpm.changelog_write_failed")).
			WithOperation("writeChangelogFile").
			WithContext("path", path)
	}

	return nil
}

// renderScalars renders the simple scalar directives, in YAP's conventional order.
func renderScalars(c *Config) []string {
	effDeb := effectiveDeb(c)

	lines := renderScalarDirective("pkgname", c.Name)
	lines = append(lines, renderPkgBaseDirectives(c)...)
	lines = append(lines, renderPkgVerDirectives(c)...)
	lines = append(lines, renderScalarDirective("pkgrel", c.Release)...)
	lines = append(lines, renderScalarDirective("epoch", c.Epoch)...)
	lines = append(lines, renderScalarDirective("pkgdesc", c.Description)...)
	lines = append(lines, renderScalarDirective("maintainer", c.Maintainer)...)
	lines = append(lines, renderScalarDirective("url", c.Homepage)...)
	lines = append(lines, renderScalarDirective("section", c.Section)...)
	lines = append(lines, renderScalarDirective("priority", c.Priority)...)
	lines = append(lines, renderScalarDirective("bugs", effDeb.Fields[debFieldBugs])...)
	lines = append(lines, renderScalarDirective("debconf_template", effDeb.Scripts.Templates)...)
	lines = append(lines, renderScalarDirective("debconf_config", effDeb.Scripts.Config)...)

	return lines
}

// renderPkgBaseDirectives renders pkgbase=<name> (a parsed PKGBUILD leaves
// PkgBase empty without an explicit directive, while ToPKGBUILD always sets
// it to Name), plus a pkgbase__pacman=() override when ArchLinux.Pkgbase
// diverges from Name.
func renderPkgBaseDirectives(c *Config) []string {
	lines := renderScalarDirective("pkgbase", c.Name)

	archBase := effectiveArchLinux(c).Pkgbase
	if archBase != "" && archBase != c.Name {
		lines = append(lines, renderScalarDirective("pkgbase__pacman", archBase)...)
	}

	return lines
}

// renderDependencyArrays renders the dependency-relation arrays and backup=().
func renderDependencyArrays(c *Config) []string {
	effDeb := effectiveDeb(c)

	lines := renderOverridableArray(c, "depends", getDepends)
	lines = append(lines, renderOverridableArray(c, "optdepends", getRecommends)...)
	lines = append(lines, renderOverridableArray(c, "suggests", getSuggests)...)
	lines = append(lines, renderOverridableArray(c, "conflicts", getConflicts)...)
	lines = append(lines, renderOverridableArray(c, "replaces", getReplaces)...)
	lines = append(lines, renderOverridableArray(c, "provides", getProvides)...)
	lines = append(lines, renderArrayDirective("breaks", effDeb.Breaks)...)
	lines = append(lines, renderArrayDirective("predepends", effDeb.Predepends)...)
	lines = append(lines, renderArrayDirective("backup", backupPaths(sortedContents(c)))...)

	return lines
}

// getDepends, getRecommends, getSuggests, getConflicts, getReplaces, and
// getProvides are the field accessors renderOverridableArray/
// renderDependencyArrays pass to compare a packager override against base.
func getDepends(o *Overridables) []string    { return o.Depends }
func getRecommends(o *Overridables) []string { return o.Recommends }
func getSuggests(o *Overridables) []string   { return o.Suggests }
func getConflicts(o *Overridables) []string  { return o.Conflicts }
func getReplaces(o *Overridables) []string   { return o.Replaces }
func getProvides(o *Overridables) []string   { return o.Provides }

// licenseArray wraps c.License in a single-element slice, or nil when unset.
func licenseArray(c *Config) []string {
	if c.License == "" {
		return nil
	}

	return []string{c.License}
}

// backupPaths returns the leading-slash-stripped destination of every
// config-marked content entry, in Contents order.
func backupPaths(contents Contents) []string {
	paths := make([]string, 0, len(contents))

	for _, entry := range contents {
		if entry.IsConfig() {
			paths = append(paths, strings.TrimPrefix(entry.Destination, "/"))
		}
	}

	return paths
}

// sortedContents returns c.Contents sorted by Destination, matching the
// deterministic order PrepareForPackager (and thus ToPKGBUILD) produces, so
// backup=() and package() are byte-identical regardless of the declaration
// order in the source spec.
func sortedContents(c *Config) Contents {
	contents := slices.Clone(c.Contents)
	sort.Slice(contents, func(i, j int) bool {
		return contents[i].Destination < contents[j].Destination
	})

	return contents
}

// archForYAP converts an nfpm/goreleaser arch spelling to YAP's canonical
// name, mirroring convert_to.go's resolveArch tail exactly ("all"->"any",
// then constants.NormalizeArchitecture) so RenderPKGBUILD's arch=() inverts
// ToPKGBUILD's arch resolution byte-for-byte.
func archForYAP(nfpmArch string) string {
	if nfpmArch == "" {
		return ""
	}

	if nfpmArch == constants.ArchNfpmAll {
		return constants.ArchAny
	}

	return constants.NormalizeArchitecture(nfpmArch)
}

// archSlice wraps arch in a single-element slice, or nil when empty.
func archSlice(arch string) []string {
	if arch == "" {
		return nil
	}

	return []string{arch}
}

// renderArchDirectives renders arch=() from the base Info.Arch, plus one
// arch__<suffix>=() per packager whose RPM/Deb/APK/ArchLinux.Arch override
// resolves to a different YAP arch, mirroring convert_to.go's packagerArch.
func renderArchDirectives(c *Config) []string {
	base := archForYAP(c.Arch)
	lines := renderArrayDirective("arch", archSlice(base))

	packagerArches := map[string]string{
		PackagerRPM:       effectiveRPM(c).Arch,
		PackagerDeb:       effectiveDeb(c).Arch,
		PackagerAPK:       effectiveAPK(c).Arch,
		PackagerArchLinux: effectiveArchLinux(c).Arch,
	}

	for _, pkg := range suffixOrder {
		yapArch := archForYAP(packagerArches[pkg])
		if yapArch == "" || yapArch == base {
			continue
		}

		lines = append(lines, renderArrayDirective("arch__"+packagerSuffix[pkg], archSlice(yapArch))...)
	}

	return lines
}

// effectiveRPM merges Overrides[rpm].RPM over the base RPM struct, matching
// the single-view ForPackager would compute for the rpm packager.
func effectiveRPM(c *Config) RPM {
	if ov, ok := c.Overrides[PackagerRPM]; ok {
		return mergeRPM(&c.RPM, &ov.RPM)
	}

	return c.RPM
}

// effectiveDeb merges Overrides[deb].Deb over the base Deb struct.
func effectiveDeb(c *Config) Deb {
	if ov, ok := c.Overrides[PackagerDeb]; ok {
		return mergeDeb(&c.Deb, &ov.Deb)
	}

	return c.Deb
}

// effectiveAPK merges Overrides[apk].APK over the base APK struct.
func effectiveAPK(c *Config) APK {
	if ov, ok := c.Overrides[PackagerAPK]; ok {
		return mergeAPK(&c.APK, &ov.APK)
	}

	return c.APK
}

// effectiveArchLinux merges Overrides[archlinux].ArchLinux over the base
// ArchLinux struct.
func effectiveArchLinux(c *Config) ArchLinux {
	if ov, ok := c.Overrides[PackagerArchLinux]; ok {
		return mergeArchLinux(&c.ArchLinux, &ov.ArchLinux)
	}

	return c.ArchLinux
}

// renderScalarDirective renders a double-quoted scalar directive, or nil
// when value is empty. Double quotes are required, not stylistic: YAP's
// PKGBUILD parser (pkg/set.StringifyAssign) strips a matched pair of
// surrounding double quotes from a scalar value but performs no single-quote
// removal, so a single-quoted scalar would round-trip with its quote
// characters still attached.
func renderScalarDirective(name, value string) []string {
	if value == "" {
		return nil
	}

	return []string{name + "=" + doubleQuote(value)}
}

// doubleQuote wraps s in double quotes, backslash-escaping the characters
// bash gives special meaning inside a double-quoted string ("\"", "\\",
// "$", "`") so the value survives StringifyAssign's quote-stripping and the
// parser's subsequent shell.Expand parameter-expansion pass unchanged.
func doubleQuote(s string) string {
	var b strings.Builder

	b.WriteByte('"')

	for _, r := range s {
		switch r {
		case '"', '\\', '$', '`':
			b.WriteByte('\\')
		}

		b.WriteRune(r)
	}

	b.WriteByte('"')

	return b.String()
}

// renderArrayDirective renders an array directive: every element single
// quoted, inline when the whole directive fits maxDirectiveLineLength, one
// element per line otherwise. Returns nil for an empty values slice.
func renderArrayDirective(name string, values []string) []string {
	if len(values) == 0 {
		return nil
	}

	quoted := make([]string, len(values))
	for i, v := range values {
		quoted[i] = shell.SingleQuote(v)
	}

	inline := name + "=(" + strings.Join(quoted, " ") + ")"
	if len(inline) <= maxDirectiveLineLength {
		return []string{inline}
	}

	lines := make([]string, 0, len(values)+2)
	lines = append(lines, name+"=(")

	for _, q := range quoted {
		lines = append(lines, "  "+q)
	}

	return append(lines, ")")
}

// renderOverridableArray renders the base array directive plus one
// name__<suffix>=() per packager whose Overrides entry sets a different,
// non-empty value for get, so a single PKGBUILD keeps every override.
func renderOverridableArray(c *Config, name string, get func(*Overridables) []string) []string {
	base := get(&c.Overridables)
	lines := renderArrayDirective(name, base)

	for _, pkg := range suffixOrder {
		ov, ok := c.Overrides[pkg]
		if !ok {
			continue
		}

		values := get(ov)
		if len(values) == 0 || slices.Equal(values, base) {
			continue
		}

		lines = append(lines, renderArrayDirective(name+"__"+packagerSuffix[pkg], values)...)
	}

	return lines
}

// renderPkgVerDirectives renders the base pkgver=() plus one
// pkgver__<suffix>= per packager whose FoldVersion encoding differs from the
// base, so every packager resolves the version FoldVersion would compute for
// it directly from ToPKGBUILD.
func renderPkgVerDirectives(c *Config) []string {
	base, _ := FoldVersion("", c.Version, c.Prerelease, c.VersionMetadata)
	lines := renderScalarDirective("pkgver", base)

	for _, pkg := range suffixOrder {
		folded, _ := FoldVersion(pkg, c.Version, c.Prerelease, c.VersionMetadata)
		if folded == base {
			continue
		}

		lines = append(lines, renderScalarDirective("pkgver__"+packagerSuffix[pkg], folded)...)
	}

	return lines
}

// renderScriptletFile reads the nfpm scriptlet at path (already two-space
// indented by readScriptlet, matching ToPKGBUILD's own transform byte for
// byte) and wraps it in a PKGBUILD function named functionName. Returns nil
// for an empty path.
func renderScriptletFile(c *Config, functionName, path string) ([]string, error) {
	if path == "" {
		return nil, nil
	}

	body, err := readScriptlet(c.BaseDir(), path)
	if err != nil {
		return nil, err
	}

	return []string{functionName + "() {\n" + body + "}", ""}, nil
}

// renderOverridableScriptlet renders the base scriptlet function plus one
// functionName__<suffix>() per packager whose Overrides entry points get at
// a different, non-empty scriptlet path.
func renderOverridableScriptlet(
	c *Config, functionName string, get func(*Overridables) string,
) ([]string, error) {
	base := get(&c.Overridables)

	lines, err := renderScriptletFile(c, functionName, base)
	if err != nil {
		return nil, err
	}

	for _, pkg := range suffixOrder {
		ov, ok := c.Overrides[pkg]
		if !ok {
			continue
		}

		path := get(ov)
		if path == "" || path == base {
			continue
		}

		suffixLines, err := renderScriptletFile(c, functionName+"__"+packagerSuffix[pkg], path)
		if err != nil {
			return nil, err
		}

		lines = append(lines, suffixLines...)
	}

	return lines, nil
}

// renderDualScriptlet renders the apk and archlinux views of one shared
// PKGBUILD hook (pre_upgrade/post_upgrade) as __apk/__pacman suffixed
// functions ONLY, never as an unsuffixed base function. ToPKGBUILD populates
// PreUpgrade/PostUpgrade solely for the apk and archlinux packagers, and an
// unsuffixed pre_upgrade() directive has base priority — it would apply
// unconditionally to every distro, including deb/rpm ones ToPKGBUILD never
// sets it for. Suffixing both (even when their content is identical) is what
// keeps deb/rpm PKGBUILD.PreUpgrade/PostUpgrade empty after a round trip.
func renderDualScriptlet(c *Config, functionName, apkPath, archPath string) ([]string, error) {
	lines, err := renderScriptletFile(c, functionName+"__apk", apkPath)
	if err != nil {
		return nil, err
	}

	archLines, err := renderScriptletFile(c, functionName+"__pacman", archPath)
	if err != nil {
		return nil, err
	}

	return append(lines, archLines...), nil
}

// renderScriptlets renders every scriptlet function: the packager-agnostic
// preinst/postinst/prerm/postrm, RPM's pretrans/posttrans, and the shared
// pre_upgrade/post_upgrade hook apk and archlinux both populate.
func renderScriptlets(c *Config) ([]string, error) {
	var lines []string

	generic := []struct {
		name string
		get  func(*Overridables) string
	}{
		{"preinst", func(o *Overridables) string { return o.Scripts.PreInstall }},
		{"postinst", func(o *Overridables) string { return o.Scripts.PostInstall }},
		{"prerm", func(o *Overridables) string { return o.Scripts.PreRemove }},
		{"postrm", func(o *Overridables) string { return o.Scripts.PostRemove }},
	}

	for _, spec := range generic {
		funcLines, err := renderOverridableScriptlet(c, spec.name, spec.get)
		if err != nil {
			return nil, err
		}

		lines = append(lines, funcLines...)
	}

	effRPM := effectiveRPM(c)

	preTransLines, err := renderScriptletFile(c, "pretrans", effRPM.Scripts.PreTrans)
	if err != nil {
		return nil, err
	}

	lines = append(lines, preTransLines...)

	postTransLines, err := renderScriptletFile(c, "posttrans", effRPM.Scripts.PostTrans)
	if err != nil {
		return nil, err
	}

	lines = append(lines, postTransLines...)

	effAPK := effectiveAPK(c)
	effArch := effectiveArchLinux(c)

	preUpgradeLines, err := renderDualScriptlet(c, "pre_upgrade",
		effAPK.Scripts.PreUpgrade, effArch.Scripts.PreUpgrade)
	if err != nil {
		return nil, err
	}

	lines = append(lines, preUpgradeLines...)

	postUpgradeLines, err := renderDualScriptlet(c, "post_upgrade",
		effAPK.Scripts.PostUpgrade, effArch.Scripts.PostUpgrade)
	if err != nil {
		return nil, err
	}

	return append(lines, postUpgradeLines...), nil
}

// renderPackageFunc wraps BuildPackageScript's raw, unindented install
// commands in a package() function. The body is intentionally not
// re-indented: ToPKGBUILD stores BuildPackageScript's output verbatim in
// PKGBUILD.Package, so leaving it as-is here is what makes a parsed
// RenderPKGBUILD output's Package field compare equal to ToPKGBUILD's.
func renderPackageFunc(c *Config) []string {
	body, _ := BuildPackageScript(sortedContents(c), false)

	lines := []string{"package() {"}
	if body != "" {
		lines = append(lines, strings.Split(strings.TrimRight(body, "\n"), "\n")...)
	}

	return append(lines, "}")
}
