// Package common provides shared interfaces and base implementations for package builders.
package common

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/M0Rf30/yap/v2/pkg/constants"
	"github.com/M0Rf30/yap/v2/pkg/dnfinstall"
	"github.com/M0Rf30/yap/v2/pkg/errors"
	"github.com/M0Rf30/yap/v2/pkg/files"
	"github.com/M0Rf30/yap/v2/pkg/i18n"
	"github.com/M0Rf30/yap/v2/pkg/logger"
	"github.com/M0Rf30/yap/v2/pkg/options"
	"github.com/M0Rf30/yap/v2/pkg/pkgbuild"
	"github.com/M0Rf30/yap/v2/pkg/platform"
)

const (
	linuxOS = "linux"
)

// Ccache environment variable values shared between BuildCcacheEnvSlice and tests.
const (
	ccacheEnvCC         = "CC=gcc"
	ccacheEnvCXX        = "CXX=g++"
	ccacheEnvSloppiness = "CCACHE_SLOPPINESS=time_macros,include_file_mtime"
	ccacheEnvNoHashDir  = "CCACHE_NOHASHDIR=1"
)

const (
	updateCommand = "update"
	formatDeb     = "deb"
	formatPacman  = "pacman"
	formatApk     = "apk"

	// Distribution identifiers
	distroDebian = "debian"
	distroFedora = "fedora"
	distroAlpine = "alpine"
	distroArch   = "arch"
	distroUbuntu = "ubuntu"
)

// envMutex serializes access to os.Setenv calls in SetupCrossStripEnv to prevent
// race conditions in parallel builds. When multiple packages targeting different
// architectures are built concurrently, they would otherwise stomp on each other's
// environment variables.
var envMutex sync.Mutex

// depVersionRegex matches version operators in dependency strings like "gcc<=11.0".
var depVersionRegex = regexp.MustCompile(`(<=|>=|=|>|<)`)

// BaseBuilder provides common functionality that can be embedded in concrete builders.
type BaseBuilder struct {
	PKGBUILD *pkgbuild.PKGBUILD
	Format   string // Package format: "apk", "deb", "rpm", "pacman"
}

// NewBaseBuilder creates a new base builder instance.
func NewBaseBuilder(pkgBuild *pkgbuild.PKGBUILD, format string) *BaseBuilder {
	return &BaseBuilder{
		PKGBUILD: pkgBuild,
		Format:   format,
	}
}

// ProcessDependencies processes dependency strings with version operators.
// This consolidates the duplicated dependency processing logic across package formats.
func (bb *BaseBuilder) ProcessDependencies(depends []string) []string {
	processed := make([]string, len(depends))

	for index, depend := range depends {
		result := depVersionRegex.Split(depend, -1)
		if len(result) == 2 {
			name := result[0]
			operator := strings.Trim(depend, result[0]+result[1])
			version := result[1]

			switch bb.Format {
			case formatDeb:
				processed[index] = fmt.Sprintf("%s (%s %s)", name, operator, version)
			case constants.FormatRPM:
				processed[index] = fmt.Sprintf("%s %s %s", name, operator, version)
			default:
				processed[index] = depend
			}
		} else {
			processed[index] = depend
		}
	}

	return processed
}

// BuildPackageName constructs standardized package names for different formats.
// Used by: APK, DEB, Pacman
// Not used by: RPM (RPM does not include epoch in the filename, only in metadata)
func (bb *BaseBuilder) BuildPackageName(extension string) string {
	name := fmt.Sprintf("%s-%s-%s", bb.PKGBUILD.PkgName, bb.PKGBUILD.PkgVer, bb.PKGBUILD.PkgRel)

	// Handle epoch for certain package types
	if bb.PKGBUILD.Epoch != "" && (extension == constants.ExtPacmanZst || extension == constants.ExtRPM) {
		name = fmt.Sprintf("%s-%s:%s-%s", bb.PKGBUILD.PkgName, bb.PKGBUILD.Epoch,
			bb.PKGBUILD.PkgVer, bb.PKGBUILD.PkgRel)
	}

	switch extension {
	case constants.ExtAPK, constants.ExtRPM:
		name += "." + bb.PKGBUILD.ArchComputed
	case constants.ExtDEB:
		name = fmt.Sprintf("%s_%s-%s_%s", bb.PKGBUILD.PkgName, bb.PKGBUILD.PkgVer,
			bb.PKGBUILD.PkgRel, bb.PKGBUILD.ArchComputed)
	default:
		name += "-" + bb.PKGBUILD.ArchComputed
	}

	return name + extension
}

// TranslateArchitecture translates architecture names for the target package format.
func (bb *BaseBuilder) TranslateArchitecture() {
	archMapping := constants.GetArchMapping()
	bb.PKGBUILD.ArchComputed = archMapping.TranslateArch(bb.Format, bb.PKGBUILD.ArchComputed)
}

// SetTargetArchitecture sets the target architecture for cross-compilation and translates it.
// If targetArch is empty, it uses the current ArchComputed value.
// Architecture-independent packages (arch=any) are never overridden — they always
// produce arch-all packages regardless of the cross-compilation target.
// This consolidates the duplicated architecture handling pattern across all builders.
func (bb *BaseBuilder) SetTargetArchitecture(targetArch string) {
	if targetArch != "" && bb.PKGBUILD.ArchComputed != pkgbuild.ArchAny {
		bb.PKGBUILD.ArchComputed = targetArch
	}

	bb.TranslateArchitecture()
}

// LogCrossCompilation logs cross-compilation build initiation if target architecture differs.
// This consolidates duplicated cross-compilation logging across builders.
func (bb *BaseBuilder) LogCrossCompilation(targetArch string) {
	if targetArch != "" && targetArch != bb.PKGBUILD.ArchComputed {
		logger.Info(i18n.T("logger.cross_compilation.starting_cross_compilation_build"),
			"package", bb.PKGBUILD.PkgName,
			"target_arch", targetArch,
			"build_arch", bb.PKGBUILD.ArchComputed)
	}
}

// SetupEnvironmentDeps gets the build environment dependency package names
// (without install flags) for the format.
func (bb *BaseBuilder) SetupEnvironmentDeps(golang bool) []string {
	buildDeps := constants.GetBuildDeps()

	var deps []string

	switch bb.Format {
	case constants.FormatAPK:
		deps = buildDeps.APK
	case constants.FormatDEB:
		deps = buildDeps.DEB
	case constants.FormatRPM:
		deps = buildDeps.RPM
	case constants.FormatPacman:
		deps = buildDeps.Pacman
	}

	if golang {
		// APK uses different Go setup
		if bb.Format == constants.FormatAPK {
			platform.CheckGO()
		} else {
			logger.Info(i18n.T(
				"logger.common.info.go_detected_version_check"))

			err := platform.GOSetup()
			if err != nil {
				logger.Warn(
					i18n.T(
						"logger.common.warn.failed_to_setup_go"),
					"error", err)
			}
		}
	}

	return deps
}

// SetupEnvironmentDependencies gets the build environment dependencies for
// the format, including install flags. Calls SetupEnvironmentDeps internally.
func (bb *BaseBuilder) SetupEnvironmentDependencies(golang bool) []string {
	deps := bb.SetupEnvironmentDeps(golang)
	installArgs := constants.GetInstallArgs(bb.Format)

	allArgs := make([]string, len(installArgs)+len(deps))
	copy(allArgs, installArgs)
	copy(allArgs[len(installArgs):], deps)

	return allArgs
}

// CreateFileWalker creates a configured file walker for the package format.
func (bb *BaseBuilder) CreateFileWalker() *files.Walker {
	walkOpts := files.WalkOptions{
		BackupFiles: bb.PKGBUILD.Backup,
	}

	// Configure format-specific options
	switch bb.Format {
	case formatPacman:
		// Pacman skips dot files
		walkOpts.SkipDotFiles = true
	case formatApk:
		// APK skips control files starting with '.'
		walkOpts.SkipPatterns = []string{".*"}
	}

	return files.NewWalker(bb.PKGBUILD.PackageDir, walkOpts)
}

// LogPackageCreated logs successful package creation with consistent formatting.
// When running under sudo, ownership of the artifact is transferred to the
// original invoking user so downstream consumers (CI agents, etc.) can read it
// without elevated privileges.
func (bb *BaseBuilder) LogPackageCreated(artifactPath string) {
	logger.Info(i18n.T("logger.common.info.package_artifact_created"),
		"format", bb.Format,
		"pkgver", bb.PKGBUILD.PkgVer,
		"pkgrel", bb.PKGBUILD.PkgRel,
		"artifact", artifactPath)

	if err := platform.PreserveOwnership(artifactPath); err != nil {
		logger.Warn(i18n.T("logger.common.warn.failed_to_get_original"),
			"path", artifactPath,
			"error", err)
	}
}

// FormatRelease formats the package release string with distribution-specific suffixes.
// For DEB packages: always appends a distro suffix (codename or distro name) for proper
// repository targeting. The suffix is guarded against double-append for split packages.
// For RPM packages: appends RPM distro suffix and codename (only when codename is set)
// For other formats: returns release unchanged
func (bb *BaseBuilder) FormatRelease(distroSuffixMap map[string]string) {
	switch bb.Format {
	case formatDeb:
		// DEB always appends a distro suffix for proper repository targeting
		var suffix string
		if bb.PKGBUILD.Codename != "" {
			suffix = bb.PKGBUILD.Codename
		} else {
			suffix = bb.PKGBUILD.Distro
		}

		// Guard against double-append when called multiple times for split packages
		if suffix != "" && !strings.HasSuffix(bb.PKGBUILD.PkgRel, suffix) {
			bb.PKGBUILD.PkgRel += suffix
		}
	case constants.FormatRPM:
		// RPM only appends when codename is set and distro is in the map
		if bb.PKGBUILD.Codename == "" {
			return
		}

		if suffix, exists := distroSuffixMap[bb.PKGBUILD.Distro]; exists {
			bb.PKGBUILD.PkgRel += suffix + bb.PKGBUILD.Codename
		}
	}
}

// PrepareBackupFilePaths ensures all backup file paths have a leading slash.
// This is required by RPM and some other package formats.
func (bb *BaseBuilder) PrepareBackupFilePaths() []string {
	backupFiles := make([]string, 0, len(bb.PKGBUILD.Backup))

	for _, filePath := range bb.PKGBUILD.Backup {
		if !strings.HasPrefix(filePath, "/") {
			filePath = "/" + filePath
		}

		backupFiles = append(backupFiles, filePath)
	}

	return backupFiles
}

// getPackageManager returns the package manager command for the given format.
func getPackageManager(format string) string {
	switch format {
	case constants.FormatDEB:
		return "apt-get"
	case constants.FormatRPM:
		return "dnf"
	case constants.FormatPacman:
		return formatPacman
	case constants.FormatAPK:
		return formatApk
	default:
		return ""
	}
}

// getExtension returns the file extension for the given format.
func getExtension(format string) string {
	switch format {
	case constants.FormatDEB:
		return constants.ExtDEB
	case constants.FormatRPM:
		return constants.ExtRPM
	case constants.FormatPacman:
		return constants.ExtPacmanZst
	case constants.FormatAPK:
		return constants.ExtAPK
	default:
		return ""
	}
}

// getUpdateCommand returns the update command for the given format.
// For RPM, "update" would upgrade packages interactively (prompting [y/N]);
// "makecache" only refreshes repo metadata, matching apt/apk semantics.
func getUpdateCommand(format string) string {
	switch format {
	case constants.FormatDEB:
		return updateCommand
	case constants.FormatRPM:
		return "makecache"
	case constants.FormatPacman:
		return "-Sy"
	case constants.FormatAPK:
		return updateCommand
	default:
		return ""
	}
}

// Prepare installs build dependencies using the appropriate package manager.
// This consolidates duplicated Prepare methods across all builders.
func (bb *BaseBuilder) Prepare(ctx context.Context, makeDepends []string, targetArch string) error {
	// Non-cross-compile path: install makedepends directly.
	if targetArch == "" || targetArch == bb.PKGBUILD.ArchComputed {
		// RPM: use dnfinstall
		if bb.Format == constants.FormatRPM {
			return bb.installRPMDeps(ctx, makeDepends)
		}

		installArgs := constants.GetInstallArgs(bb.Format)

		return bb.PKGBUILD.GetDepends(ctx, getPackageManager(bb.Format), installArgs, makeDepends)
	}

	logger.Info(i18n.T("logger.cross_compilation.detected_target_architecture"),
		"target_arch", targetArch,
		"build_arch", bb.PKGBUILD.ArchComputed)

	// Register the foreign architecture and its ports repository before any
	// :<arch> qualifier is sent to apt; without this the install would fail
	// because archive.ubuntu.com only carries the build-host arch.
	// Note: ensureCrossArchRepo is idempotent — safe to call even when
	// `yap prepare -t` already ran and registered the arch.
	if err := bb.ensureCrossArchRepo(targetArch); err != nil {
		return err
	}

	// Install the cross-compiler toolchain itself (gcc-<triplet>, g++-<triplet>,
	// binutils-<triplet>, libc6-dev-<arch>-cross). These are host-arch packages
	// — they execute on the build host and emit target-arch code — so they go
	// through the normal (non-:arch-qualified) install path. Without this step
	// the build sandbox would only get target-arch *libraries* (from the
	// qualified makedepends below) but no compiler to link them with.
	crossDeps := bb.getCrossCompilerDependencies(targetArch)
	if len(crossDeps) > 0 {
		logger.Info(i18n.T("logger.common.info.installing_cross_compiler_toolchain"), "target_arch", targetArch,
			"packages", strings.Join(crossDeps, ", "))

		installArgs := constants.GetInstallArgs(bb.Format)
		if err := bb.PKGBUILD.GetDepends(ctx, getPackageManager(bb.Format), installArgs, crossDeps); err != nil {
			return err
		}

		// Newly installed cross-compiler binaries (e.g. /usr/bin/aarch64-linux-gnu-gcc)
		// must be wrapped by ccache symlinks if ccache is in use; otherwise the
		// /usr/lib/ccache PATH entries silently bypass them.
		bb.refreshCcacheSymlinks()
	}

	// Qualify makedepends with the target architecture so the package
	// manager installs the target-arch variant of each library.
	//
	// DEB only: ":arm64" qualifiers work because ensureCrossArchRepo above
	// registered the architecture with dpkg --add-architecture and added the
	// ports repo.
	//
	// RPM is intentionally excluded: Rocky/Fedora x86_64 containers do not
	// carry aarch64 -devel packages in their repos. The cross-compiler
	// toolchain bundles its own sysroot, so host-arch -devel packages are
	// used directly (the cross-compiler is pointed at them via PKG_CONFIG_PATH
	// and CROSS_COMPILE set in SetupCrossCompilationEnvironment).
	if bb.Format == constants.FormatDEB {
		return bb.installCrossDeps(ctx, makeDepends, constants.GetInstallArgs(bb.Format), targetArch)
	}

	// RPM: use dnfinstall
	if bb.Format == constants.FormatRPM {
		return bb.installRPMDeps(ctx, makeDepends)
	}

	installArgs := constants.GetInstallArgs(bb.Format)

	return bb.PKGBUILD.GetDepends(ctx, getPackageManager(bb.Format), installArgs, makeDepends)
}

// PrepareEnvironment sets up the build environment with necessary tools.
// This consolidates duplicated PrepareEnvironment methods across all builders.
// Toolchain validation is always skipped here: PrepareEnvironment is called by
// "yap prepare" whose job is to *install* the cross-compiler — the toolchain
// cannot be present before it is installed.  Validation runs later during the
// build stage (builder.processFunction → SetupCrossCompilationEnvironment).
func (bb *BaseBuilder) PrepareEnvironment(ctx context.Context, golang bool, targetArch string) error {
	return bb.prepareEnvironmentWithValidation(golang, targetArch, true)
}

// prepareEnvironmentWithValidation sets up the build environment with optional toolchain validation.
// This version allows callers to skip toolchain validation if needed.
func (bb *BaseBuilder) prepareEnvironmentWithValidation(
	golang bool,
	targetArch string,
	skipValidation bool,
) error {
	deps := bb.SetupEnvironmentDeps(golang)

	// Add cross-compilation dependencies if target architecture is different
	if targetArch != "" && targetArch != bb.PKGBUILD.ArchComputed {
		// Register the foreign architecture and its ports repo before apt-get
		// install pulls cross-arch libraries. This keeps `yap prepare` and `yap
		// build` symmetrical: both paths reach apt with the right sources in
		// place regardless of how the image was prebuilt.
		if err := bb.ensureCrossArchRepo(targetArch); err != nil {
			return err
		}

		if err := bb.handleCrossCompilation(targetArch, skipValidation, &deps); err != nil {
			return err
		}
	}

	ctx := context.Background()
	installArgs := constants.GetInstallArgs(bb.Format)
	pm := getPackageManager(bb.Format)

	logger.Info(i18n.T("logger.common.info.installing_environment_dependencies_via"), "pm", pm,
		"packages", len(deps),
		"flags", len(installArgs))

	// Route through GetDepends so apk/apt/dnf-yum hit the in-process installers
	// and only pacman/zypper still hit the subprocess. Mirrors what
	// pkg/builders/common cross-deps does and avoids the prepare-time
	// "unauthenticated packages" failure on apt-get with --repo flags.
	if err := bb.PKGBUILD.GetDepends(ctx, pm, installArgs, deps); err != nil {
		return err
	}

	// Refresh ccache compiler symlinks so freshly installed cross-compilers are
	// wrapped automatically when /usr/lib/ccache (or /usr/lib64/ccache) is in
	// PATH.
	bb.refreshCcacheSymlinks()

	return nil
}

// refreshCcacheSymlinks runs the distribution-provided helper that creates
// /usr/lib/ccache/<compiler> symlinks for every compiler currently installed on
// the system. Without this step, cross-compilers installed at runtime would
// bypass ccache because their symlinks are generated only at package install
// time. The call is best-effort: it logs and returns on failure so the build
// can proceed without caching.
func (bb *BaseBuilder) refreshCcacheSymlinks() {
	if _, err := exec.LookPath("ccache"); err != nil {
		return
	}

	bin, err := exec.LookPath("update-ccache-symlinks")
	if err != nil {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// bin is the absolute path returned by exec.LookPath — it cannot be
	// influenced by user input and yap is expected to run as root.
	if err := exec.CommandContext(ctx, bin).Run(); err != nil {
		logger.Warn(i18n.T("logger.common.warn.update_ccache_symlinks_failed"), "error", err)
	}
}

// Update updates the package manager's package database.
// This consolidates duplicated Update methods across all builders.
// For DEB, --allow-insecure-repositories is passed so that apt writes
// Packages index files for unsigned extra repos (e.g. --repo flags) to
// /var/lib/apt/lists/. Without it, apt silently skips those repos and
// aptcache.Reload() cannot see their packages for arch classification.
func (bb *BaseBuilder) Update(ctx context.Context) error {
	cmd := getUpdateCommand(bb.Format)
	if bb.Format == constants.FormatDEB {
		return bb.PKGBUILD.GetUpdates(ctx, getPackageManager(bb.Format), cmd, "--allow-insecure-repositories")
	}

	return bb.PKGBUILD.GetUpdates(ctx, getPackageManager(bb.Format), cmd)
}

// installRPMDeps installs RPM dependencies using the dnfinstall package.
// This is called by Prepare and cross-compilation paths when the format is RPM.
func (bb *BaseBuilder) installRPMDeps(ctx context.Context, deps []string) error {
	if len(deps) == 0 {
		return nil
	}

	logger.Info(i18n.T("logger.common.info.installing_rpm_dependencies_via"), "packages", len(deps),
		"packages_list", strings.Join(deps, ", "))

	if err := dnfinstall.Install(ctx, deps); err != nil {
		return errors.Wrap(err, errors.ErrTypeBuild, "failed to install RPM dependencies").
			WithOperation("installRPMDeps").
			WithContext("packages", strings.Join(deps, ", "))
	}

	return nil
}

// BuildCcacheEnvSlice returns ccache environment variables as a "KEY=VALUE" slice
// for safe concurrent use. Unlike SetupCcache, it does NOT mutate the process
// environment (no os.Setenv calls). Returns nil if ccache is not available.
//
// CC/CXX are set to bare compiler names (gcc, g++). ccache wrapping is achieved
// by prepending the ccache symlink directory to PATH, matching the approach used
// by the cross-compilation path (BuildCrossEnvSlice). This avoids the
// "CC=ccache gcc" pattern which breaks some autoconf configure scripts that
// perform word-splitting on $CC in ways that fail with multi-word values.
func (bb *BaseBuilder) BuildCcacheEnvSlice() []string {
	// Check if ccache is available in the system using Go's exec.LookPath
	_, err := exec.LookPath("ccache")
	if err != nil {
		// ccache not available
		return nil
	}

	// Build ccache environment variables as a slice without calling os.Setenv.
	// CC/CXX are set to the bare compiler (e.g. CC=gcc). ccache wrapping is
	// achieved via PATH — we prepend the ccache symlink directory so that
	// resolving "gcc" finds /usr/lib*/ccache/gcc → /usr/bin/ccache first.
	vars := []string{
		ccacheEnvCC,
		ccacheEnvCXX,
		"CCACHE_BASEDIR=" + bb.PKGBUILD.StartDir,
		ccacheEnvSloppiness,
		ccacheEnvNoHashDir,
		// CCACHE_DIR is intentionally left unset so ccache resolves to its default
		// $HOME/.cache/ccache. Persistent caches (e.g. Kubernetes volumes) mounted
		// at that path are then shared across builds within the same user account.
	}

	// Prepend ccache symlink directory to PATH so compilers resolve through
	// ccache transparently. Check both common locations used by RHEL/Fedora
	// (/usr/lib64/ccache) and Debian/Ubuntu (/usr/lib/ccache).
	for _, dir := range []string{"/usr/lib64/ccache", "/usr/lib/ccache"} {
		if fi, err := os.Stat(dir); err == nil && fi.IsDir() {
			currentPath := os.Getenv("PATH")
			vars = append(vars, "PATH="+dir+":"+currentPath)

			break
		}
	}

	return vars
}

// PrepareScriptletWithHelpers prepends the PKGBUILD helper function preamble to a scriptlet body.
// Returns the body unchanged if no preamble is needed.
func (bb *BaseBuilder) PrepareScriptletWithHelpers(body string) string {
	preamble := bb.PKGBUILD.HelperFunctionsPreamble(body)
	if preamble == "" {
		return body
	}

	return preamble + body
}

// BuildCrossStripEnvSlice returns STRIP and OBJCOPY environment variables as a
// "KEY=VALUE" slice for safe concurrent use. Unlike SetupCrossStripEnv, it does NOT
// mutate the process environment (no os.Setenv calls). Returns nil if no cross-compilation
// is active or if the toolchain cannot be resolved.
//
// The returned slice can be passed to ApplyOptions() to configure cross-compilation
// binutils for the strip/objcopy pass, making it safe to call from multiple goroutines
// simultaneously (parallel builds).
func (bb *BaseBuilder) BuildCrossStripEnvSlice(targetArch string) []string {
	if targetArch == "" || targetArch == bb.PKGBUILD.ArchComputed {
		return nil
	}

	toolchain, err := bb.resolveToolchainPackages(targetArch)
	if err != nil {
		logger.Warn(i18n.T("logger.common.warn.cross_strip_env_failed"), "target_arch", targetArch, "error", err)

		return nil
	}

	prefix := toolchain.binutilsPrefix()
	if prefix == "" {
		return nil
	}

	logger.Debug(i18n.T("logger.common.debug.cross_strip_env_configured"), "STRIP", prefix+"-strip",
		"OBJCOPY", prefix+"-objcopy")

	return []string{
		"STRIP=" + prefix + "-strip",
		"OBJCOPY=" + prefix + "-objcopy",
	}
}

// SetupCrossStripEnv sets STRIP and OBJCOPY in the process environment so that
// any code path that still reads them via os.Getenv (e.g. external tooling)
// sees the cross-compilation binutils. Internal callers should prefer
// CrossStripEnvMap + ApplyOptionsWithEnv, which is parallel-safe.
// No-op when targetArch is empty or equals the build arch. Guarded by envMutex
// to serialize concurrent invocations.
func (bb *BaseBuilder) SetupCrossStripEnv(targetArch string) {
	env := bb.BuildCrossStripEnvSlice(targetArch)
	if env == nil {
		return
	}

	envMutex.Lock()
	defer envMutex.Unlock()

	for _, kv := range env {
		if k, v, ok := strings.Cut(kv, "="); ok {
			if err := os.Setenv(k, v); err != nil {
				logger.Warn(i18n.T("logger.common.warn.cross_strip_env_set_failed"), "var", k, "error", err)
			}
		}
	}
}

// ApplyOptions runs the PKGBUILD option handlers (strip, docs, libtool, etc.)
// against the package directory using the process environment for STRIP/OBJCOPY.
// Prefer ApplyOptionsWithEnv for parallel builds.
func (bb *BaseBuilder) ApplyOptions() error {
	return bb.ApplyOptionsWithEnv(nil)
}

// ApplyOptionsWithEnv is the env-overlay variant of ApplyOptions. The env map
// is consulted before os.Getenv for STRIP/OBJCOPY during the strip pass.
// Pair with BuildCrossStripEnvSlice() (parsed to a map) to scope cross-strip
// toolchain selection without mutating the global process environment.
func (bb *BaseBuilder) ApplyOptionsWithEnv(env map[string]string) error {
	return options.ApplyWithEnv(bb.PKGBUILD.PackageDir, options.Options{
		DebugEnabled:     bb.PKGBUILD.DebugEnabled,
		DocsEnabled:      bb.PKGBUILD.DocsEnabled,
		EmptyDirsEnabled: bb.PKGBUILD.EmptyDirsEnabled,
		LibtoolEnabled:   bb.PKGBUILD.LibtoolEnabled,
		PurgeEnabled:     bb.PKGBUILD.PurgeEnabled,
		StaticEnabled:    bb.PKGBUILD.StaticEnabled,
		StripEnabled:     bb.PKGBUILD.StripEnabled,
		ZipManEnabled:    bb.PKGBUILD.ZipManEnabled,
	}, env)
}

// CrossStripEnvMap parses BuildCrossStripEnvSlice into a map[K]V usable with
// ApplyOptionsWithEnv. Returns nil if no cross-strip env is needed.
func (bb *BaseBuilder) CrossStripEnvMap(targetArch string) map[string]string {
	slice := bb.BuildCrossStripEnvSlice(targetArch)
	if slice == nil {
		return nil
	}

	m := make(map[string]string, len(slice))
	for _, kv := range slice {
		if k, v, ok := strings.Cut(kv, "="); ok {
			m[k] = v
		}
	}

	return m
}
