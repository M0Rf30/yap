// Package core provides unified package building interfaces and common functionality.
package core

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/M0Rf30/yap/v2/pkg/constants"
	"github.com/M0Rf30/yap/v2/pkg/osutils"
	"github.com/M0Rf30/yap/v2/pkg/pkgbuild"
)

// Packager is the common interface implemented by all package managers.
type Packager interface {
	// BuildPackage starts the package building process and writes the final artifact
	// to the specified output path. It returns an error if any issues occur during the build.
	BuildPackage(output string) error
	// Install reads the path where the final artifact will be written. It returns
	// any error if encountered.
	Install(output string) error
	// Prepare appends the dependencies required to build all the projects. It
	// returns any error if encountered.
	Prepare(depends []string) error
	// PrepareEnvironment reads a flag to install golang tools on request, on the
	// build machine. It returns any error if encountered.
	PrepareEnvironment(flag bool) error
	// PrepareFakeroot sets up the environment for building the final artifact in a fakeroot context.
	// It takes an output path where the artifact will be written and returns an error if any issues
	// occur.
	PrepareFakeroot(output string) error
	// Update performs a package manager update operation. It returns any error if
	// encountered.
	Update() error
}

// BasePackageManager provides common functionality for all package managers.
type BasePackageManager struct {
	PKGBUILD *pkgbuild.PKGBUILD
	Config   *Config
}

// NewBasePackageManager creates a new base package manager with common functionality.
func NewBasePackageManager(pkgBuild *pkgbuild.PKGBUILD, config *Config) *BasePackageManager {
	return &BasePackageManager{
		PKGBUILD: pkgBuild,
		Config:   config,
	}
}

// BuildPackageName constructs standardized package names based on format.
func (bpm *BasePackageManager) BuildPackageName(format string) string {
	switch format {
	case constants.FormatAPK:
		return fmt.Sprintf("%s-%s-r%s.%s.apk",
			bpm.PKGBUILD.PkgName,
			bpm.PKGBUILD.PkgVer,
			bpm.PKGBUILD.PkgRel,
			bpm.PKGBUILD.ArchComputed)
	case constants.FormatDEB:
		return fmt.Sprintf("%s_%s-%s_%s.deb",
			bpm.PKGBUILD.PkgName,
			bpm.PKGBUILD.PkgVer,
			bpm.PKGBUILD.PkgRel,
			bpm.PKGBUILD.ArchComputed)
	case constants.FormatRPM:
		name := fmt.Sprintf("%s-%s-%s.%s.rpm",
			bpm.PKGBUILD.PkgName,
			bpm.PKGBUILD.PkgVer,
			bpm.PKGBUILD.PkgRel,
			bpm.PKGBUILD.ArchComputed)
		if bpm.PKGBUILD.Epoch != "" {
			name = fmt.Sprintf("%s-%s:%s-%s.%s.rpm",
				bpm.PKGBUILD.PkgName,
				bpm.PKGBUILD.Epoch,
				bpm.PKGBUILD.PkgVer,
				bpm.PKGBUILD.PkgRel,
				bpm.PKGBUILD.ArchComputed)
		}

		return name
	case constants.FormatPacman:
		name := fmt.Sprintf("%s-%s-%s-%s.pkg.tar.zst",
			bpm.PKGBUILD.PkgName,
			bpm.PKGBUILD.PkgVer,
			bpm.PKGBUILD.PkgRel,
			bpm.PKGBUILD.ArchComputed)
		if bpm.PKGBUILD.Epoch != "" {
			name = fmt.Sprintf("%s-%s:%s-%s-%s.pkg.tar.zst",
				bpm.PKGBUILD.PkgName,
				bpm.PKGBUILD.Epoch,
				bpm.PKGBUILD.PkgVer,
				bpm.PKGBUILD.PkgRel,
				bpm.PKGBUILD.ArchComputed)
		}

		return name
	default:
		return fmt.Sprintf("%s-%s-%s-%s",
			bpm.PKGBUILD.PkgName,
			bpm.PKGBUILD.PkgVer,
			bpm.PKGBUILD.PkgRel,
			bpm.PKGBUILD.ArchComputed)
	}
}

// PrepareCommon handles common preparation tasks.
func (bpm *BasePackageManager) PrepareCommon(makeDepends []string) error {
	return bpm.PKGBUILD.GetDepends(bpm.Config.InstallCmd, bpm.Config.InstallArgs, makeDepends)
}

// PrepareEnvironmentCommon handles common environment preparation.
func (bpm *BasePackageManager) PrepareEnvironmentCommon(golang bool) error {
	args := make([]string, len(bpm.Config.InstallArgs)+len(bpm.Config.BuildEnvDeps))
	copy(args, bpm.Config.InstallArgs)
	copy(args[len(bpm.Config.InstallArgs):], bpm.Config.BuildEnvDeps)

	if golang {
		if bpm.Config.Name == constants.FormatAPK {
			osutils.CheckGO()

			args = append(args, "go")
		} else {
			err := osutils.GOSetup()
			if err != nil {
				return err
			}
		}
	}

	useSudo := bpm.Config.Name != constants.FormatAPK

	return osutils.Exec(useSudo, "", bpm.Config.InstallCmd, args...)
}

// UpdateCommon handles common update operations.
func (bpm *BasePackageManager) UpdateCommon() error {
	if len(bpm.Config.UpdateArgs) == 0 {
		return nil // Some package managers don't need explicit update
	}

	return bpm.PKGBUILD.GetUpdates(bpm.Config.InstallCmd, bpm.Config.UpdateArgs...)
}

// InstallCommon handles common installation tasks.
func (bpm *BasePackageManager) InstallCommon(artifactsPath, packageName string) error {
	pkgFilePath := filepath.Join(artifactsPath, packageName)
	args := make([]string, len(bpm.Config.InstallArgs)+1)
	copy(args, bpm.Config.InstallArgs)
	args[len(bpm.Config.InstallArgs)] = pkgFilePath

	useSudo := bpm.Config.Name != constants.FormatAPK

	return osutils.Exec(useSudo, "", bpm.Config.InstallCmd, args...)
}

// LogPackageCreated logs successful package creation.
func (bpm *BasePackageManager) LogPackageCreated(artifactPath string) {
	pkgLogger := osutils.WithComponent(bpm.PKGBUILD.PkgName)
	pkgLogger.Info("package artifact created", osutils.Logger.Args(
		"pkgver", bpm.PKGBUILD.PkgVer,
		"pkgrel", bpm.PKGBUILD.PkgRel,
		"artifact", artifactPath))
}

// ValidateArtifactsPath ensures the artifacts path exists.
func (bpm *BasePackageManager) ValidateArtifactsPath(artifactsPath string) error {
	if _, err := os.Stat(artifactsPath); os.IsNotExist(err) {
		return fmt.Errorf("artifacts path does not exist: %s", artifactsPath)
	}

	return nil
}

// SetComputedFields sets commonly computed fields based on configuration.
func (bpm *BasePackageManager) SetComputedFields() {
	if archMapping, exists := bpm.Config.ArchMap[bpm.PKGBUILD.ArchComputed]; exists {
		bpm.PKGBUILD.ArchComputed = archMapping
	}

	if groupMapping, exists := bpm.Config.GroupMap[bpm.PKGBUILD.Section]; exists {
		bpm.PKGBUILD.Section = groupMapping
	}
}

// SetInstalledSize calculates and sets the installed size.
func (bpm *BasePackageManager) SetInstalledSize() error {
	size, err := osutils.GetDirSize(bpm.PKGBUILD.PackageDir)
	if err != nil {
		return err
	}

	bpm.PKGBUILD.InstalledSize = size / 1024 // Convert to KB

	return nil
}
