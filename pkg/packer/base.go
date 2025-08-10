// Package packer provides unified package building interface and common functionality.
package packer

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/M0Rf30/yap/v2/pkg/osutils"
	"github.com/M0Rf30/yap/v2/pkg/pkgbuild"
)

const (
	// PackageManagerAPK represents the APK package manager name.
	PackageManagerAPK = "apk"
)

// PackageManagerConfig holds common configuration for all package managers.
type PackageManagerConfig struct {
	Name         string
	InstallCmd   string
	InstallArgs  []string
	UpdateArgs   []string
	ArchMap      map[string]string
	DistroMap    map[string]string
	GroupMap     map[string]string
	BuildEnvDeps []string
}

// BasePackageManager provides common functionality for all package managers.
type BasePackageManager struct {
	PKGBUILD *pkgbuild.PKGBUILD
	Config   *PackageManagerConfig
}

// NewBasePackageManager creates a new base package manager with common functionality.
func NewBasePackageManager(
	pkgBuild *pkgbuild.PKGBUILD,
	config *PackageManagerConfig,
) *BasePackageManager {
	return &BasePackageManager{
		PKGBUILD: pkgBuild,
		Config:   config,
	}
}

// ProcessDepends processes dependency strings with version operators.
// This consolidates the duplicated dependency processing logic across dpkg and rpm.
func (bpm *BasePackageManager) ProcessDepends(depends []string, format string) []string {
	pattern := `(?m)(<|<=|>=|=|>|<)`
	regex := regexp.MustCompile(pattern)
	processed := make([]string, len(depends))

	for index, depend := range depends {
		result := regex.Split(depend, -1)
		if len(result) == 2 {
			name := result[0]
			operator := strings.Trim(depend, result[0]+result[1])
			version := result[1]

			switch format {
			case "deb":
				processed[index] = name + " (" + operator + " " + version + ")"
			case "rpm":
				processed[index] = name + " " + operator + " " + version
			default:
				processed[index] = depend
			}
		} else {
			processed[index] = depend
		}
	}

	return processed
}

// BuildPackageName constructs standardized package names.
func (bpm *BasePackageManager) BuildPackageName(extension string) string {
	name := bpm.PKGBUILD.PkgName + "-" + bpm.PKGBUILD.PkgVer + "-" + bpm.PKGBUILD.PkgRel

	// Handle epoch for certain package types
	if bpm.PKGBUILD.Epoch != "" && (extension == ".pkg.tar.zst" || extension == ".rpm") {
		name = bpm.PKGBUILD.PkgName + "-" + bpm.PKGBUILD.Epoch + ":" +
			bpm.PKGBUILD.PkgVer + "-" + bpm.PKGBUILD.PkgRel
	}

	switch extension {
	case ".apk":
		name += "." + bpm.PKGBUILD.ArchComputed
	case ".deb":
		name = bpm.PKGBUILD.PkgName + "_" + bpm.PKGBUILD.PkgVer + "-" +
			bpm.PKGBUILD.PkgRel + "_" + bpm.PKGBUILD.ArchComputed
	default:
		name += "-" + bpm.PKGBUILD.ArchComputed
	}

	return name + extension
}

// PrepareCommon handles common preparation tasks.
func (bpm *BasePackageManager) PrepareCommon(makeDepends []string) error {
	return bpm.PKGBUILD.GetDepends(bpm.Config.InstallCmd, bpm.Config.InstallArgs, makeDepends)
}

// PrepareEnvironmentCommon handles common environment preparation.
func (bpm *BasePackageManager) PrepareEnvironmentCommon(golang bool) error {
	newArgs := make([]string, len(bpm.Config.InstallArgs)+len(bpm.Config.BuildEnvDeps))
	copy(newArgs, bpm.Config.InstallArgs)
	copy(newArgs[len(bpm.Config.InstallArgs):], bpm.Config.BuildEnvDeps)

	if golang {
		if bpm.Config.Name == PackageManagerAPK {
			osutils.CheckGO()

			newArgs = append(newArgs, "go")
		} else {
			err := osutils.GOSetup()
			if err != nil {
				return err
			}
		}
	}

	return osutils.Exec(bpm.Config.Name == PackageManagerAPK, "", bpm.Config.InstallCmd, newArgs...)
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
	newArgs := make([]string, len(bpm.Config.InstallArgs)+1)
	copy(newArgs, bpm.Config.InstallArgs)
	newArgs[len(bpm.Config.InstallArgs)] = pkgFilePath

	useSudo := bpm.Config.Name != PackageManagerAPK

	return osutils.Exec(useSudo, "", bpm.Config.InstallCmd, newArgs...)
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

// SetComputedFields sets commonly computed fields.
func (bpm *BasePackageManager) SetComputedFields() {
	if archMapping, exists := bpm.Config.ArchMap[bpm.PKGBUILD.ArchComputed]; exists {
		bpm.PKGBUILD.ArchComputed = archMapping
	}

	if groupMapping, exists := bpm.Config.GroupMap[bpm.PKGBUILD.Section]; exists {
		bpm.PKGBUILD.Section = groupMapping
	}
}
