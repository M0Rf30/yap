// Package project provides multi-package project management and build orchestration.
package project

import (
	"context"
	"errors"
	"fmt"
	"runtime"
	"sort"
	"sync"

	yerrors "github.com/M0Rf30/yap/v2/pkg/errors"
	"github.com/M0Rf30/yap/v2/pkg/i18n"
	"github.com/M0Rf30/yap/v2/pkg/logger"
)

// buildProjectsParallel builds multiple projects in parallel for better performance.
// If shouldInstall is true, each package is installed immediately after building
// (Arch Linux style), making it available for other packages building in parallel.
// Uses context cancellation to stop all workers when an error occurs.
func (mpc *MultipleProject) buildProjectsParallel(projects []*Project, maxWorkers int,
	shouldInstall bool) error {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	projectChan := make(chan *Project, len(projects))
	errorChan := make(chan error, len(projects))

	var waitGroup sync.WaitGroup

	// Start workers
	for workerNum := range maxWorkers {
		waitGroup.Add(1)

		go func(workerID int) {
			defer waitGroup.Done()

			workerIDStr := fmt.Sprintf("worker-%d", workerID)

			for proj := range projectChan {
				pkgName := proj.Builder.PKGBUILD.PkgName
				mpc.runWorker(ctx, cancel, proj, pkgName, workerIDStr, shouldInstall, errorChan)
			}
		}(workerNum)
	}

	// Send projects to workers (drain channel on cancel)
	go func() {
		defer close(projectChan)

		for _, proj := range projects {
			select {
			case <-ctx.Done():
				return
			case projectChan <- proj:
			}
		}
	}()

	// Wait for completion
	go func() {
		waitGroup.Wait()
		close(errorChan)
	}()

	// Check for errors
	var errs []error

	for err := range errorChan {
		if err != nil {
			if len(errs) == 0 {
				cancel()
			}

			errs = append(errs, err)
		}
	}

	if len(errs) > 0 {
		return errors.Join(errs...)
	}

	return nil
}

// buildProjectsSequential builds projects one at a time in the order they appear
// in the project list (file order from yap.json). If a project has HasToInstall set,
// it is installed immediately after building, before the next project is processed.
// This is the default (v1-compatible) build mode.
func (mpc *MultipleProject) buildProjectsSequential(ctx context.Context, projects []*Project) error {
	for _, proj := range projects {
		pkgName := proj.Builder.PKGBUILD.PkgName

		logger.Debug(i18n.T("logger.creating_package"),
			"package", pkgName,
			"version", proj.Builder.PKGBUILD.PkgVer,
			"release", proj.Builder.PKGBUILD.PkgRel)

		if err := proj.Builder.Compile(ctx, mpc.Opts.NoBuild); err != nil {
			return err
		}

		if !mpc.Opts.NoBuild {
			if err := mpc.createPackage(proj); err != nil {
				return err
			}

			if proj.HasToInstall {
				if err := mpc.installPackage(proj); err != nil {
					return err
				}
			}
		}

		// ToPkgName is also enforced by getProjectsInRange, but we check here to log
		// the stopping event, consistent with other build loop implementations.
		if mpc.Opts.ToPkgName != "" && pkgName == mpc.Opts.ToPkgName {
			logger.Info(i18n.T("logger.stopping_build_at_target_package"),
				"target_package", mpc.Opts.ToPkgName)

			return nil
		}
	}

	return nil
}

// runWorker executes the build pipeline for a single project in a worker goroutine.
// It handles compilation, package creation, and optional installation.
// Returns early if context is cancelled or if ToPkgName is reached.
func (mpc *MultipleProject) runWorker(ctx context.Context, cancel context.CancelFunc,
	proj *Project, pkgName, workerIDStr string, shouldInstall bool, errorChan chan<- error) {
	// Check if cancelled before starting work
	select {
	case <-ctx.Done():
		return
	default:
	}

	logger.Debug(i18n.T("logger.creating_package"),
		"package", pkgName,
		"version", proj.Builder.PKGBUILD.PkgVer,
		"release", proj.Builder.PKGBUILD.PkgRel,
		"worker_id", workerIDStr)

	// Step 1: Build the package
	err := proj.Builder.Compile(ctx, mpc.Opts.NoBuild)
	if err != nil {
		cancel()

		errorChan <- err

		return
	}

	if !mpc.Opts.NoBuild {
		// Step 2: Create the package file
		err := mpc.createPackage(proj)
		if err != nil {
			cancel()

			errorChan <- err

			return
		}

		// Step 3: Install immediately (Arch Linux style) or extract for cross-compilation
		if shouldInstall {
			if err := mpc.installPackageForWorker(proj, pkgName, workerIDStr); err != nil {
				cancel()

				errorChan <- err

				return
			}
		}
	}

	if mpc.Opts.ToPkgName != "" && pkgName == mpc.Opts.ToPkgName {
		return
	}
}

// buildProjectsInOrder builds projects in dependency-aware batches with parallel processing
// within each batch.
// The topological sort already ensures runtime dependencies are built before dependents.
func (mpc *MultipleProject) buildProjectsInOrder(buildOrder [][]*Project, maxWorkers int) error {
	totalPackages := 0
	for _, batch := range buildOrder {
		totalPackages += len(batch)
	}

	// Build runtime dependency map to identify packages that are needed by others
	runtimeDependencyMap := mpc.buildRuntimeDependencyMap()

	logger.Info(i18n.T("logger.dependency_aware_build_process_starting"))
	logger.Info(i18n.T("logger.project.runtime_dependency_map"))

	for pkgName, isRuntimeDep := range runtimeDependencyMap {
		if isRuntimeDep {
			logger.Info(i18n.T("logger.project.runtime_dependency_detected"),
				"package", pkgName,
				"action", "will_be_installed")
		}
	}

	logger.Info(i18n.T("logger.project.starting_dependencyaware_build_process"),
		"total_batches", len(buildOrder),
		"total_packages", totalPackages,
		"max_parallel_workers", maxWorkers)

	processedPackages := 0

	for batchIndex, batch := range buildOrder {
		batchWorkers := min(maxWorkers, len(batch))

		var batchPackages []string
		for _, proj := range batch {
			batchPackages = append(batchPackages, proj.Builder.PKGBUILD.PkgName)
		}

		logger.Debug(i18n.T("logger.processing_build_batch"),
			"batch_number", batchIndex+1,
			"batch_size", len(batch),
			"packages", batchPackages,
			"parallel_workers", batchWorkers,
			"progress", fmt.Sprintf("%d/%d packages", processedPackages, totalPackages))

		// Build current batch in parallel - but handle runtime dependencies specially
		err := mpc.buildBatchWithDependencyInstall(
			batch,
			batchWorkers,
			runtimeDependencyMap,
			batchIndex+1)
		if err != nil {
			return err
		}

		processedPackages += len(batch)
	}

	return nil
}

// buildBatchWithDependencyInstall builds a batch of projects with Arch Linux-style
// dependency handling: packages are installed immediately after building, making them
// available for subsequent packages in the same batch.
// This ensures that runtime dependencies (depends) are available during the build phase,
// matching the behavior of Arch Linux's makepkg.
func (mpc *MultipleProject) buildBatchWithDependencyInstall(projects []*Project, maxWorkers int,
	runtimeDependencyMap map[string]bool, batchNumber int,
) error {
	// Separate runtime dependencies from regular packages
	var runtimeDeps, regularPackages []*Project

	for _, proj := range projects {
		if runtimeDependencyMap[proj.Builder.PKGBUILD.PkgName] {
			runtimeDeps = append(runtimeDeps, proj)
		} else {
			regularPackages = append(regularPackages, proj)
		}
	}

	// Log the build strategy for the current batch
	logger.Info(i18n.T("logger.batch_build_strategy"),
		"runtime_dependencies", len(runtimeDeps),
		"regular_packages", len(regularPackages),
		"batch_number", batchNumber)

	// Phase 1: Build and install runtime dependencies first
	// Install immediately after building to make them available for dependent packages
	if err := mpc.buildRuntimeDependenciesInOrder(runtimeDeps, maxWorkers); err != nil {
		return err
	}

	// Phase 2: Build and install regular packages
	// Regular packages may depend on runtime deps from this batch
	return mpc.buildAndInstallRegularPackages(regularPackages, maxWorkers)
}

// buildAndInstallRegularPackages handles the building and installation of regular packages.
// Uses the standard parallel build without immediate installation for non-dependency packages.
func (mpc *MultipleProject) buildAndInstallRegularPackages(
	regularPackages []*Project, maxWorkers int) error {
	if len(regularPackages) == 0 {
		return nil
	}

	logger.Info(
		i18n.T("logger.project.building_regular_packages"),
		"count", len(regularPackages))

	// Build packages in parallel
	err := mpc.buildProjectsParallel(regularPackages, maxWorkers, false)
	if err != nil {
		return err
	}

	// Install packages that are marked for installation
	for _, proj := range regularPackages {
		if !mpc.Opts.NoBuild && proj.HasToInstall {
			err := mpc.installPackage(proj)
			if err != nil {
				return err
			}
		}

		// Stop if the target package has been built
		if mpc.Opts.ToPkgName != "" && proj.Builder.PKGBUILD.PkgName == mpc.Opts.ToPkgName {
			logger.Info(i18n.T("logger.stopping_build_at_target_package"),
				"target_package", mpc.Opts.ToPkgName)

			return nil // Use a sentinel error or other mechanism if specific exit is needed
		}
	}

	return nil
}

// buildRuntimeDependenciesInOrder builds runtime dependencies in dependency-aware parallel batches.
// Independent runtime dependencies can build in parallel, but dependent ones wait for their
// dependencies.
func (mpc *MultipleProject) buildRuntimeDependenciesInOrder(
	runtimeDeps []*Project, maxWorkers int) error {
	logger.Info(
		i18n.T("logger.project.runtime_dependencies_build_optimization"),
		"count", len(runtimeDeps))

	// Build dependency graph for runtime dependencies only
	runtimeProjectMap, runtimeDependsOn, runtimeDependedBy :=
		mpc.buildRuntimeDependencyGraph(runtimeDeps)

	// Perform topological sort on runtime dependencies
	runtimeBatches, err :=
		mpc.topologicalSortRuntimeDeps(runtimeProjectMap, runtimeDependsOn, runtimeDependedBy)
	if err != nil {
		return err
	}

	logger.Info(
		i18n.T("logger.project.runtime_dependencies_batching_complete"),
		"batches", len(runtimeBatches))

	return mpc.buildAndInstallRuntimeBatches(runtimeBatches, maxWorkers)
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

// buildAndInstallRuntimeBatches builds and installs runtime dependency batches.
// Uses immediate installation after each package build (Arch Linux style).
func (mpc *MultipleProject) buildAndInstallRuntimeBatches(
	runtimeBatches [][]*Project, maxWorkers int) error {
	// Build and install each batch of runtime dependencies
	for batchIndex, batch := range runtimeBatches {
		batchSize := len(batch)

		var batchPackages []string
		for _, proj := range batch {
			batchPackages = append(batchPackages, proj.Builder.PKGBUILD.PkgName)
		}

		logger.Info(i18n.T("logger.project.building_runtime_dependency_batch"),
			"batch", batchIndex+1,
			"parallel_packages", batchSize,
			"packages", batchPackages)

		// Build and install this batch in parallel with immediate installation
		// This ensures packages are installed as soon as they're built,
		// making them available for other packages building in parallel
		err := mpc.buildProjectsParallel(batch, min(maxWorkers, batchSize), true)
		if err != nil {
			return err
		}
	}

	return nil
}

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
			return nil, fmt.Errorf("%w: %v", ErrCircularDependency, problematicPackages)
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
			return nil, fmt.Errorf("%w: %w", ErrCircularRuntimeDependency, err)
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

// getProjectsInRange returns the subset of projects based on --from and --to flags.
// Returns all projects if no flags are set, or only projects in the specified range.
func (mpc *MultipleProject) getProjectsInRange() []*Project {
	// If no filtering is specified, return all projects
	if mpc.Opts.FromPkgName == "" && mpc.Opts.ToPkgName == "" {
		return mpc.Projects
	}

	var filtered []*Project

	startProcessing := (mpc.Opts.FromPkgName == "")

	for _, proj := range mpc.Projects {
		pkgName := proj.Builder.PKGBUILD.PkgName

		// Check if this is the FromPkgName - start processing from here
		if mpc.Opts.FromPkgName != "" && !startProcessing && pkgName == mpc.Opts.FromPkgName {
			startProcessing = true

			logger.Debug(i18n.T("logger.project.found_from_package"),
				"package", mpc.Opts.FromPkgName)
		}

		// If we should be processing, include this package
		if startProcessing {
			filtered = append(filtered, proj)

			// Check if this is the ToPkgName - stop after this package
			if mpc.Opts.ToPkgName != "" && pkgName == mpc.Opts.ToPkgName {
				logger.Debug(i18n.T("logger.project.found_to_package"),
					"package", mpc.Opts.ToPkgName)

				break
			}
		}
	}

	if len(filtered) > 0 {
		var pkgNames []string
		for _, proj := range filtered {
			pkgNames = append(pkgNames, proj.Builder.PKGBUILD.PkgName)
		}

		logger.Info(i18n.T("logger.project.filtering_projects"),
			"total", len(mpc.Projects),
			"filtered", len(filtered),
			"packages", pkgNames)
	}

	return filtered
}

// checkPkgsRange checks the range of packages from `fromPkgName` to `toPkgName`.
//
// It takes two parameters:
// - fromPkgName: string representing the name of the starting package.
// - toPkgName: string representing the name of the ending package.
func (mpc *MultipleProject) checkPkgsRange(fromPkgName, toPkgName string) error {
	var firstIndex, lastIndex int

	if fromPkgName != "" {
		idx, err := mpc.findPackageInProjects(fromPkgName)
		if err != nil {
			return err
		}

		firstIndex = idx
	}

	if toPkgName != "" {
		idx, err := mpc.findPackageInProjects(toPkgName)
		if err != nil {
			return err
		}

		lastIndex = idx
	}

	if fromPkgName != "" && toPkgName != "" && firstIndex > lastIndex {
		return yerrors.New(yerrors.ErrTypeInternal, i18n.T("logger.invalid_package_order")).
			WithOperation("checkPkgsRange").
			WithContext("required_first", fromPkgName).
			WithContext("required_second", toPkgName)
	}

	return nil
}

// findPackageInProjects finds a package in the MultipleProject struct.
//
// pkgName: the name of the package to find.
// Returns the index of the package if found, and an error if not found.
func (mpc *MultipleProject) findPackageInProjects(pkgName string) (int, error) {
	for i, proj := range mpc.Projects {
		if pkgName == proj.Builder.PKGBUILD.PkgName {
			return i, nil
		}
	}

	return -1, yerrors.New(yerrors.ErrTypeInternal, "package not found in projects").
		WithOperation("findPackageInProjects").
		WithContext("package", pkgName)
}
