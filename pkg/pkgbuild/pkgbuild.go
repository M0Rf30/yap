// Package pkgbuild provides PKGBUILD structure and manipulation functionality.
package pkgbuild

import (
	"bufio"
	"context"
	stderrors "errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"text/template"

	"github.com/github/go-spdx/v2/spdxexp"
	mvdanshell "mvdan.cc/sh/v3/shell"
	"mvdan.cc/sh/v3/syntax"

	"github.com/M0Rf30/yap/v2/pkg/apkindex"
	"github.com/M0Rf30/yap/v2/pkg/aptcache"
	"github.com/M0Rf30/yap/v2/pkg/aptinstall"
	"github.com/M0Rf30/yap/v2/pkg/aptrepo"
	"github.com/M0Rf30/yap/v2/pkg/constants"
	"github.com/M0Rf30/yap/v2/pkg/errors"
	"github.com/M0Rf30/yap/v2/pkg/files"
	"github.com/M0Rf30/yap/v2/pkg/i18n"
	"github.com/M0Rf30/yap/v2/pkg/logger"
	"github.com/M0Rf30/yap/v2/pkg/pacmandb"
	"github.com/M0Rf30/yap/v2/pkg/platform"
	"github.com/M0Rf30/yap/v2/pkg/rpmdb"
	"github.com/M0Rf30/yap/v2/pkg/set"
	"github.com/M0Rf30/yap/v2/pkg/shell"
)

// Architecture constants.
const (
	ArchAny     = "any"
	ArchAarch64 = "aarch64"
	ArchArmv7   = "armv7"
)

// FuncBody is a tagged string type used exclusively for PKGBUILD function
// bodies.  AddItem uses this type to distinguish function declarations from
// plain string variable values so that mapFunctions does not misidentify
// variables like "maintainer" as helper function definitions.
type FuncBody string

const (
	dependsKey        = "depends"
	licenseKey        = "license"
	alpineDistro      = "alpine"
	archDistro        = "arch"
	armLinuxGnueabihf = "arm-linux-gnueabihf"
	i686Arch          = "i686"
	x86_64Arch        = "x86_64"
	ppc64leArch       = "ppc64le"
	s390xArch         = "s390x"
	riscv64Arch       = "riscv64"
	pkgdescKey        = "pkgdesc"
	pkgbaseKey        = "pkgbase"
	pkgnameKey        = "pkgname"
	pkgrelKey         = "pkgrel"
	pkgverKey         = "pkgver"
	makedependsKey    = "makedepends"
	noextractKey      = "noextract"
	sourceKey         = "source"
	b2sumsKey         = "b2sums"
	customKey         = "CUSTOM"
	armv7hArch        = "armv7h"
	mipsArch          = "mips"
	cksumsKey         = "cksums"
	proprietaryKey    = "PROPRIETARY"
	sha224sumsKey     = "sha224sums"
	sha256sumsKey     = "sha256sums"
	sha384sumsKey     = "sha384sums"
	sha512sumsKey     = "sha512sums"
	aptGetPM          = "apt-get"
	aptPM             = "apt"
	dpkgPM            = "dpkg"
	apkPM             = "apk"
)

// Priority constants for PKGBUILD directive matching.
// Higher values indicate a more specific (and therefore preferred) match.
const (
	// prioritySkip means this directive does not apply to the current system.
	prioritySkip = -1
	// priorityBase is the default priority for non-qualified directives.
	priorityBase = 0
	// priorityPackagerMatch is for directives matching the package manager (e.g. "apt").
	priorityPackagerMatch = 1
	// priorityDistroMatch is for directives matching the distro name (e.g. "ubuntu").
	priorityDistroMatch = 2
	// priorityFullDistroMatch is for directives matching distro+codename (e.g. "ubuntu_jammy").
	priorityFullDistroMatch = 3
	// priorityArchMatch is for architecture-specific directives.
	priorityArchMatch = 4
	// priorityArchDistroBoost is added to distro priority for arch+distro combined directives.
	priorityArchDistroBoost = 4
)

// PKGBUILD defines all the fields accepted by the yap specfile (variables,
// arrays, functions).
// templating and other rpm/deb descriptors.
type PKGBUILD struct {
	Arch            []string
	ArchComputed    string
	Backup          []string
	Build           string
	BuildArch       string // Build architecture for cross-compilation (where compilation happens)
	BuildDate       int64
	Changelog       string `json:"changelog,omitempty"`
	Checksum        string
	Codename        string
	Commit          string
	Conflicts       []string
	Copyright       []string
	CustomArrays    map[string][]string
	CustomVariables map[string]string
	DataHash        string
	DebConfig       string
	DebTemplate     string
	Depends         []string
	Distro          string
	Enhances        []string
	Supplements     []string
	Epoch           string
	Files           []string
	FullDistroName  string
	Group           string
	HashSums        []string
	HelperFunctions map[string]string
	Home            string
	HostArch        string // Host architecture for cross-compilation (where package will run)
	// RepoDir is the git repository root. Walks up from the yap.json directory
	// to find a .git dir; falls back to the parent of the yap.json directory.
	RepoDir           string
	Install           string
	InstalledSize     int64
	License           []string
	Maintainer        string
	MakeDepends       []string
	OptDepends        []string
	Options           []string
	NoExtract         []string
	Origin            string
	Package           string
	PackageDir        string
	PkgBase           string // pkgbase — shared base name for split packages; equals PkgName when not a split package
	PkgDesc           string
	PkgDest           string
	PkgName           string
	PkgNames          []string          // pkgname array — populated for split packages; empty for single packages
	SplitPackageFuncs map[string]string // package_<name>() bodies, keyed by sub-package name
	PkgRel            string
	PkgType           string
	PkgVer            string
	PostInst          string
	PostRm            string
	PostTrans         string
	PostUpgrade       string
	PreInst           string
	Prepare           string
	PreRm             string
	PreTrans          string
	PreUpgrade        string
	priorities        map[string]int
	archSourceURI     []string  // arch-specific source_<arch> entries, merged into SourceURI at Finalize
	archHashSums      []string  // arch-specific sha*sums_<arch> entries, merged into HashSums at Finalize
	topLevelSnap      *PKGBUILD // snapshot of overrideable fields before any package_<name>() runs
	Priority          string
	Provides          []string
	Replaces          []string
	Section           string
	SourceDir         string
	SourceURI         []string
	StartDir          string
	TargetArch        string // Target architecture for cross-compilation (what we're building for)
	URL               string
	DebugEnabled      bool
	DocsEnabled       bool
	EmptyDirsEnabled  bool
	LibtoolEnabled    bool
	PurgeEnabled      bool
	StaticEnabled     bool
	StripEnabled      bool
	ZipManEnabled     bool
	YAPVersion        string
}

// AddItem adds an item to the PKGBUILD.
//
// It takes a key string and data of any type as parameters.
// It returns an error.
func (pkgBuild *PKGBUILD) AddItem(key string, data any) error {
	key, priority, err := pkgBuild.parseDirective(key)
	if err != nil || priority == -1 {
		return err
	}

	// Allow base items (priority 0) to be added even if a higher-priority item
	// was already set. This enables order-independent accumulation of arch-specific
	// sources/checksums: base items always go to SourceURI/HashSums, arch-specific
	// items accumulate in archSourceURI/archHashSums and are merged by Finalize().
	oldPriority := pkgBuild.priorities[key]
	if priority < oldPriority && priority != priorityBase {
		return nil
	}

	// Update priority only if new priority is higher (or if this is the first time)
	if priority > oldPriority {
		pkgBuild.priorities[key] = priority
	}

	pkgBuild.mapVariables(key, data)
	pkgBuild.mapArrays(key, data, priority)
	pkgBuild.mapFunctions(key, data)

	return nil
}

// ComputeArchitecture checks if the specified architecture is supported.
// If "any", sets to "any". Otherwise, checks if current architecture is supported.
// Logs error if not supported, then sets to current architecture if supported.
func (pkgBuild *PKGBUILD) ComputeArchitecture() {
	isSupported := set.Contains(pkgBuild.Arch, ArchAny)
	if isSupported {
		pkgBuild.ArchComputed = ArchAny

		return
	}

	currentArch := platform.GetArchitecture()

	isSupported = set.Contains(pkgBuild.Arch, currentArch)
	if !isSupported {
		logger.Fatal(i18n.T("errors.pkgbuild.unsupported_architecture"),
			"pkgname", pkgBuild.PkgName,
			"arch", strings.Join(pkgBuild.Arch, " "))
	}

	pkgBuild.ArchComputed = currentArch
}

// CreateSpec reads the filepath where the specfile will be written and the
// content of the specfile. Specfile generation is done using go templates for
// every different distro family. It returns any error if encountered.
func (pkgBuild *PKGBUILD) CreateSpec(filePath string, tmpl *template.Template) error {
	cleanFilePath := filepath.Clean(filePath)

	file, err := os.Create(cleanFilePath)
	if err != nil {
		return err
	}

	defer func() {
		err := file.Close()
		if err != nil {
			logger.Warn(i18n.T("logger.createspec.warn.failed_to_close_pkgbuild_1"),
				"path", cleanFilePath,
				"error", err)
		}
	}()

	return tmpl.Execute(file, pkgBuild)
}

// filterInstalledPackages checks which packages are not installed and returns only those.
// Uses pure-Go database readers where available; falls back to subprocess only
// when no pure-Go reader is available (legacy BerkeleyDB RPM hosts).
func filterInstalledPackages(packageManager string, packages []string) []string {
	if len(packages) == 0 {
		return nil
	}

	switch packageManager {
	case aptGetPM, aptPM, dpkgPM:
		return filterInstalledDEB(packages)
	case apkPM:
		return filterInstalledAPK(packages)
	case "pacman":
		return filterInstalledPacman(packages)
	case "rpm", "yum", "dnf", "zypper":
		return filterInstalledRPM(packages)
	default:
		return packages
	}
}

// filterInstalledDEB uses the in-process aptcache (reads /var/lib/dpkg/status)
// to check which DEB packages are already installed — no subprocess needed.
//
// Handles :arch qualifiers correctly: a foreign-arch qualifier (e.g. "libssl-dev:arm64"
// on an amd64 host) cannot be answered from a single Installed bit, so those are
// reported as missing and let apt-get install decide (it is idempotent).
func filterInstalledDEB(packages []string) []string {
	cache := aptcache.Load()
	hostDebArch := constants.GetArchMapping().TranslateArch(constants.FormatDEB, runtime.GOARCH)

	missing := make([]string, 0, len(packages))

	for _, pkg := range packages {
		// Strip deb version constraint syntax "name (>= 1.0)" first.
		name, _, _ := strings.Cut(pkg, " (")
		// Separate arch qualifier if present.
		bareName, qualifier, hasQualifier := strings.Cut(name, ":")
		bareName = strings.TrimSpace(bareName)
		qualifier = strings.TrimSpace(qualifier)

		// Foreign-arch qualifier cannot be answered reliably; treat as missing.
		if hasQualifier && qualifier != "" && qualifier != hostDebArch {
			missing = append(missing, pkg)
			continue
		}

		info, ok := cache.Lookup(bareName)
		if !ok || !info.Installed {
			missing = append(missing, pkg)
		}
	}

	return missing
}

// filterInstalledAPK reads /lib/apk/db/installed to check which APK packages are installed.
func filterInstalledAPK(packages []string) []string {
	const apkInstalledDB = "/lib/apk/db/installed"

	installed := apkInstalledSet(apkInstalledDB)
	if installed == nil {
		// DB unreadable — assume nothing is installed so we don't skip deps.
		return packages
	}

	missing := make([]string, 0, len(packages))

	for _, pkg := range packages {
		name := stripVersionConstraint(pkg)
		if !installed[name] {
			missing = append(missing, pkg)
		}
	}

	return missing
}

// apkInstalledSet parses the APK installed database and returns a set of
// installed package names. Returns nil on read error.
func apkInstalledSet(path string) map[string]bool {
	f, err := os.Open(path) // #nosec G304 — constant path
	if err != nil {
		return nil
	}

	defer func() { _ = f.Close() }()

	installed := make(map[string]bool)
	scanner := bufio.NewScanner(f)

	var currentPkg string

	for scanner.Scan() {
		line := scanner.Text()

		if line == "" {
			if currentPkg != "" {
				installed[currentPkg] = true
				currentPkg = ""
			}

			continue
		}

		if pkg, ok := strings.CutPrefix(line, "P:"); ok {
			currentPkg = pkg
		}
	}

	if currentPkg != "" {
		installed[currentPkg] = true
	}

	return installed
}

// filterInstalledPacman checks /var/lib/pacman/local/ for installed packages.
// Each installed package has a subdirectory named "<pkgname>-<version>-<pkgrel>/".
func filterInstalledPacman(packages []string) []string {
	const pacmanLocalDB = "/var/lib/pacman/local"

	entries, err := os.ReadDir(pacmanLocalDB)
	if err != nil {
		return packages
	}

	installed := make(map[string]bool, len(entries))

	for _, e := range entries {
		if !e.IsDir() || e.Name() == "ALPM_DB_VERSION" {
			continue
		}

		name := pacmanDirToName(e.Name())
		if name != "" {
			installed[name] = true
		}
	}

	missing := make([]string, 0, len(packages))

	for _, pkg := range packages {
		name := stripVersionConstraint(pkg)
		if !installed[name] {
			missing = append(missing, pkg)
		}
	}

	return missing
}

// pacmanDirToName extracts the package name from a pacman local DB directory
// name of the form "<name>-<version>-<pkgrel>". pkgver cannot contain hyphens
// (per Arch wiki), so stripping the last two hyphen-delimited segments is safe.
func pacmanDirToName(dir string) string {
	idx := strings.LastIndex(dir, "-")
	if idx < 0 {
		return ""
	}

	withoutPkgrel := dir[:idx]

	idx = strings.LastIndex(withoutPkgrel, "-")
	if idx < 0 {
		return ""
	}

	return withoutPkgrel[:idx]
}

// filterInstalledRPM uses the pure-Go pkg/rpmdb SQLite reader when available
// (Fedora 33+, RHEL 9+, Rocky 9+, Alma 9+, openSUSE 15.5+). Falls back to
// `rpm -q` subprocess for legacy BerkeleyDB hosts.
func filterInstalledRPM(packages []string) []string {
	db, err := rpmdb.Open()
	if err != nil {
		if stderrors.Is(err, rpmdb.ErrLegacyDB) {
			logger.Debug("rpmdb: legacy BDB host, falling back to subprocess")
		} else {
			logger.Warn("rpmdb: open failed, falling back to subprocess", "error", err)
		}

		return filterInstalledRPMSubprocess(packages)
	}

	return db.FilterInstalled(context.Background(), packages)
}

// filterInstalledRPMSubprocess is the legacy BerkeleyDB fallback.
func filterInstalledRPMSubprocess(packages []string) []string {
	missing := make([]string, 0, len(packages))

	for _, pkg := range packages {
		err := shell.Exec(context.Background(), true, "", "rpm", "-q", pkg)
		if err != nil {
			missing = append(missing, pkg)
		}
	}

	return missing
}

// GetDepends reads the package manager name, its arguments and all the
// dependencies required to build the package. It returns any error if
// encountered.
func (pkgBuild *PKGBUILD) GetDepends(packageManager string, args, makeDepends []string) error {
	if len(makeDepends) == 0 {
		return nil
	}

	// Filter out already-installed packages to avoid sudo prompts
	missingPackages := filterInstalledPackages(packageManager, makeDepends)

	// If all packages are already installed, nothing to do
	if len(missingPackages) == 0 {
		logger.Info(i18n.T("logger.pkgbuild.info.all_packages_installed"),
			"count", len(makeDepends))

		return nil
	}

	// Log which packages need installation
	if len(missingPackages) < len(makeDepends) {
		logger.Info(i18n.T("logger.pkgbuild.info.packages_already_installed"),
			"installed", len(makeDepends)-len(missingPackages),
			"missing", len(missingPackages))
	}

	// Resolve virtual packages to concrete providers (apt-get only)
	if packageManager == aptGetPM {
		missingPackages = resolveVirtualPackages(missingPackages)
	}

	// Pure-Go installers — no subprocess fallback. If installation fails,
	// return an error so the user fixes their PKGBUILD or repos rather than
	// silently falling back to a different code path.
	switch packageManager {
	case apkPM:
		if err := apkindex.Install(context.Background(), missingPackages); err != nil {
			return errors.Wrap(err, errors.ErrTypeBuild, "apkindex install failed").
				WithOperation("GetDepends")
		}

		return nil

	case aptGetPM, aptPM:
		if err := aptinstall.Install(context.Background(), missingPackages); err != nil {
			return errors.Wrap(err, errors.ErrTypeBuild, "aptinstall failed").
				WithOperation("GetDepends")
		}

		return nil
	}

	// RPM (dnf/yum/zypper) and Pacman -S have no pure-Go installer yet, so
	// they still go through the package manager subprocess.
	args = append(args, missingPackages...)

	return shell.ExecWithSudo(context.Background(), false, "", packageManager, args...)
}

// resolveVirtualPackages checks each dependency and replaces virtual packages
// (those with no installation candidate) with the first concrete provider.
// This handles cases where a package like "service-discover" is a virtual
// package provided by multiple concrete packages.
func resolveVirtualPackages(deps []string) []string {
	resolved := make([]string, 0, len(deps))

	for _, dep := range deps {
		resolved = append(resolved, resolveVirtualPackage(dep))
	}

	return resolved
}

// resolveVirtualPackage checks if a package is a virtual package and returns
// the first concrete provider if so, or the original package name otherwise.
// It uses the in-process apt cache to avoid spawning apt-cache subprocesses.
func resolveVirtualPackage(pkg string) string {
	cache := aptcache.Load()

	resolved := cache.ResolveVirtual(pkg)
	if resolved != pkg {
		logger.Info("resolved virtual package",
			"virtual", pkg, "provider", resolved)
	}

	return resolved
}

// stripVersionConstraint strips any version constraint suffix from a package
// spec like "musl-dev>=1.2" or "foo!=1.0" → "musl-dev" / "foo".
// Handles all common operators: !=, >=, <=, ~, =, >, <.
// Order matters: !=, >=, <= must be checked before single-char operators.
func stripVersionConstraint(spec string) string {
	spec = strings.TrimSpace(spec)

	// Multi-char operators first (longest match wins).
	for _, op := range []string{"!=", ">=", "<="} {
		if before, _, ok := strings.Cut(spec, op); ok {
			return strings.TrimSpace(before)
		}
	}

	// Single-char operators.
	for _, op := range []string{"~", "=", ">", "<"} {
		if before, _, ok := strings.Cut(spec, op); ok {
			return strings.TrimSpace(before)
		}
	}

	return spec
}

// GetUpdates refreshes package indexes for the given package manager.
// Uses pure-Go implementations where available — no subprocess fallback:
//   - apt-get → pkg/aptrepo (parses sources, fetches InRelease + Packages.*)
//   - apk → pkg/apkindex (fetches APKINDEX.tar.gz from each repo)
//   - pacman → pkg/pacmandb (parses pacman.conf, fetches *.db files)
//
// RPM-family package managers (dnf/yum/zypper) still use subprocess because
// no pure-Go implementation exists yet.
func (pkgBuild *PKGBUILD) GetUpdates(packageManager string, args ...string) error {
	switch packageManager {
	case aptGetPM, aptPM:
		n, err := aptrepo.Update(context.Background())
		if err != nil {
			return errors.Wrap(err, errors.ErrTypeBuild, "aptrepo update failed").
				WithOperation("GetUpdates")
		}

		if n == 0 {
			return errors.New(errors.ErrTypeBuild, "aptrepo update fetched zero indexes").
				WithOperation("GetUpdates")
		}

		logger.Info("aptrepo: pure-Go update succeeded", "indexes", n)
		aptcache.Reload()

		return nil

	case apkPM:
		if _, err := apkindex.Update(context.Background()); err != nil {
			return errors.Wrap(err, errors.ErrTypeBuild, "apkindex update failed").
				WithOperation("GetUpdates")
		}

		return nil

	case "pacman":
		n, err := pacmandb.Sync(context.Background())
		if err != nil {
			return errors.Wrap(err, errors.ErrTypeBuild, "pacmandb sync failed").
				WithOperation("GetUpdates")
		}

		if n == 0 {
			return errors.New(errors.ErrTypeBuild, "pacmandb sync fetched zero repos").
				WithOperation("GetUpdates")
		}

		logger.Info("pacmandb: pure-Go sync succeeded", "repos", n)

		return nil
	}

	return shell.ExecWithSudo(context.Background(), false, "", packageManager, args...)
}

// Init initializes the PKGBUILD struct.
//
// It sets up the priorities map and assigns the full distribution name based on
// the Distro and Codename fields.
func (pkgBuild *PKGBUILD) Init() {
	pkgBuild.priorities = make(map[string]int)
	pkgBuild.CustomVariables = make(map[string]string)
	pkgBuild.CustomArrays = make(map[string][]string)
	pkgBuild.HelperFunctions = make(map[string]string)
	pkgBuild.SplitPackageFuncs = make(map[string]string)

	pkgBuild.FullDistroName = pkgBuild.Distro
	if pkgBuild.Codename != "" {
		pkgBuild.FullDistroName += "_" + pkgBuild.Codename
	}

	pkgBuild.archSourceURI = nil
	pkgBuild.archHashSums = nil

	// Apply option defaults so PKGBUILDs without an options=() array still
	// get the correct behaviour (e.g. emptydirs=true keeps empty dirs).
	pkgBuild.processOptions()
}

// Finalize merges arch-specific source and checksum accumulators into the
// base SourceURI and HashSums slices. Must be called after all AddItem calls
// are complete (i.e. after parsing). Order-independent: arch entries are
// always appended after the base entries regardless of declaration order.
func (pkgBuild *PKGBUILD) Finalize() {
	pkgBuild.SourceURI = append(pkgBuild.SourceURI, pkgBuild.archSourceURI...)
	pkgBuild.HashSums = append(pkgBuild.HashSums, pkgBuild.archHashSums...)
	pkgBuild.archSourceURI = nil
	pkgBuild.archHashSums = nil

	// For split packages, capture the top-level overrideable fields now —
	// before any package_<name>() function runs — so both compileSplitPackages
	// and createSplitPackages can restore clean values per sub-package.
	if pkgBuild.IsSplitPackage() {
		snap := pkgBuild.SnapshotSplitOverrides()
		pkgBuild.topLevelSnap = &snap
	}
}

// splitOverrideKeys is the set of variable names that package_<name>() functions
// may override, matching makepkg's pkgbuild_schema_package_overrides.
// The __distro and _arch suffix variants are handled automatically by AddItem/parseDirective.
var splitOverrideKeys = map[string]struct{}{
	pkgdescKey: {}, archDistro: {}, "url": {}, licenseKey: {}, "groups": {},
	dependsKey: {}, "optdepends": {}, "provides": {}, "conflicts": {}, "replaces": {},
	"backup": {}, "options": {}, "install": {}, "changelog": {},
}

// copySplitOverrideFields copies the scalar and slice fields that
// package_<name>() functions may override from src into dst, and resets the
// priority entries for those keys so AddItem will accept new values.
// This is the single source of truth for which fields are overrideable.
func copySplitOverrideFields(dst, src *PKGBUILD) {
	dst.PkgDesc = src.PkgDesc
	dst.URL = src.URL
	dst.License = append([]string(nil), src.License...)
	dst.Depends = append([]string(nil), src.Depends...)
	dst.OptDepends = append([]string(nil), src.OptDepends...)
	dst.Provides = append([]string(nil), src.Provides...)
	dst.Conflicts = append([]string(nil), src.Conflicts...)
	dst.Replaces = append([]string(nil), src.Replaces...)
	dst.Backup = append([]string(nil), src.Backup...)
	dst.Options = append([]string(nil), src.Options...)

	// Restore the priority entries for overrideable keys so that AddItem
	// will accept the next sub-package's overrides rather than treating
	// the previous sub-package's higher-priority value as already winning.
	for key := range splitOverrideKeys {
		dst.priorities[key] = src.priorities[key]
	}
}

// SnapshotSplitOverrides returns a copy of the PKGBUILD capturing the
// top-level values of all fields that package_<name>() functions may override.
// Call once before the split-package loop; pass to RestoreSplitOverrides before
// each sub-package to prevent overrides from bleeding between sub-packages.
func (pkgBuild *PKGBUILD) SnapshotSplitOverrides() PKGBUILD {
	var snap PKGBUILD

	snap.priorities = make(map[string]int)
	copySplitOverrideFields(&snap, pkgBuild)

	return snap
}

// RestoreSplitOverrides resets all overrideable fields to the snapshot values.
func (pkgBuild *PKGBUILD) RestoreSplitOverrides(snap *PKGBUILD) {
	copySplitOverrideFields(pkgBuild, snap)
}

// RestoreTopLevelOverrides resets all overrideable fields to the top-level
// values captured by Finalize, preventing one sub-package's overrides from
// bleeding into the next. No-op for non-split packages.
func (pkgBuild *PKGBUILD) RestoreTopLevelOverrides() {
	if pkgBuild.topLevelSnap != nil {
		copySplitOverrideFields(pkgBuild, pkgBuild.topLevelSnap)
	}
}

// ParseSplitOverrides parses the body of a package_<name>() function and applies
// any recognized override variables (pkgdesc, depends, conflicts, etc.) to the
// PKGBUILD struct via AddItem — giving full __distro and _arch suffix support for free.
//
// funcBody is the raw function body string as stored in SplitPackageFuncs (no braces).
func (pkgBuild *PKGBUILD) ParseSplitOverrides(funcBody string) error {
	// Wrap the body in a dummy function so the parser sees valid bash syntax,
	// then walk only the assignments inside it.
	wrapped := "_yap_split_fn() {\n" + funcBody + "\n}"

	f, err := syntax.NewParser(syntax.Variant(syntax.LangBash)).Parse(strings.NewReader(wrapped), "")
	if err != nil {
		// Parse errors are non-fatal for override extraction — the body will still
		// be executed by the shell interpreter; we just won't have static overrides.
		logger.Warn("failed to parse split-package function body for override extraction",
			"error", err)

		return nil
	}

	syntax.Walk(f, func(node syntax.Node) bool {
		fd, ok := node.(*syntax.FuncDecl)
		if !ok {
			return true
		}

		// Walk assignments inside the dummy function body.
		syntax.Walk(fd.Body, func(inner syntax.Node) bool {
			pkgBuild.applyOverrideAssign(inner)

			return true
		})

		return false // don't recurse further into the FuncDecl
	})

	return nil
}

// applyOverrideAssign checks whether a syntax node is an assignment for a
// recognized split-package override variable and, if so, applies it via AddItem.
func (pkgBuild *PKGBUILD) applyOverrideAssign(node syntax.Node) {
	assign, ok := node.(*syntax.Assign)
	if !ok {
		return
	}

	name := assign.Name.Value

	// Strip any __distro or _arch suffix to get the base key,
	// then check if it's a recognized override var.
	baseKey, _, hasDistro := strings.Cut(name, "__")
	if !hasDistro {
		// No __ separator — try single _ for arch suffix (e.g. depends_x86_64).
		// Use the last underscore as the split point.
		if idx := strings.LastIndex(name, "_"); idx != -1 {
			baseKey = name[:idx]
		} else {
			baseKey = name
		}
	}

	if _, known := splitOverrideKeys[baseKey]; !known {
		return
	}

	// Apply via AddItem — handles __distro / _arch priority automatically.
	if assign.Array != nil {
		var arrVal []string

		for _, line := range set.StringifyArray(assign) {
			arrVal, _ = mvdanshell.Fields(line, os.Getenv)
		}

		_ = pkgBuild.AddItem(name, arrVal)
	} else {
		varVal, _ := mvdanshell.Expand(set.StringifyAssign(assign), os.Getenv)
		_ = pkgBuild.AddItem(name, varVal)
	}
}

// IsSplitPackage reports whether this PKGBUILD defines multiple packages
// (i.e. pkgname is an array with more than one entry).
func (pkgBuild *PKGBUILD) IsSplitPackage() bool {
	return len(pkgBuild.PkgNames) > 1
}

// EffectivePkgBase returns the base name for this package. For split packages
// this is PkgBase (set from the pkgbase= directive). For single packages it
// falls back to PkgName.
func (pkgBuild *PKGBUILD) EffectivePkgBase() string {
	if pkgBuild.PkgBase != "" {
		return pkgBuild.PkgBase
	}

	return pkgBuild.PkgName
}

// BuildScriptPreamble generates a bash preamble that declares custom scalar
// variables, custom array variables, and helper function definitions so that
// they are available inside build(), prepare(), and package() scripts.
//
// The preamble is prepended to every script body before execution.  Declarations
// are emitted in sorted order so that the output is deterministic.
func (pkgBuild *PKGBUILD) BuildScriptPreamble() string {
	var preamble strings.Builder

	// Emit custom scalar variables (e.g. _prefix="/usr")
	varKeys := make([]string, 0, len(pkgBuild.CustomVariables))
	for k := range pkgBuild.CustomVariables {
		varKeys = append(varKeys, k)
	}

	sort.Strings(varKeys)

	for _, k := range varKeys {
		preamble.WriteString(k)
		preamble.WriteString("='")
		preamble.WriteString(strings.ReplaceAll(pkgBuild.CustomVariables[k], "'", "'\\''"))
		preamble.WriteString("'\n")
	}

	// Emit custom array variables (e.g. _modules=('a' 'b'))
	arrKeys := make([]string, 0, len(pkgBuild.CustomArrays))
	for k := range pkgBuild.CustomArrays {
		arrKeys = append(arrKeys, k)
	}

	sort.Strings(arrKeys)

	for _, k := range arrKeys {
		preamble.WriteString(k)
		preamble.WriteString("=(")

		for idx, value := range pkgBuild.CustomArrays[k] {
			if idx > 0 {
				preamble.WriteByte(' ')
			}

			preamble.WriteByte('\'')
			preamble.WriteString(strings.ReplaceAll(value, "'", "'\\''"))
			preamble.WriteByte('\'')
		}

		preamble.WriteString(")\n")
	}

	// Emit helper function definitions in sorted order
	funcKeys := make([]string, 0, len(pkgBuild.HelperFunctions))
	for k := range pkgBuild.HelperFunctions {
		funcKeys = append(funcKeys, k)
	}

	sort.Strings(funcKeys)

	for _, k := range funcKeys {
		preamble.WriteString(pkgBuild.HelperFunctions[k])
		preamble.WriteByte('\n')
	}

	return preamble.String()
}

// HelperFunctionsPreamble returns a bash snippet containing only the helper
// function definitions that are referenced by the given scriptlet body.
// Unlike BuildScriptPreamble it does NOT emit custom scalar or array variable
// declarations, which are build-time values and have no meaning inside
// package-manager scriptlets (preinst, postinst, prerm, postrm).
//
// Only helpers whose names appear as call-sites in scriptletBody are included,
// so build-time helpers like _package or _package_systemd are not injected
// into install-time scripts.
func (pkgBuild *PKGBUILD) HelperFunctionsPreamble(scriptletBody string) string {
	if len(pkgBuild.HelperFunctions) == 0 {
		return ""
	}

	funcKeys := make([]string, 0, len(pkgBuild.HelperFunctions))
	for k := range pkgBuild.HelperFunctions {
		funcKeys = append(funcKeys, k)
	}

	sort.Strings(funcKeys)

	var preamble strings.Builder

	for _, k := range funcKeys {
		// Only include helpers that are actually called in the scriptlet body.
		// This prevents build-time helpers (e.g. _package, _package_systemd)
		// from being injected into package-manager scriptlets.
		if strings.Contains(scriptletBody, k) {
			preamble.WriteString(pkgBuild.HelperFunctions[k])
			preamble.WriteByte('\n')
		}
	}

	return preamble.String()
}

// RenderSpec initializes a new template with custom functions and parses the provided script.
// It adds two custom functions to the template:
//
//	"join": Takes a slice of strings and joins them into a single string, separated by commas,
//	while also trimming any leading or trailing spaces.
//	"multiline": Takes a string and replaces newline characters with a newline followed by a space,
//	effectively formatting the string for better readability in multi-line contexts.
//
// The method returns the parsed template, which can be used for rendering with data.
func (pkgBuild *PKGBUILD) RenderSpec(script string) *template.Template {
	tmpl := template.New("template").Funcs(template.FuncMap{
		"join": func(strs []string) string {
			return strings.Trim(strings.Join(strs, ", "), " ")
		},
		"multiline": func(strs string) string {
			ret := strings.ReplaceAll(strs, "\n", "\n ")

			return strings.Trim(ret, " \n")
		},
	})

	template.Must(tmpl.Parse(script))

	return tmpl
}

// SetMainFolders sets the main folders for the PKGBUILD.
//
// It takes no parameters.
// It does not return anything.
func (pkgBuild *PKGBUILD) SetMainFolders() {
	switch pkgBuild.Distro {
	case alpineDistro:
		pkgBuild.PackageDir = filepath.Join(pkgBuild.StartDir, "apk", "pkg", pkgBuild.PkgName)
	default:
		var folderName string
		if pkgBuild.Distro == archDistro {
			folderName = "pkg"
		} else {
			folderName = "pkg-" + pkgBuild.Distro
		}

		// For split packages each sub-package gets its own subdirectory so that
		// package_foo() and package_bar() can install into separate trees.
		if pkgBuild.IsSplitPackage() {
			pkgBuild.PackageDir = filepath.Join(pkgBuild.StartDir, folderName, pkgBuild.PkgName)
		} else {
			pkgBuild.PackageDir = filepath.Join(pkgBuild.StartDir, folderName)
		}
	}
}

// SetPackageDirForSplit overrides PackageDir for a specific sub-package name.
// Called by Builder.Compile when iterating split sub-packages.
func (pkgBuild *PKGBUILD) SetPackageDirForSplit(subPkgName string) {
	switch pkgBuild.Distro {
	case alpineDistro:
		pkgBuild.PackageDir = filepath.Join(pkgBuild.StartDir, "apk", "pkg", subPkgName)
	default:
		var folderName string
		if pkgBuild.Distro == archDistro {
			folderName = "pkg"
		} else {
			folderName = "pkg-" + pkgBuild.Distro
		}

		pkgBuild.PackageDir = filepath.Join(pkgBuild.StartDir, folderName, subPkgName)
	}
}

// BuildEnvironmentSlice returns the package-specific environment variables as a
// "KEY=VALUE" slice that can be merged with os.Environ() for safe concurrent use.
// Unlike SetEnvironmentVariables, it does NOT mutate the process environment, making
// it safe to call from multiple goroutines simultaneously (parallel builds).
//
// The returned slice includes:
//   - pkgdir, srcdir, startdir, repodir — per-package directory paths
//     (repodir = git repository root; empty string if not in a git repo)
//   - pkgname, pkgver, pkgrel — per-package identity fields
//   - SOURCE_DATE_EPOCH — for reproducible builds (if set)
func (pkgBuild *PKGBUILD) BuildEnvironmentSlice() []string {
	env := []string{
		"pkgdir=" + pkgBuild.PackageDir,
		"srcdir=" + pkgBuild.SourceDir,
		"startdir=" + pkgBuild.StartDir,
		"repodir=" + pkgBuild.RepoDir,
		"pkgname=" + pkgBuild.PkgName,
		"pkgver=" + pkgBuild.PkgVer,
		"pkgrel=" + pkgBuild.PkgRel,
		"CARCH=" + pkgBuild.GetTargetArchitecture(),
	}

	// Propagate SOURCE_DATE_EPOCH to build scripts for reproducible builds.
	if sde := os.Getenv("SOURCE_DATE_EPOCH"); sde != "" {
		env = append(env, "SOURCE_DATE_EPOCH="+sde)
	}

	return env
}

// SetEnvironmentVariables sets the environment variables for the PKGBUILD execution context.
// This should be called just before executing build/package functions to ensure
// each package uses its own directories, even when building multiple packages.
//
// NOTE: For parallel builds, prefer BuildEnvironmentSlice() which does not mutate
// the global process environment and is safe to call from multiple goroutines.
//
// It returns an error if setting any environment variable fails.
func (pkgBuild *PKGBUILD) SetEnvironmentVariables() error {
	err := os.Setenv("pkgdir", pkgBuild.PackageDir)
	if err != nil {
		return err
	}

	err = os.Setenv("srcdir", pkgBuild.SourceDir)
	if err != nil {
		return err
	}

	err = os.Setenv("startdir", pkgBuild.StartDir)
	if err != nil {
		return err
	}

	err = os.Setenv("CARCH", pkgBuild.GetTargetArchitecture())
	if err != nil {
		return err
	}

	// Always refresh pkgname/pkgver/pkgrel so that scripts for each package
	// in a multi-package build see their own values, not stale ones left in
	// the environment by the previously built package.
	err = os.Setenv("pkgname", pkgBuild.PkgName)
	if err != nil {
		return err
	}

	err = os.Setenv("pkgver", pkgBuild.PkgVer)
	if err != nil {
		return err
	}

	err = os.Setenv("pkgrel", pkgBuild.PkgRel)
	if err != nil {
		return err
	}

	// Resolve and export SOURCE_DATE_EPOCH for reproducible builds.
	// If already set in the environment (by the user or CI), it is preserved;
	// otherwise it is derived from the PKGBUILD file modification time.
	_, err = files.ResolveSourceDateEpoch(pkgBuild.Home)

	return err
}

// ValidateGeneral checks that mandatory items are correctly provided by the PKGBUILD
// file.
func (pkgBuild *PKGBUILD) ValidateGeneral() error {
	var checkErrors []string

	// Validate license
	if !pkgBuild.checkLicense() {
		checkErrors = append(checkErrors, "license")

		logger.Error(i18n.T("logger.validategeneral.error.invalid_spdx_license_identifier_1"),
			"pkgname", pkgBuild.PkgName)
		logger.Info(i18n.T("logger.validategeneral.info.you_can_find_valid_1"),
			"🌐", "https://spdx.org/licenses/")
	}

	// Check source and hash sums
	if len(pkgBuild.SourceURI) != len(pkgBuild.HashSums) {
		checkErrors = append(checkErrors, "source-hash mismatch")

		logger.Error(i18n.T("logger.validategeneral.error.number_of_sources_and_1"),
			"pkgname", pkgBuild.PkgName)
	}

	// Check for package() function — not required for split packages, which use
	// package_<name>() functions instead (detected by PkgNames being non-empty).
	if pkgBuild.Package == "" && !pkgBuild.IsSplitPackage() {
		checkErrors = append(checkErrors, "package function")

		logger.Error(i18n.T("logger.validategeneral.error.missing_package_function_1"),
			"pkgname", pkgBuild.PkgName)
	}

	// Return error if there are validation errors
	if len(checkErrors) > 0 {
		return errors.New(errors.ErrTypeValidation,
			fmt.Sprintf("pkgbuild validation failed for %q: %s",
				pkgBuild.PkgName, strings.Join(checkErrors, ", "))).
			WithOperation("Validate")
	}

	return nil
}

// ValidateMandatoryItems checks that mandatory items are correctly provided by the PKGBUILD
// file.
func (pkgBuild *PKGBUILD) ValidateMandatoryItems() error {
	var validationErrors []string

	// Check mandatory variables
	mandatoryChecks := map[string]string{
		pkgdescKey: pkgBuild.PkgDesc,
		pkgnameKey: pkgBuild.PkgName,
		pkgrelKey:  pkgBuild.PkgRel,
		pkgverKey:  pkgBuild.PkgVer,
	}

	for variable, value := range mandatoryChecks {
		if value == "" {
			validationErrors = append(validationErrors, variable)
		}
	}

	// Return error if there are validation errors
	if len(validationErrors) > 0 {
		return errors.New(errors.ErrTypeValidation,
			fmt.Sprintf("missing mandatory variables: %s",
				strings.Join(validationErrors, ", "))).
			WithOperation("ValidateMandatoryItems")
	}

	return nil
}

// mapArrays reads an array name and its content and maps them to the PKGBUILD
// struct.
//
// mapArrays maps an array key+value into the PKGBUILD struct.
// priority is forwarded so that arch-specific source/checksum arrays
// (priority > priorityBase) are APPENDED to the base arrays rather than
// replacing them — matching makepkg behaviour where source_aarch64 is
// concatenated onto source, not a replacement.
//
//nolint:gocyclo,cyclop // Central dispatch function for PKGBUILD array field mapping
func (pkgBuild *PKGBUILD) mapArrays(key string, data any, priority int) {
	if pkgBuild.mapChecksumsArrays(key, data, priority) {
		return
	}

	switch key {
	case archDistro:
		arrVal, ok := data.([]string)
		if !ok {
			return
		}

		pkgBuild.Arch = arrVal
	case "copyright":
		arrVal, ok := data.([]string)
		if !ok {
			return
		}

		pkgBuild.Copyright = arrVal
	case "license":
		arrVal, ok := data.([]string)
		if !ok {
			return
		}

		pkgBuild.License = arrVal
	case dependsKey:
		arrVal, ok := data.([]string)
		if !ok {
			return
		}

		pkgBuild.Depends = arrVal
	case "options":
		arrVal, ok := data.([]string)
		if !ok {
			return
		}

		pkgBuild.Options = arrVal
		pkgBuild.processOptions()
	case "optdepends":
		arrVal, ok := data.([]string)
		if !ok {
			return
		}

		pkgBuild.OptDepends = arrVal
	case makedependsKey:
		arrVal, ok := data.([]string)
		if !ok {
			return
		}

		pkgBuild.MakeDepends = arrVal
	case "provides":
		arrVal, ok := data.([]string)
		if !ok {
			return
		}

		pkgBuild.Provides = arrVal
	case "conflicts":
		arrVal, ok := data.([]string)
		if !ok {
			return
		}

		pkgBuild.Conflicts = arrVal
	case "enhances":
		arrVal, ok := data.([]string)
		if !ok {
			return
		}

		pkgBuild.Enhances = arrVal
	case "supplements":
		arrVal, ok := data.([]string)
		if !ok {
			return
		}

		pkgBuild.Supplements = arrVal
	case "replaces":
		arrVal, ok := data.([]string)
		if !ok {
			return
		}

		pkgBuild.Replaces = arrVal
	case sourceKey:
		arrVal, ok := data.([]string)
		if !ok {
			return
		}

		if priority > priorityBase {
			// Arch-specific source_<arch>: accumulate separately; merged by Finalize().
			pkgBuild.archSourceURI = append(pkgBuild.archSourceURI, arrVal...)
		} else {
			pkgBuild.SourceURI = arrVal
		}
	case noextractKey:
		arrVal, ok := data.([]string)
		if !ok {
			return
		}

		pkgBuild.NoExtract = arrVal
	case "backup":
		arrVal, ok := data.([]string)
		if !ok {
			return
		}

		pkgBuild.Backup = arrVal
	case pkgnameKey:
		// Split-package form: pkgname=('foo' 'bar')
		// Store the list; PkgName is set to the first entry so single-package
		// code paths continue to work unchanged.
		arrVal, ok := data.([]string)
		if !ok {
			return
		}

		pkgBuild.PkgNames = arrVal

		if len(arrVal) > 0 {
			pkgBuild.PkgName = arrVal[0]
			_ = os.Setenv(pkgnameKey, arrVal[0])
		}
	default:
		// Store unknown arrays (e.g. _modules, _extra_files) as custom arrays.
		// They will be declared as bash array variables in the build script preamble.
		if arrVal, ok := data.([]string); ok {
			if pkgBuild.CustomArrays == nil {
				pkgBuild.CustomArrays = make(map[string][]string)
			}

			pkgBuild.CustomArrays[key] = arrVal
		}
	}
}

// mapChecksumsArrays handles mapping of checksum arrays and returns true if handled.
// priority is used to decide append vs replace: arch-specific checksum arrays
// (e.g. sha256sums_aarch64, priority > priorityBase) are appended to HashSums
// to match the corresponding arch-specific source entries.
func (pkgBuild *PKGBUILD) mapChecksumsArrays(key string, data any, priority int) bool {
	switch key {
	case sha512sumsKey, sha384sumsKey, sha256sumsKey, sha224sumsKey, b2sumsKey, cksumsKey:
		if arrVal, ok := data.([]string); ok {
			if priority > priorityBase {
				// Arch-specific: accumulate separately; merged by Finalize().
				pkgBuild.archHashSums = append(pkgBuild.archHashSums, arrVal...)
			} else {
				pkgBuild.HashSums = arrVal
			}
		}

		return true
	default:
		return false
	}
}

// mapFunctions reads a function name and its content and maps them to the
// PKGBUILD struct. Any function name not matching a known PKGBUILD lifecycle
// hook is stored as a helper function and will be injected as a preamble into
// build, prepare, and package scripts so that callers such as _package() or
// _install_files() are available at runtime.
//
// data must be of type FuncBody; plain string values (variables) are ignored so
// that scalar variables such as "maintainer" are not confused with functions.
//
//nolint:gocyclo,cyclop // switch statement with many cases
func (pkgBuild *PKGBUILD) mapFunctions(key string, data any) {
	fb, ok := data.(FuncBody)
	if !ok {
		return
	}

	body := string(fb)

	switch key {
	case "build":
		pkgBuild.Build = body
	case "package":
		pkgBuild.Package = body
	case "preinst":
		pkgBuild.PreInst = body
	case "prepare":
		pkgBuild.Prepare = body
	case "postinst":
		pkgBuild.PostInst = body
	case "posttrans":
		pkgBuild.PostTrans = body
	case "prerm":
		pkgBuild.PreRm = body
	case "pretrans":
		pkgBuild.PreTrans = body
	case "postrm":
		pkgBuild.PostRm = body
	case "pre_upgrade":
		pkgBuild.PreUpgrade = body
	case "post_upgrade":
		pkgBuild.PostUpgrade = body
	default:
		// package_<name>() functions are split-package packaging functions — store
		// them separately so they are NOT injected into build/prepare preambles.
		if subName, ok := strings.CutPrefix(key, "package_"); ok {
			if pkgBuild.SplitPackageFuncs == nil {
				pkgBuild.SplitPackageFuncs = make(map[string]string)
			}

			pkgBuild.SplitPackageFuncs[subName] = body

			return
		}

		// Store any other function (e.g. _install_helper, _common_setup)
		// as a helper. The full definition is reconstructed so it can be prepended to
		// build scripts verbatim.
		if pkgBuild.HelperFunctions == nil {
			pkgBuild.HelperFunctions = make(map[string]string)
		}

		pkgBuild.HelperFunctions[key] = fmt.Sprintf("%s() {\n%s\n}", key, body)
	}
}

// mapVariables reads a variable name and its content and maps them to the
// PKGBUILD struct.
//
//nolint:gocyclo,cyclop // Central dispatch function for PKGBUILD field mapping
func (pkgBuild *PKGBUILD) mapVariables(key string, data any) {
	var err error

	switch key {
	case pkgnameKey:
		strVal, ok := data.(string)
		if !ok {
			return
		}

		err = os.Setenv(key, strVal)
		pkgBuild.PkgName = strVal
	case pkgbaseKey:
		strVal, ok := data.(string)
		if !ok {
			return
		}

		err = os.Setenv(key, strVal)
		pkgBuild.PkgBase = strVal
	case "epoch":
		strVal, ok := data.(string)
		if !ok {
			return
		}

		err = os.Setenv(key, strVal)
		pkgBuild.Epoch = strVal
	case "pkgver":
		strVal, ok := data.(string)
		if !ok {
			return
		}

		err = os.Setenv(key, strVal)
		pkgBuild.PkgVer = strVal
	case "pkgrel":
		strVal, ok := data.(string)
		if !ok {
			return
		}

		err = os.Setenv(key, strVal)
		pkgBuild.PkgRel = strVal
	case "pkgdesc":
		strVal, ok := data.(string)
		if !ok {
			return
		}

		pkgBuild.PkgDesc = strVal
	case "maintainer":
		strVal, ok := data.(string)
		if !ok {
			return
		}

		err = os.Setenv(key, strVal)
		pkgBuild.Maintainer = strVal
	case "section":
		strVal, ok := data.(string)
		if !ok {
			return
		}

		pkgBuild.Section = strVal
	case "priority":
		strVal, ok := data.(string)
		if !ok {
			return
		}

		pkgBuild.Priority = strVal
	case "url":
		strVal, ok := data.(string)
		if !ok {
			return
		}

		err = os.Setenv(key, strVal)
		pkgBuild.URL = strVal
	case "origin":
		strVal, ok := data.(string)
		if !ok {
			return
		}

		pkgBuild.Origin = strVal
	case "commit":
		strVal, ok := data.(string)
		if !ok {
			return
		}

		pkgBuild.Commit = strVal
	case "debconf_template":
		strVal, ok := data.(string)
		if !ok {
			return
		}

		pkgBuild.DebTemplate = strVal
	case "debconf_config":
		strVal, ok := data.(string)
		if !ok {
			return
		}

		pkgBuild.DebConfig = strVal
	case "install":
		strVal, ok := data.(string)
		if !ok {
			return
		}

		pkgBuild.Install = strVal
	case "changelog":
		strVal, ok := data.(string)
		if !ok {
			return
		}

		pkgBuild.Changelog = strVal
	case "target_arch":
		strVal, ok := data.(string)
		if !ok {
			return
		}

		// CLI flag takes precedence: only apply PKGBUILD-file value when not already set.
		if pkgBuild.TargetArch == "" {
			pkgBuild.TargetArch = strVal
		}
	case "build_arch":
		strVal, ok := data.(string)
		if !ok {
			return
		}

		pkgBuild.BuildArch = strVal
	case "host_arch":
		strVal, ok := data.(string)
		if !ok {
			return
		}

		pkgBuild.HostArch = strVal
	default:
		// Store unknown scalar variables (e.g. _prefix, _destdir) as custom variables
		// and expose them as environment variables so they are available in build scripts
		// via the sh interpreter's inherited environment.
		if strVal, ok := data.(string); ok {
			if pkgBuild.CustomVariables == nil {
				pkgBuild.CustomVariables = make(map[string]string)
			}

			pkgBuild.CustomVariables[key] = strVal

			err = os.Setenv(key, strVal)
		}
	}

	if err != nil {
		logger.Fatal(i18n.T("errors.pkgbuild.failed_to_set_variable"),
			"variable", key)
	}
}

// parseDirective is a function that takes an input string and returns a key,
// priority, and an error.
func (pkgBuild *PKGBUILD) parseDirective(input string) (key string, priority int, err error) {
	// Check for combined architecture + distribution syntax first (_arch__distro)
	if key, priority, found := pkgBuild.parseCombinedArchDistro(input); found {
		return key, priority, nil
	}

	// Check for architecture-specific syntax (single underscore, no double underscore)
	if key, priority, found := pkgBuild.parseArchitectureOnly(input); found {
		return key, priority, nil
	}

	// Parse distribution-only syntax
	return pkgBuild.parseDistributionOnly(input)
}

// parseCombinedArchDistro handles combined architecture + distribution syntax
func (pkgBuild *PKGBUILD) parseCombinedArchDistro(input string) (
	key string, priority int, found bool,
) {
	if !strings.Contains(input, "_") || !strings.Contains(input, "__") {
		return "", 0, false
	}

	parts := strings.SplitN(input, "__", 2)
	if len(parts) != 2 {
		return "", 0, false
	}

	archPart := parts[0]
	distributionPart := parts[1]

	if !strings.Contains(archPart, "_") {
		return "", 0, false
	}

	archSplit := strings.Split(archPart, "_")
	if len(archSplit) < 2 {
		return "", 0, false
	}

	possibleArch := strings.Join(archSplit[1:], "_")
	key = archSplit[0]

	if !pkgBuild.isValidArchitecture(possibleArch) {
		return "", 0, false
	}

	effectiveArch := pkgBuild.TargetArch
	if effectiveArch == "" {
		effectiveArch = platform.GetArchitecture()
	}

	if possibleArch != effectiveArch {
		return key, prioritySkip, true // Invalid architecture for current system
	}

	// Check distribution part
	distPriority := pkgBuild.getDistributionPriority(distributionPart)
	if distPriority > priorityBase {
		return key, distPriority + priorityArchDistroBoost, true // Add boost for arch+distro combinations
	}

	return key, prioritySkip, true
}

// parseArchitectureOnly handles architecture-specific syntax
func (pkgBuild *PKGBUILD) parseArchitectureOnly(input string) (
	key string, priority int, found bool,
) {
	if !strings.Contains(input, "_") || strings.Contains(input, "__") {
		return "", 0, false
	}

	archSplit := strings.Split(input, "_")
	if len(archSplit) < 2 {
		return "", 0, false
	}

	possibleArch := strings.Join(archSplit[1:], "_")
	key = archSplit[0]

	if !pkgBuild.isValidArchitecture(possibleArch) {
		return "", 0, false
	}

	effectiveArch := pkgBuild.TargetArch
	if effectiveArch == "" {
		effectiveArch = platform.GetArchitecture()
	}

	if possibleArch == effectiveArch {
		return key, priorityArchMatch, true // Higher priority than distribution-specific
	}

	return key, prioritySkip, true // Invalid architecture for current system
}

// parseDistributionOnly handles distribution-only syntax
func (pkgBuild *PKGBUILD) parseDistributionOnly(input string) (
	key string, priority int, err error,
) {
	split := strings.Split(input, "__")
	key = split[0]

	if len(split) == 1 {
		return key, priorityBase, nil
	}

	if len(split) != 2 {
		return key, priorityBase, errors.New(errors.ErrTypeConfiguration,
			fmt.Sprintf(i18n.T("errors.pkgbuild.invalid_directive_use"), input)).
			WithOperation("parseDirective")
	}

	if pkgBuild.FullDistroName == "" {
		return key, priorityBase, nil
	}

	directive := split[1]
	priority = pkgBuild.getDistributionPriority(directive)

	return key, priority, nil
}

// getDistributionPriority returns the priority for a distribution directive
func (pkgBuild *PKGBUILD) getDistributionPriority(directive string) int {
	switch {
	case directive == pkgBuild.FullDistroName:
		return priorityFullDistroMatch
	case constants.DistrosSet.Contains(directive) &&
		directive == pkgBuild.Distro:
		return priorityDistroMatch
	case constants.PackagersSet.Contains(directive) &&
		directive == constants.DistroPackageManager[pkgBuild.Distro]:
		return priorityPackagerMatch
	default:
		return prioritySkip
	}
}

// checkLicense checks if the license of the PKGBUILD is valid.
//
// This function takes no parameters.
//
// It first checks if the license is either "PROPRIETARY" or "CUSTOM". If it is,
// the function returns true, indicating that the license is valid.
//
// If the license is not one of the above, the function uses the spdxexp package
// to validate the license.
//
// Returns a boolean indicating if the license is valid.
func (pkgBuild *PKGBUILD) checkLicense() bool {
	for _, license := range pkgBuild.License {
		if license == proprietaryKey || license == customKey {
			return true
		}
	}

	isValid, _ := spdxexp.ValidateLicenses(pkgBuild.License)

	return isValid
}

// optionDefaults maps each makepkg option name to its default enabled state.
// Options not listed here are ignored. Negated form ("!name") always inverts.
var optionDefaults = map[string]bool{
	"debug":      false,
	"docs":       true,
	"emptydirs":  true,
	"libtool":    true,
	"purge":      false,
	"staticlibs": true,
	"strip":      true,
	"zipman":     false,
}

func (pkgBuild *PKGBUILD) processOptions() {
	// Apply makepkg defaults.
	pkgBuild.DebugEnabled = optionDefaults["debug"]
	pkgBuild.DocsEnabled = optionDefaults["docs"]
	pkgBuild.EmptyDirsEnabled = optionDefaults["emptydirs"]
	pkgBuild.LibtoolEnabled = optionDefaults["libtool"]
	pkgBuild.PurgeEnabled = optionDefaults["purge"]
	pkgBuild.StaticEnabled = optionDefaults["staticlibs"]
	pkgBuild.StripEnabled = optionDefaults["strip"]
	pkgBuild.ZipManEnabled = optionDefaults["zipman"]

	for _, option := range pkgBuild.Options {
		negated := strings.HasPrefix(option, "!")
		name := strings.TrimPrefix(option, "!")

		if _, known := optionDefaults[name]; !known {
			continue
		}

		pkgBuild.applyOption(name, !negated)
	}
}

// applyOption sets the PKGBUILD flag corresponding to the given option name.
func (pkgBuild *PKGBUILD) applyOption(name string, enabled bool) {
	switch name {
	case "debug":
		pkgBuild.DebugEnabled = enabled
	case "docs":
		pkgBuild.DocsEnabled = enabled
	case "emptydirs":
		pkgBuild.EmptyDirsEnabled = enabled
	case "libtool":
		pkgBuild.LibtoolEnabled = enabled
	case "purge":
		pkgBuild.PurgeEnabled = enabled
	case "staticlibs":
		pkgBuild.StaticEnabled = enabled
	case "strip":
		pkgBuild.StripEnabled = enabled
	case "zipman":
		pkgBuild.ZipManEnabled = enabled
	}
}

// isValidArchitecture checks if the provided architecture string is a valid architecture.
func (pkgBuild *PKGBUILD) isValidArchitecture(arch string) bool {
	validArchitectures := []string{
		x86_64Arch, i686Arch, ArchAarch64, armv7hArch, "armv6h", "armv5",
		"ppc64", ppc64leArch, s390xArch, mipsArch, "mipsle", riscv64Arch,
		"pentium4", // Arch Linux 32 support
		ArchAny,    // Architecture-independent packages
	}

	return set.Contains(validArchitectures, arch)
}

// IsCrossCompilation checks if cross-compilation is enabled for this PKGBUILD.
// Returns true if any of the cross-compilation architecture variables are set.
func (pkgBuild *PKGBUILD) IsCrossCompilation() bool {
	return pkgBuild.TargetArch != "" || pkgBuild.BuildArch != "" || pkgBuild.HostArch != ""
}

// GetTargetArchitecture returns the target architecture for cross-compilation.
// If a target architecture is explicitly set in the PKGBUILD, it returns that.
// Otherwise, it returns the computed architecture from the standard architecture processing.
func (pkgBuild *PKGBUILD) GetTargetArchitecture() string {
	if pkgBuild.TargetArch != "" {
		return pkgBuild.TargetArch
	}

	return pkgBuild.ArchComputed
}
