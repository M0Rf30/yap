// Package project provides multi-package project management and build orchestration.
package project

import (
	"github.com/M0Rf30/yap/v2/pkg/i18n"
	"github.com/M0Rf30/yap/v2/pkg/logger"
)

// displayVerboseDependencyInfo shows detailed dependency information for all projects.
func (mpc *MultipleProject) displayVerboseDependencyInfo() {
	logger.Debug(i18n.T("logger.dependency_analysis_starting"))

	// Create dependency map for internal packages
	packageMap := mpc.buildPackageMap()

	// Build runtime dependency map to determine which packages should be installed
	runtimeDependencyMap := mpc.buildRuntimeDependencyMap()

	// Display detailed dependency information
	for _, proj := range mpc.Projects {
		mpc.displayPackageDependencyInfo(proj, packageMap, runtimeDependencyMap)
	}

	logger.Debug(i18n.T("logger.dependency_analysis_complete"))
}

// displayPackageDependencyInfo shows dependency information for a specific package.
func (mpc *MultipleProject) displayPackageDependencyInfo(
	proj *Project, packageMap map[string]*Project, runtimeDependencyMap map[string]bool,
) {
	pkgName := proj.Builder.PKGBUILD.PkgName
	pkgVer := proj.Builder.PKGBUILD.PkgVer
	pkgRel := proj.Builder.PKGBUILD.PkgRel

	logger.Debug(i18n.T("logger.package_information"),
		"package", pkgName,
		"version", pkgVer,
		"release", pkgRel)

	// Show runtime dependencies
	if len(proj.Builder.PKGBUILD.Depends) > 0 {
		mpc.displayDependencies("Runtime Dependencies", proj.Builder.PKGBUILD.Depends, packageMap)
	}

	// Show make dependencies
	if len(proj.Builder.PKGBUILD.MakeDepends) > 0 {
		mpc.displayDependencies("Build Dependencies", proj.Builder.PKGBUILD.MakeDepends, packageMap)
	}

	// Show installation flag
	shouldInstall := proj.HasToInstall || runtimeDependencyMap[pkgName]

	if shouldInstall {
		var installType string
		if proj.HasToInstall {
			installType = "explicit"
		} else {
			installType = "runtime_dependency"
		}

		logger.Debug(i18n.T("logger.package_installation_planned"),
			"package", pkgName,
			"install_type", installType,
			"action", "install_after_build")
	} else {
		logger.Debug(i18n.T("logger.package_installation_planned"),
			"package", pkgName,
			"action", "build_only")
	}
}

// displayDependencies is a helper function to display dependency information.
func (mpc *MultipleProject) displayDependencies(
	title string, deps []string, packageMap map[string]*Project) {
	logger.Debug(i18n.T("logger.dependency_category"),
		"category", title)

	var internalDeps, externalDeps []string

	for _, dep := range deps {
		depName := extractPackageName(dep)
		if _, exists := packageMap[depName]; exists {
			internalDeps = append(internalDeps, dep+" (internal)")
		} else {
			externalDeps = append(externalDeps, dep+" (external)")
		}
	}

	// Display internal dependencies
	for _, dep := range internalDeps {
		logger.Debug(i18n.T("logger.internal_dependency"),
			"dependency", dep,
			"type", "internal")
	}

	// Display external dependencies
	for _, dep := range externalDeps {
		logger.Debug(i18n.T("logger.external_dependency"),
			"dependency", dep,
			"type", "external")
	}
}
