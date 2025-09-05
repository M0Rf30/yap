// Package common provides shared interfaces and base implementations for package builders.
package common

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/M0Rf30/yap/v2/pkg/constants"
	"github.com/M0Rf30/yap/v2/pkg/files"
	"github.com/M0Rf30/yap/v2/pkg/i18n"
	"github.com/M0Rf30/yap/v2/pkg/logger"
	"github.com/M0Rf30/yap/v2/pkg/pkgbuild"
	"github.com/M0Rf30/yap/v2/pkg/platform"
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

// BuilderConstructor defines the signature for builder constructor functions.
type BuilderConstructor func(*pkgbuild.PKGBUILD) Builder

// builderRegistry holds registered builder constructors
var builderRegistry = make(map[string]BuilderConstructor)

// RegisterBuilder registers a builder constructor for a specific format.
func RegisterBuilder(format string, constructor BuilderConstructor) {
	builderRegistry[format] = constructor
}

// NewBuilder creates a new builder for the specified package format.
// This provides a unified factory interface for all package builders.
func NewBuilder(pkgBuild *pkgbuild.PKGBUILD, format string) Builder {
	constructor, exists := builderRegistry[format]
	if !exists {
		logger.Fatal(i18n.T("errors.builder.unsupported_format"), "format", format)
		return nil
	}

	return constructor(pkgBuild)
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
