package common

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/M0Rf30/yap/v2/pkg/aptcache"
	"github.com/M0Rf30/yap/v2/pkg/constants"
	"github.com/M0Rf30/yap/v2/pkg/errors"
	"github.com/M0Rf30/yap/v2/pkg/i18n"
	"github.com/M0Rf30/yap/v2/pkg/logger"
	"github.com/M0Rf30/yap/v2/pkg/repo"
)

// aptCacheDownloadClosure resolves the transitive closure of `seeds`,
// downloads every resulting .deb into dir, and returns the resolved
// PackageInfo slice in dependency order plus the list of unresolvable
// names.
//
// Reloads the aptcache singleton first so freshly-added cross-arch repos
// (see pkg/repo/cross.go) are visible to the resolver.
func aptCacheDownloadClosure(
	ctx context.Context, dir string, seeds []string,
) ([]*aptcache.PackageInfo, []string, error) {
	cache := aptcache.Reload()
	return cache.DownloadClosure(ctx, dir, seeds)
}

// crossCompileParams holds pre-computed cross-compilation parameters
// shared between SetupCrossCompilationEnvironment and BuildCrossEnvSlice.
type crossCompileParams struct {
	gccExecutable    string
	gppExecutable    string
	binutilsPrefix   string
	ccPrefix         string
	rustTarget       string
	rustTargetUpper  string
	goArch           string
	hostTriplet      string
	buildTriplet     string
	configureWrapper string
	ccacheAvailable  bool
}

// extractCCPrefix extracts the cross-compiler prefix from a gcc executable name.
// e.g. "aarch64-linux-gnu-gcc" -> "aarch64-linux-gnu"
func extractCCPrefix(gccExecutable string) string {
	if before, _, ok := strings.Cut(gccExecutable, "-gcc"); ok {
		return before
	}

	if before, _, ok := strings.Cut(gccExecutable, "gcc"); ok {
		return strings.TrimSuffix(before, "-")
	}

	return ""
}

// buildConfigureWrapper creates a bash function wrapper for autoconf cross-compilation.
// Returns an empty string if either triplet is empty.
func buildConfigureWrapper(hostTriplet, buildTriplet string) string {
	if hostTriplet == "" || buildTriplet == "" {
		return ""
	}

	return fmt.Sprintf(`
# YAP cross-compilation configure wrapper
configure_cross() {
  if [ -x ./configure ]; then
    ./configure --host=%s --build=%s "$@"
  elif [ -x configure ]; then
    configure --host=%s --build=%s "$@"
  else
    echo "Warning: configure script not found" >&2
    return 1
  fi
}

# Export the function so it's available in build scripts
export -f configure_cross 2>/dev/null || true
`, hostTriplet, buildTriplet, hostTriplet, buildTriplet)
}

// resolveToolchainPackages resolves the toolchain packages for the given target arch and format.
func (bb *BaseBuilder) resolveToolchainPackages(targetArch string) (CrossToolchain, error) {
	toolchain, exists := CrossToolchainMap[targetArch]
	if !exists {
		return CrossToolchain{}, errors.New(errors.ErrTypeBuild, "no cross-compilation toolchain available").
			WithOperation("resolveToolchainPackages").
			WithContext("targetArch", targetArch)
	}

	distro := bb.formatToDistro()

	packages, exists := toolchain[distro]
	if !exists {
		for _, p := range toolchain {
			packages = p
			break
		}

		if packages.GCCPackage == "" {
			return CrossToolchain{}, errors.New(errors.ErrTypeBuild, "no cross-compilation toolchain available").
				WithOperation("resolveToolchainPackages").
				WithContext("targetArch", targetArch).
				WithContext("distro", distro)
		}
	}

	return packages, nil
}

// formatToDistro maps the builder's package format to a representative distribution name
// used for cross-compilation toolchain lookup.
func (bb *BaseBuilder) formatToDistro() string {
	switch bb.Format {
	case constants.FormatDEB:
		return distroDebian
	case constants.FormatRPM:
		return distroFedora
	case constants.FormatAPK:
		return distroAlpine
	case constants.FormatPacman:
		return distroArch
	default:
		return distroDebian
	}
}

// buildCrossParams computes cross-compilation parameters from the toolchain packages.
// This is shared between SetupCrossCompilationEnvironment and BuildCrossEnvSlice.
func (bb *BaseBuilder) buildCrossParams(targetArch string, toolchainPackages *CrossToolchain) crossCompileParams {
	_, ccacheErr := exec.LookPath("ccache")

	gccExecutable := toolchainPackages.GetExecutableName(toolchainPackages.GCCPackage)
	gppExecutable := toolchainPackages.GetExecutableName(toolchainPackages.GPlusPlusPackage)
	binutilsPrefix := toolchainPackages.binutilsPrefix()

	ccPrefix := extractCCPrefix(gccExecutable)

	rustTarget := bb.getRustTargetArchitecture(targetArch)

	rustTargetUpper := ""
	if rustTarget != "" {
		rustTargetUpper = strings.ToUpper(strings.ReplaceAll(rustTarget, "-", "_"))
	}

	goArch := bb.getGoTargetArchitecture(targetArch)
	hostTriplet := bb.getGNUTriplet(targetArch)
	buildTriplet := bb.getGNUTriplet(bb.PKGBUILD.ArchComputed)

	configureWrapper := buildConfigureWrapper(hostTriplet, buildTriplet)

	return crossCompileParams{
		gccExecutable:    gccExecutable,
		gppExecutable:    gppExecutable,
		binutilsPrefix:   binutilsPrefix,
		ccPrefix:         ccPrefix,
		rustTarget:       rustTarget,
		rustTargetUpper:  rustTargetUpper,
		goArch:           goArch,
		hostTriplet:      hostTriplet,
		buildTriplet:     buildTriplet,
		configureWrapper: configureWrapper,
		ccacheAvailable:  ccacheErr == nil,
	}
}

// SkipToolchainValidation controls whether cross-compilation toolchain validation is performed.
// This is set by command-line flags and used by PrepareEnvironment.
var SkipToolchainValidation bool

// formatToRepresentativeDistro maps each package format to a canonical representative
// distribution name used for cross-compilation toolchain lookup.
var formatToRepresentativeDistro = map[string]string{
	constants.FormatDEB:    distroUbuntu,
	constants.FormatRPM:    distroFedora,
	constants.FormatAPK:    distroAlpine,
	constants.FormatPacman: distroArch,
}

// isPerlModule returns true for Perl module packages (suffix "-perl").
// These are host-only build tools despite having Multi-Arch: same in the
// apt index (they ship arch-specific .so files but run on the build host).
func isPerlModule(name string) bool {
	return strings.HasSuffix(name, "-perl")
}

func partitionArchAllDeps(deps []string) (archSpecific, archAll []string) {
	cache := aptcache.Load()

	for _, dep := range deps {
		// Strip version constraint for the lookup: "libssl-dev (>= 1.0)" → "libssl-dev"
		name, _, _ := strings.Cut(dep, " (")
		// Strip any existing arch qualifier
		name, _, _ = strings.Cut(name, ":")

		// Perl modules (*-perl) ship arch-specific .so files so apt marks them
		// Multi-Arch: same, but they are host-side build tools (e.g. used by
		// autoconf/automake) and must never be qualified with the target arch.
		if isPerlModule(name) {
			archAll = append(archAll, dep)
			continue
		}

		info, found := cache.Lookup(name)
		if !found {
			// Package not in apt cache at all (no apt list entry and no dpkg
			// status entry). This is rare: a custom-repo package that is also
			// not installed host-side. We can't know its Multi-Arch policy, so
			// fall through to arch-specific qualification. If a dpkg conflict
			// surfaces here, the user needs to either add their repo to the
			// apt lists or pre-install the host-arch variant before building.
			//
			// Note: a previous version of this branch checked info.Installed,
			// but aptcache.Lookup returns a zero-value PackageInfo when not
			// found, so that flag could never be true here. Lookup-and-found
			// covers the dpkg-installed case via the merged status overlay
			// (see pkg/aptcache.Cache.loadDpkgStatus).
			archSpecific = append(archSpecific, dep)

			continue
		}

		// Architecture: all — no arch-specific variant exists.
		if info.ArchitectureAll() {
			archAll = append(archAll, dep)
			continue
		}

		// Essential packages (e.g. bash) conflict when installed for a foreign
		// arch alongside the host-arch version.
		if info.Essential {
			archAll = append(archAll, dep)
			continue
		}

		// Multi-Arch: foreign / allowed — a single host-arch copy satisfies
		// dependencies from any architecture. These are tools and daemons
		// (cmake, git, systemd, perl, python3, …) that run on the build host.
		// They must NOT be qualified with the target arch.
		//
		// Multi-Arch: same — dev libraries that must be installed separately
		// per architecture. These DO get the :arm64 qualifier.
		//
		// Multi-Arch: absent / no — qualify only if not already installed
		// (installing the same package for two arches without Multi-Arch: same
		// causes dpkg conflicts).
		if info.MultiArchForeign() {
			archAll = append(archAll, dep)
			continue
		}

		if !info.MultiArchSame() && info.Installed {
			archAll = append(archAll, dep)
			continue
		}

		archSpecific = append(archSpecific, dep)
	}

	return archSpecific, archAll
}

// partitionArchAllDepsForExtract is a relaxed variant of partitionArchAllDeps
// used by DownloadAndExtractCrossDeps. Since extraction bypasses dpkg, there
// are no multi-arch conflicts. The only packages left unqualified are:
//   - Architecture: all — no arch-specific variant exists.
//   - Multi-Arch: foreign/allowed — host tools (cmake, git, systemd, perl…).
//   - Essential packages — overwriting host binaries would break the env.
//
// Packages already installed for the host arch are still qualified with the
// target arch because extraction overwrites files without dpkg conflict checks.
func partitionArchAllDepsForExtract(deps []string) (archSpecific, archAll []string) {
	cache := aptcache.Load()

	for _, dep := range deps {
		name, _, _ := strings.Cut(dep, " (")
		name, _, _ = strings.Cut(name, ":")

		// Perl modules: same exception as partitionArchAllDeps.
		if isPerlModule(name) {
			archAll = append(archAll, dep)
			continue
		}

		info, found := cache.Lookup(name)
		if !found {
			// Not in apt cache — assume arch-specific so apt can surface a clear error.
			archSpecific = append(archSpecific, dep)
			continue
		}

		if info.ArchitectureAll() {
			archAll = append(archAll, dep)
			continue
		}

		// Essential packages still conflict even with extraction because
		// overwriting host binaries (e.g. /bin/bash) with target-arch
		// binaries would break the build environment.
		if info.Essential {
			archAll = append(archAll, dep)
			continue
		}

		// Multi-Arch: foreign/allowed — host tools that must not be
		// qualified with the target arch (same reasoning as partitionArchAllDeps).
		if info.MultiArchForeign() {
			archAll = append(archAll, dep)
			continue
		}

		archSpecific = append(archSpecific, dep)
	}

	return archSpecific, archAll
}

// qualifyDepsForTargetArch rewrites a list of package names so they are
// installed for the target (cross) architecture rather than the host arch.
//
// DEB: appends ":arm64" (or the appropriate DEB arch name) — requires the
// target architecture to be registered with dpkg --add-architecture first
// (handled by the Docker images).
//
// RPM: appends ".aarch64" (or the appropriate RPM arch name) — dnf/yum
// accept "pkgname.arch" to pin the architecture of an install.
//
// Packages that already carry an arch qualifier are left unchanged to avoid
// double-suffixing.  Version constraints in DEB format ("pkg (>= 1.0)") are
// handled by suffixing only the name token.
func qualifyDepsForTargetArch(deps []string, format, targetArch string) []string {
	archMapping := constants.GetArchMapping()
	fmtArch := archMapping.TranslateArch(format, targetArch)

	qualified := make([]string, len(deps))

	for i, dep := range deps {
		switch format {
		case constants.FormatDEB:
			// DEB version constraint: "libssl-dev (>= 1.0)" — suffix name only.
			// Skip if already qualified (contains ':').
			if strings.Contains(dep, ":") {
				qualified[i] = dep
				continue
			}

			if idx := strings.Index(dep, " ("); idx != -1 {
				qualified[i] = dep[:idx] + ":" + fmtArch + dep[idx:]
			} else {
				qualified[i] = dep + ":" + fmtArch
			}

		case constants.FormatRPM:
			// RPM: "pkgname.arch" — skip if already has a dot-arch suffix
			// (heuristic: last token after final '.' is a known arch string).
			if idx := strings.LastIndex(dep, "."); idx != -1 {
				suffix := dep[idx+1:]
				if suffix == constants.ArchX86_64 || suffix == constants.ArchAarch64 ||
					suffix == constants.ArchI686 || suffix == constants.ArchArmv7hl ||
					suffix == constants.ArchNoarch || suffix == constants.ArchPpc64le ||
					suffix == constants.ArchS390x {
					qualified[i] = dep
					continue
				}
			}

			qualified[i] = dep + "." + fmtArch

		default:
			qualified[i] = dep
		}
	}

	return qualified
}

// DownloadAndExtractCrossDeps downloads runtime dependencies and extracts them
// directly to the root filesystem without registering them in the dpkg database.
// This avoids the circular dependency problem where arch-all meta-packages
// (e.g. carbonio-core) depend on arch-specific packages (e.g. carbonio-openldap)
// that conflict with the target-arch variants needed for cross-compilation.
//
// The function partitions deps the same way as installCrossDeps: arch-all
// packages are downloaded unqualified, arch-specific ones are qualified with
// the target architecture (e.g. :arm64). All packages are extracted to "/"
// without dpkg, so there is no conflict checking.
//
// **Transitive resolution**: the declared PKGBUILD dependencies are walked
// through aptcache.DownloadClosure so transitive runtime libraries that
// the declared deps themselves need are pulled in too. Without this, a
// PKGBUILD listing only carbonio-ffmpeg fails to cross-link because
// libavcodec.so has DT_NEEDED entries for libvpx.so / libx264.so which
// come from sibling packages (carbonio-libvpx, carbonio-x264) the
// PKGBUILD did not declare.
func (bb *BaseBuilder) DownloadAndExtractCrossDeps(
	ctx context.Context,
	deps []string,
	targetArch string,
) error {
	if bb.Format != constants.FormatDEB {
		// Non-DEB formats: fall back to normal install (no cross-arch conflict).
		// RPM/APK/Pacman don't have a multiarch install story like dpkg, so we
		// don't do the download+extract dance — we just let the native package
		// manager install the deps as if it were a host build.
		logger.Info("cross-build deps: using native install (no closure extract)",
			"format", bb.Format,
			"target_arch", targetArch,
			"deps", len(deps))

		// RPM: use dnfinstall
		if bb.Format == constants.FormatRPM {
			return bb.installRPMDeps(ctx, deps)
		}

		installArgs := constants.GetInstallArgs(bb.Format)

		return bb.PKGBUILD.GetDepends(ctx, getPackageManager(bb.Format), installArgs, deps)
	}

	// Use the extract-safe partitioning: since we download+extract (not dpkg -i),
	// there are no dpkg conflicts, so packages already installed for the host arch
	// must still be downloaded as target-arch.
	archSpecific, archAll := partitionArchAllDepsForExtract(deps)
	qualified := qualifyDepsForTargetArch(archSpecific, bb.Format, targetArch)

	seeds := make([]string, 0, len(archAll)+len(qualified))
	seeds = append(seeds, archAll...)
	seeds = append(seeds, qualified...)

	if len(seeds) == 0 {
		logger.Info("cross-build deps: no runtime deps to fetch",
			"target_arch", targetArch)

		return nil
	}

	logger.Info("fetching cross-build runtime deps",
		"target_arch", targetArch,
		"arch_specific_count", len(qualified),
		"arch_all_count", len(archAll),
		"arch_specific", strings.Join(qualified, ", "),
		"arch_all", strings.Join(archAll, ", "))

	// Create a temporary directory for downloaded .deb files.
	tmpDir, err := os.MkdirTemp("", "yap-cross-deps-*")
	if err != nil {
		return errors.Wrap(err, errors.ErrTypeFileSystem, "create temp dir for cross deps").
			WithOperation("DownloadAndExtractCrossDeps")
	}

	defer func() { _ = os.RemoveAll(tmpDir) }()

	// Resolve transitive closure, then download every .deb in the closure.
	// aptcache skips packages already marked Installed (i.e. present in the
	// build container's dpkg status) so we don't re-extract libc6, glibc,
	// etc. Their dep edges are still walked so a transitive-only library
	// reachable only through an installed package is still pulled in.
	resolved, unresolved, err := aptCacheDownloadClosure(
		ctx, tmpDir, seeds)
	if err != nil {
		return errors.Wrap(err, errors.ErrTypeBuild, "download cross deps closure").
			WithOperation("DownloadAndExtractCrossDeps")
	}

	if len(unresolved) > 0 {
		// Not fatal: virtual / file-based / arch-only deps frequently can't be
		// resolved from a generic apt index but the link will still succeed
		// because they live elsewhere on the system. Surface so a CI run can
		// grep for them.
		logger.Warn("unresolvable transitive cross-deps (continuing)",
			"packages", strings.Join(unresolved, ", "))
	}

	seedSet := make(map[string]bool, len(seeds))
	for _, s := range seeds {
		// Use the same name-normalisation rules as ResolveDeps so the
		// "direct vs transitive" log classification matches.
		name, _, _ := strings.Cut(s, ":")
		name, _, _ = strings.Cut(name, " (")
		seedSet[strings.TrimSpace(name)] = true
	}

	logger.Info("resolved cross-deps closure",
		"declared", len(seeds),
		"closure", len(resolved),
		"transitive", len(resolved)-countDirect(resolved, seedSet))

	for _, info := range resolved {
		if info == nil || info.Filename == "" {
			continue
		}

		debPath := filepath.Join(tmpDir, filepath.Base(info.Filename))
		origin := "direct"

		if !seedSet[info.Name] {
			origin = "transitive"
		}

		logger.Debug("extracting cross-build dep",
			"package", info.Name,
			"arch", info.Architecture,
			"origin", origin)

		if err := ExtractDEB(debPath, "/"); err != nil {
			return errors.Wrap(err, errors.ErrTypeBuild, "extract cross dep").
				WithOperation("DownloadAndExtractCrossDeps").
				WithContext("package", info.Name)
		}
	}

	return nil
}

// countDirect returns the number of resolved entries whose name appears in
// the seedSet — i.e. were directly declared by the PKGBUILD rather than
// pulled in transitively. Used purely for logger summary.
func countDirect(resolved []*aptcache.PackageInfo, seedSet map[string]bool) int {
	n := 0

	for _, p := range resolved {
		if p != nil && seedSet[p.Name] {
			n++
		}
	}

	return n
}

// installCrossDeps installs DEB cross-compilation dependencies in two passes.
// Architecture:all packages are installed first so they are present when
// arch-specific (target-arch) packages are installed — this satisfies
// transitive dependencies that arch-specific packages may have on arch-all
// packages (e.g. carbonio-openldap:arm64 → carbonio-core which is arch:all).
func (bb *BaseBuilder) installCrossDeps(
	ctx context.Context,
	makeDepends,
	installArgs []string,
	targetArch string,
) error {
	archSpecific, archAll := partitionArchAllDeps(makeDepends)
	qualified := qualifyDepsForTargetArch(archSpecific, bb.Format, targetArch)

	logger.Info("Qualifying makedepends for target architecture",
		"target_arch", targetArch,
		"format", bb.Format,
		"arch_specific_count", len(qualified),
		"arch_all_count", len(archAll),
		"arch_specific", strings.Join(qualified, ", "),
		"arch_all", strings.Join(archAll, ", "))

	// Install arch-all (host) packages first so they are available to
	// satisfy transitive dependencies of the target-arch packages.
	if err := bb.installArchAllDeps(ctx, archAll, installArgs); err != nil {
		return err
	}

	// RPM: use dnfinstall
	if bb.Format == constants.FormatRPM {
		return bb.installRPMDeps(ctx, qualified)
	}

	return bb.PKGBUILD.GetDepends(ctx, getPackageManager(bb.Format), installArgs, qualified)
}

// installArchAllDeps installs the host arch-all dependency subset
// (extracted from installCrossDeps to lower its nesting complexity).
func (bb *BaseBuilder) installArchAllDeps(ctx context.Context, archAll, installArgs []string) error {
	if len(archAll) == 0 {
		return nil
	}

	if bb.Format == constants.FormatRPM {
		return bb.installRPMDeps(ctx, archAll)
	}

	return bb.PKGBUILD.GetDepends(ctx, getPackageManager(bb.Format), installArgs, archAll)
}

// ensureCrossArchRepo registers the foreign architecture (dpkg
// --add-architecture) and adds the matching ports apt source so the package
// manager can resolve target-arch libraries and the cross-compiler. The work
// is restricted to DEB-based distros: RPM/APK/Pacman targets either ship the
// toolchain in the base repos or rely on a bundled sysroot.
func (bb *BaseBuilder) ensureCrossArchRepo(targetArch string) error {
	if bb.Format != constants.FormatDEB {
		return nil
	}

	return repo.SetupCrossAPT(repo.CrossAptOptions{
		Distro:     bb.PKGBUILD.Distro,
		Codename:   bb.PKGBUILD.Codename,
		TargetArch: targetArch,
	})
}

// getCrossCompilerDependencies returns cross-compiler dependencies for target architecture.
// This function uses the centralized CrossToolchainMap to get toolchain packages for the
// specified target architecture based on the builder's package format.
func (bb *BaseBuilder) getCrossCompilerDependencies(targetArch string) []string {
	// Get the distribution key for this format
	distro, exists := formatToRepresentativeDistro[bb.Format]
	if !exists {
		return []string{}
	}

	// Get the toolchain for this architecture and distribution
	toolchain, err := GetCrossToolchain(targetArch, distro)
	if err != nil {
		// Architecture not supported for this distribution
		return []string{}
	}

	// Return all packages needed for this toolchain
	return (&toolchain).GetAllPackages()
}

// handleCrossCompilation handles cross-compilation setup including validation
// and dependency collection. This helper reduces nesting complexity in
// prepareEnvironmentWithValidation.
func (bb *BaseBuilder) handleCrossCompilation(
	targetArch string,
	skipValidation bool,
	deps *[]string,
) error {
	logger.Info(i18n.T("logger.cross_compilation.detected_target_architecture"),
		"target_arch", targetArch,
		"build_arch", bb.PKGBUILD.ArchComputed)

	// Validate toolchain availability before attempting installation
	if !skipValidation {
		if err := bb.validateCrossToolchain(targetArch); err != nil {
			return err
		}
	} else {
		logger.Info("Skipping toolchain validation", "target_arch", targetArch)
	}

	// Add cross-compilation dependencies
	crossDeps := bb.getCrossCompilerDependencies(targetArch)
	if len(crossDeps) > 0 {
		logger.Info(
			i18n.T(
				"logger.cross_compilation.installing_cross_compiler_packages"),
			"target_arch", targetArch,
			"packages", strings.Join(crossDeps, ", "))
	}

	*deps = append(*deps, crossDeps...)

	return nil
}

// validateCrossToolchain validates that the cross-compilation toolchain is available.
func (bb *BaseBuilder) validateCrossToolchain(targetArch string) error {
	logger.Debug("Validating cross-compilation toolchain availability",
		"target_arch", targetArch,
		"format", bb.Format)

	if err := ValidateToolchain(targetArch, bb.Format); err != nil {
		// Return detailed validation error with installation instructions
		return err
	}

	logger.Debug("Cross-compilation toolchain validation passed",
		"target_arch", targetArch,
		"format", bb.Format)

	return nil
}

// crossCCEnv returns C/C++ and binutils cross-compilation environment variables.
func (bb *BaseBuilder) crossCCEnv(params *crossCompileParams) []string {
	var env []string

	env = append(env,
		"CC="+params.gccExecutable,
		"CXX="+params.gppExecutable,
	)

	if params.ccacheAvailable {
		// ccache wraps the cross-compiler transparently via the
		// /usr/lib/ccache/<cross-compiler> symlinks that are ahead of the
		// real compiler on PATH. No extra env var is needed.
		logger.Info("ccache active for cross-compilation",
			"cc", params.gccExecutable,
			"via", "/usr/lib/ccache/"+params.gccExecutable)
	}

	env = append(env,
		"AR="+params.binutilsPrefix+"-ar",
		"STRIP="+params.binutilsPrefix+"-strip",
		"RANLIB="+params.binutilsPrefix+"-ranlib",
		"OBJDUMP="+params.binutilsPrefix+"-objdump",
		"OBJCOPY="+params.binutilsPrefix+"-objcopy",
		"LD="+params.binutilsPrefix+"-ld",
		"NM="+params.binutilsPrefix+"-nm",
	)

	// Generate a standard CMake cross-compilation toolchain file and point
	// CMAKE_TOOLCHAIN_FILE at it.
	// Note: targetArch is not used by writeCMakeToolchainFile, but we pass
	// empty string for consistency with the original implementation.
	cmakeToolchain, err := writeCMakeToolchainFile(
		"", params.gccExecutable, params.gppExecutable, params.ccPrefix)
	if err != nil {
		logger.Warn("failed to write CMake toolchain file", "error", err)
	} else {
		env = append(env, "CMAKE_TOOLCHAIN_FILE="+cmakeToolchain)
	}

	return env
}

// crossRustEnv returns Rust cross-compilation environment variables.
func (bb *BaseBuilder) crossRustEnv(params *crossCompileParams,
	targetArch string) []string {
	if params.rustTarget == "" {
		return nil
	}

	env := []string{
		"CARGO_BUILD_TARGET=" + params.rustTarget,
		"RUSTC_TARGET=" + params.rustTarget,
		// Cargo's linker must be a C compiler (gcc), not the raw linker (ld).
		"CARGO_TARGET_" + params.rustTargetUpper + "_LINKER=" +
			params.gccExecutable,
		// Rust build script CC/CXX: use bare cross-compiler; ccache wraps
		// via /usr/lib/ccache/<cross-compiler> symlinks on PATH.
		"TARGET_" + params.rustTargetUpper + "_CC=" + params.gccExecutable,
		"TARGET_" + params.rustTargetUpper + "_CXX=" + params.gppExecutable,
		// Prevent the host's -m64 (or other host-arch flags) from leaking
		// into C code compiled by Rust's cc crate for the target.
		"CFLAGS_" + params.rustTargetUpper + "=-O2 -fPIC",
	}

	logger.Info(i18n.T("logger.cross_compilation.rust_cross_compilation_configured"),
		"rust_target", params.rustTarget,
		"target_arch", targetArch)

	return env
}

// crossGoEnv returns Go cross-compilation environment variables.
func (bb *BaseBuilder) crossGoEnv(params *crossCompileParams,
	targetArch string) []string {
	if params.goArch == "" {
		return nil
	}

	goOS := linuxOS // Default to Linux for cross-compilation
	env := []string{
		"GOOS=" + goOS,
		"GOARCH=" + params.goArch,
		// CGO: bare cross-compiler; ccache wraps via CCACHE_PREFIX.
		"CGO_ENABLED=1",
		"CC_FOR_TARGET=" + params.gccExecutable,
		"CXX_FOR_TARGET=" + params.gppExecutable,
	}

	logger.Info(i18n.T("logger.cross_compilation.go_cross_compilation_configured"),
		"goos", goOS,
		"goarch", params.goArch,
		"target_arch", targetArch)

	return env
}

// crossAutotoolsEnv returns autotools and pkg-config cross-compilation
// environment variables.
func (bb *BaseBuilder) crossAutotoolsEnv(params *crossCompileParams,
	targetArch string) []string {
	var env []string

	// Autotools: CC_FOR_BUILD/CXX_FOR_BUILD must produce host-arch executables
	// Set common cross-compilation variables.
	env = append(env,
		"CC_FOR_BUILD=gcc",
		"CXX_FOR_BUILD=g++",
		"CFLAGS_FOR_BUILD=",
		"CXXFLAGS_FOR_BUILD=",
		"CROSS_COMPILE="+params.ccPrefix+"-",
		"CROSS_COMPILE_HOST="+params.ccPrefix,
	)

	// Configure pkg-config for cross-compilation: prepend toolchain paths to
	// any existing PKG_CONFIG_PATH.
	crossPkgConfigPaths := []string{
		"/usr/lib/" + params.ccPrefix + "/pkgconfig",
		"/usr/local/lib/" + params.ccPrefix + "/pkgconfig",
	}

	existingPkgConfig := os.Getenv("PKG_CONFIG_PATH")
	if existingPkgConfig != "" {
		crossPkgConfigPaths = append(crossPkgConfigPaths, existingPkgConfig)
	}

	env = append(env,
		"PKG_CONFIG_PATH="+strings.Join(crossPkgConfigPaths, ":"),
		"PKG_CONFIG_LIBDIR=/usr/lib/"+params.ccPrefix+"/pkgconfig",
	)

	// Set up autoconf cross-compilation configuration
	if params.hostTriplet != "" && params.buildTriplet != "" {
		// Pre-populate autoconf type-size cache variables.
		if sizeVars, ok := autoconfSizeVars[targetArch]; ok {
			for k, v := range sizeVars {
				env = append(env, k+"="+v)
			}
		}

		// Set the wrapper in the environment
		env = append(env, "YAP_CONFIGURE_WRAPPER="+params.configureWrapper)

		logger.Info(i18n.T(
			"logger.cross_compilation.autoconf_cross_compilation_configured"),
			"host_triplet", params.hostTriplet,
			"build_triplet", params.buildTriplet)
	}

	return env
}

// BuildCrossEnvSlice returns the cross-compilation environment variables as a
// "KEY=VALUE" slice for safe concurrent use. Unlike SetupCrossCompilationEnvironment,
// it does NOT mutate the process environment (no os.Setenv calls).
// Returns nil if no cross-compilation is needed (targetArch == "" or == build arch).
func (bb *BaseBuilder) BuildCrossEnvSlice(targetArch string) ([]string, error) {
	if targetArch == "" || targetArch == bb.PKGBUILD.ArchComputed {
		// No cross-compilation needed
		return nil, nil
	}

	// Resolve toolchain packages for the target architecture
	toolchainPackages, err := bb.resolveToolchainPackages(targetArch)
	if err != nil {
		return nil, err
	}

	// Compute cross-compilation parameters
	params := bb.buildCrossParams(targetArch, &toolchainPackages)

	// Build environment slice by calling per-toolchain helpers
	var envSlice []string

	envSlice = append(envSlice, bb.crossCCEnv(&params)...)
	envSlice = append(envSlice, bb.crossRustEnv(&params, targetArch)...)
	envSlice = append(envSlice, bb.crossGoEnv(&params, targetArch)...)
	envSlice = append(envSlice, bb.crossAutotoolsEnv(&params, targetArch)...)

	logger.Info(i18n.T("logger.cross_compilation.cross_compilation_environment_configured"),
		"target_arch", targetArch,
		"cc", params.gccExecutable,
		"cxx", params.gppExecutable)

	return envSlice, nil
}

// archTargets holds the Rust target triple, Go GOARCH value, and GNU
// system triplet for a single YAP architecture. Grouping them in a single
// table eliminates the "added Rust support but forgot Go/GNU" failure mode
// that previously haunted three parallel maps.
type archTargets struct {
	rustTarget string
	goArch     string
	gnuTriplet string
}

const (
	rustUnknownLinux      = "-unknown-linux-gnu"
	rustArmv7HfTarget     = "armv7-unknown-linux-gnueabihf"
	rustArmHfTarget       = "arm-unknown-linux-gnueabihf"
	rustRiscv64GcTarget   = "riscv64gc" + "-unknown-linux-gnu"
	rustPowerpc64leTarget = "powerpc64le" + "-unknown-linux-gnu"

	// autoconf cache variable names for type sizes.
	acCvSizeofChar     = "ac_cv_sizeof_char"
	acCvSizeofShort    = "ac_cv_sizeof_short"
	acCvSizeofInt      = "ac_cv_sizeof_int"
	acCvSizeofLong     = "ac_cv_sizeof_long"
	acCvSizeofLongLong = "ac_cv_sizeof_long_long"
	acCvSizeofVoidP    = "ac_cv_sizeof_void_p"
	acCvSizeofSizeT    = "ac_cv_sizeof_size_t"
	acCvSizeofOffT     = "ac_cv_sizeof_off_t"
	acCvSizeofWcharT   = "ac_cv_sizeof_wchar_t"
	acCvCBigendian     = "ac_cv_c_bigendian"
)

// archTargetTable is the single source of truth for architecture metadata
// consumed by cross-compilation environment setup. Add a new arch here once;
// Rust/Go/autoconf all benefit simultaneously.
var archTargetTable = map[string]archTargets{
	constants.ArchAarch64: {
		rustTarget: "aarch64" + rustUnknownLinux,
		goArch:     constants.ArchArm64,
		gnuTriplet: constants.TripletAarch64Linux,
	},
	constants.ArchArmv7: {
		rustTarget: rustArmv7HfTarget,
		goArch:     constants.ArchArm,
		gnuTriplet: constants.TripletArmLinuxHf,
	},
	constants.ArchArmv6: {
		rustTarget: rustArmHfTarget,
		goArch:     constants.ArchArm,
		gnuTriplet: constants.TripletArmLinuxHf,
	},
	constants.ArchI686: {
		rustTarget: constants.ArchI686 + rustUnknownLinux,
		goArch:     "386",
		gnuTriplet: constants.TripletI686Linux,
	},
	constants.ArchX86_64: {
		rustTarget: constants.ArchX86_64 + rustUnknownLinux,
		goArch:     constants.ArchAmd64,
		gnuTriplet: constants.TripletX8664Linux,
	},
	constants.ArchPpc64le: {
		rustTarget: rustPowerpc64leTarget,
		goArch:     constants.ArchPpc64le,
		gnuTriplet: constants.TripletPpc64leLinux,
	},
	constants.ArchS390x: {
		rustTarget: constants.ArchS390x + rustUnknownLinux,
		goArch:     constants.ArchS390x,
		gnuTriplet: constants.TripletS390xLinux,
	},
	constants.ArchRiscv64: {
		rustTarget: rustRiscv64GcTarget,
		goArch:     constants.ArchRiscv64,
		gnuTriplet: constants.TripletRiscv64Linux,
	},
}

// autoconfSizeVars holds standard autoconf cache variables for type sizes
// that cannot be probed at configure time during cross-compilation (test
// binaries cannot execute on the build host). Values are LP64 for 64-bit
// targets and ILP32 for 32-bit targets.
var autoconfSizeVars = map[string]map[string]string{
	constants.ArchAarch64: {
		acCvSizeofChar:     "1",
		acCvSizeofShort:    "2",
		acCvSizeofInt:      "4",
		acCvSizeofLong:     "8",
		acCvSizeofLongLong: "8",
		acCvSizeofVoidP:    "8",
		acCvSizeofSizeT:    "8",
		acCvSizeofOffT:     "8",
		acCvSizeofWcharT:   "4",
		acCvCBigendian:     "no",
	},
	constants.ArchArmv7: {
		acCvSizeofChar:     "1",
		acCvSizeofShort:    "2",
		acCvSizeofInt:      "4",
		acCvSizeofLong:     "4",
		acCvSizeofLongLong: "8",
		acCvSizeofVoidP:    "4",
		acCvSizeofSizeT:    "4",
		acCvSizeofOffT:     "8",
		acCvSizeofWcharT:   "4",
		acCvCBigendian:     "no",
	},
	constants.ArchArmv6: {
		acCvSizeofChar:     "1",
		acCvSizeofShort:    "2",
		acCvSizeofInt:      "4",
		acCvSizeofLong:     "4",
		acCvSizeofLongLong: "8",
		acCvSizeofVoidP:    "4",
		acCvSizeofSizeT:    "4",
		acCvSizeofOffT:     "8",
		acCvSizeofWcharT:   "4",
		acCvCBigendian:     "no",
	},
	constants.ArchI686: {
		acCvSizeofChar:     "1",
		acCvSizeofShort:    "2",
		acCvSizeofInt:      "4",
		acCvSizeofLong:     "4",
		acCvSizeofLongLong: "8",
		acCvSizeofVoidP:    "4",
		acCvSizeofSizeT:    "4",
		acCvSizeofOffT:     "8",
		acCvSizeofWcharT:   "4",
		acCvCBigendian:     "no",
	},
	constants.ArchX86_64: {
		acCvSizeofChar:     "1",
		acCvSizeofShort:    "2",
		acCvSizeofInt:      "4",
		acCvSizeofLong:     "8",
		acCvSizeofLongLong: "8",
		acCvSizeofVoidP:    "8",
		acCvSizeofSizeT:    "8",
		acCvSizeofOffT:     "8",
		acCvSizeofWcharT:   "4",
		acCvCBigendian:     "no",
	},
	constants.ArchPpc64le: {
		acCvSizeofChar:     "1",
		acCvSizeofShort:    "2",
		acCvSizeofInt:      "4",
		acCvSizeofLong:     "8",
		acCvSizeofLongLong: "8",
		acCvSizeofVoidP:    "8",
		acCvSizeofSizeT:    "8",
		acCvSizeofOffT:     "8",
		acCvSizeofWcharT:   "4",
		acCvCBigendian:     "no",
	},
	constants.ArchS390x: {
		acCvSizeofChar:     "1",
		acCvSizeofShort:    "2",
		acCvSizeofInt:      "4",
		acCvSizeofLong:     "8",
		acCvSizeofLongLong: "8",
		acCvSizeofVoidP:    "8",
		acCvSizeofSizeT:    "8",
		acCvSizeofOffT:     "8",
		acCvSizeofWcharT:   "4",
		acCvCBigendian:     "yes",
	},
	constants.ArchRiscv64: {
		acCvSizeofChar:     "1",
		acCvSizeofShort:    "2",
		acCvSizeofInt:      "4",
		acCvSizeofLong:     "8",
		acCvSizeofLongLong: "8",
		acCvSizeofVoidP:    "8",
		acCvSizeofSizeT:    "8",
		acCvSizeofOffT:     "8",
		acCvSizeofWcharT:   "4",
		acCvCBigendian:     "no",
	},
}

// ValidateTargetArch returns an error if arch is not a recognised cross-compilation
// target. Returns nil when arch is empty (native build) or known.
func ValidateTargetArch(arch string) error {
	if arch == "" {
		return nil
	}

	if _, ok := archTargetTable[arch]; ok {
		return nil
	}

	known := make([]string, 0, len(archTargetTable))
	for k := range archTargetTable {
		known = append(known, k)
	}

	sort.Strings(known)

	knownStr := strings.Join(known, ", ")

	return errors.New(errors.ErrTypeValidation, fmt.Sprintf("unsupported target architecture %q — known: %s", arch, knownStr)). //nolint:lll
																	WithOperation("ValidateTargetArch")
}

// writeCMakeToolchainFile writes a standard CMake cross-compilation toolchain
// file to a temp path and returns the path. The file is written once per
// target arch; subsequent calls for the same arch return the existing path.
// The file is cleaned up when the process exits (os.CreateTemp uses the OS
// temp dir which is cleaned on reboot, but we also register an atexit via
// a finalizer-free approach: the caller sets CMAKE_TOOLCHAIN_FILE and the
// file persists for the process lifetime).
func writeCMakeToolchainFile(targetArch, gccExecutable, gppExecutable, ccPrefix string) (string, error) {
	path := filepath.Join(os.TempDir(), "yap-cross-"+targetArch+".cmake")

	// Return existing file if already written (idempotent).
	if _, err := os.Stat(path); err == nil {
		return path, nil
	}

	sysroot := "/usr/" + ccPrefix

	// On Debian/Ubuntu multiarch layouts, cross-arch packages install their
	// libraries under /usr/lib/<triplet> and headers under /usr/include/<triplet>
	// (and /usr/include itself), which are NOT under /usr/<triplet>. Add these
	// paths so cmake find_package() / find_library() can locate them.
	multarchLib := "/usr/lib/" + ccPrefix

	// YAP_CMAKE_EXTRA_ROOT_PATH lets a PKGBUILD extend CMAKE_FIND_ROOT_PATH with
	// project-specific install prefixes (e.g. /opt/<vendor>/common where sibling
	// "install: true" packages from the same yap.json land). The env var is read
	// by cmake at configure time via $ENV{...}, so PKGBUILDs can export it
	// without yap regenerating the toolchain file.
	content := fmt.Sprintf(`# Auto-generated by yap for cross-compilation to %s
# Do not edit — regenerated on each build.
set(CMAKE_SYSTEM_NAME Linux)
set(CMAKE_SYSTEM_PROCESSOR %s)

set(CMAKE_C_COMPILER   %s)
set(CMAKE_CXX_COMPILER %s)

set(CMAKE_FIND_ROOT_PATH %s %s /usr)
if(DEFINED ENV{YAP_CMAKE_EXTRA_ROOT_PATH})
  string(REPLACE ":" ";" _yap_extra_paths "$ENV{YAP_CMAKE_EXTRA_ROOT_PATH}")
  list(PREPEND CMAKE_FIND_ROOT_PATH ${_yap_extra_paths})
endif()
set(CMAKE_FIND_ROOT_PATH_MODE_PROGRAM NEVER)
set(CMAKE_FIND_ROOT_PATH_MODE_LIBRARY ONLY)
set(CMAKE_FIND_ROOT_PATH_MODE_INCLUDE ONLY)
`, targetArch, targetArch, gccExecutable, gppExecutable, sysroot, multarchLib)

	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		return "", errors.Wrap(err, errors.ErrTypeFileSystem, "writing CMake toolchain file").
			WithOperation("writeCMakeToolchainFile")
	}

	return path, nil
}

// getRustTargetArchitecture maps YAP architecture names to Rust target triples.
func (bb *BaseBuilder) getRustTargetArchitecture(arch string) string {
	return archTargetTable[arch].rustTarget
}

// getGoTargetArchitecture maps YAP architecture names to Go GOARCH values.
func (bb *BaseBuilder) getGoTargetArchitecture(arch string) string {
	return archTargetTable[arch].goArch
}

// getGNUTriplet maps YAP architecture names to GNU system triplets for autoconf.
// These triplets follow the format: cpu-vendor-os, e.g., aarch64-linux-gnu.
// This is used for autoconf's --host and --build flags during cross-compilation.
func (bb *BaseBuilder) getGNUTriplet(arch string) string {
	return archTargetTable[arch].gnuTriplet
}
