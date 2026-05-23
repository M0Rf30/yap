// Package project provides multi-package project management and build orchestration.
package project

import (
	"context"
	"fmt"

	"github.com/M0Rf30/yap/v2/pkg/i18n"
	"github.com/M0Rf30/yap/v2/pkg/logger"
	"golang.org/x/sync/errgroup"
)

// buildProjectsParallel builds multiple projects in parallel for better performance.
// If shouldInstall is true, each package is installed immediately after building
// (Arch Linux style), making it available for other packages building in parallel.
// Uses context cancellation to stop all workers when an error occurs.
func (mpc *MultipleProject) buildProjectsParallel(projects []*Project, maxWorkers int,
	shouldInstall bool) error {
	g, gctx := errgroup.WithContext(context.Background())
	g.SetLimit(maxWorkers)

	for workerNum, proj := range projects {
		proj := proj
		workerNum := workerNum
		g.Go(func() error {
			pkgName := proj.Builder.PKGBUILD.PkgName
			workerIDStr := fmt.Sprintf("worker-%d", workerNum)

			logger.Debug(i18n.T("logger.creating_package"),
				"package", pkgName,
				"version", proj.Builder.PKGBUILD.PkgVer,
				"release", proj.Builder.PKGBUILD.PkgRel,
				"worker_id", workerIDStr)

			// Check if cancelled before starting work
			select {
			case <-gctx.Done():
				return gctx.Err()
			default:
			}

			// Step 1: Build the package
			if err := proj.Builder.Compile(gctx, mpc.Opts.NoBuild); err != nil {
				return err
			}

			if !mpc.Opts.NoBuild {
				// Step 2: Create the package file
				if err := mpc.createPackage(proj); err != nil {
					return err
				}

				// Step 3: Install immediately (Arch Linux style) or extract for cross-compilation
				if shouldInstall {
					if err := mpc.installPackageForWorker(proj, pkgName, workerIDStr); err != nil {
						return err
					}
				}
			}

			if mpc.Opts.ToPkgName != "" && pkgName == mpc.Opts.ToPkgName {
				return nil
			}

			return nil
		})
	}

	return g.Wait()
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
