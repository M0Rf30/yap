package nfpm

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"sort"

	"gopkg.in/yaml.v3"

	"github.com/M0Rf30/yap/v2/pkg/constants"
	"github.com/M0Rf30/yap/v2/pkg/errors"
	"github.com/M0Rf30/yap/v2/pkg/files"
)

// Packager name constants — the exact strings valid in `overrides:` keys and
// Content.Packager.
const (
	// PackagerDeb selects the Debian packager.
	PackagerDeb = "deb"
	// PackagerRPM selects the RPM packager.
	PackagerRPM = "rpm"
	// PackagerAPK selects the Alpine APK packager.
	PackagerAPK = "apk"
	// PackagerArchLinux selects the Arch Linux pacman packager.
	PackagerArchLinux = "archlinux"
	// PackagerIPK selects the OpenWrt IPK packager.
	PackagerIPK = "ipk"
)

// Packagers is the canonical ordered set of supported packager names.
var Packagers = []string{PackagerDeb, PackagerRPM, PackagerAPK, PackagerArchLinux, PackagerIPK}

// platformLinux is the only Platform value YAP's nfpm support accepts.
const platformLinux = "linux"

// defaultPriority is the Debian/RPM priority WithDefaults falls back to.
const defaultPriority = "optional"

// PackagerForFormat maps a yap package format (constants.FormatDEB / FormatRPM /
// FormatAPK / FormatPacman) to the nfpm packager name. Returns "" when unknown
// (including constants.FormatDEB's IPK counterpart, which yap doesn't build).
func PackagerForFormat(format string) string {
	switch format {
	case constants.FormatDEB:
		return PackagerDeb
	case constants.FormatRPM:
		return PackagerRPM
	case constants.FormatAPK:
		return PackagerAPK
	case constants.FormatPacman:
		return PackagerArchLinux
	default:
		return ""
	}
}

// FormatForPackager is the inverse of PackagerForFormat; returns "" when
// unknown (including PackagerIPK, which has no yap build format).
func FormatForPackager(packager string) string {
	switch packager {
	case PackagerDeb:
		return constants.FormatDEB
	case PackagerRPM:
		return constants.FormatRPM
	case PackagerAPK:
		return constants.FormatAPK
	case PackagerArchLinux:
		return constants.FormatPacman
	default:
		return ""
	}
}

// isKnownPackager reports whether packager is one of Packagers.
func isKnownPackager(packager string) bool {
	return slices.Contains(Packagers, packager)
}

// Parse reads a nfpm.yaml document, expands ${VAR}/$VAR with os.Getenv,
// resolves signature passphrases from the environment, applies defaults and
// validates. Strict: unknown top-level (or nested) keys are an error.
func Parse(r io.Reader) (*Config, error) {
	return ParseWithEnvMapping(r, nil)
}

// ParseWithEnvMapping is Parse with a custom ${VAR}/$VAR resolver (nil =>
// os.Getenv).
func ParseWithEnvMapping(r io.Reader, mapping func(string) string) (*Config, error) {
	if mapping == nil {
		mapping = os.Getenv
	}

	data, err := io.ReadAll(r)
	if err != nil {
		return nil, errors.Wrap(err, errors.ErrTypeConfiguration, "failed to read nfpm spec").
			WithOperation("Parse")
	}

	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)

	var cfg Config
	if err := dec.Decode(&cfg); err != nil {
		return nil, errors.Wrap(err, errors.ErrTypeConfiguration, "failed to decode nfpm spec").
			WithOperation("Parse")
	}

	cfg.expandEnv(mapping)
	cfg.resolvePassphrases()
	cfg.WithDefaults()

	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	return &cfg, nil
}

// Load parses the nfpm spec at path and records its directory for later
// relative resolution (see BaseDir).
func Load(path string) (*Config, error) {
	f, err := os.Open(filepath.Clean(path))
	if err != nil {
		return nil, errors.Wrap(err, errors.ErrTypeFileSystem, "failed to open nfpm spec").
			WithOperation("Load").
			WithContext("path", path)
	}
	defer func() { _ = f.Close() }()

	cfg, err := Parse(f)
	if err != nil {
		return nil, err
	}

	cfg.SetBaseDir(filepath.Dir(path))

	return cfg, nil
}

// BaseDir returns the directory of the file Load read, or "" for a Config
// produced by Parse/ParseWithEnvMapping.
func (c *Config) BaseDir() string {
	return c.baseDir
}

// SetBaseDir overrides the directory used to resolve relative src/script
// paths.
func (c *Config) SetBaseDir(dir string) {
	c.baseDir = dir
}

// WithDefaults fills platform=linux, arch=amd64, version=v0.0.0-rc0,
// description="no description given", umask=0o02, mtime=SOURCE_DATE_EPOCH||now,
// release="1", priority="optional", and splits Version per VersionSchema.
// It always returns c, for chaining.
func (c *Config) WithDefaults() *Config {
	if c.Platform == "" {
		c.Platform = platformLinux
	}

	if c.Arch == "" {
		c.Arch = constants.ArchNfpmAmd64
	}

	if c.Version == "" {
		c.Version = "v0.0.0-rc0"
	}

	if c.Description == "" {
		c.Description = "no description given"
	}

	if c.Umask == 0 {
		c.Umask = 0o02
	}

	if c.Release == "" {
		c.Release = "1"
	}

	if c.Priority == "" {
		c.Priority = defaultPriority
	}

	if c.MTime.IsZero() {
		c.MTime = files.SourceDateEpochFromEnv()
	}

	if c.VersionSchema == "" || c.VersionSchema == "semver" {
		version, prerelease, metadata := SplitVersion(c.Version)
		c.Version = version

		if c.Prerelease == "" {
			c.Prerelease = prerelease
		}

		if c.VersionMetadata == "" {
			c.VersionMetadata = metadata
		}
	}

	return c
}

// Validate enforces: Name non-empty, Version non-empty, Arch non-empty unless
// a per-packager arch is set, Platform=="linux", no duplicate Content
// destinations per packager, known packager keys in Overrides, and known
// Content.Type values. Errors are pkg/errors YapError with ErrTypeValidation
// and a WithContext("field", …).
func (c *Config) Validate() error {
	if err := c.validateIdentity(); err != nil {
		return err
	}

	if err := c.validateOverrideKeys(); err != nil {
		return err
	}

	if err := validateContentTypes(c.Contents, ""); err != nil {
		return err
	}

	overrideKeys := make([]string, 0, len(c.Overrides))
	for key := range c.Overrides {
		overrideKeys = append(overrideKeys, key)
	}

	sort.Strings(overrideKeys)

	for _, key := range overrideKeys {
		if err := validateContentTypes(c.Overrides[key].Contents, key); err != nil {
			return err
		}
	}

	for _, packager := range Packagers {
		if err := c.validateNoDuplicateDestinations(packager); err != nil {
			return err
		}
	}

	return nil
}

// validateIdentity checks the scalar identity fields: Name, Version, Arch
// (or a per-packager Arch), and Platform.
func (c *Config) validateIdentity() error {
	if c.Name == "" {
		return errors.New(errors.ErrTypeValidation, "name is required").
			WithOperation("Validate").WithContext("field", "name")
	}

	if c.Version == "" {
		return errors.New(errors.ErrTypeValidation, "version is required").
			WithOperation("Validate").WithContext("field", "version")
	}

	if c.Arch == "" && c.Deb.Arch == "" && c.RPM.Arch == "" && c.APK.Arch == "" &&
		c.ArchLinux.Arch == "" && c.IPK.Arch == "" {
		return errors.New(errors.ErrTypeValidation,
			"arch is required unless a per-packager arch is set").
			WithOperation("Validate").WithContext("field", "arch")
	}

	if c.Platform != platformLinux {
		return errors.New(errors.ErrTypeValidation,
			fmt.Sprintf("unsupported platform %q: only \"linux\" is supported", c.Platform)).
			WithOperation("Validate").WithContext("field", "platform")
	}

	return nil
}

// validateOverrideKeys checks that every Overrides key names a known
// packager.
func (c *Config) validateOverrideKeys() error {
	keys := make([]string, 0, len(c.Overrides))
	for key := range c.Overrides {
		keys = append(keys, key)
	}

	sort.Strings(keys)

	for _, key := range keys {
		if !isKnownPackager(key) {
			return errors.New(errors.ErrTypeValidation, fmt.Sprintf("unknown packager override %q", key)).
				WithOperation("Validate").
				WithContext("field", "overrides").
				WithContext("packager", key)
		}
	}

	return nil
}

// validateContentTypes checks that every entry in cs has a known
// Content.Type. packager is used only to enrich the error context ("" for
// the base Overridables).
func validateContentTypes(cs Contents, packager string) error {
	for _, content := range cs {
		if !knownContentTypes[content.Type] {
			msg := fmt.Sprintf("unknown content type %q", content.Type)

			err := errors.New(errors.ErrTypeValidation, msg).
				WithOperation("Validate").
				WithContext("field", "contents").
				WithContext("destination", content.Destination)
			if packager != "" {
				err = err.WithContext("packager", packager)
			}

			return err
		}
	}

	return nil
}

// resolvedContentsForValidation returns the raw (unexpanded) Contents list
// that ForPackager would use as its starting point for packager: the
// override's Contents when non-empty, else the base Contents, filtered by
// per-entry Packager restriction. It never touches disk — see
// validateNoDuplicateDestinations for why.
func (c *Config) resolvedContentsForValidation(packager string) Contents {
	contents := c.Contents
	if ov, ok := c.Overrides[packager]; ok && len(ov.Contents) > 0 {
		contents = ov.Contents
	}

	out := make(Contents, 0, len(contents))

	for _, content := range contents {
		if content.Packager != "" && content.Packager != packager {
			continue
		}

		out = append(out, content)
	}

	return out
}

// validateNoDuplicateDestinations checks for duplicate Content.Destination
// values within one packager's override-resolved (but not glob/tree
// expanded) content set — expansion requires disk access that Validate must
// not depend on, since it also runs for Parse (no BaseDir).
func (c *Config) validateNoDuplicateDestinations(packager string) error {
	contents := c.resolvedContentsForValidation(packager)

	seen := make(map[string]string, len(contents))

	for _, content := range contents {
		dest := cleanDestination(content.Destination)

		if prevSource, ok := seen[dest]; ok {
			return errors.New(errors.ErrTypeValidation,
				fmt.Sprintf("duplicate content destination %q for packager %q (sources %q and %q)",
					dest, packager, prevSource, content.Source)).
				WithOperation("Validate").
				WithContext("field", "contents").
				WithContext("packager", packager).
				WithContext("destination", dest)
		}

		seen[dest] = content.Source
	}

	return nil
}

// ForPackager returns a copy of Info with Overrides[packager] merged over
// the base Overridables (per-field replace semantics: a non-empty override
// field replaces the base field wholesale — matching nfpm), with Contents
// already run through PrepareForPackager. packager must be one of Packagers.
func (c *Config) ForPackager(packager string) (*Info, error) {
	if !isKnownPackager(packager) {
		return nil, errors.New(errors.ErrTypeValidation, fmt.Sprintf("unknown packager %q", packager)).
			WithOperation("ForPackager").WithContext("field", "packager")
	}

	info := c.Info

	if ov, ok := c.Overrides[packager]; ok {
		mergeOverridables(&info.Overridables, ov)
	}

	prepared, err := info.Contents.PrepareForPackager(
		c.BaseDir(), packager, info.Umask, info.DisableGlobbing, info.MTime)
	if err != nil {
		return nil, err
	}

	info.Contents = prepared

	return &info, nil
}

// expandEnv expands ${VAR}/$VAR in every field nfpm exposes to variable
// substitution: the Info-only scalars, the base Overridables, and every
// Overrides entry.
func (c *Config) expandEnv(mapping func(string) string) {
	c.Name = os.Expand(c.Name, mapping)
	c.Arch = os.Expand(c.Arch, mapping)
	c.Platform = os.Expand(c.Platform, mapping)
	c.Version = os.Expand(c.Version, mapping)
	c.Release = os.Expand(c.Release, mapping)
	c.Prerelease = os.Expand(c.Prerelease, mapping)
	c.Homepage = os.Expand(c.Homepage, mapping)
	c.Maintainer = os.Expand(c.Maintainer, mapping)
	c.Vendor = os.Expand(c.Vendor, mapping)
	c.Description = os.Expand(c.Description, mapping)

	expandOverridables(&c.Overridables, mapping)

	for _, ov := range c.Overrides {
		expandOverridables(ov, mapping)
	}
}

// expandOverridables expands every ${VAR}/$VAR-eligible field within one
// Overridables value in place.
func expandOverridables(o *Overridables, mapping func(string) string) {
	expandStringSlice(o.Depends, mapping)
	expandStringSlice(o.Recommends, mapping)
	expandStringSlice(o.Suggests, mapping)
	expandStringSlice(o.Provides, mapping)
	expandStringSlice(o.Replaces, mapping)
	expandStringSlice(o.Conflicts, mapping)

	o.RPM.Packager = os.Expand(o.RPM.Packager, mapping)
	expandStringSlice(o.RPM.Requires.Post, mapping)
	o.RPM.Signature.KeyFile = os.Expand(o.RPM.Signature.KeyFile, mapping)
	o.RPM.Signature.KeyID = os.Expand(o.RPM.Signature.KeyID, mapping)

	expandStringMap(o.Deb.Fields, mapping)
	expandStringSlice(o.Deb.Predepends, mapping)
	o.Deb.Signature.KeyFile = os.Expand(o.Deb.Signature.KeyFile, mapping)
	o.Deb.Signature.KeyID = os.Expand(o.Deb.Signature.KeyID, mapping)

	o.APK.Signature.KeyFile = os.Expand(o.APK.Signature.KeyFile, mapping)
	o.APK.Signature.KeyID = os.Expand(o.APK.Signature.KeyID, mapping)

	expandStringMap(o.IPK.Fields, mapping)
	expandStringSlice(o.IPK.Predepends, mapping)

	for _, content := range o.Contents {
		if !content.Expand {
			continue
		}

		content.Source = os.Expand(content.Source, mapping)
		content.Destination = os.Expand(content.Destination, mapping)
	}
}

// expandStringSlice expands every element of ss in place.
func expandStringSlice(ss []string, mapping func(string) string) {
	for i, s := range ss {
		ss[i] = os.Expand(s, mapping)
	}
}

// expandStringMap expands every value of m in place.
func expandStringMap(m map[string]string, mapping func(string) string) {
	for k, v := range m {
		m[k] = os.Expand(v, mapping)
	}
}

// resolvePassphrases fills every Signature.KeyPassphrase field (env-only,
// never read from YAML) from NFPM_<PACKAGER>_PASSPHRASE, falling back to
// NFPM_PASSPHRASE. Applied to the base Overridables and every Overrides
// entry, matching expandEnv's scope. Deliberately reads the real process
// environment, not the caller-supplied ${VAR} mapping used by expandEnv.
func (c *Config) resolvePassphrases() {
	resolveOverridablesPassphrases(&c.Overridables)

	for _, ov := range c.Overrides {
		resolveOverridablesPassphrases(ov)
	}
}

// resolveOverridablesPassphrases fills the signature passphrase fields of a
// single Overridables value.
func resolveOverridablesPassphrases(o *Overridables) {
	o.RPM.Signature.KeyPassphrase = passphraseFromEnv("RPM")
	o.Deb.Signature.KeyPassphrase = passphraseFromEnv("DEB")
	o.APK.Signature.KeyPassphrase = passphraseFromEnv("APK")
}

// passphraseFromEnv resolves NFPM_<packagerEnv>_PASSPHRASE, falling back to
// NFPM_PASSPHRASE, returning "" when neither is set.
func passphraseFromEnv(packagerEnv string) string {
	if v, ok := os.LookupEnv("NFPM_" + packagerEnv + "_PASSPHRASE"); ok {
		return v
	}

	if v, ok := os.LookupEnv("NFPM_PASSPHRASE"); ok {
		return v
	}

	return ""
}

// mergeOverridables merges ov over dst using nfpm's per-field replace
// semantics: a non-zero override field replaces the corresponding base
// field wholesale (never appended/merged element-wise).
func mergeOverridables(dst, ov *Overridables) {
	dst.Replaces = mergeStrings(dst.Replaces, ov.Replaces)
	dst.Provides = mergeStrings(dst.Provides, ov.Provides)
	dst.Depends = mergeStrings(dst.Depends, ov.Depends)
	dst.Recommends = mergeStrings(dst.Recommends, ov.Recommends)
	dst.Suggests = mergeStrings(dst.Suggests, ov.Suggests)
	dst.Conflicts = mergeStrings(dst.Conflicts, ov.Conflicts)

	if len(ov.Contents) > 0 {
		dst.Contents = ov.Contents
	}

	dst.Scripts = mergeScripts(dst.Scripts, ov.Scripts)
	dst.RPM = mergeRPM(&dst.RPM, &ov.RPM)
	dst.Deb = mergeDeb(&dst.Deb, &ov.Deb)
	dst.APK = mergeAPK(&dst.APK, &ov.APK)
	dst.ArchLinux = mergeArchLinux(&dst.ArchLinux, &ov.ArchLinux)
	dst.IPK = mergeIPK(&dst.IPK, &ov.IPK)
}

// mergeString returns ov when it is non-empty, else dst.
func mergeString(dst, ov string) string {
	if ov != "" {
		return ov
	}

	return dst
}

// mergeBool returns true when ov is true, else dst — the boolean shape of
// "a non-zero override replaces the base field wholesale".
func mergeBool(dst, ov bool) bool {
	if ov {
		return true
	}

	return dst
}

// mergeStrings returns ov when it is non-empty, else dst. Never appends.
func mergeStrings(dst, ov []string) []string {
	if len(ov) > 0 {
		return ov
	}

	return dst
}

// mergeStringMap returns ov when it is non-empty, else dst. Never merges
// individual keys.
func mergeStringMap(dst, ov map[string]string) map[string]string {
	if len(ov) > 0 {
		return ov
	}

	return dst
}

func mergeScripts(dst, ov Scripts) Scripts {
	return Scripts{
		PreInstall:  mergeString(dst.PreInstall, ov.PreInstall),
		PostInstall: mergeString(dst.PostInstall, ov.PostInstall),
		PreRemove:   mergeString(dst.PreRemove, ov.PreRemove),
		PostRemove:  mergeString(dst.PostRemove, ov.PostRemove),
	}
}

func mergeRPM(dst, ov *RPM) RPM {
	return RPM{
		Arch:        mergeString(dst.Arch, ov.Arch),
		BuildHost:   mergeString(dst.BuildHost, ov.BuildHost),
		Scripts:     mergeRPMScripts(dst.Scripts, ov.Scripts),
		Requires:    RPMRequires{Post: mergeStrings(dst.Requires.Post, ov.Requires.Post)},
		Group:       mergeString(dst.Group, ov.Group),
		Summary:     mergeString(dst.Summary, ov.Summary),
		Compression: mergeString(dst.Compression, ov.Compression),
		Signature:   mergeRPMSignature(dst.Signature, ov.Signature),
		Packager:    mergeString(dst.Packager, ov.Packager),
		Prefixes:    mergeStrings(dst.Prefixes, ov.Prefixes),
		Ghosts:      mergeStrings(dst.Ghosts, ov.Ghosts),
	}
}

func mergeRPMScripts(dst, ov RPMScripts) RPMScripts {
	return RPMScripts{
		PreTrans:  mergeString(dst.PreTrans, ov.PreTrans),
		PostTrans: mergeString(dst.PostTrans, ov.PostTrans),
		Verify:    mergeString(dst.Verify, ov.Verify),
	}
}

func mergeRPMSignature(dst, ov RPMSignature) RPMSignature {
	return RPMSignature{
		KeyFile:       mergeString(dst.KeyFile, ov.KeyFile),
		KeyID:         mergeString(dst.KeyID, ov.KeyID),
		KeyPassphrase: mergeString(dst.KeyPassphrase, ov.KeyPassphrase),
	}
}

func mergeDeb(dst, ov *Deb) Deb {
	return Deb{
		Arch:        mergeString(dst.Arch, ov.Arch),
		ArchVariant: mergeString(dst.ArchVariant, ov.ArchVariant),
		Scripts:     mergeDebScripts(dst.Scripts, ov.Scripts),
		Triggers:    mergeDebTriggers(&dst.Triggers, &ov.Triggers),
		Breaks:      mergeStrings(dst.Breaks, ov.Breaks),
		Signature:   mergeDebSignature(&dst.Signature, &ov.Signature),
		Compression: mergeString(dst.Compression, ov.Compression),
		Fields:      mergeStringMap(dst.Fields, ov.Fields),
		Predepends:  mergeStrings(dst.Predepends, ov.Predepends),
	}
}

func mergeDebScripts(dst, ov DebScripts) DebScripts {
	return DebScripts{
		Rules:     mergeString(dst.Rules, ov.Rules),
		Templates: mergeString(dst.Templates, ov.Templates),
		Config:    mergeString(dst.Config, ov.Config),
	}
}

func mergeDebTriggers(dst, ov *DebTriggers) DebTriggers {
	return DebTriggers{
		Interest:        mergeStrings(dst.Interest, ov.Interest),
		InterestAwait:   mergeStrings(dst.InterestAwait, ov.InterestAwait),
		InterestNoAwait: mergeStrings(dst.InterestNoAwait, ov.InterestNoAwait),
		Activate:        mergeStrings(dst.Activate, ov.Activate),
		ActivateAwait:   mergeStrings(dst.ActivateAwait, ov.ActivateAwait),
		ActivateNoAwait: mergeStrings(dst.ActivateNoAwait, ov.ActivateNoAwait),
	}
}

func mergeDebSignature(dst, ov *DebSignature) DebSignature {
	return DebSignature{
		KeyFile:       mergeString(dst.KeyFile, ov.KeyFile),
		KeyID:         mergeString(dst.KeyID, ov.KeyID),
		Method:        mergeString(dst.Method, ov.Method),
		Type:          mergeString(dst.Type, ov.Type),
		Signer:        mergeString(dst.Signer, ov.Signer),
		KeyPassphrase: mergeString(dst.KeyPassphrase, ov.KeyPassphrase),
	}
}

func mergeAPK(dst, ov *APK) APK {
	return APK{
		Arch:      mergeString(dst.Arch, ov.Arch),
		Signature: mergeAPKSignature(dst.Signature, ov.Signature),
		Scripts:   mergeAPKScripts(dst.Scripts, ov.Scripts),
	}
}

func mergeAPKScripts(dst, ov APKScripts) APKScripts {
	return APKScripts{
		PreUpgrade:  mergeString(dst.PreUpgrade, ov.PreUpgrade),
		PostUpgrade: mergeString(dst.PostUpgrade, ov.PostUpgrade),
	}
}

func mergeAPKSignature(dst, ov APKSignature) APKSignature {
	return APKSignature{
		KeyFile:       mergeString(dst.KeyFile, ov.KeyFile),
		KeyID:         mergeString(dst.KeyID, ov.KeyID),
		KeyName:       mergeString(dst.KeyName, ov.KeyName),
		KeyPassphrase: mergeString(dst.KeyPassphrase, ov.KeyPassphrase),
	}
}

func mergeArchLinux(dst, ov *ArchLinux) ArchLinux {
	return ArchLinux{
		Pkgbase:  mergeString(dst.Pkgbase, ov.Pkgbase),
		Arch:     mergeString(dst.Arch, ov.Arch),
		Packager: mergeString(dst.Packager, ov.Packager),
		Scripts:  mergeArchLinuxScripts(dst.Scripts, ov.Scripts),
	}
}

func mergeArchLinuxScripts(dst, ov ArchLinuxScripts) ArchLinuxScripts {
	return ArchLinuxScripts{
		PreUpgrade:  mergeString(dst.PreUpgrade, ov.PreUpgrade),
		PostUpgrade: mergeString(dst.PostUpgrade, ov.PostUpgrade),
	}
}

func mergeIPK(dst, ov *IPK) IPK {
	return IPK{
		ABIVersion:    mergeString(dst.ABIVersion, ov.ABIVersion),
		Alternatives:  mergeIPKAlternatives(dst.Alternatives, ov.Alternatives),
		Arch:          mergeString(dst.Arch, ov.Arch),
		AutoInstalled: mergeBool(dst.AutoInstalled, ov.AutoInstalled),
		Essential:     mergeBool(dst.Essential, ov.Essential),
		Fields:        mergeStringMap(dst.Fields, ov.Fields),
		Predepends:    mergeStrings(dst.Predepends, ov.Predepends),
		Tags:          mergeStrings(dst.Tags, ov.Tags),
	}
}

// mergeIPKAlternatives returns ov when it is non-empty, else dst — the
// whole slice replaces wholesale, matching every other override field.
func mergeIPKAlternatives(dst, ov []IPKAlternative) []IPKAlternative {
	if len(ov) > 0 {
		return ov
	}

	return dst
}
