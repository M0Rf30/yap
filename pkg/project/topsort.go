// Package project provides multi-package project management and build orchestration.
package project

import (
	"errors"
	"fmt"
	"runtime"
	"sort"

	yerrors "github.com/M0Rf30/yap/v2/pkg/errors"
	"github.com/M0Rf30/yap/v2/pkg/i18n"
	"github.com/M0Rf30/yap/v2/pkg/logger"
)

// topologicalSort performs Kahn's algorithm to sort projects by dependencies.
// Returns batches of projects that can be built in parallel within each batch.
// Fundamental packages (those depended on by many others) are prioritized within each batch.
func (mpc *MultipleProject) topologicalSort(projectMap map[string]*Project,
	dependsOn map[string][]string, dependedBy map[string][]string,
) ([][]*Project, error) {
	logger.Info(i18n.T("logger.performing_topological_sort_for_build_order"))

	// Calculate dependency popularity to identify fundamental packages
	popularity := mpc.calculateDependencyPopularity()

	logger.Debug(i18n.T("logger.dependency_popularity_analysis") + ":")

	for pkgName, count := range popularity {
		if count > 0 {
			logger.Debug(i18n.T("logger.dependency_popularity"),
				"package", pkgName,
				"dependent_count", count)
		}
	}

	var result [][]*Project

	inDegree := make(map[string]int)

	// Calculate in-degrees (number of dependencies)
	for pkgName := range projectMap {
		inDegree[pkgName] = len(dependsOn[pkgName])
	}

	batchNum := 1
	// Process in batches (packages with same dependency level can be built in parallel)
	for len(inDegree) > 0 {
		var currentBatch []*Project

		var toRemove []string

		var batchPackages []string

		// Find all packages with no dependencies (in-degree = 0)
		var candidatePackages []string

		for pkgName, degree := range inDegree {
			if degree == 0 {
				candidatePackages = append(candidatePackages, pkgName)
			}
		}

		// Sort candidates by popularity (fundamental packages first)
		sort.Slice(candidatePackages, func(i, j int) bool {
			return popularity[candidatePackages[i]] > popularity[candidatePackages[j]]
		})

		// Build current batch with sorted packages
		for _, pkgName := range candidatePackages {
			currentBatch = append(currentBatch, projectMap[pkgName])
			toRemove = append(toRemove, pkgName)
			batchPackages = append(batchPackages, fmt.Sprintf("%s(deps:%d)", pkgName, popularity[pkgName]))
		}

		if len(currentBatch) == 0 {
			// Log the problematic packages for debugging
			var problematicPackages []string
			for pkgName, degree := range inDegree {
				problematicPackages = append(problematicPackages, fmt.Sprintf("%s(%d)", pkgName, degree))
			}

			logger.Error(i18n.T("logger.circular_dependency_detected"),
				"remaining_packages", problematicPackages)

			//nolint:err113 // sentinel error chain required for errors.Is matching
			return nil, yerrors.Wrap(ErrCircularDependency, yerrors.ErrTypeInternal, "circular dependency detected").
				WithOperation("topologicalSort").
				WithContext("problematic_packages", problematicPackages)
		}

		logger.Info(i18n.T("logger.build_batch_determined"),
			"batch_number", batchNum,
			"batch_size", len(currentBatch),
			"packages", batchPackages,
			"parallel_workers", min(runtime.NumCPU(), len(currentBatch)))

		result = append(result, currentBatch)

		// Remove processed packages and update in-degrees
		for _, pkgName := range toRemove {
			delete(inDegree, pkgName)

			// Decrease in-degree for dependent packages
			for _, dependent := range dependedBy[pkgName] {
				if _, exists := inDegree[dependent]; exists {
					inDegree[dependent]--
				}
			}
		}

		batchNum++
	}

	logger.Info(i18n.T("logger.build_order_determined"),
		"total_batches", len(result),
		"total_packages", len(projectMap))

	return result, nil
}

// topologicalSortRuntimeDeps performs topological sort specifically for runtime dependencies.
// It delegates to topologicalSort and translates ErrCircularDependency into
// ErrCircularRuntimeDependency so callers can distinguish the two error kinds.
func (mpc *MultipleProject) topologicalSortRuntimeDeps(projectMap map[string]*Project,
	dependsOn map[string][]string, dependedBy map[string][]string,
) ([][]*Project, error) {
	result, err := mpc.topologicalSort(projectMap, dependsOn, dependedBy)
	if err != nil {
		// Translate the generic circular-dependency error into the runtime-specific one
		// so that callers (and tests) can distinguish build-order cycles from
		// runtime-dependency cycles via errors.Is / errors.As.
		if errors.Is(err, ErrCircularDependency) {
			//nolint:err113 // sentinel error chain required for errors.Is matching
			return nil, yerrors.Wrap(ErrCircularRuntimeDependency, yerrors.ErrTypeInternal, "circular runtime dependency detected"). //nolint:lll
																			WithOperation("topologicalSortRuntimeDeps")
		}

		return nil, err
	}

	return result, nil
}

// resolveDependencies builds a dependency graph and returns projects in topologically sorted order.
// Returns slices of project batches that can be built in parallel within each batch.
// Only considers dependencies that exist within the provided project set.
func (mpc *MultipleProject) resolveDependencies(projects []*Project) ([][]*Project, error) {
	logger.Info(i18n.T("logger.analyzing_package_dependencies"))

	// Build dependency graph
	projectMap := mpc.buildPackageMap(projects)
	dependsOn := make(map[string][]string)
	dependedBy := make(map[string][]string)

	// Index projects by package name - only for projects in range
	for _, proj := range projects {
		pkgName := proj.Builder.PKGBUILD.PkgName
		dependsOn[pkgName] = nil
		dependedBy[pkgName] = nil
	}

	logger.Info(
		i18n.T("logger.project.building_dependency_graph"),
		"total_packages", len(projectMap))

	// Build dependency relationships - only within filtered projects
	totalDeps := 0

	for _, proj := range projects {
		pkgName := proj.Builder.PKGBUILD.PkgName

		var packageDeps []string

		// Check runtime dependencies
		processDependencies(proj.Builder.PKGBUILD.Depends, projectMap, func(depName string) {
			dependsOn[pkgName] = append(dependsOn[pkgName], depName)
			dependedBy[depName] = append(dependedBy[depName], pkgName)
			packageDeps = append(packageDeps, depName+" (runtime)")
			totalDeps++
		})

		// Check make dependencies (build-time dependencies)
		processDependencies(proj.Builder.PKGBUILD.MakeDepends, projectMap, func(depName string) {
			dependsOn[pkgName] = append(dependsOn[pkgName], depName)
			dependedBy[depName] = append(dependedBy[depName], pkgName)
			packageDeps = append(packageDeps, depName+" (make)")
			totalDeps++
		})

		if len(packageDeps) > 0 {
			logger.Info(i18n.T("logger.package_dependencies_found"),
				"package", pkgName,
				"depends_on", packageDeps)
		}
	}

	logger.Info(i18n.T("logger.dependency_analysis_complete"),
		"total_internal_dependencies", totalDeps)

	// Perform topological sort using Kahn's algorithm
	return mpc.topologicalSort(projectMap, dependsOn, dependedBy)
}

// calculateDependencyPopularity returns a map of package names to how many other packages
// depend on them.
// This helps identify "fundamental" packages that should be built first.
func (mpc *MultipleProject) calculateDependencyPopularity() map[string]int {
	popularity := make(map[string]int)
	packageMap := mpc.buildPackageMap()

	// Initialize popularity for all packages
	for _, proj := range mpc.Projects {
		popularity[proj.Builder.PKGBUILD.PkgName] = 0
	}

	// Count how many packages depend on each package (both runtime and make dependencies)
	for _, proj := range mpc.Projects {
		// Count runtime dependencies
		processDependencies(proj.Builder.PKGBUILD.Depends, packageMap, func(depName string) {
			popularity[depName]++
		})
		// Count make dependencies
		processDependencies(proj.Builder.PKGBUILD.MakeDepends, packageMap, func(depName string) {
			popularity[depName]++
		})
	}

	return popularity
}

// buildRuntimeDependencyMap creates a map of packages that are runtime dependencies
// of other packages.
func (mpc *MultipleProject) buildRuntimeDependencyMap() map[string]bool {
	dependencyMap := make(map[string]bool)
	packageMap := mpc.buildPackageMap()

	// Check which packages are runtime dependencies of others
	for _, proj := range mpc.Projects {
		if len(proj.Builder.PKGBUILD.Depends) > 0 {
			processDependencies(proj.Builder.PKGBUILD.Depends, packageMap, func(depName string) {
				dependencyMap[depName] = true
			})
		}
	}

	return dependencyMap
}

// buildRuntimeDependencyGraph creates dependency maps for runtime dependencies.
func (mpc *MultipleProject) buildRuntimeDependencyGraph(runtimeDeps []*Project) (
	projectMap map[string]*Project, dependsOn map[string][]string, dependedBy map[string][]string,
) {
	runtimeProjectMap := make(map[string]*Project)
	runtimeDependsOn := make(map[string][]string)
	runtimeDependedBy := make(map[string][]string)
	// Index runtime dependency projects
	for _, proj := range runtimeDeps {
		pkgName := proj.Builder.PKGBUILD.PkgName
		runtimeProjectMap[pkgName] = proj
		runtimeDependsOn[pkgName] = nil
		runtimeDependedBy[pkgName] = nil
	}

	// Build dependency relationships between runtime dependencies only
	for _, proj := range runtimeDeps {
		pkgName := proj.Builder.PKGBUILD.PkgName
		mpc.addRuntimeDependencies(proj, pkgName, runtimeProjectMap, runtimeDependsOn, runtimeDependedBy)
	}

	return runtimeProjectMap, runtimeDependsOn, runtimeDependedBy
}

// addRuntimeDependencies adds dependency relationships for a single project.
func (mpc *MultipleProject) addRuntimeDependencies(proj *Project, pkgName string,
	runtimeProjectMap map[string]*Project, runtimeDependsOn, runtimeDependedBy map[string][]string,
) {
	// Check runtime dependencies
	for _, dep := range proj.Builder.PKGBUILD.Depends {
		depName := extractPackageName(dep)
		if _, exists := runtimeProjectMap[depName]; exists {
			runtimeDependsOn[pkgName] = append(runtimeDependsOn[pkgName], depName)
			runtimeDependedBy[depName] = append(runtimeDependedBy[depName], pkgName)
			logger.Debug(
				i18n.T("logger.project.runtime_dependency_found"),
				"dependent", pkgName,
				"dependency", depName)
		}
	}

	// Check make dependencies
	for _, dep := range proj.Builder.PKGBUILD.MakeDepends {
		depName := extractPackageName(dep)
		if _, exists := runtimeProjectMap[depName]; exists {
			runtimeDependsOn[pkgName] = append(runtimeDependsOn[pkgName], depName)
			runtimeDependedBy[depName] = append(runtimeDependedBy[depName], pkgName)
			logger.Debug(
				i18n.T("logger.project.make_dependency_found"),
				"dependent", pkgName,
				"dependency", depName)
		}
	}
}

// buildPackageMap returns a map from package name to project for the given projects.
// If projects is nil, uses mpc.Projects. The map is freshly allocated on each call.
func (mpc *MultipleProject) buildPackageMap(projects ...[]*Project) map[string]*Project {
	var source []*Project
	if len(projects) > 0 && projects[0] != nil {
		source = projects[0]
	} else {
		source = mpc.Projects
	}

	packageMap := make(map[string]*Project, len(source))
	for _, proj := range source {
		packageMap[proj.Builder.PKGBUILD.PkgName] = proj
	}

	return packageMap
}
