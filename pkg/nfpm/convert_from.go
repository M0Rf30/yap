package nfpm

import (
	"fmt"
	"io/fs"
	"os"
	"os/user"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/M0Rf30/yap/v2/pkg/constants"
	"github.com/M0Rf30/yap/v2/pkg/errors"
	"github.com/M0Rf30/yap/v2/pkg/files"
	"github.com/M0Rf30/yap/v2/pkg/i18n"
	"github.com/M0Rf30/yap/v2/pkg/pkgbuild"
)

// FromOptions configures FromPKGBUILD.
type FromOptions struct {
	// Packager scopes emitted packager-specific sections ("rpm", "deb", "apk"
	// or "archlinux"); "" populates every section applicable to the resolved
	// PKGBUILD fields present.
	Packager string
	// ContentsFrom, when non-empty, walks that staged directory tree
	// (typically a PKGBUILD's pkgdir) into Contents entries.
	ContentsFrom string
	// OutputDir is the directory non-empty scriptlet bodies are written to as
	// sibling shell scripts (e.g. "<name>.preinstall.sh"). When empty, the
	// would-be relative filenames are still recorded on Scripts.*, but the
	// files themselves are not written; FromPKGBUILD reports this via the
	// returned messages instead.
	OutputDir string
}

// archToNFPM maps YAP's canonical architecture names to nfpm's goreleaser-style spellings.
var archToNFPM = map[string]string{
	constants.ArchX86_64:  constants.ArchNfpmAmd64,
	constants.ArchAarch64: constants.ArchNfpmArm64,
	constants.ArchI686:    constants.ArchNfpm386,
	constants.ArchArmv7:   constants.ArchNfpmArmv7,
	constants.ArchArmv6:   constants.ArchNfpmArmv6,
	constants.ArchAny:     constants.ArchNfpmAll,
}

// packagerSuffix maps an nfpm packager name to the YAP PKGBUILD directive
// suffix that selects it (e.g. depends__apt).
var packagerSuffix = map[string]string{
	PackagerDeb:       "apt",
	PackagerRPM:       "yum",
	PackagerAPK:       "apk",
	PackagerArchLinux: "pacman",
}

// suffixOrder is the stable iteration order used whenever packager-specific
// directives are considered, so generated PKGBUILDs are deterministic.
var suffixOrder = []string{PackagerDeb, PackagerRPM, PackagerAPK, PackagerArchLinux}

// FromPKGBUILD builds an nfpm Config from a parsed, single-package PKGBUILD.
// The returned []string carries one message per PKGBUILD feature that has no
// nfpm equivalent, is actually present, and was therefore dropped.
func FromPKGBUILD(p *pkgbuild.PKGBUILD, opts FromOptions) (*Config, []string, error) {
	if len(p.PkgNames) > 1 {
		return nil, nil, errors.New(errors.ErrTypeValidation,
			i18n.T("errors.nfpm.split_package_unsupported")).
			WithOperation("FromPKGBUILD").
			WithContext("pkgbase", p.PkgBase)
	}

	cfg := &Config{}

	fromPKGBUILDCore(cfg, p)
	fromPKGBUILDDeps(cfg, p)
	fromPKGBUILDPackagerSections(cfg, p, opts.Packager)

	messages := make([]string, 0)

	scriptMessages, err := fromPKGBUILDScripts(cfg, p, opts)
	if err != nil {
		return nil, nil, err
	}

	messages = append(messages, scriptMessages...)

	if err := fromPKGBUILDContents(cfg, p, opts.ContentsFrom); err != nil {
		return nil, nil, err
	}

	messages = append(messages, fromPKGBUILDDroppedFunctions(p)...)
	messages = append(messages, fromPKGBUILDDroppedArrays(p)...)
	messages = append(messages, fromPKGBUILDDroppedScalars(p)...)

	return cfg, messages, nil
}

// archForNFPM converts a YAP canonical (or aliased) architecture name to its
// nfpm/goreleaser spelling.
func archForNFPM(yapArch string) string {
	canonical := constants.NormalizeArchitecture(yapArch)
	if mapped, ok := archToNFPM[canonical]; ok {
		return mapped
	}

	return canonical
}

// splitPkgVer reverses the deb/rpm PkgVer encoding ("ver~pre+meta") back into
// discrete version/prerelease/metadata fields via SplitVersion. The
// apk/archlinux encoding (bare concatenation, no separator) is genuinely
// ambiguous to unfold, so a PkgVer without "~" is treated as an opaque,
// unsplit version.
func splitPkgVer(pkgVer string) (version, prerelease, metadata string) {
	if !strings.Contains(pkgVer, "~") {
		return pkgVer, "", ""
	}

	normalized := strings.Replace(pkgVer, "~", "-", 1)

	return SplitVersion(normalized)
}

// fromPKGBUILDCore fills the packager-agnostic Info scalars.
func fromPKGBUILDCore(cfg *Config, p *pkgbuild.PKGBUILD) {
	info := &cfg.Info

	info.Name = p.PkgName
	info.Platform = "linux"
	info.Umask = 0o02
	info.Description = p.PkgDesc
	info.Maintainer = p.Maintainer
	info.Homepage = p.URL
	info.Section = p.Section
	info.Priority = p.Priority
	info.Release = p.PkgRel
	info.Epoch = p.Epoch

	if len(p.License) > 0 {
		info.License = strings.Join(p.License, " ")
	}

	if len(p.Arch) > 0 {
		info.Arch = archForNFPM(p.Arch[0])
	}

	info.Version, info.Prerelease, info.VersionMetadata = splitPkgVer(p.PkgVer)

	if p.BuildDate != 0 {
		info.MTime = time.Unix(p.BuildDate, 0).UTC()
	} else {
		info.MTime = files.SourceDateEpochFromEnv()
	}
}

// fromPKGBUILDDeps copies the packager-agnostic dependency lists.
func fromPKGBUILDDeps(cfg *Config, p *pkgbuild.PKGBUILD) {
	cfg.Depends = p.Depends
	cfg.Recommends = p.OptDepends
	cfg.Suggests = p.Suggests
	cfg.Conflicts = p.Conflicts
	cfg.Replaces = p.Replaces
	cfg.Provides = p.Provides
}

// fromPKGBUILDPackagerSections fills the RPM/Deb/ArchLinux structs (non-script
// fields) from resolved PKGBUILD data. packager scopes which section(s) are
// populated; "" populates every section with applicable data.
func fromPKGBUILDPackagerSections(cfg *Config, p *pkgbuild.PKGBUILD, packager string) {
	if packager == "" || packager == PackagerRPM {
		cfg.RPM.Group = p.Group
	}

	if packager == "" || packager == PackagerDeb {
		cfg.Deb.Breaks = p.Breaks
		cfg.Deb.Predepends = p.PreDepends
		cfg.Deb.Scripts.Templates = p.DebTemplate
		cfg.Deb.Scripts.Config = p.DebConfig
		fromPKGBUILDDebFields(cfg, p)
	}

	if (packager == "" || packager == PackagerArchLinux) &&
		p.PkgBase != "" && p.PkgBase != p.PkgName {
		cfg.ArchLinux.Pkgbase = p.PkgBase
	}
}

// fromPKGBUILDDebFields fills the deb.fields entries with a direct PKGBUILD
// equivalent (Bugs, Built-Using, Multi-Arch, Source), mirroring
// convert_to.go's mapDebFields exactly.
func fromPKGBUILDDebFields(cfg *Config, p *pkgbuild.PKGBUILD) {
	fields := make(map[string]string, 4)

	if p.Bugs != "" {
		fields["Bugs"] = p.Bugs
	}

	if len(p.BuiltUsing) > 0 {
		fields["Built-Using"] = strings.Join(p.BuiltUsing, ", ")
	}

	if p.MultiArch != "" {
		fields["Multi-Arch"] = p.MultiArch
	}

	if p.SourcePkg != "" {
		fields["Source"] = p.SourcePkg
	}

	if len(fields) > 0 {
		cfg.Deb.Fields = fields
	}
}

// scriptSpec pairs a PKGBUILD scriptlet body with the nfpm scripts.* field(s)
// it fills and the file suffix used when writing it to disk.
type scriptSpec struct {
	body string
	hook string
	dsts []*string
}

// fromPKGBUILDScripts writes every non-empty scriptlet body to a sibling
// "<name>.<hook>.sh" file under opts.OutputDir and points the matching
// scripts.* field(s) at the relative filename. When opts.OutputDir is empty
// the files are not written and a message reports so.
func fromPKGBUILDScripts(cfg *Config, p *pkgbuild.PKGBUILD, opts FromOptions) ([]string, error) {
	specs := scriptSpecs(cfg, p, opts.Packager)

	var notWritten []string

	for _, spec := range specs {
		if spec.body == "" {
			continue
		}

		rel := cfg.Name + "." + spec.hook + ".sh"
		for _, dst := range spec.dsts {
			*dst = rel
		}

		if opts.OutputDir == "" {
			notWritten = append(notWritten, rel)

			continue
		}

		if err := writeScriptFile(opts.OutputDir, rel, spec.body); err != nil {
			return nil, err
		}
	}

	if len(notWritten) == 0 {
		return nil, nil
	}

	return []string{
		fmt.Sprintf(i18n.T("messages.nfpm.convert.scripts_not_written"), strings.Join(notWritten, ", ")),
	}, nil
}

// scriptSpecs builds the list of scriptlet bodies to convert, scoped by packager.
func scriptSpecs(cfg *Config, p *pkgbuild.PKGBUILD, packager string) []scriptSpec {
	specs := []scriptSpec{
		{p.PreInst, "preinstall", []*string{&cfg.Scripts.PreInstall}},
		{p.PostInst, "postinstall", []*string{&cfg.Scripts.PostInstall}},
		{p.PreRm, "preremove", []*string{&cfg.Scripts.PreRemove}},
		{p.PostRm, "postremove", []*string{&cfg.Scripts.PostRemove}},
	}

	if packager == "" || packager == PackagerRPM {
		specs = append(specs,
			scriptSpec{p.PreTrans, "pretrans", []*string{&cfg.RPM.Scripts.PreTrans}},
			scriptSpec{p.PostTrans, "posttrans", []*string{&cfg.RPM.Scripts.PostTrans}},
		)
	}

	var preUpgradeDsts, postUpgradeDsts []*string

	if packager == "" || packager == PackagerAPK {
		preUpgradeDsts = append(preUpgradeDsts, &cfg.APK.Scripts.PreUpgrade)
		postUpgradeDsts = append(postUpgradeDsts, &cfg.APK.Scripts.PostUpgrade)
	}

	if packager == "" || packager == PackagerArchLinux {
		preUpgradeDsts = append(preUpgradeDsts, &cfg.ArchLinux.Scripts.PreUpgrade)
		postUpgradeDsts = append(postUpgradeDsts, &cfg.ArchLinux.Scripts.PostUpgrade)
	}

	if len(preUpgradeDsts) > 0 {
		specs = append(specs, scriptSpec{p.PreUpgrade, "preupgrade", preUpgradeDsts})
	}

	if len(postUpgradeDsts) > 0 {
		specs = append(specs, scriptSpec{p.PostUpgrade, "postupgrade", postUpgradeDsts})
	}

	return specs
}

// writeScriptFile writes body (a bare, two-space-indented bash function body)
// to dir/name as a standalone POSIX shell script.
func writeScriptFile(dir, name, body string) error {
	if err := files.ExistsMakeDir(dir); err != nil {
		return errors.Wrap(err, errors.ErrTypeFileSystem,
			i18n.T("errors.nfpm.script_write_failed")).
			WithOperation("writeScriptFile").
			WithContext("dir", dir)
	}

	content := "#!/bin/sh\n" + dedentTwoSpaces(strings.TrimRight(body, "\n")) + "\n"
	path := filepath.Join(dir, name)

	if err := os.WriteFile(path, []byte(content), 0o755); err != nil { //nolint:gosec // executable
		return errors.Wrap(err, errors.ErrTypeFileSystem,
			i18n.T("errors.nfpm.script_write_failed")).
			WithOperation("writeScriptFile").
			WithContext("path", path)
	}

	return nil
}

// dedentTwoSpaces strips one leading two-space indent level from every line.
func dedentTwoSpaces(body string) string {
	lines := strings.Split(body, "\n")
	for i, line := range lines {
		lines[i] = strings.TrimPrefix(line, "  ")
	}

	return strings.Join(lines, "\n")
}

// fromPKGBUILDContents converts p.Backup into "config" Contents entries and,
// when contentsFrom is set, walks that staged tree into the remaining entries.
func fromPKGBUILDContents(cfg *Config, p *pkgbuild.PKGBUILD, contentsFrom string) error {
	backupDests := make(map[string]bool, len(p.Backup))
	contents := make(Contents, 0, len(p.Backup))

	for _, b := range p.Backup {
		dest := "/" + strings.TrimPrefix(b, "/")
		backupDests[dest] = true

		entry := &Content{Destination: dest, Type: TypeConfig}

		if contentsFrom != "" {
			candidate := filepath.Join(contentsFrom, b)
			if info, statErr := os.Lstat(candidate); statErr == nil {
				entry.Source = candidate
				entry.FileInfo = fileInfoFor(info)
			}
		}

		contents = append(contents, entry)
	}

	if contentsFrom != "" {
		walked, err := walkContentsFrom(contentsFrom, backupDests)
		if err != nil {
			return err
		}

		contents = append(contents, walked...)
	}

	sort.Slice(contents, func(i, j int) bool {
		return contents[i].Destination < contents[j].Destination
	})

	cfg.Contents = contents

	return nil
}

// walkContentsFrom walks root and converts every entry except the root itself
// into a dir/symlink/file Content, skipping destinations already covered by skip.
func walkContentsFrom(root string, skip map[string]bool) (Contents, error) {
	var contents Contents

	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if path == root {
			return nil
		}

		rel, relErr := filepath.Rel(root, path)
		if relErr != nil {
			return relErr
		}

		dest := "/" + filepath.ToSlash(rel)
		if skip[dest] {
			return nil
		}

		info, infoErr := entry.Info()
		if infoErr != nil {
			return infoErr
		}

		content, contentErr := contentForEntry(path, dest, info)
		if contentErr != nil {
			return contentErr
		}

		contents = append(contents, content)

		return nil
	})
	if err != nil {
		return nil, errors.Wrap(err, errors.ErrTypeFileSystem,
			i18n.T("errors.nfpm.contents_walk_failed")).
			WithOperation("walkContentsFrom").
			WithContext("root", root)
	}

	return contents, nil
}

// fileInfoFor builds a ContentFileInfo from on-disk metadata: owner/group
// resolved to names when possible, mode masked to permission bits, and mtime.
func fileInfoFor(info fs.FileInfo) *ContentFileInfo {
	owner, group := ownerGroup(info)

	return &ContentFileInfo{
		Owner: owner,
		Group: group,
		Mode:  info.Mode().Perm(),
		MTime: info.ModTime(),
	}
}

// contentForEntry converts one filesystem entry into a Content of the
// matching type, with on-disk mode/owner/group/mtime metadata attached.
func contentForEntry(path, dest string, info fs.FileInfo) (*Content, error) {
	fileInfo := fileInfoFor(info)

	if info.Mode()&fs.ModeSymlink != 0 {
		target, err := os.Readlink(path)
		if err != nil {
			return nil, err
		}

		return &Content{Destination: dest, Type: TypeSymlink, Source: target, FileInfo: fileInfo}, nil
	}

	if info.IsDir() {
		return &Content{Destination: dest, Type: TypeDir, FileInfo: fileInfo}, nil
	}

	return &Content{Destination: dest, Type: TypeFile, Source: path, FileInfo: fileInfo}, nil
}

// ownerGroup resolves the owning uid/gid of info to names, falling back to
// their numeric string form when no matching name is registered.
func ownerGroup(info fs.FileInfo) (owner, group string) {
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return "", ""
	}

	return lookupUserName(stat.Uid), lookupGroupName(stat.Gid)
}

// lookupUserName resolves uid to a username, or its decimal string.
func lookupUserName(uid uint32) string {
	if u, err := user.LookupId(strconv.FormatUint(uint64(uid), 10)); err == nil {
		return u.Username
	}

	return strconv.FormatUint(uint64(uid), 10)
}

// lookupGroupName resolves gid to a group name, or its decimal string.
func lookupGroupName(gid uint32) string {
	if g, err := user.LookupGroupId(strconv.FormatUint(uint64(gid), 10)); err == nil {
		return g.Name
	}

	return strconv.FormatUint(uint64(gid), 10)
}

// droppedMsg formats a dropped-feature message, applying fmt.Sprintf when
// args are given.
func droppedMsg(key string, args ...any) string {
	msg := i18n.T(key)
	if len(args) == 0 {
		return msg
	}

	return fmt.Sprintf(msg, args...)
}

// sortedKeys returns the sorted keys of a string-keyed map, for
// deterministic dropped-feature messages.
func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}

	sort.Strings(keys)

	return keys
}

// fromPKGBUILDDroppedFunctions reports prepare()/build()/check() bodies and
// helper functions, none of which have an nfpm equivalent.
func fromPKGBUILDDroppedFunctions(p *pkgbuild.PKGBUILD) []string {
	var messages []string

	if p.Prepare != "" {
		messages = append(messages, droppedMsg("messages.nfpm.convert.dropped_prepare"))
	}

	if p.Build != "" {
		messages = append(messages, droppedMsg("messages.nfpm.convert.dropped_build"))
	}

	if p.Check != "" {
		messages = append(messages, droppedMsg("messages.nfpm.convert.dropped_check"))
	}

	if len(p.HelperFunctions) > 0 {
		joined := strings.Join(sortedKeys(p.HelperFunctions), ", ")
		messages = append(messages, droppedMsg("messages.nfpm.convert.dropped_helper_functions", joined))
	}

	return messages
}

// fromPKGBUILDDroppedArrays reports array-valued PKGBUILD features with no
// nfpm equivalent.
func fromPKGBUILDDroppedArrays(p *pkgbuild.PKGBUILD) []string {
	var messages []string

	if len(p.SourceURI) > 0 || len(p.HashSums) > 0 {
		messages = append(messages, droppedMsg("messages.nfpm.convert.dropped_sources"))
	}

	if len(p.CustomVariables) > 0 {
		joined := strings.Join(sortedKeys(p.CustomVariables), ", ")
		messages = append(messages, droppedMsg("messages.nfpm.convert.dropped_custom_variables", joined))
	}

	if len(p.CustomArrays) > 0 {
		joined := strings.Join(sortedKeys(p.CustomArrays), ", ")
		messages = append(messages, droppedMsg("messages.nfpm.convert.dropped_custom_arrays", joined))
	}

	if len(p.MakeDepends) > 0 {
		joined := strings.Join(p.MakeDepends, ", ")
		messages = append(messages, droppedMsg("messages.nfpm.convert.dropped_makedepends", joined))
	}

	if len(p.Options) > 0 {
		joined := strings.Join(p.Options, ", ")
		messages = append(messages, droppedMsg("messages.nfpm.convert.dropped_options", joined))
	}

	if len(p.NoExtract) > 0 {
		joined := strings.Join(p.NoExtract, ", ")
		messages = append(messages, droppedMsg("messages.nfpm.convert.dropped_no_extract", joined))
	}

	if len(p.Enhances) > 0 {
		joined := strings.Join(p.Enhances, ", ")
		messages = append(messages, droppedMsg("messages.nfpm.convert.dropped_enhances", joined))
	}

	if len(p.Supplements) > 0 {
		joined := strings.Join(p.Supplements, ", ")
		messages = append(messages, droppedMsg("messages.nfpm.convert.dropped_supplements", joined))
	}

	return messages
}

// fromPKGBUILDDroppedScalars reports scalar-valued PKGBUILD features with no
// nfpm equivalent.
func fromPKGBUILDDroppedScalars(p *pkgbuild.PKGBUILD) []string {
	var messages []string

	if p.Install != "" {
		messages = append(messages, droppedMsg("messages.nfpm.convert.dropped_install", p.Install))
	}

	if len(p.Copyright) > 0 {
		messages = append(messages, droppedMsg("messages.nfpm.convert.dropped_copyright"))
	}

	return messages
}
