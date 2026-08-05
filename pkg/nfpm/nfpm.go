package nfpm

import (
	"os"
	"time"
)

// Config is the whole nfpm.yaml document: the base Info plus an optional set
// of per-packager overrides keyed by packager name (see Packagers).
type Config struct {
	Info      `yaml:",inline"`
	Overrides map[string]*Overridables `yaml:"overrides,omitempty"`

	// baseDir is the directory containing the spec file this Config was
	// loaded from (set by Load, empty for Parse). Relative Content.Source
	// and script paths resolve against it. Unexported: it is bookkeeping,
	// never part of the on-disk YAML document.
	baseDir string
}

// Info holds the package-identity metadata that sits at the top level of an
// nfpm.yaml document, alongside the overridable fields inlined from
// Overridables.
type Info struct {
	Overridables `yaml:",inline"`

	// Name is the package name. Required.
	Name string `yaml:"name"`
	// Arch is the target architecture in nfpm's own vocabulary (e.g. "amd64",
	// "all"). Required unless a per-packager Arch is set instead.
	Arch string `yaml:"arch,omitempty"`
	// Platform is the target OS. YAP only supports "linux".
	Platform string `yaml:"platform,omitempty"`
	// Version is the package version. Required.
	Version string `yaml:"version"`
	// VersionSchema selects how Version is interpreted: "semver" (the
	// default, also used when empty) or "none" to disable SplitVersion.
	VersionSchema string `yaml:"version_schema,omitempty"`
	// Epoch is the packaging epoch, prefixed to Version by RPM/Deb.
	Epoch string `yaml:"epoch,omitempty"`
	// Release is the packaging release/revision number.
	Release string `yaml:"release,omitempty"`
	// Prerelease is the semver prerelease identifier (e.g. "beta.1").
	Prerelease string `yaml:"prerelease,omitempty"`
	// VersionMetadata is the semver build-metadata identifier (e.g. a commit
	// hash), appended after a "+".
	VersionMetadata string `yaml:"version_metadata,omitempty"`
	// Section is the Debian/RPM package section (e.g. "utils").
	Section string `yaml:"section,omitempty"`
	// Priority is the Debian/RPM package priority (e.g. "optional").
	Priority string `yaml:"priority,omitempty"`
	// Maintainer identifies the package maintainer ("Name <email>").
	Maintainer string `yaml:"maintainer,omitempty"`
	// Description is the package's human-readable description.
	Description string `yaml:"description,omitempty"`
	// Vendor names the organisation producing the package.
	Vendor string `yaml:"vendor,omitempty"`
	// Homepage is the upstream project URL.
	Homepage string `yaml:"homepage,omitempty"`
	// License is the SPDX (or free-form) license identifier.
	License string `yaml:"license,omitempty"`
	// Changelog points at a goreleaser/chglog YAML file (see changelog.go),
	// relative to the spec's directory.
	Changelog string `yaml:"changelog,omitempty"`
	// DisableGlobbing treats every Content.Source as a literal path instead
	// of a glob pattern, and additionally skips tree expansion and on-disk
	// existence/metadata resolution entirely (see PrepareForPackager).
	DisableGlobbing bool `yaml:"disable_globbing,omitempty"`
	// Umask masks the on-disk permission bits copied into packaged content
	// when no explicit Content.FileInfo.Mode is set.
	Umask os.FileMode `yaml:"umask,omitempty"`
	// MTime is the timestamp stamped onto packaged content that doesn't
	// declare its own Content.FileInfo.MTime.
	MTime time.Time `yaml:"mtime,omitempty"`
}

// Overridables holds every field that can be overridden per packager under
// `overrides:`. It is inlined into Info for the base document and reused
// verbatim as the value type of Config.Overrides.
type Overridables struct {
	// Replaces lists packages this package replaces/obsoletes.
	Replaces []string `yaml:"replaces,omitempty"`
	// Provides lists virtual packages/capabilities this package provides.
	Provides []string `yaml:"provides,omitempty"`
	// Depends lists mandatory runtime dependencies.
	Depends []string `yaml:"depends,omitempty"`
	// Recommends lists strongly-suggested (but optional) dependencies.
	Recommends []string `yaml:"recommends,omitempty"`
	// Suggests lists weakly-suggested optional dependencies.
	Suggests []string `yaml:"suggests,omitempty"`
	// Conflicts lists packages that cannot be installed alongside this one.
	Conflicts []string `yaml:"conflicts,omitempty"`
	// Contents is the list of files/directories/symlinks/trees shipped by
	// the package.
	Contents Contents `yaml:"contents,omitempty"`
	// Scripts holds the packager-agnostic lifecycle scriptlets.
	Scripts Scripts `yaml:"scripts,omitempty"`
	// RPM holds RPM-specific settings.
	RPM RPM `yaml:"rpm,omitempty"`
	// Deb holds Debian-specific settings.
	Deb Deb `yaml:"deb,omitempty"`
	// APK holds Alpine APK-specific settings.
	APK APK `yaml:"apk,omitempty"`
	// ArchLinux holds Arch Linux pacman-specific settings.
	ArchLinux ArchLinux `yaml:"archlinux,omitempty"`
	// IPK holds OpenWrt IPK-specific settings.
	IPK IPK `yaml:"ipk,omitempty"`
}

// Scripts holds the four packager-agnostic lifecycle scriptlet paths shared
// by every packaging format.
type Scripts struct {
	// PreInstall runs before install/upgrade.
	PreInstall string `yaml:"preinstall,omitempty"`
	// PostInstall runs after install/upgrade.
	PostInstall string `yaml:"postinstall,omitempty"`
	// PreRemove runs before removal.
	PreRemove string `yaml:"preremove,omitempty"`
	// PostRemove runs after removal.
	PostRemove string `yaml:"postremove,omitempty"`
}

// RPM holds settings specific to the RPM packager.
type RPM struct {
	// Arch overrides the top-level Arch for RPM packages only.
	Arch string `yaml:"arch,omitempty"`
	// BuildHost is recorded in the RPM header as the build host name.
	BuildHost string `yaml:"buildhost,omitempty"`
	// Scripts holds RPM-only scriptlets (pretrans/posttrans/verify).
	Scripts RPMScripts `yaml:"scripts,omitempty"`
	// Requires holds RPM-only dependency lists beyond Depends.
	Requires RPMRequires `yaml:"requires,omitempty"`
	// Group is the legacy RPM %group classification.
	Group string `yaml:"group,omitempty"`
	// Summary is the RPM one-line summary (falls back to Description).
	Summary string `yaml:"summary,omitempty"`
	// Compression selects the RPM payload compression: gzip|lzma|xz|zstd.
	Compression string `yaml:"compression,omitempty"`
	// Signature holds RPM package-signing settings.
	Signature RPMSignature `yaml:"signature,omitempty"`
	// Packager is recorded in the RPM header as the packager identity.
	Packager string `yaml:"packager,omitempty"`
	// Prefixes lists relocatable installation prefixes.
	Prefixes []string `yaml:"prefixes,omitempty"`
	// Ghosts lists additional %ghost paths beyond those declared in Contents.
	Ghosts []string `yaml:"ghost_files,omitempty"`
}

// RPMScripts holds the RPM-only lifecycle scriptlets that have no equivalent
// in Scripts.
type RPMScripts struct {
	// PreTrans runs before the RPM transaction begins.
	PreTrans string `yaml:"pretrans,omitempty"`
	// PostTrans runs after the RPM transaction completes.
	PostTrans string `yaml:"posttrans,omitempty"`
	// Verify runs during `rpm --verify`.
	Verify string `yaml:"verify,omitempty"`
}

// RPMRequires holds RPM-only dependency lists that don't map to Overridables.
type RPMRequires struct {
	// Post lists dependencies required only by the %post scriptlet.
	Post []string `yaml:"post,omitempty"`
}

// RPMSignature holds RPM package-signing settings.
type RPMSignature struct {
	// KeyFile is the path to the PGP private key used to sign the package.
	KeyFile string `yaml:"key_file,omitempty"`
	// KeyID selects which key to use when KeyFile holds a keyring.
	KeyID string `yaml:"key_id,omitempty"`
	// KeyPassphrase is never read from YAML; it is resolved from
	// NFPM_RPM_PASSPHRASE, falling back to NFPM_PASSPHRASE.
	KeyPassphrase string `yaml:"-"`
}

// Deb holds settings specific to the Debian packager.
type Deb struct {
	// Arch overrides the top-level Arch for Debian packages only.
	Arch string `yaml:"arch,omitempty"`
	// ArchVariant is appended to Arch (e.g. "v7" for armhf/v7).
	ArchVariant string `yaml:"arch_variant,omitempty"`
	// Scripts holds Debian-only scriptlets/maintainer-script assets.
	Scripts DebScripts `yaml:"scripts,omitempty"`
	// Triggers holds dpkg trigger declarations.
	Triggers DebTriggers `yaml:"triggers,omitempty"`
	// Breaks lists packages broken by this one.
	Breaks []string `yaml:"breaks,omitempty"`
	// Signature holds Debian package-signing settings.
	Signature DebSignature `yaml:"signature,omitempty"`
	// Compression selects the Debian payload compression, optionally with a
	// level: "gzip|xz|zstd|none" or "algo:level".
	Compression string `yaml:"compression,omitempty"`
	// Fields holds arbitrary extra debian/control fields.
	Fields map[string]string `yaml:"fields,omitempty"`
	// Predepends lists dpkg Pre-Depends entries.
	Predepends []string `yaml:"predepends,omitempty"`
}

// DebScripts holds Debian-only maintainer-script assets that have no
// equivalent in Scripts.
type DebScripts struct {
	// Rules is the path to a custom debian/rules file.
	Rules string `yaml:"rules,omitempty"`
	// Templates is the path to a debconf templates file.
	Templates string `yaml:"templates,omitempty"`
	// Config is the path to a debconf config script.
	Config string `yaml:"config,omitempty"`
}

// DebTriggers holds dpkg trigger declarations, split by directive and await
// semantics.
type DebTriggers struct {
	// Interest lists paths this package is interested in.
	Interest []string `yaml:"interest,omitempty"`
	// InterestAwait lists interest-await triggers.
	InterestAwait []string `yaml:"interest_await,omitempty"`
	// InterestNoAwait lists interest-noawait triggers.
	InterestNoAwait []string `yaml:"interest_noawait,omitempty"`
	// Activate lists paths this package activates on other packages.
	Activate []string `yaml:"activate,omitempty"`
	// ActivateAwait lists activate-await triggers.
	ActivateAwait []string `yaml:"activate_await,omitempty"`
	// ActivateNoAwait lists activate-noawait triggers.
	ActivateNoAwait []string `yaml:"activate_noawait,omitempty"`
}

// DebSignature holds Debian package-signing settings.
type DebSignature struct {
	// KeyFile is the path to the PGP private key used to sign the package.
	KeyFile string `yaml:"key_file,omitempty"`
	// KeyID selects which key to use when KeyFile holds a keyring.
	KeyID string `yaml:"key_id,omitempty"`
	// Method selects the signing tool: dpkg-sig|debsign.
	Method string `yaml:"method,omitempty"`
	// Type selects the debsign signature role: origin|maint|archive.
	Type string `yaml:"type,omitempty"`
	// Signer identifies the signer for debsign.
	Signer string `yaml:"signer,omitempty"`
	// KeyPassphrase is never read from YAML; it is resolved from
	// NFPM_DEB_PASSPHRASE, falling back to NFPM_PASSPHRASE.
	KeyPassphrase string `yaml:"-"`
}

// APK holds settings specific to the Alpine APK packager.
type APK struct {
	// Arch overrides the top-level Arch for APK packages only.
	Arch string `yaml:"arch,omitempty"`
	// Signature holds APK package-signing settings.
	Signature APKSignature `yaml:"signature,omitempty"`
	// Scripts holds APK-only scriptlets (preupgrade/postupgrade).
	Scripts APKScripts `yaml:"scripts,omitempty"`
}

// APKScripts holds the APK-only lifecycle scriptlets that have no
// equivalent in Scripts.
type APKScripts struct {
	// PreUpgrade runs before an in-place upgrade.
	PreUpgrade string `yaml:"preupgrade,omitempty"`
	// PostUpgrade runs after an in-place upgrade.
	PostUpgrade string `yaml:"postupgrade,omitempty"`
}

// APKSignature holds APK package-signing settings.
type APKSignature struct {
	// KeyFile is the path to the RSA private key used to sign the package.
	KeyFile string `yaml:"key_file,omitempty"`
	// KeyID selects which key to use when KeyFile holds multiple keys.
	KeyID string `yaml:"key_id,omitempty"`
	// KeyName is the name embedded in the APK signature metadata.
	KeyName string `yaml:"key_name,omitempty"`
	// KeyPassphrase is never read from YAML; it is resolved from
	// NFPM_APK_PASSPHRASE, falling back to NFPM_PASSPHRASE.
	KeyPassphrase string `yaml:"-"`
}

// ArchLinux holds settings specific to the Arch Linux pacman packager.
type ArchLinux struct {
	// Pkgbase overrides the pacman pkgbase (defaults to Name).
	Pkgbase string `yaml:"pkgbase,omitempty"`
	// Arch overrides the top-level Arch for pacman packages only.
	Arch string `yaml:"arch,omitempty"`
	// Packager is recorded in the pacman .PKGINFO as the packager identity.
	Packager string `yaml:"packager,omitempty"`
	// Scripts holds pacman-only scriptlets (preupgrade/postupgrade).
	Scripts ArchLinuxScripts `yaml:"scripts,omitempty"`
}

// ArchLinuxScripts holds the pacman-only lifecycle scriptlets that have no
// equivalent in Scripts.
type ArchLinuxScripts struct {
	// PreUpgrade runs before an in-place upgrade.
	PreUpgrade string `yaml:"preupgrade,omitempty"`
	// PostUpgrade runs after an in-place upgrade.
	PostUpgrade string `yaml:"postupgrade,omitempty"`
}

// IPK holds settings specific to the OpenWrt IPK packager.
type IPK struct {
	// ABIVersion is the OpenWrt ABI version suffix.
	ABIVersion string `yaml:"abi_version,omitempty"`
	// Alternatives declares update-alternatives entries.
	Alternatives []IPKAlternative `yaml:"alternatives,omitempty"`
	// Arch overrides the top-level Arch for IPK packages only.
	Arch string `yaml:"arch,omitempty"`
	// AutoInstalled marks the package as automatically installed.
	AutoInstalled bool `yaml:"auto_installed,omitempty"`
	// Essential marks the package as essential to the base system.
	Essential bool `yaml:"essential,omitempty"`
	// Fields holds arbitrary extra control fields.
	Fields map[string]string `yaml:"fields,omitempty"`
	// Predepends lists Pre-Depends entries.
	Predepends []string `yaml:"predepends,omitempty"`
	// Tags lists opkg package tags.
	Tags []string `yaml:"tags,omitempty"`
}

// IPKAlternative declares a single update-alternatives entry for the IPK
// packager.
type IPKAlternative struct {
	// Priority is the alternatives priority (higher wins).
	Priority int `yaml:"priority,omitempty"`
	// Target is the real path the alternative resolves to.
	Target string `yaml:"target,omitempty"`
	// LinkName is the managed symlink path.
	LinkName string `yaml:"link_name,omitempty"`
}
