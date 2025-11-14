// Package common provides shared interfaces and base implementations for package builders.
package common

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/M0Rf30/yap/v2/pkg/constants"
	"github.com/M0Rf30/yap/v2/pkg/files"
	"github.com/M0Rf30/yap/v2/pkg/i18n"
	"github.com/M0Rf30/yap/v2/pkg/logger"
	"github.com/M0Rf30/yap/v2/pkg/pkgbuild"
	"github.com/M0Rf30/yap/v2/pkg/platform"
	"github.com/M0Rf30/yap/v2/pkg/shell"
)

const (
	updateCommand = "update"
)

// Builder defines the common interface that all package builders must implement.
// This unifies the behavior across different package formats (APK, DEB, RPM, Pacman).
type Builder interface {
	// BuildPackage creates the package file at the specified artifacts path
	BuildPackage(artifactsPath string, targetArch string) error

	// PrepareFakeroot sets up the package metadata and prepares the build environment
	PrepareFakeroot(artifactsPath string, targetArch string) error

	// Install installs the built package (requires appropriate package manager)
	Install(artifactsPath string) error

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
	pattern := `(?m)(<|<=|>=|=|>|<)`
	regex := regexp.MustCompile(pattern)
	processed := make([]string, len(depends))

	for index, depend := range depends {
		result := regex.Split(depend, -1)
		if len(result) == 2 {
			name := result[0]
			operator := strings.Trim(depend, result[0]+result[1])
			version := result[1]

			switch bb.Format {
			case "deb":
				processed[index] = fmt.Sprintf("%s (%s %s)", name, operator, version)
			case "rpm":
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
	if bb.PKGBUILD.Epoch != "" && (extension == ".pkg.tar.zst" || extension == ".rpm") {
		name = fmt.Sprintf("%s-%s:%s-%s", bb.PKGBUILD.PkgName, bb.PKGBUILD.Epoch,
			bb.PKGBUILD.PkgVer, bb.PKGBUILD.PkgRel)
	}

	switch extension {
	case ".apk":
		name += "." + bb.PKGBUILD.ArchComputed
	case ".deb":
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
	case "pacman":
		// Pacman skips dot files
		options.SkipDotFiles = true
	case "apk":
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
	case "deb":
		bb.PKGBUILD.PkgRel += bb.PKGBUILD.Codename
	case "rpm":
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
		return "pacman"
	case constants.FormatAPK:
		return "apk"
	default:
		return ""
	}
}

// getExtension returns the file extension for the given format.
func getExtension(format string) string {
	switch format {
	case constants.FormatDEB:
		return ".deb"
	case constants.FormatRPM:
		return ".rpm"
	case constants.FormatPacman:
		return ".pkg.tar.zst"
	case constants.FormatAPK:
		return ".apk"
	default:
		return ""
	}
}

// getUpdateCommand returns the update command for the given format.
func getUpdateCommand(format string) string {
	switch format {
	case constants.FormatDEB:
		return updateCommand
	case constants.FormatRPM:
		return updateCommand
	case constants.FormatPacman:
		return "-Sy"
	case constants.FormatAPK:
		return updateCommand
	default:
		return ""
	}
}

// Install installs the built package using the appropriate package manager.
// This consolidates duplicated Install methods across all builders.
func (bb *BaseBuilder) Install(artifactsPath string) error {
	pkgName := bb.BuildPackageName(getExtension(bb.Format))
	pkgPath := filepath.Join(artifactsPath, pkgName)

	if bb.Format == constants.FormatPacman {
		// Pacman uses special install args for local files
		return shell.Exec(false, "", "pacman", "-U", "--noconfirm", pkgPath)
	}

	installArgs := constants.GetInstallArgs(bb.Format)
	installArgs = append(installArgs, pkgPath)
	useSudo := bb.Format == constants.FormatAPK

	return shell.ExecWithSudo(useSudo, "", getPackageManager(bb.Format), installArgs...)
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
	}

	return bb.PKGBUILD.GetDepends(getPackageManager(bb.Format), installArgs, makeDepends)
}

// PrepareEnvironment sets up the build environment with necessary tools.
// This consolidates duplicated PrepareEnvironment methods across all builders.
func (bb *BaseBuilder) PrepareEnvironment(golang bool, targetArch string) error {
	allArgs := bb.SetupEnvironmentDependencies(golang)

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

		allArgs = append(allArgs, crossDeps...)
	}

	useSudo := bb.Format == constants.FormatAPK

	// Install dependencies first
	err := shell.ExecWithSudo(useSudo, "", getPackageManager(bb.Format), allArgs...)
	if err != nil {
		return err
	}

	// Set up cross-compilation environment after dependencies are installed
	if targetArch != "" && targetArch != bb.PKGBUILD.ArchComputed {
		err = bb.SetupCrossCompilationEnvironment(targetArch)
		if err != nil {
			return fmt.Errorf("failed to setup cross-compilation environment: %w", err)
		}
	}

	return nil
}

// getCrossCompilerDependencies returns cross-compiler dependencies for target architecture
func (bb *BaseBuilder) getCrossCompilerDependencies(targetArch string) []string {
	// Map target architecture to appropriate cross-compiler package names
	crossCompilerMap := map[string]map[string][]string{
		"deb": {
			"aarch64": {"gcc-aarch64-linux-gnu", "g++-aarch64-linux-gnu"},
			"armv7":   {"gcc-arm-linux-gnueabihf", "g++-arm-linux-gnueabihf"},
			"armv6":   {"gcc-arm-linux-gnueabihf", "g++-arm-linux-gnueabihf"},
			"i386":    {"gcc-i686-linux-gnu", "g++-i686-linux-gnu"},
			"ppc64le": {"gcc-powerpc64le-linux-gnu", "g++-powerpc64le-linux-gnu"},
			"s390x":   {"gcc-s390x-linux-gnu", "g++-s390x-linux-gnu"},
		},
		"rpm": {
			"aarch64": {"gcc-aarch64-linux-gnu", "gcc-c++-aarch64-linux-gnu"},
			"armv7":   {"gcc-arm-linux-gnu", "gcc-c++-arm-linux-gnu"},
			"i686":    {"gcc-i686-linux-gnu", "gcc-c++-i686-linux-gnu"},
			"ppc64le": {"gcc-ppc64le-linux-gnu", "gcc-c++-ppc64le-linux-gnu"},
			"s390x":   {"gcc-s390x-linux-gnu", "gcc-c++-s390x-linux-gnu"},
		},
		"apk": {
			"aarch64": {"gcc-aarch64", "musl-aarch64", "musl-dev"},
			"armv7":   {"gcc-armv7", "musl-armv7", "musl-dev"},
			"armv6":   {"gcc-armhf", "musl-armhf", "musl-dev"},
		},
		"pacman": {
			"aarch64": {"aarch64-linux-gnu-gcc", "aarch64-linux-gnu-binutils"},
			"armv7":   {"arm-none-eabi-gcc", "arm-none-eabi-binutils"},
		},
	}

	if archDeps, exists := crossCompilerMap[bb.Format]; exists {
		if deps, exists := archDeps[targetArch]; exists {
			return deps
		}
	}

	return []string{}
}

// Update updates the package manager's package database.
// This consolidates duplicated Update methods across all builders.
func (bb *BaseBuilder) Update() error {
	return bb.PKGBUILD.GetUpdates(getPackageManager(bb.Format), getUpdateCommand(bb.Format))
}

// SetupCcache checks if ccache is available and configures the build environment to use it.
// This function sets up environment variables to enable ccache for faster compilation.
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
//
//nolint:gocyclo,cyclop
//nolint:cyclop
//nolint:gocyclo
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
		return fmt.Errorf("no cross-compilation toolchain available for architecture: %s", targetArch)
	}

	// Determine the distribution-specific toolchain packages
	var distro string

	switch bb.Format {
	case constants.FormatDEB:
		distro = "debian"
	case constants.FormatRPM:
		distro = "fedora"
	case constants.FormatAPK:
		distro = "alpine"
	case constants.FormatPacman:
		distro = "arch"
	default:
		distro = "debian" // fallback
	}

	toolchainPackages, exists := toolchain[distro]
	if !exists {
		// Try to find a toolchain for any distribution
		for _, distroToolchain := range toolchain {
			toolchainPackages = distroToolchain
			break
		}

		if toolchainPackages.GCCPackage == "" {
			return fmt.Errorf("no cross-compilation toolchain available for %s on %s", targetArch, distro)
		}
	}

	// Set up C/C++ cross-compilation environment variables
	_ = os.Setenv("CC", toolchainPackages.GCCPackage)
	_ = os.Setenv("CXX", toolchainPackages.GPlusPlusPackage)

	// Extract binutils prefix for tool names
	// BinutilsPackage is like "aarch64-linux-gnu-binutils", we need "aarch64-linux-gnu-ar", etc.
	binutilsPrefix := strings.TrimSuffix(toolchainPackages.BinutilsPackage, "-binutils")
	if binutilsPrefix == toolchainPackages.BinutilsPackage {
		// If no -binutils suffix, try without suffix (for special cases)
		binutilsPrefix = strings.TrimSuffix(toolchainPackages.BinutilsPackage, "binutils")
	}

	_ = os.Setenv("AR", binutilsPrefix+"-ar")
	_ = os.Setenv("STRIP", binutilsPrefix+"-strip")
	_ = os.Setenv("RANLIB", binutilsPrefix+"-ranlib")
	_ = os.Setenv("OBJDUMP", binutilsPrefix+"-objdump")
	_ = os.Setenv("OBJCOPY", binutilsPrefix+"-objcopy")
	_ = os.Setenv("LD", binutilsPrefix+"-ld")
	_ = os.Setenv("NM", binutilsPrefix+"-nm")

	// Calculate CROSS_COMPILE prefix
	ccPrefix := ""

	if strings.Contains(toolchainPackages.GCCPackage, "-gcc") {
		// Extract prefix before -gcc
		parts := strings.Split(toolchainPackages.GCCPackage, "-gcc")
		if len(parts) > 0 {
			ccPrefix = parts[0] + "-"
		}
	} else if strings.Contains(toolchainPackages.GCCPackage, "gcc") {
		// Extract prefix before gcc
		parts := strings.Split(toolchainPackages.GCCPackage, "gcc")
		if len(parts) > 0 {
			ccPrefix = parts[0] + "-"
		}
	}

	// Set up Rust cross-compilation environment variables
	rustTarget := bb.getRustTargetArchitecture(targetArch)
	if rustTarget != "" {
		_ = os.Setenv("CARGO_BUILD_TARGET", rustTarget)
		_ = os.Setenv("RUSTC_TARGET", rustTarget)
		rustTargetUpper := strings.ToUpper(strings.ReplaceAll(rustTarget, "-", "_"))
		_ = os.Setenv("CARGO_TARGET_"+rustTargetUpper+"_LINKER",
			binutilsPrefix+"-ld")

		// Set CC and CXX for Rust's build script integration
		// Note: GCCPackage and GPlusPlusPackage already contain the full command name
		_ = os.Setenv("TARGET_"+rustTargetUpper+"_CC",
			toolchainPackages.GCCPackage)
		_ = os.Setenv("TARGET_"+rustTargetUpper+"_CXX",
			toolchainPackages.GPlusPlusPackage)

		logger.Info(i18n.T("logger.cross_compilation.rust_cross_compilation_configured"),
			"rust_target", rustTarget,
			"target_arch", targetArch)
	}

	// Set up Go cross-compilation environment variables
	goArch := bb.getGoTargetArchitecture(targetArch)

	goOS := "linux" // Default to Linux for cross-compilation
	if goArch != "" {
		_ = os.Setenv("GOOS", goOS)
		_ = os.Setenv("GOARCH", goArch)

		// Set up CGO for cross-compilation
		// Note: GCCPackage and GPlusPlusPackage already contain the full command name
		_ = os.Setenv("CGO_ENABLED", "1")
		_ = os.Setenv("CC_FOR_TARGET", toolchainPackages.GCCPackage)
		_ = os.Setenv("CXX_FOR_TARGET", toolchainPackages.GPlusPlusPackage)

		logger.Info(i18n.T("logger.cross_compilation.go_cross_compilation_configured"),
			"goos", goOS,
			"goarch", goArch,
			"target_arch", targetArch)
	}

	// Set common cross-compilation variables
	_ = os.Setenv("CROSS_COMPILE", ccPrefix+"-")
	_ = os.Setenv("TARGET_ARCH", targetArch)
	_ = os.Setenv("HOST_ARCH", bb.PKGBUILD.ArchComputed)
	_ = os.Setenv("BUILD_ARCH", bb.PKGBUILD.ArchComputed)

	// Configure pkg-config for cross-compilation
	pkgConfigPath := "/usr/lib/" + ccPrefix + "/pkgconfig:/usr/local/lib/" +
		ccPrefix + "/pkgconfig"
	_ = os.Setenv("PKG_CONFIG_PATH", pkgConfigPath)
	_ = os.Setenv("PKG_CONFIG_LIBDIR", "/usr/lib/"+ccPrefix+"/pkgconfig")

	// Set up autoconf cross-compilation configuration
	// Get the GNU triplets for the build and host architectures
	hostTriplet := bb.getGNUTriplet(targetArch)
	buildTriplet := bb.getGNUTriplet(bb.PKGBUILD.ArchComputed)

	if hostTriplet != "" && buildTriplet != "" {
		// Configure autoconf for cross-compilation
		// These environment variables inform autoconf that we're cross-compiling
		_ = os.Setenv("ac_cv_host", hostTriplet)
		_ = os.Setenv("ac_cv_build", buildTriplet)

		// Create configure wrapper function that can be used in PKGBUILDs
		// This is a bash function that automatically adds --host and --build flags
		configureWrapper := fmt.Sprintf(`
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

		// Set the wrapper in the environment (will be available to the build script)
		_ = os.Setenv("YAP_CONFIGURE_WRAPPER", configureWrapper)

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
	rustTargets := map[string]string{
		"aarch64": "aarch64-unknown-linux-gnu",
		"armv7":   "armv7-unknown-linux-gnueabihf",
		"armv6":   "arm-unknown-linux-gnueabihf",
		"i686":    "i686-unknown-linux-gnu",
		"x86_64":  "x86_64-unknown-linux-gnu",
		"ppc64le": "powerpc64le-unknown-linux-gnu",
		"s390x":   "s390x-unknown-linux-gnu",
		"riscv64": "riscv64gc-unknown-linux-gnu",
	}

	if target, exists := rustTargets[arch]; exists {
		return target
	}

	return ""
}

// getGoTargetArchitecture maps YAP architecture names to Go GOARCH values
func (bb *BaseBuilder) getGoTargetArchitecture(arch string) string {
	goArchs := map[string]string{
		"aarch64": "arm64",
		"armv7":   "arm",
		"armv6":   "arm",
		"i686":    "386",
		"x86_64":  "amd64",
		"ppc64le": "ppc64le",
		"s390x":   "s390x",
		"riscv64": "riscv64",
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
		"aarch64": "aarch64-linux-gnu",
		"armv7":   "arm-linux-gnueabihf",
		"armv6":   "arm-linux-gnueabihf",
		"i686":    "i686-linux-gnu",
		"x86_64":  "x86_64-linux-gnu",
		"ppc64le": "powerpc64le-linux-gnu",
		"s390x":   "s390x-linux-gnu",
		"riscv64": "riscv64-linux-gnu",
	}

	if triplet, exists := gnuTriplets[arch]; exists {
		return triplet
	}

	return ""
}
