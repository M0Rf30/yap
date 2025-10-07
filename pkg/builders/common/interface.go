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
	BuildPackage(artifactsPath string) error

	// PrepareFakeroot sets up the package metadata and prepares the build environment
	PrepareFakeroot(artifactsPath string) error

	// Install installs the built package (requires appropriate package manager)
	Install(artifactsPath string) error

	// Prepare installs build dependencies and prepares the build environment
	Prepare(makeDepends []string) error

	// PrepareEnvironment sets up the build environment with necessary tools
	PrepareEnvironment(golang bool) error

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
func (bb *BaseBuilder) Prepare(makeDepends []string) error {
	installArgs := constants.GetInstallArgs(bb.Format)
	return bb.PKGBUILD.GetDepends(getPackageManager(bb.Format), installArgs, makeDepends)
}

// PrepareEnvironment sets up the build environment with necessary tools.
// This consolidates duplicated PrepareEnvironment methods across all builders.
func (bb *BaseBuilder) PrepareEnvironment(golang bool) error {
	allArgs := bb.SetupEnvironmentDependencies(golang)
	useSudo := bb.Format == constants.FormatAPK

	return shell.ExecWithSudo(useSudo, "", getPackageManager(bb.Format), allArgs...)
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
		err = os.MkdirAll(ccacheDir, 0o755)
		if err != nil {
			logger.Warn(i18n.T("logger.setupccache.warn.failed_to_create_ccache_dir_1"),
				"dir", ccacheDir, "error", err)
		}
	}

	_ = os.Setenv("CCACHE_DIR", ccacheDir)

	return nil
}
