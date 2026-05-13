// Package common provides shared interfaces and base implementations for package builders.
package common

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"

	"github.com/M0Rf30/yap/v2/pkg/constants"
	"github.com/M0Rf30/yap/v2/pkg/errors"
	"github.com/M0Rf30/yap/v2/pkg/files"
	"github.com/M0Rf30/yap/v2/pkg/i18n"
	"github.com/M0Rf30/yap/v2/pkg/logger"
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
// This consolidates the duplicated architecture handling pattern across all builders.
func (bb *BaseBuilder) SetTargetArchitecture(targetArch string) {
	if targetArch != "" {
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
	options := files.WalkOptions{
		BackupFiles: bb.PKGBUILD.Backup,
	}

	// Configure format-specific options
	switch bb.Format {
	case formatPacman:
		// Pacman skips dot files
		options.SkipDotFiles = true
	case formatApk:
		// APK skips control files starting with '.'
		options.SkipPatterns = []string{".*"}
	}

	return files.NewWalker(bb.PKGBUILD.PackageDir, options)
}

// LogPackageCreated logs successful package creation with consistent formatting.
func (bb *BaseBuilder) LogPackageCreated(artifactPath string) {
	logger.Info(i18n.T("logger.logpackagecreated.info.package_artifact_created_3"),
		"format", bb.Format,
		"pkgver", bb.PKGBUILD.PkgVer,
		"pkgrel", bb.PKGBUILD.PkgRel,
		"artifact", artifactPath)
}

// FormatRelease formats the package release string with distribution-specific suffixes.
// For DEB packages: appends codename or distro name
// For RPM packages: appends RPM distro suffix and codename
// For other formats: returns release unchanged
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

// partitionArchAllDeps splits a DEB dependency list into two groups:
// arch-specific packages (need ":arm64" qualifier) and packages that must
// be installed without an arch qualifier.
//
// A package is left unqualified when:
//   - Architecture: all  — no arch-specific variant exists in the repos.
//   - Essential: yes     — pre-installed on the host; installing the foreign-arch
//     variant alongside it would cause a conflict (e.g. bash).
//   - Multi-Arch: no (or absent) AND already installed for the host arch —
//     the package cannot coexist across architectures.
//
// Packages that cannot be queried are assumed arch-specific so apt can
// surface a clear error if something is truly missing.
func partitionArchAllDeps(deps []string) (archSpecific, archAll []string) {
	for _, dep := range deps {
		// Strip version constraint for the lookup: "libssl-dev (>= 1.0)" → "libssl-dev"
		name, _, _ := strings.Cut(dep, " (")
		// Strip any existing arch qualifier
		name, _, _ = strings.Cut(name, ":")

		out, err := exec.CommandContext(context.Background(), "apt-cache", "show", name).Output() // #nosec G204
		if err != nil {
			archSpecific = append(archSpecific, dep)
			continue
		}

		info := string(out)

		// Architecture: all — no arch-specific variant.
		if strings.Contains(info, "Architecture: all") {
			archAll = append(archAll, dep)
			continue
		}

		// Essential packages (e.g. bash) conflict when installed for a foreign arch
		// alongside the host-arch version.
		if strings.Contains(info, "Essential: yes") {
			archAll = append(archAll, dep)
			continue
		}

		// Multi-Arch: no (or absent) + already installed → would conflict.
		multiArchForeign := strings.Contains(info, "Multi-Arch: foreign") ||
			strings.Contains(info, "Multi-Arch: allowed") ||
			strings.Contains(info, "Multi-Arch: same")
		if !multiArchForeign {
			dpkgOut, dpkgErr := exec.CommandContext(context.Background(), "dpkg", "-s", name).Output() // #nosec G204
			if dpkgErr == nil && strings.Contains(string(dpkgOut), "Status: install ok installed") {
				archAll = append(archAll, dep)
				continue
			}
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

// Prepare installs build dependencies using the appropriate package manager.
// This consolidates duplicated Prepare methods across all builders.
func (bb *BaseBuilder) Prepare(makeDepends []string, targetArch string) error {
	installArgs := constants.GetInstallArgs(bb.Format)

	// Add cross-compilation dependencies if target architecture is different
	if targetArch != "" && targetArch != bb.PKGBUILD.ArchComputed {
		logger.Info(i18n.T("logger.cross_compilation.detected_target_architecture"),
			"target_arch", targetArch,
			"build_arch", bb.PKGBUILD.ArchComputed)

		crossDeps := bb.getCrossCompilerDependencies(targetArch)
		if len(crossDeps) > 0 {
			logger.Info(i18n.T("logger.cross_compilation.installing_cross_compiler_packages"),
				"target_arch", targetArch,
				"packages", strings.Join(crossDeps, ", "))
		}

		installArgs = append(installArgs, crossDeps...)

		// Qualify makedepends with the target architecture so the package
		// manager installs the target-arch variant of each library.
		//
		// DEB only: ":arm64" qualifiers work because the Docker images register
		// the target arch with dpkg --add-architecture and add the ports repo.
		//
		// RPM is intentionally excluded: Rocky/Fedora x86_64 containers do not
		// carry aarch64 -devel packages in their repos. The cross-compiler
		// toolchain bundles its own sysroot, so host-arch -devel packages are
		// used directly (the cross-compiler is pointed at them via PKG_CONFIG_PATH
		// and CROSS_COMPILE set in SetupCrossCompilationEnvironment).
		if bb.Format == constants.FormatDEB {
			// Separate Architecture:all packages — they have no arch-specific
			// variant and must be installed without an arch qualifier.
			archSpecific, archAll := partitionArchAllDeps(makeDepends)
			qualified := qualifyDepsForTargetArch(archSpecific, bb.Format, targetArch)
			qualified = append(qualified, archAll...)
			makeDepends = qualified
			logger.Info("Qualifying makedepends for target architecture",
				"target_arch", targetArch,
				"format", bb.Format,
				"packages", strings.Join(makeDepends, ", "))
		}
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

// Update updates the package manager's package database.
// This consolidates duplicated Update methods across all builders.
func (bb *BaseBuilder) Update() error {
	return bb.PKGBUILD.GetUpdates(getPackageManager(bb.Format), getUpdateCommand(bb.Format))
}

// SetupCcache checks if ccache is available and configures the build environment to use it.
// This function sets up environment variables to enable ccache for faster compilation.
// It uses a mutex to serialize access to os.Setenv to prevent race conditions in parallel builds.
func (bb *BaseBuilder) SetupCcache() error {
	// Check if ccache is available in the system using Go's exec.LookPath
	_, err := exec.LookPath("ccache")
	ccacheAvailable := err == nil // ccache is available if command is found in PATH

	if !ccacheAvailable {
		// ccache not found, log and continue without ccache
		logger.Info(i18n.T("logger.setupccache.info.ccache_not_found_skipping_1"),
			"package", bb.PKGBUILD.PkgName)

		return nil
	}

	// Serialize access to os.Setenv to prevent race conditions in parallel builds
	envMutex.Lock()
	defer envMutex.Unlock()

	// Set up ccache environment variables
	// These variables will be used by the build process to enable ccache
	// CC and CXX are set to use ccache wrapper
	// Additionally, set some common ccache configuration options
	_ = os.Setenv("CC", "ccache gcc")
	_ = os.Setenv("CXX", "ccache g++")
	_ = os.Setenv("CCACHE_BASEDIR", bb.PKGBUILD.StartDir)
	_ = os.Setenv("CCACHE_SLOPPINESS", "time_macros,include_file_mtime")
	_ = os.Setenv("CCACHE_NOHASHDIR", "1")

	logger.Info(i18n.T("logger.setupccache.info.ccache_enabled_for_build_1"),
		"package", bb.PKGBUILD.PkgName)

	// Create ccache directory if it doesn't exist
	ccacheDir := filepath.Join(bb.PKGBUILD.StartDir, ".ccache")
	if _, err := os.Stat(ccacheDir); os.IsNotExist(err) {
		err = os.MkdirAll(ccacheDir, 0o750)
		if err != nil {
			logger.Warn(i18n.T("logger.setupccache.warn.failed_to_create_ccache_dir_1"),
				"dir", ccacheDir, "error", err)
		}
	}

	_ = os.Setenv("CCACHE_DIR", ccacheDir)

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
		_ = os.Setenv("CCACHE_PREFIX", "ccache")
	}

	_ = os.Setenv("AR", binutilsPrefix+"-ar")
	_ = os.Setenv("STRIP", binutilsPrefix+"-strip")
	_ = os.Setenv("RANLIB", binutilsPrefix+"-ranlib")
	_ = os.Setenv("OBJDUMP", binutilsPrefix+"-objdump")
	_ = os.Setenv("OBJCOPY", binutilsPrefix+"-objcopy")
	_ = os.Setenv("LD", binutilsPrefix+"-ld")
	_ = os.Setenv("NM", binutilsPrefix+"-nm")

	// Set up Rust cross-compilation environment variables
	if rustTarget != "" {
		_ = os.Setenv("CARGO_BUILD_TARGET", rustTarget)
		_ = os.Setenv("RUSTC_TARGET", rustTarget)
		_ = os.Setenv("CARGO_TARGET_"+rustTargetUpper+"_LINKER",
			binutilsPrefix+"-ld")

		// Rust build script CC/CXX: use bare cross-compiler; ccache wraps
		// via CCACHE_PREFIX set above.
		_ = os.Setenv("TARGET_"+rustTargetUpper+"_CC", gccExecutable)
		_ = os.Setenv("TARGET_"+rustTargetUpper+"_CXX", gppExecutable)
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

	// Set common cross-compilation variables.
	// CROSS_COMPILE is the canonical indicator (e.g. "aarch64-linux-gnu-").
	// We intentionally do NOT set TARGET_ARCH/HOST_ARCH/BUILD_ARCH because
	// GNU make's implicit LINK.c rule expands $(TARGET_ARCH) verbatim into
	// compile commands, breaking any package that uses the default link rule.
	_ = os.Setenv("CROSS_COMPILE", ccPrefix+"-")

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

	return nil
}

// getRustTargetArchitecture maps YAP architecture names to Rust target triples
func (bb *BaseBuilder) getRustTargetArchitecture(arch string) string {
	const (
		rustAarch64Target = "aarch64-unknown-linux-gnu"
		rustArmv7Target   = "armv7-unknown-linux-gnueabihf"
		rustUnknownLinux  = "-unknown-linux-gnu"
	)

	rustTargets := map[string]string{
		constants.ArchAarch64: rustAarch64Target,
		constants.ArchArmv7:   rustArmv7Target,
		constants.ArchArmv6:   "arm-unknown-linux-gnueabihf",
		constants.ArchI686:    constants.ArchI686 + rustUnknownLinux,
		constants.ArchX86_64:  constants.ArchX86_64 + rustUnknownLinux,
		constants.ArchPpc64le: "powerpc64le" + rustUnknownLinux,
		constants.ArchS390x:   constants.ArchS390x + rustUnknownLinux,
		constants.ArchRiscv64: "riscv64gc" + rustUnknownLinux,
	}

	if target, exists := rustTargets[arch]; exists {
		return target
	}

	return ""
}

// getGoTargetArchitecture maps YAP architecture names to Go GOARCH values
func (bb *BaseBuilder) getGoTargetArchitecture(arch string) string {
	goArchs := map[string]string{
		constants.ArchAarch64: constants.ArchArm64,
		constants.ArchArmv7:   constants.ArchArm,
		constants.ArchArmv6:   constants.ArchArm,
		constants.ArchI686:    "386",
		constants.ArchX86_64:  constants.ArchAmd64,
		constants.ArchPpc64le: constants.ArchPpc64le,
		constants.ArchS390x:   constants.ArchS390x,
		constants.ArchRiscv64: constants.ArchRiscv64,
	}

	if goArch, exists := goArchs[arch]; exists {
		return goArch
	}

	return ""
}

// getGNUTriplet maps YAP architecture names to GNU system triplets for autoconf.
// These triplets follow the format: cpu-vendor-os, e.g., aarch64-linux-gnu.
// This is used for autoconf's --host and --build flags during cross-compilation.
func (bb *BaseBuilder) getGNUTriplet(arch string) string {
	gnuTriplets := map[string]string{
		constants.ArchAarch64: constants.TripletAarch64Linux,
		constants.ArchArmv7:   constants.TripletArmLinuxHf,
		constants.ArchArmv6:   constants.TripletArmLinuxHf,
		constants.ArchI686:    constants.TripletI686Linux,
		constants.ArchX86_64:  constants.TripletX8664Linux,
		constants.ArchPpc64le: constants.TripletPpc64leLinux,
		constants.ArchS390x:   constants.TripletS390xLinux,
		constants.ArchRiscv64: constants.TripletRiscv64Linux,
	}

	if triplet, exists := gnuTriplets[arch]; exists {
		return triplet
	}

	return ""
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

// ReadAndValidateChangelog reads the changelog from PKGBUILD and returns nil, nil if no changelog is present.
func (bb *BaseBuilder) ReadAndValidateChangelog() ([]byte, error) {
	data, err := bb.PKGBUILD.ReadChangelog()
	if err != nil {
		return nil, err
	}

	return data, nil
}
