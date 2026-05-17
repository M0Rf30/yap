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
	"github.com/M0Rf30/yap/v2/pkg/errors"
	"github.com/M0Rf30/yap/v2/pkg/files"
	"github.com/M0Rf30/yap/v2/pkg/i18n"
	"github.com/M0Rf30/yap/v2/pkg/logger"
	"github.com/M0Rf30/yap/v2/pkg/options"
	"github.com/M0Rf30/yap/v2/pkg/pkgbuild"
	"github.com/M0Rf30/yap/v2/pkg/platform"
	"github.com/M0Rf30/yap/v2/pkg/shell"
)

const (
	linuxOS = "linux"
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

// envMutex serializes access to os.Setenv calls in SetupCcache and
// SetupCrossCompilationEnvironment to prevent race conditions in parallel builds.
// When multiple packages targeting different architectures are built concurrently,
// they would otherwise stomp on each other's environment variables.
var envMutex sync.Mutex

// depVersionRegex matches version operators in dependency strings like "gcc<=11.0".
var depVersionRegex = regexp.MustCompile(`(<=|>=|=|>|<)`)

// Builder defines the common interface that all package builders must implement.
// This unifies the behavior across different package formats (APK, DEB, RPM, Pacman).
type Builder interface {
	// BuildPackage creates the package file at the specified artifacts path
	BuildPackage(artifactsPath string, targetArch string) (string, error)

	// PrepareFakeroot sets up the package metadata and prepares the build environment
	PrepareFakeroot(artifactsPath string, targetArch string) error

	// Prepare installs build dependencies and prepares the build environment
	Prepare(makeDepends []string, targetArch string) error

	// PrepareEnvironment sets up the build environment with necessary tools
	PrepareEnvironment(golang bool, targetArch string) error

	// Update updates the package manager's package database
	Update() error
}

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

// SetupEnvironmentDependencies gets the build environment dependencies for the format.
func (bb *BaseBuilder) SetupEnvironmentDependencies(golang bool) []string {
	buildDeps := constants.GetBuildDeps()
	installArgs := constants.GetInstallArgs(bb.Format)

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

	allArgs := make([]string, len(installArgs)+len(deps))
	copy(allArgs, installArgs)
	copy(allArgs[len(installArgs):], deps)

	if golang {
		// APK uses different Go setup
		if bb.Format == constants.FormatAPK {
			platform.CheckGO()
		} else {
			logger.Info(i18n.T("logger.setupenvironmentdependencies.info.go_detected_version_check_1"))

			err := platform.GOSetup()
			if err != nil {
				logger.Warn(i18n.T("logger.setupenvironmentdependencies.warn.failed_to_setup_go_1"),
					"error", err)
			}
		}
	}

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

// installCrossToolchain installs the cross-compiler toolchain (gcc/g++/binutils
// and libc6-dev-*-cross on DEB) as a dedicated apt/dnf/pacman call.
//
// This is a *separate* call from makedepends installation because GetDepends
// short-circuits when its makeDepends slice is empty (e.g. Go-only PKGBUILDs).
// Folding the toolchain packages into installArgs and relying on the
// makedepends call to flush them would silently drop them in that case,
// leaving binutils-<target>-strip absent from PATH and breaking later strip
// passes on foreign-arch ELFs.
func (bb *BaseBuilder) installCrossToolchain(targetArch string, installArgs []string) error {
	crossDeps := bb.getCrossCompilerDependencies(targetArch)
	if len(crossDeps) == 0 {
		return nil
	}

	logger.Info(i18n.T("logger.cross_compilation.installing_cross_compiler_packages"),
		"target_arch", targetArch,
		"packages", strings.Join(crossDeps, ", "))

	return bb.PKGBUILD.GetDepends(getPackageManager(bb.Format), installArgs, crossDeps)
}

// Prepare installs build dependencies using the appropriate package manager.
// This consolidates duplicated Prepare methods across all builders.
func (bb *BaseBuilder) Prepare(makeDepends []string, targetArch string) error {
	installArgs := constants.GetInstallArgs(bb.Format)

	// Non-cross-compile path: install makedepends directly.
	if targetArch == "" || targetArch == bb.PKGBUILD.ArchComputed {
		return bb.PKGBUILD.GetDepends(getPackageManager(bb.Format), installArgs, makeDepends)
	}

	logger.Info(i18n.T("logger.cross_compilation.detected_target_architecture"),
		"target_arch", targetArch,
		"build_arch", bb.PKGBUILD.ArchComputed)

	// Register the foreign architecture and its ports repository before any
	// :<arch> qualifier is sent to apt; without this the install would fail
	// because archive.ubuntu.com only carries the build-host arch.
	if err := bb.ensureCrossArchRepo(targetArch); err != nil {
		return err
	}

	if err := bb.installCrossToolchain(targetArch, installArgs); err != nil {
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
		return bb.installCrossDeps(makeDepends, installArgs, targetArch)
	}

	return bb.PKGBUILD.GetDepends(getPackageManager(bb.Format), installArgs, makeDepends)
}

// PrepareEnvironment sets up the build environment with necessary tools.
// This consolidates duplicated PrepareEnvironment methods across all builders.
// Toolchain validation is always skipped here: PrepareEnvironment is called by
// "yap prepare" whose job is to *install* the cross-compiler — the toolchain
// cannot be present before it is installed.  Validation runs later during the
// build stage (builder.processFunction → SetupCrossCompilationEnvironment).
func (bb *BaseBuilder) PrepareEnvironment(golang bool, targetArch string) error {
	return bb.prepareEnvironmentWithValidation(golang, targetArch, true)
}

// prepareEnvironmentWithValidation sets up the build environment with optional toolchain validation.
// This version allows callers to skip toolchain validation if needed.
func (bb *BaseBuilder) prepareEnvironmentWithValidation(golang bool, targetArch string, skipValidation bool) error {
	allArgs := bb.SetupEnvironmentDependencies(golang)

	// Add cross-compilation dependencies if target architecture is different
	if targetArch != "" && targetArch != bb.PKGBUILD.ArchComputed {
		// Register the foreign architecture and its ports repo before apt-get
		// install pulls cross-arch libraries. This keeps `yap prepare` and `yap
		// build` symmetrical: both paths reach apt with the right sources in
		// place regardless of how the image was prebuilt.
		if err := bb.ensureCrossArchRepo(targetArch); err != nil {
			return err
		}

		if err := bb.handleCrossCompilation(targetArch, skipValidation, &allArgs); err != nil {
			return err
		}
	}

	useSudo := bb.Format == constants.FormatAPK

	// Install dependencies first
	err := shell.ExecWithSudo(context.Background(), useSudo, "", getPackageManager(bb.Format), allArgs...)
	if err != nil {
		return err
	}

	// Refresh ccache compiler symlinks so freshly installed cross-compilers are
	// wrapped automatically when /usr/lib/ccache (or /usr/lib64/ccache) is in PATH.
	bb.refreshCcacheSymlinks()

	// Set up cross-compilation environment after dependencies are installed
	if targetArch != "" && targetArch != bb.PKGBUILD.ArchComputed {
		err = bb.SetupCrossCompilationEnvironment(targetArch)
		if err != nil {
			return errors.Wrap(err, errors.ErrTypeBuild, "failed to setup cross-compilation environment").
				WithOperation("prepareEnvironmentWithValidation")
		}
	}

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
	if err := exec.CommandContext(ctx, bin).Run(); err != nil { // #nosec G204
		logger.Warn("ccache: update-ccache-symlinks failed", "error", err)
	}
}

// Update updates the package manager's package database.
// This consolidates duplicated Update methods across all builders.
func (bb *BaseBuilder) Update() error {
	return bb.PKGBUILD.GetUpdates(getPackageManager(bb.Format), getUpdateCommand(bb.Format))
}

// SetupCcache checks if ccache is available and configures the build environment to use it.
// This function sets up environment variables to enable ccache for faster compilation.
// It uses a mutex to serialize access to os.Setenv to prevent race conditions in parallel builds.
//
// Idempotent: returns immediately if YAP_CCACHE_SETUP is already set. The
// ccache configuration is process-global (os.Setenv), so re-running it for
// every package in a yap.json iteration only produces redundant log lines
// and pointless envMutex contention.
//
// Note: SetupCcache and SetupCrossCompilationEnvironment write to the same
// CC/CXX variables. When both are needed for a build, the call site invokes
// SetupCcache first, then SetupCrossCompilationEnvironment, which overwrites
// CC/CXX with the cross-compiler and relies on CCACHE_PREFIX to wrap it.
// The memoization here is per-process, so a yap.json batch that mixes
// cross- and non-cross-arch builds will not re-run ccache setup between
// them — that pattern was already racy with the process-global os.Setenv
// approach and is tracked separately.
func (bb *BaseBuilder) SetupCcache() error {
	// Already configured for this process — skip.
	if os.Getenv("YAP_CCACHE_SETUP") == "1" {
		logger.Debug("ccache already configured; skipping",
			"package", bb.PKGBUILD.PkgName)

		return nil
	}

	// Check if ccache is available in the system using Go's exec.LookPath
	_, err := exec.LookPath("ccache")
	ccacheAvailable := err == nil // ccache is available if command is found in PATH

	if !ccacheAvailable {
		// ccache not found, log and continue without ccache.
		// Mark "setup attempted" so subsequent packages don't repeat the lookup.
		_ = os.Setenv("YAP_CCACHE_SETUP", "1")

		logger.Info(i18n.T("logger.setupccache.info.ccache_not_found_skipping_1"),
			"package", bb.PKGBUILD.PkgName)

		return nil
	}

	// Serialize access to os.Setenv to prevent race conditions in parallel builds
	envMutex.Lock()
	defer envMutex.Unlock()

	// Set up ccache environment variables. CC/CXX are wrapped only when the
	// build is not cross-compiling — for cross builds SetupCrossCompilationEnvironment
	// runs after this function and substitutes the bare cross-compiler, relying
	// on the /usr/lib/ccache (or /usr/lib64/ccache) symlinks to invoke ccache.
	_ = os.Setenv("CC", "ccache gcc")
	_ = os.Setenv("CXX", "ccache g++")
	_ = os.Setenv("CCACHE_BASEDIR", bb.PKGBUILD.StartDir)
	_ = os.Setenv("CCACHE_SLOPPINESS", "time_macros,include_file_mtime")
	_ = os.Setenv("CCACHE_NOHASHDIR", "1")

	// CCACHE_DIR is intentionally left unset so ccache resolves to its default
	// $HOME/.cache/ccache. Persistent caches (e.g. Kubernetes volumes) mounted
	// at that path are then shared across builds within the same user account.

	// Mark setup so subsequent packages in the same process skip the full
	// re-configuration. See the function-level comment for rationale.
	_ = os.Setenv("YAP_CCACHE_SETUP", "1")

	logger.Info(i18n.T("logger.setupccache.info.ccache_enabled_for_build_1"),
		"package", bb.PKGBUILD.PkgName)

	return nil
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
