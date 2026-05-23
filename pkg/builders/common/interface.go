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
	ccacheEnvCC         = "CC=ccache gcc"
	ccacheEnvCXX        = "CXX=ccache g++"
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
				"logger.setupenvironmentdependencies.info.go_detected_version_check_1"))

			err := platform.GOSetup()
			if err != nil {
				logger.Warn(
					i18n.T(
						"logger.setupenvironmentdependencies.warn.failed_to_setup_go_1"),
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
	logger.Info(i18n.T("logger.logpackagecreated.info.package_artifact_created_3"),
		"format", bb.Format,
		"pkgver", bb.PKGBUILD.PkgVer,
		"pkgrel", bb.PKGBUILD.PkgRel,
		"artifact", artifactPath)

	if err := platform.PreserveOwnership(artifactPath); err != nil {
		logger.Warn(i18n.T("logger.preserveownership.warn.failed_to_get_original_1"),
			"path", artifactPath,
			"error", err)
	}
}

// FormatRelease formats the package release string with distribution-specific suffixes.
// For RPM packages: appends RPM distro suffix and codename (only when codename is set)
// For other formats: returns release unchanged
// Note: DEB has its own getRelease() method in pkg/builders/deb/dpkg.go with different
// fallback behavior—it always appends a distro suffix (codename or distro name) for
// proper repository targeting, while this method only appends when codename is set.
func (bb *BaseBuilder) FormatRelease(distroSuffixMap map[string]string) {
	if bb.PKGBUILD.Codename == "" {
		return
	}

	switch bb.Format {
	case formatDeb:
		bb.PKGBUILD.PkgRel += bb.PKGBUILD.Codename
	case constants.FormatRPM:
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

	logger.Info("installing environment dependencies via package manager",
		"pm", pm,
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
		logger.Warn("ccache: update-ccache-symlinks failed", "error", err)
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

	logger.Info("installing RPM dependencies via dnfinstall",
		"packages", len(deps),
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
func (bb *BaseBuilder) BuildCcacheEnvSlice() []string {
	// Check if ccache is available in the system using Go's exec.LookPath
	_, err := exec.LookPath("ccache")
	if err != nil {
		// ccache not available
		return nil
	}

	// Build ccache environment variables as a slice without calling os.Setenv.
	// CC/CXX are wrapped only when the build is not cross-compiling — for cross
	// builds SetupCrossCompilationEnvironment runs after this function and
	// substitutes the bare cross-compiler, relying on the /usr/lib/ccache
	// (or /usr/lib64/ccache) symlinks to invoke ccache.
	return []string{
		ccacheEnvCC,
		ccacheEnvCXX,
		"CCACHE_BASEDIR=" + bb.PKGBUILD.StartDir,
		ccacheEnvSloppiness,
		ccacheEnvNoHashDir,
		// CCACHE_DIR is intentionally left unset so ccache resolves to its default
		// $HOME/.cache/ccache. Persistent caches (e.g. Kubernetes volumes) mounted
		// at that path are then shared across builds within the same user account.
	}
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

// SetupCrossStripEnv sets STRIP and OBJCOPY in the process environment so that
// the Go-side strip/objcopy pass (options.Apply → binary.StripFile) uses the
// cross-compilation binutils rather than the host tools.
//
// This must be called before ApplyOptions() whenever a cross-compilation target
// is active. It is a no-op when targetArch is empty or equals the build arch.
func (bb *BaseBuilder) SetupCrossStripEnv(targetArch string) {
	if targetArch == "" || targetArch == bb.PKGBUILD.ArchComputed {
		return
	}

	toolchain, err := bb.resolveToolchainPackages(targetArch)
	if err != nil {
		logger.Warn("cross-strip env: failed to resolve toolchain, strip will use native tools",
			"target_arch", targetArch, "error", err)

		return
	}

	prefix := toolchain.binutilsPrefix()
	if prefix == "" {
		return
	}

	envMutex.Lock()
	defer envMutex.Unlock()

	if err := os.Setenv("STRIP", prefix+"-strip"); err != nil {
		logger.Warn("cross-strip env: failed to set STRIP", "error", err)
	}

	if err := os.Setenv("OBJCOPY", prefix+"-objcopy"); err != nil {
		logger.Warn("cross-strip env: failed to set OBJCOPY", "error", err)
	}

	logger.Debug("cross-strip env configured",
		"STRIP", prefix+"-strip",
		"OBJCOPY", prefix+"-objcopy")
}

// ApplyOptions runs the PKGBUILD option handlers (strip, docs, libtool, etc.)
// against the package directory. This consolidates the duplicated options.Apply
// call across DEB, RPM, and Pacman builders.
func (bb *BaseBuilder) ApplyOptions() error {
	return options.Apply(bb.PKGBUILD.PackageDir, options.Options{
		DebugEnabled:     bb.PKGBUILD.DebugEnabled,
		DocsEnabled:      bb.PKGBUILD.DocsEnabled,
		EmptyDirsEnabled: bb.PKGBUILD.EmptyDirsEnabled,
		LibtoolEnabled:   bb.PKGBUILD.LibtoolEnabled,
		PurgeEnabled:     bb.PKGBUILD.PurgeEnabled,
		StaticEnabled:    bb.PKGBUILD.StaticEnabled,
		StripEnabled:     bb.PKGBUILD.StripEnabled,
		ZipManEnabled:    bb.PKGBUILD.ZipManEnabled,
	})
}
