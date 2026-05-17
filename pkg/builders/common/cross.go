package common

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/M0Rf30/yap/v2/pkg/aptcache"
	"github.com/M0Rf30/yap/v2/pkg/constants"
	"github.com/M0Rf30/yap/v2/pkg/errors"
	"github.com/M0Rf30/yap/v2/pkg/i18n"
	"github.com/M0Rf30/yap/v2/pkg/logger"
	"github.com/M0Rf30/yap/v2/pkg/repo"
)

// aptGetDownload runs "apt-get download" for the given packages into dir.
func aptGetDownload(dir string, pkgs []string) error {
	args := append([]string{"download", "--allow-unauthenticated", "-o", "Dir::Cache::Archives=" + dir}, pkgs...)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	cmd := exec.CommandContext(ctx, "apt-get", args...) // #nosec G204
	cmd.Dir = dir

	out, err := cmd.CombinedOutput()
	if err != nil {
		if len(out) > 0 {
			logger.Warn("apt-get download failed", "output", strings.TrimSpace(string(out)))
		}

		return err
	}

	return nil
}

// Note: fmt is still used for fmt.Sprintf in SetupCrossCompilationEnvironment

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

// isHostOnlyPackage returns true for packages that must run on the build host
// and should never be qualified with the target architecture during cross-builds.
// Installing these as :arm64 would pull in conflicting transitive dependencies
// (e.g. perl:arm64 alongside host perl, or make:arm64 alongside host make).
func isHostOnlyPackage(name string) bool {
	// Perl modules and interpreters
	if strings.HasSuffix(name, "-perl") || name == "perl" || name == "perl-base" ||
		name == "perl-modules" {
		return true
	}

	// Common build tools that are always host-arch
	switch name {
	case "make", "re2c", "bison", "byacc", "flex", "gawk",
		"autoconf", "automake", "libtool", "cmake", "meson",
		"pkg-config", "git", "patch", "m4":
		return true
	}

	return false
}

func partitionArchAllDeps(deps []string) (archSpecific, archAll []string) {
	cache := aptcache.Load()

	for _, dep := range deps {
		// Strip version constraint for the lookup: "libssl-dev (>= 1.0)" → "libssl-dev"
		name, _, _ := strings.Cut(dep, " (")
		// Strip any existing arch qualifier
		name, _, _ = strings.Cut(name, ":")

		// Perl modules (*-perl), interpreters (perl, perl-base), and common
		// build tools run on the build host, not the target. Qualifying them
		// with the target arch pulls in conflicting transitive dependencies
		// (e.g. perl:arm64 conflicts with host perl). Keep them unqualified.
		if isHostOnlyPackage(name) {
			archAll = append(archAll, dep)
			continue
		}

		info, found := cache.Lookup(name)
		if !found {
			// Package not in apt cache (e.g. custom repo packages).
			// If it is already installed for the host arch, qualifying it with the
			// target arch would cause a conflict (same package, two architectures,
			// Multi-Arch not set). Treat it as arch-all to avoid the conflict.
			if info.Installed {
				archAll = append(archAll, dep)
			} else {
				archSpecific = append(archSpecific, dep)
			}

			continue
		}

		// Architecture: all — no arch-specific variant.
		if info.ArchitectureAll() {
			archAll = append(archAll, dep)
			continue
		}

		// Essential packages (e.g. bash) conflict when installed for a foreign arch
		// alongside the host-arch version.
		if info.Essential {
			archAll = append(archAll, dep)
			continue
		}

		// Multi-Arch: no (or absent) + already installed → would conflict.
		if !info.MultiArchForeign() && info.Installed {
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
//   - Host-only tools (perl, make, etc.) — must run on the build host.
//
// Packages already installed for the host arch are still qualified with the
// target arch because extraction overwrites files without dpkg conflict checks.
func partitionArchAllDepsForExtract(deps []string) (archSpecific, archAll []string) {
	cache := aptcache.Load()

	for _, dep := range deps {
		name, _, _ := strings.Cut(dep, " (")
		name, _, _ = strings.Cut(name, ":")

		if isHostOnlyPackage(name) {
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
// the target architecture (e.g. :arm64). All packages are extracted via
// dpkg -x (pure Go equivalent) so no dependency resolution occurs.
func (bb *BaseBuilder) DownloadAndExtractCrossDeps(deps []string, targetArch string) error {
	if bb.Format != constants.FormatDEB {
		// Non-DEB formats: fall back to normal install (no cross-arch conflict).
		installArgs := constants.GetInstallArgs(bb.Format)

		return bb.PKGBUILD.GetDepends(getPackageManager(bb.Format), installArgs, deps)
	}

	// Use the extract-safe partitioning: since we download+extract (not dpkg -i),
	// there are no dpkg conflicts, so packages already installed for the host arch
	// must still be downloaded as target-arch.
	archSpecific, archAll := partitionArchAllDepsForExtract(deps)
	qualified := qualifyDepsForTargetArch(archSpecific, bb.Format, targetArch)

	all := make([]string, 0, len(archAll)+len(qualified))
	all = append(all, archAll...)
	all = append(all, qualified...)

	if len(all) == 0 {
		return nil
	}

	logger.Info("Downloading and extracting cross-build runtime deps",
		"target_arch", targetArch,
		"arch_specific", strings.Join(qualified, ", "),
		"arch_all", strings.Join(archAll, ", "))

	// Create a temporary directory for downloaded .deb files.
	tmpDir, err := os.MkdirTemp("", "yap-cross-deps-*")
	if err != nil {
		return errors.Wrap(err, errors.ErrTypeFileSystem, "create temp dir for cross deps").
			WithOperation("DownloadAndExtractCrossDeps")
	}

	defer func() { _ = os.RemoveAll(tmpDir) }()

	// Download all packages in one apt-get download call.
	if err := aptGetDownload(tmpDir, all); err != nil {
		return errors.Wrap(err, errors.ErrTypeBuild, "apt-get download cross deps").
			WithOperation("DownloadAndExtractCrossDeps")
	}

	// Extract each downloaded .deb to the root filesystem.
	entries, err := os.ReadDir(tmpDir)
	if err != nil {
		return errors.Wrap(err, errors.ErrTypeFileSystem, "read cross deps dir").
			WithOperation("DownloadAndExtractCrossDeps").
			WithContext("path", tmpDir)
	}

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".deb") {
			continue
		}

		debPath := tmpDir + "/" + entry.Name()

		logger.Info("Extracting cross-build runtime dep",
			"package", entry.Name())

		if err := extractDEB(debPath, "/"); err != nil {
			return errors.Wrap(err, errors.ErrTypeBuild, "extract cross dep").
				WithOperation("DownloadAndExtractCrossDeps").
				WithContext("package", entry.Name())
		}
	}

	return nil
}

// installCrossDeps installs DEB cross-compilation dependencies in two passes.
// Architecture:all packages are installed first so they are present when
// arch-specific (target-arch) packages are installed — this satisfies
// transitive dependencies that arch-specific packages may have on arch-all
// packages (e.g. carbonio-openldap:arm64 → carbonio-core which is arch:all).
func (bb *BaseBuilder) installCrossDeps(makeDepends, installArgs []string, targetArch string) error {
	archSpecific, archAll := partitionArchAllDeps(makeDepends)
	qualified := qualifyDepsForTargetArch(archSpecific, bb.Format, targetArch)

	logger.Info("Qualifying makedepends for target architecture",
		"target_arch", targetArch,
		"format", bb.Format,
		"arch_specific", strings.Join(qualified, ", "),
		"arch_all", strings.Join(archAll, ", "))

	// Install arch-all (host) packages first so they are available to
	// satisfy transitive dependencies of the target-arch packages.
	if len(archAll) > 0 {
		if err := bb.PKGBUILD.GetDepends(getPackageManager(bb.Format), installArgs, archAll); err != nil {
			return err
		}
	}

	return bb.PKGBUILD.GetDepends(getPackageManager(bb.Format), installArgs, qualified)
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

// handleCrossCompilation handles cross-compilation setup including validation and dependency collection.
// This helper reduces nesting complexity in PrepareEnvironmentWithValidation.
func (bb *BaseBuilder) handleCrossCompilation(targetArch string, skipValidation bool, allArgs *[]string) error {
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
		logger.Info(i18n.T("logger.cross_compilation.installing_cross_compiler_packages"),
			"target_arch", targetArch,
			"packages", strings.Join(crossDeps, ", "))
	}

	*allArgs = append(*allArgs, crossDeps...)

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

// SetupCrossCompilationEnvironment configures environment variables for cross-compilation.
// This function sets up environment variables for C/C++, Rust, and Go cross-compilation.
// It uses a mutex to serialize access to os.Setenv to prevent race conditions in parallel builds.
//
//nolint:gocyclo,cyclop // SetupCrossCompilationEnvironment is inherently complex; splitting would harm readability
func (bb *BaseBuilder) SetupCrossCompilationEnvironment(targetArch string) error {
	if targetArch == "" || targetArch == bb.PKGBUILD.ArchComputed {
		// No cross-compilation needed
		return nil
	}

	// Idempotency: the cross-compile env is process-global (os.Setenv) and
	// changes only when the target architecture changes. Subsequent packages
	// in the same yap.json that target the same arch don't need to re-run
	// the setup. Skipping also avoids:
	//   - log spam ("Rust/Go/autoconf cross-compilation configured" per pkg);
	//   - PKG_CONFIG_PATH duplication (each call appends the existing value
	//     back to the prepended toolchain paths, so paths multiply per pkg);
	//   - unnecessary contention on envMutex during parallel builds.
	if os.Getenv("YAP_CROSS_ENV_FOR") == targetArch {
		logger.Debug("cross-compilation environment already configured; skipping",
			"target_arch", targetArch)

		return nil
	}

	logger.Info(i18n.T("logger.cross_compilation.setting_up_cross_compilation_environment"),
		"target_arch", targetArch,
		"build_arch", bb.PKGBUILD.ArchComputed)

	// Get the appropriate cross-compiler toolchain for the target architecture
	toolchain, exists := CrossToolchainMap[targetArch]
	if !exists {
		return errors.New(errors.ErrTypeBuild, "no cross-compilation toolchain available").
			WithOperation("SetupCrossCompilationEnvironment").
			WithContext("targetArch", targetArch)
	}

	// Determine the distribution-specific toolchain packages
	var distro string

	switch bb.Format {
	case constants.FormatDEB:
		distro = distroDebian
	case constants.FormatRPM:
		distro = distroFedora
	case constants.FormatAPK:
		distro = distroAlpine
	case constants.FormatPacman:
		distro = distroArch
	default:
		distro = distroDebian // fallback
	}

	toolchainPackages, exists := toolchain[distro]
	if !exists {
		// Try to find a toolchain for any distribution
		for _, distroToolchain := range toolchain {
			toolchainPackages = distroToolchain
			break
		}

		if toolchainPackages.GCCPackage == "" {
			return errors.New(errors.ErrTypeBuild, "no cross-compilation toolchain available").
				WithOperation("SetupCrossCompilationEnvironment").
				WithContext("targetArch", targetArch).
				WithContext("distro", distro)
		}
	}

	// Set up C/C++ cross-compilation environment variables
	// Check if ccache is available and wrap the cross-compiler commands with it
	_, ccacheErr := exec.LookPath("ccache")
	ccacheAvailable := ccacheErr == nil

	// Convert package names to executable names
	gccExecutable := toolchainPackages.GetExecutableName(toolchainPackages.GCCPackage)
	gppExecutable := toolchainPackages.GetExecutableName(toolchainPackages.GPlusPlusPackage)

	// Extract binutils prefix for cross tool names (e.g. "aarch64-linux-gnu" →
	// "aarch64-linux-gnu-ar").  Use the canonical binutilsPrefix() helper which
	// handles both "binutils-<prefix>" (Debian/Alpine) and "<prefix>-binutils"
	// (Arch) naming conventions correctly.
	binutilsPrefix := toolchainPackages.binutilsPrefix()

	// Calculate CROSS_COMPILE prefix from the executable name
	ccPrefix := ""

	if strings.Contains(gccExecutable, "-gcc") {
		// Extract prefix before -gcc (e.g., "aarch64-linux-gnu-gcc" -> "aarch64-linux-gnu")
		parts := strings.Split(gccExecutable, "-gcc")
		if len(parts) > 0 {
			ccPrefix = parts[0]
		}
	} else if strings.Contains(gccExecutable, "gcc") {
		// Extract prefix before gcc
		parts := strings.Split(gccExecutable, "gcc")
		if len(parts) > 0 {
			ccPrefix = strings.TrimSuffix(parts[0], "-")
		}
	}

	// Set up Rust cross-compilation environment variables
	rustTarget := bb.getRustTargetArchitecture(targetArch)

	rustTargetUpper := ""
	if rustTarget != "" {
		rustTargetUpper = strings.ToUpper(strings.ReplaceAll(rustTarget, "-", "_"))
	}

	// Set up Go cross-compilation environment variables
	goArch := bb.getGoTargetArchitecture(targetArch)

	// Set up autoconf cross-compilation configuration
	// Get the GNU triplets for the build and host architectures
	hostTriplet := bb.getGNUTriplet(targetArch)
	buildTriplet := bb.getGNUTriplet(bb.PKGBUILD.ArchComputed)

	var configureWrapper string
	if hostTriplet != "" && buildTriplet != "" {
		// Create configure wrapper function that can be used in PKGBUILDs
		// This is a bash function that automatically adds --host and --build flags
		configureWrapper = fmt.Sprintf(`
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

	// Serialize access to os.Setenv to prevent race conditions in parallel builds
	envMutex.Lock()
	defer envMutex.Unlock()

	// For cross-compilation, set CC/CXX to the bare cross-compiler and use
	// CCACHE_PREFIX so ccache wraps it transparently.  Setting CC="ccache
	// aarch64-linux-gnu-gcc" breaks build systems (e.g. OpenSSL's Configure)
	// that derive the cross-prefix by stripping a known suffix from CC.
	_ = os.Setenv("CC", gccExecutable)
	_ = os.Setenv("CXX", gppExecutable)

	if ccacheAvailable {
		// ccache wraps the cross-compiler transparently via the
		// /usr/lib/ccache/<cross-compiler> symlinks that are ahead of the
		// real compiler on PATH. No extra env var is needed.
		logger.Info("ccache active for cross-compilation",
			"cc", gccExecutable,
			"via", "/usr/lib/ccache/"+gccExecutable)
	}

	_ = os.Setenv("AR", binutilsPrefix+"-ar")
	_ = os.Setenv("STRIP", binutilsPrefix+"-strip")
	_ = os.Setenv("RANLIB", binutilsPrefix+"-ranlib")
	_ = os.Setenv("OBJDUMP", binutilsPrefix+"-objdump")
	_ = os.Setenv("OBJCOPY", binutilsPrefix+"-objcopy")
	_ = os.Setenv("LD", binutilsPrefix+"-ld")
	_ = os.Setenv("NM", binutilsPrefix+"-nm")

	// Generate a standard CMake cross-compilation toolchain file and point
	// CMAKE_TOOLCHAIN_FILE at it. PKGBUILDs can override this by passing
	// -DCMAKE_TOOLCHAIN_FILE=... explicitly to cmake, which takes precedence
	// over the environment variable.
	if cmakeToolchain, err := writeCMakeToolchainFile(targetArch, gccExecutable, gppExecutable, ccPrefix); err != nil {
		logger.Warn("failed to write CMake toolchain file", "error", err)
	} else {
		_ = os.Setenv("CMAKE_TOOLCHAIN_FILE", cmakeToolchain)
	}

	// Set up Rust cross-compilation environment variables
	if rustTarget != "" {
		_ = os.Setenv("CARGO_BUILD_TARGET", rustTarget)
		_ = os.Setenv("RUSTC_TARGET", rustTarget)

		// Cargo's linker must be a C compiler (gcc), not the raw linker (ld).
		// Using ld directly causes link failures because Cargo invokes the
		// linker as a C compiler wrapper (passes -Wl,... flags etc.).
		_ = os.Setenv("CARGO_TARGET_"+rustTargetUpper+"_LINKER", gccExecutable)

		// Rust build script CC/CXX: use bare cross-compiler; ccache wraps
		// via /usr/lib/ccache/<cross-compiler> symlinks on PATH.
		_ = os.Setenv("TARGET_"+rustTargetUpper+"_CC", gccExecutable)
		_ = os.Setenv("TARGET_"+rustTargetUpper+"_CXX", gppExecutable)

		// Prevent the host's -m64 (or other host-arch flags) from leaking
		// into C code compiled by Rust's cc crate for the target. The cc
		// crate reads CFLAGS_<TARGET> (underscores, upper-case) and uses it
		// instead of deriving flags from the host CFLAGS.
		_ = os.Setenv("CFLAGS_"+rustTargetUpper, "-O2 -fPIC")
	}

	// Set up Go cross-compilation environment variables
	goOS := linuxOS // Default to Linux for cross-compilation
	if goArch != "" {
		_ = os.Setenv("GOOS", goOS)
		_ = os.Setenv("GOARCH", goArch)

		// CGO: bare cross-compiler; ccache wraps via CCACHE_PREFIX.
		_ = os.Setenv("CGO_ENABLED", "1")
		_ = os.Setenv("CC_FOR_TARGET", gccExecutable)
		_ = os.Setenv("CXX_FOR_TARGET", gppExecutable)
	}

	// Autotools: CC_FOR_BUILD/CXX_FOR_BUILD must produce host-arch executables
	// (build-time code generators, lex/yacc tools, etc.). Without these, autotools
	// may inherit the cross-compiler and produce target-arch binaries that cannot
	// run on the build host.
	_ = os.Setenv("CC_FOR_BUILD", "gcc")
	_ = os.Setenv("CXX_FOR_BUILD", "g++")
	_ = os.Setenv("CFLAGS_FOR_BUILD", "")
	_ = os.Setenv("CXXFLAGS_FOR_BUILD", "")

	// Set common cross-compilation variables.
	// CROSS_COMPILE is the canonical indicator (e.g. "aarch64-linux-gnu-").
	// We intentionally do NOT set TARGET_ARCH/HOST_ARCH/BUILD_ARCH because
	// GNU make's implicit LINK.c rule expands $(TARGET_ARCH) verbatim into
	// compile commands, breaking any package that uses the default link rule.
	_ = os.Setenv("CROSS_COMPILE", ccPrefix+"-")

	// CROSS_COMPILE_HOST is CROSS_COMPILE without the trailing dash.
	// Convenience for PKGBUILDs that need the bare triplet (e.g. aarch64-linux-gnu)
	// without having to strip it via ${CROSS_COMPILE%-}.
	_ = os.Setenv("CROSS_COMPILE_HOST", ccPrefix)

	// Configure pkg-config for cross-compilation: prepend toolchain paths to
	// any existing PKG_CONFIG_PATH.
	crossPkgConfigPaths := []string{
		"/usr/lib/" + ccPrefix + "/pkgconfig",
		"/usr/local/lib/" + ccPrefix + "/pkgconfig",
	}

	existingPkgConfig := os.Getenv("PKG_CONFIG_PATH")
	if existingPkgConfig != "" {
		crossPkgConfigPaths = append(crossPkgConfigPaths, existingPkgConfig)
	}

	_ = os.Setenv("PKG_CONFIG_PATH", strings.Join(crossPkgConfigPaths, ":"))
	_ = os.Setenv("PKG_CONFIG_LIBDIR", "/usr/lib/"+ccPrefix+"/pkgconfig")

	// Set up autoconf cross-compilation configuration
	if hostTriplet != "" && buildTriplet != "" {
		// Configure autoconf for cross-compilation
		// These environment variables inform autoconf that we're cross-compiling
		_ = os.Setenv("ac_cv_host", hostTriplet)
		_ = os.Setenv("ac_cv_build", buildTriplet)

		// Pre-populate autoconf type-size cache variables. When cross-compiling,
		// configure cannot execute test binaries to probe sizes; without these
		// cached values it either fails or silently produces wrong results.
		if sizeVars, ok := autoconfSizeVars[targetArch]; ok {
			for k, v := range sizeVars {
				_ = os.Setenv(k, v)
			}
		}

		// Set the wrapper in the environment (will be available to the build script)
		_ = os.Setenv("YAP_CONFIGURE_WRAPPER", configureWrapper)
	}

	// Mutex will be released by defer statement above

	if rustTarget != "" {
		logger.Info(i18n.T("logger.cross_compilation.rust_cross_compilation_configured"),
			"rust_target", rustTarget,
			"target_arch", targetArch)
	}

	if goArch != "" {
		logger.Info(i18n.T("logger.cross_compilation.go_cross_compilation_configured"),
			"goos", goOS,
			"goarch", goArch,
			"target_arch", targetArch)
	}

	if hostTriplet != "" && buildTriplet != "" {
		logger.Info(i18n.T("logger.cross_compilation.autoconf_cross_compilation_configured"),
			"host_triplet", hostTriplet,
			"build_triplet", buildTriplet)
	}

	logger.Info(i18n.T("logger.cross_compilation.cross_compilation_environment_configured"),
		"target_arch", targetArch,
		"cc", os.Getenv("CC"),
		"cxx", os.Getenv("CXX"))

	// Mark setup so subsequent packages targeting the same arch skip the
	// full re-configuration. Cleared implicitly by yap exiting; if a future
	// caller needs to force re-setup (e.g. arch change mid-run), they should
	// os.Unsetenv("YAP_CROSS_ENV_FOR") first.
	_ = os.Setenv("YAP_CROSS_ENV_FOR", targetArch)

	return nil
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
		"ac_cv_sizeof_char":      "1",
		"ac_cv_sizeof_short":     "2",
		"ac_cv_sizeof_int":       "4",
		"ac_cv_sizeof_long":      "8",
		"ac_cv_sizeof_long_long": "8",
		"ac_cv_sizeof_void_p":    "8",
		"ac_cv_sizeof_size_t":    "8",
		"ac_cv_sizeof_off_t":     "8",
		"ac_cv_sizeof_wchar_t":   "4",
		"ac_cv_c_bigendian":      "no",
	},
	constants.ArchArmv7: {
		"ac_cv_sizeof_char":      "1",
		"ac_cv_sizeof_short":     "2",
		"ac_cv_sizeof_int":       "4",
		"ac_cv_sizeof_long":      "4",
		"ac_cv_sizeof_long_long": "8",
		"ac_cv_sizeof_void_p":    "4",
		"ac_cv_sizeof_size_t":    "4",
		"ac_cv_sizeof_off_t":     "8",
		"ac_cv_sizeof_wchar_t":   "4",
		"ac_cv_c_bigendian":      "no",
	},
	constants.ArchArmv6: {
		"ac_cv_sizeof_char":      "1",
		"ac_cv_sizeof_short":     "2",
		"ac_cv_sizeof_int":       "4",
		"ac_cv_sizeof_long":      "4",
		"ac_cv_sizeof_long_long": "8",
		"ac_cv_sizeof_void_p":    "4",
		"ac_cv_sizeof_size_t":    "4",
		"ac_cv_sizeof_off_t":     "8",
		"ac_cv_sizeof_wchar_t":   "4",
		"ac_cv_c_bigendian":      "no",
	},
	constants.ArchI686: {
		"ac_cv_sizeof_char":      "1",
		"ac_cv_sizeof_short":     "2",
		"ac_cv_sizeof_int":       "4",
		"ac_cv_sizeof_long":      "4",
		"ac_cv_sizeof_long_long": "8",
		"ac_cv_sizeof_void_p":    "4",
		"ac_cv_sizeof_size_t":    "4",
		"ac_cv_sizeof_off_t":     "8",
		"ac_cv_sizeof_wchar_t":   "4",
		"ac_cv_c_bigendian":      "no",
	},
	constants.ArchX86_64: {
		"ac_cv_sizeof_char":      "1",
		"ac_cv_sizeof_short":     "2",
		"ac_cv_sizeof_int":       "4",
		"ac_cv_sizeof_long":      "8",
		"ac_cv_sizeof_long_long": "8",
		"ac_cv_sizeof_void_p":    "8",
		"ac_cv_sizeof_size_t":    "8",
		"ac_cv_sizeof_off_t":     "8",
		"ac_cv_sizeof_wchar_t":   "4",
		"ac_cv_c_bigendian":      "no",
	},
	constants.ArchPpc64le: {
		"ac_cv_sizeof_char":      "1",
		"ac_cv_sizeof_short":     "2",
		"ac_cv_sizeof_int":       "4",
		"ac_cv_sizeof_long":      "8",
		"ac_cv_sizeof_long_long": "8",
		"ac_cv_sizeof_void_p":    "8",
		"ac_cv_sizeof_size_t":    "8",
		"ac_cv_sizeof_off_t":     "8",
		"ac_cv_sizeof_wchar_t":   "4",
		"ac_cv_c_bigendian":      "no",
	},
	constants.ArchS390x: {
		"ac_cv_sizeof_char":      "1",
		"ac_cv_sizeof_short":     "2",
		"ac_cv_sizeof_int":       "4",
		"ac_cv_sizeof_long":      "8",
		"ac_cv_sizeof_long_long": "8",
		"ac_cv_sizeof_void_p":    "8",
		"ac_cv_sizeof_size_t":    "8",
		"ac_cv_sizeof_off_t":     "8",
		"ac_cv_sizeof_wchar_t":   "4",
		"ac_cv_c_bigendian":      "yes",
	},
	constants.ArchRiscv64: {
		"ac_cv_sizeof_char":      "1",
		"ac_cv_sizeof_short":     "2",
		"ac_cv_sizeof_int":       "4",
		"ac_cv_sizeof_long":      "8",
		"ac_cv_sizeof_long_long": "8",
		"ac_cv_sizeof_void_p":    "8",
		"ac_cv_sizeof_size_t":    "8",
		"ac_cv_sizeof_off_t":     "8",
		"ac_cv_sizeof_wchar_t":   "4",
		"ac_cv_c_bigendian":      "no",
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

	return fmt.Errorf("unsupported target architecture %q — known: %s", arch, strings.Join(known, ", "))
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

	content := fmt.Sprintf(`# Auto-generated by yap for cross-compilation to %s
# Do not edit — regenerated on each build.
set(CMAKE_SYSTEM_NAME Linux)
set(CMAKE_SYSTEM_PROCESSOR %s)

set(CMAKE_C_COMPILER   %s)
set(CMAKE_CXX_COMPILER %s)

set(CMAKE_FIND_ROOT_PATH %s)
set(CMAKE_FIND_ROOT_PATH_MODE_PROGRAM NEVER)
set(CMAKE_FIND_ROOT_PATH_MODE_LIBRARY ONLY)
set(CMAKE_FIND_ROOT_PATH_MODE_INCLUDE ONLY)
`, targetArch, targetArch, gccExecutable, gppExecutable, sysroot)

	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		return "", fmt.Errorf("writing CMake toolchain file: %w", err)
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
