// Package project provides multi-package project management and build orchestration.
package project

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"

	"github.com/go-playground/validator/v10"
	"github.com/otiai10/copy"
	"github.com/pkg/errors"

	"github.com/M0Rf30/yap/v2/pkg/builder"
	"github.com/M0Rf30/yap/v2/pkg/files"
	"github.com/M0Rf30/yap/v2/pkg/i18n"
	"github.com/M0Rf30/yap/v2/pkg/logger"
	"github.com/M0Rf30/yap/v2/pkg/packer"
	"github.com/M0Rf30/yap/v2/pkg/parser"
	"github.com/M0Rf30/yap/v2/pkg/pkgbuild"
)

var (
	// ErrCircularDependency indicates a circular dependency was detected.
	// Static errors for linting compliance.
	ErrCircularDependency = errors.New(i18n.T("errors.project.circular_dependency_detected"))
	// ErrCircularRuntimeDependency indicates a circular runtime dependency was detected.
	ErrCircularRuntimeDependency = errors.New(
		i18n.T("errors.project.circular_runtime_dependency_detected"))
)

// Config encapsulates all build configuration and state variables.
type Config struct {
	// Build configuration flags
	Verbose                 bool   // Enables verbose output for debugging
	CleanBuild              bool   // Enables clean build mode
	NoBuild                 bool   // Disables the build process
	NoMakeDeps              bool   // Disables make dependencies installation
	SkipSyncDeps            bool   // Controls whether to skip dependency synchronization
	SkipToolchainValidation bool   // Controls whether to skip cross-compilation toolchain validation
	Zap                     bool   // Controls whether to use zap functionality
	FromPkgName             string // Specifies the source package name for transformation
	ToPkgName               string // Specifies the target package name for transformation
	TargetArch              string // Specifies the target architecture for cross-compilation
	Parallel                bool   // Enables parallel dependency resolution and concurrent building

	// Internal state
	singleProject  bool          //nolint:unused // Reserved for future migration
	packageManager packer.Packer //nolint:unused // Reserved for future migration
	makeDepends    []string      //nolint:unused // Reserved for dependency tracking
	runtimeDepends []string      //nolint:unused // Reserved for dependency tracking
}

// Global variables for build configuration
var (
	// Verbose enables verbose output for debugging.
	Verbose bool
	// CleanBuild enables clean build mode.
	CleanBuild bool
	// NoBuild disables the build process.
	NoBuild bool
	// NoMakeDeps disables make dependencies installation.
	NoMakeDeps bool
	// SkipSyncDeps controls whether to skip dependency synchronization.
	SkipSyncDeps bool
	// SkipToolchainValidation controls whether to skip cross-compilation toolchain validation.
	SkipToolchainValidation bool
	// Zap controls whether to use zap functionality.
	Zap bool
	// FromPkgName specifies the source package name for transformation.
	FromPkgName string
	// ToPkgName specifies the target package name for transformation.
	ToPkgName string
	// TargetArch specifies the target architecture for cross-compilation
	TargetArch string
	// Parallel enables parallel dependency resolution and concurrent package building.
	// When false (default), packages are built sequentially respecting "install" flags.
	Parallel bool

	// Global state variables
	singleProject  bool
	packageManager packer.Packer
	makeDepends    []string
	runtimeDepends []string
)

// extractPackageName extracts the package name from a dependency string,
// ignoring version constraints and other metadata.
//
// Examples:
//   - "gcc" -> "gcc"
//   - "gcc>=11.0" -> "gcc"
//   - "python3 >=3.9" -> "python3"
//
// Returns empty string if the input is empty or contains only whitespace.
func extractPackageName(dep string) string {
	fields := strings.Fields(dep)
	if len(fields) == 0 {
		return ""
	}

	return fields[0]
}

// processDependencies iterates through a list of dependencies and processes each one
// that exists in the packageMap using the provided handler function.
// This consolidates the common pattern of extracting package names and checking existence.
func processDependencies(
	deps []string,
	packageMap map[string]*Project,
	handler func(depName string),
) {
	for _, dep := range deps {
		depName := extractPackageName(dep)
		if depName != "" {
			if _, exists := packageMap[depName]; exists {
				handler(depName)
			}
		}
	}
}

// DistroProject is an interface that defines the methods for creating and
// preparing a project for a specific distribution.
//
// It includes the following methods:
//   - Create(): error
//     This method is responsible for creating the project.
//   - Prepare(): error
//     This method is responsible for preparing the project.
type DistroProject interface {
	Create() error
	Prepare() error
}

// MultipleProject represents a collection of projects.
//
// It contains a slice of Project objects and provides methods to interact
// with multiple projects. The methods include building all the projects,
// finding a package in the projects, and creating packages for each project.
// The MultipleProject struct also contains the output directory for the
// packages and the ToPkgName field, which can be used to stop the build
// process after a specific package.
type MultipleProject struct {
	BuildDir    string     `json:"buildDir"    validate:"required"`
	Description string     `json:"description" validate:"required"`
	Name        string     `json:"name"        validate:"required"`
	Output      string     `json:"output"      validate:"required"`
	Projects    []*Project `json:"projects"    validate:"required,dive,required"`
	Config      *Config    // Configuration for this project build
}

// Project represents a single project.
//
// It contains the necessary information to build and manage the project.
// The fields include the project's name, the path to the project directory,
// the builder object, the package manager object, and a flag indicating
// whether the project has to be installed.
type Project struct {
	Builder        *builder.Builder
	BuildRoot      string
	Distro         string
	PackageManager packer.Packer
	Path           string
	Release        string
	Root           string
	Name           string `json:"name"    validate:"required,startsnotwith=.,startsnotwith=./"`
	HasToInstall   bool   `json:"install" validate:""`
}

// BuildAll builds all projects in the correct order.
// When Parallel is false (default), projects are built sequentially in file order;
// packages with HasToInstall set are installed immediately after being built.
// When Parallel is true, a topological sort determines build order and packages
// are built concurrently using a worker pool.
func (mpc *MultipleProject) BuildAll() error {
	if !singleProject {
		if err := mpc.checkPkgsRange(FromPkgName, ToPkgName); err != nil {
			return err
		}
	}

	// Show verbose dependency information at debug level regardless of build mode.
	// In sequential mode this is informational only; resolution happens in the parallel path.
	mpc.displayVerboseDependencyInfo()

	// Filter projects based on --from and --to flags before building
	projectsToProcess := mpc.getProjectsInRange()

	if !Parallel {
		// Default: sequential build in file order.
		// Packages with "install": true are installed immediately after building.
		return mpc.buildProjectsSequential(projectsToProcess)
	}

	// Parallel path: dependency-aware topological sort + worker pools
	buildOrder, err := mpc.resolveDependencies(projectsToProcess)
	if err != nil {
		return err
	}

	// Performance optimization: determine optimal parallelism
	maxWorkers := min(runtime.NumCPU(), len(projectsToProcess))

	// Process packages in dependency-aware parallel batches
	return mpc.buildProjectsInOrder(buildOrder, maxWorkers)
}

// Clean cleans up the MultipleProject by removing the package directories and
// source directories if the CleanBuild flag is set. It takes no parameters. It
// returns an error if there was a problem removing the directories.
func (mpc *MultipleProject) Clean() error {
	for _, proj := range mpc.Projects {
		if CleanBuild {
			err := os.RemoveAll(proj.Builder.PKGBUILD.SourceDir)
			if err != nil {
				return err
			}
		}

		if Zap {
			if err := mpc.cleanZapArtifacts(proj); err != nil {
				return err
			}
		}
	}

	return nil
}

// MultiProject is a function that performs multiple project operations.
//
// It takes in the following parameters:
// - distro: a string representing the distribution
// - release: a string representing the release
// - path: a string representing the path
//
// It returns an error.
func (mpc *MultipleProject) MultiProject(distro, release, path string) error {
	err := mpc.readProject(path)
	if err != nil {
		return err
	}

	err = files.ExistsMakeDir(mpc.BuildDir)
	if err != nil {
		return err
	}

	packageManager = packer.GetPackageManager(&pkgbuild.PKGBUILD{}, distro)
	if !SkipSyncDeps {
		err := packageManager.Update()
		if err != nil {
			return err
		}
	}

	err = mpc.populateProjects(distro, release, path)
	if err != nil {
		return err
	}

	if CleanBuild || Zap {
		err := mpc.Clean()
		if err != nil {
			logger.Fatal(i18n.T("logger.fatal_error"),
				"error", err)
		}
	}

	err = mpc.copyProjects()
	if err != nil {
		return err
	}

	if !NoMakeDeps {
		makeDepends = mpc.getMakeDeps()

		err := packageManager.Prepare(makeDepends, TargetArch)
		if err != nil {
			return err
		}
	}

	// Install external runtime dependencies
	if !SkipSyncDeps {
		runtimeDepends = mpc.getRuntimeDeps()

		if len(runtimeDepends) > 0 {
			logger.Debug(i18n.T("logger.installing_external_runtime_dependencies"),
				"count", len(runtimeDepends))

			err := packageManager.Prepare(runtimeDepends, TargetArch)
			if err != nil {
				return err
			}
		}
	}

	return nil
}

// buildProjectsParallel builds multiple projects in parallel for better performance.
func (mpc *MultipleProject) buildProjectsParallel(projects []*Project, maxWorkers int) error {
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
				logger.Debug(i18n.T("logger.creating_package"),
					"package", proj.Builder.PKGBUILD.PkgName,
					"version", proj.Builder.PKGBUILD.PkgVer,
					"release", proj.Builder.PKGBUILD.PkgRel,
					"worker_id", workerIDStr)

				err := proj.Builder.Compile(NoBuild)
				if err != nil {
					errorChan <- err

					return
				}

				if !NoBuild {
					err := mpc.createPackage(proj)
					if err != nil {
						errorChan <- err

						return
					}
				}

				if ToPkgName != "" && proj.Builder.PKGBUILD.PkgName == ToPkgName {
					return
				}
			}
		}(workerNum)
	}

	// Send projects to workers
	go func() {
		defer close(projectChan)

		for _, proj := range projects {
			projectChan <- proj
		}
	}()

	// Wait for completion
	go func() {
		waitGroup.Wait()
		close(errorChan)
	}()

	// Check for errors
	for err := range errorChan {
		if err != nil {
			return err
		}
	}

	return nil
}

// buildProjectsSequential builds projects one at a time in the order they appear
// in the project list (file order from yap.json). If a project has HasToInstall set,
// it is installed immediately after building, before the next project is processed.
// This is the default (v1-compatible) build mode.
func (mpc *MultipleProject) buildProjectsSequential(projects []*Project) error {
	for _, proj := range projects {
		pkgName := proj.Builder.PKGBUILD.PkgName

		logger.Debug(i18n.T("logger.creating_package"),
			"package", pkgName,
			"version", proj.Builder.PKGBUILD.PkgVer,
			"release", proj.Builder.PKGBUILD.PkgRel)

		if err := proj.Builder.Compile(NoBuild); err != nil {
			return err
		}

		if !NoBuild {
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
		if ToPkgName != "" && pkgName == ToPkgName {
			logger.Info(i18n.T("logger.stopping_build_at_target_package"),
				"target_package", ToPkgName)

			return nil
		}
	}

	return nil
}

// installPackageForWorker handles package installation or extraction for a worker.
// It uses InstallOrExtract to handle both native builds (install) and cross-compilation (extract).
func (mpc *MultipleProject) installPackageForWorker(proj *Project, pkgName, workerID string) error {
	logger.Info(i18n.T("logger.installing_package"),
		"package", pkgName,
		"worker_id", workerID)

	// Type assert to access BaseBuilder methods
	type installOrExtractor interface {
		InstallOrExtract(artifactsPath, buildDir, targetArch string) error
	}

	if installer, ok := proj.PackageManager.(installOrExtractor); ok {
		err := installer.InstallOrExtract(mpc.Output, mpc.BuildDir, TargetArch)
		if err != nil {
			logger.Error(
				i18n.T("logger.project.package_installation_failed"),
				"package", pkgName,
				"error", err)

			return err
		}
	} else {
		// Fallback to regular Install if InstallOrExtract not available
		err := proj.PackageManager.Install(mpc.Output)
		if err != nil {
			logger.Error(
				i18n.T("logger.project.package_installation_failed"),
				"package", pkgName,
				"error", err)

			return err
		}
	}

	logger.Info(i18n.T("logger.package_installed"),
		"package", pkgName,
		"worker_id", workerID)

	return nil
}

// buildAndInstallProjectsParallel builds projects in parallel with immediate installation.
// This implements Arch Linux-style dependency handling: each package is installed
// immediately after building, making it available for other packages building in parallel.
// This ensures that packages with runtime dependencies (depends) can access those
// dependencies during their build phase, even if they're in the same batch.
func (mpc *MultipleProject) buildAndInstallProjectsParallel(projects []*Project, maxWorkers int,
	shouldInstall bool) error {
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

				logger.Debug(i18n.T("logger.creating_package"),
					"package", pkgName,
					"version", proj.Builder.PKGBUILD.PkgVer,
					"release", proj.Builder.PKGBUILD.PkgRel,
					"worker_id", workerIDStr)

				// Step 1: Build the package
				err := proj.Builder.Compile(NoBuild)
				if err != nil {
					errorChan <- err

					return
				}

				if !NoBuild {
					// Step 2: Create the package file
					err := mpc.createPackage(proj)
					if err != nil {
						errorChan <- err

						return
					}

					// Step 3: Install immediately (Arch Linux style) or extract for cross-compilation
					if shouldInstall {
						if err := mpc.installPackageForWorker(proj, pkgName, workerIDStr); err != nil {
							errorChan <- err

							return
						}
					}
				}

				if ToPkgName != "" && pkgName == ToPkgName {
					return
				}
			}
		}(workerNum)
	}

	// Send projects to workers
	go func() {
		defer close(projectChan)

		for _, proj := range projects {
			projectChan <- proj
		}
	}()

	// Wait for completion
	go func() {
		waitGroup.Wait()
		close(errorChan)
	}()

	// Check for errors
	for err := range errorChan {
		if err != nil {
			return err
		}
	}

	return nil
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
		return fmt.Errorf(i18n.T("logger.invalid_package_order")+": required_first=%s required_second=%s",
			fromPkgName, toPkgName)
	}

	return nil
}

// shouldSkipFile determines if a file should be skipped during copying.
func shouldSkipFile(info os.FileInfo, src, dest string) (bool, error) {
	// Skip if destination already exists with same size and modification time
	if destInfo, err := os.Stat(dest); err == nil {
		if !info.IsDir() && info.Size() == destInfo.Size() && info.ModTime().Equal(destInfo.ModTime()) {
			return true, nil
		}
	}

	// Define a slice of file extensions to skip
	skipExtensions := []string{
		".apk", ".deb", ".pkg.tar.zst", ".rpm",
		".tar.gz", ".tar.xz", ".tar.bz2", ".zip",
	}
	for _, ext := range skipExtensions {
		if strings.HasSuffix(src, ext) {
			return true, nil
		}
	}

	// Skip temporary and build artifacts
	basename := filepath.Base(src)
	if strings.HasPrefix(basename, ".") || strings.HasSuffix(basename, ".tmp") ||
		strings.HasSuffix(basename, "~") || basename == "Thumbs.db" || basename == ".DS_Store" {
		return true, nil
	}

	return false, nil
}

// setupCopyOptions creates the copy options for the copyProjects function.
func setupCopyOptions() copy.Options {
	return copy.Options{
		OnSymlink: func(_ string) copy.SymlinkAction {
			return copy.Skip
		},
		OnDirExists: func(src, dest string) copy.DirExistsAction {
			return copy.Merge
		},
		Sync:          false, // Don't delete extra files in destination
		PreserveTimes: false, // Don't preserve modification times for better performance
		PreserveOwner: false, // Don't preserve ownership for better performance
		Skip:          shouldSkipFile,
	}
}

// copyProjects copies PKGBUILD directories for all projects, creating the
// target directory if it doesn't exist.
// It skips files with extensions: .apk, .deb, .pkg.tar.zst, and .rpm,
// as well as symlinks. Uses hardlinks when possible to reduce disk usage.
// Returns an error if any operation fails; otherwise, returns nil.
func (mpc *MultipleProject) copyProjects() error {
	singleProject := len(mpc.Projects) == 1
	copyOpt := setupCopyOptions()

	for _, proj := range mpc.Projects {
		// Ensure the target directory exists
		if err := files.ExistsMakeDir(proj.Builder.PKGBUILD.StartDir); err != nil {
			return err
		}

		// Ensure the pkgdir directory exists
		if err := files.ExistsMakeDir(proj.Builder.PKGBUILD.PackageDir); err != nil {
			return err
		}

		// Only copy if the source and destination are different
		if !singleProject {
			err := copy.Copy(proj.Builder.PKGBUILD.Home, proj.Builder.PKGBUILD.StartDir, copyOpt)
			if err != nil {
				return err
			}
		}
	}

	return nil
}

// createPackage creates packages for the MultipleProject.
//
// It takes a pointer to a MultipleProject as a receiver and a pointer to a Project as a parameter.
// It returns an error.
func (mpc *MultipleProject) createPackage(proj *Project) error {
	if mpc.Output != "" {
		absOutput, err := filepath.Abs(mpc.Output)
		if err != nil {
			return err
		}

		mpc.Output = absOutput
	}

	defer func() {
		err := os.RemoveAll(proj.Builder.PKGBUILD.PackageDir)
		if err != nil {
			logger.Warn(i18n.T("logger.failed_to_remove_package_directory"),
				"path", proj.Builder.PKGBUILD.PackageDir,
				"error", err)
		}
	}()

	err := files.ExistsMakeDir(mpc.Output)
	if err != nil {
		return err
	}

	err = proj.PackageManager.PrepareFakeroot(mpc.Output, TargetArch)
	if err != nil {
		return err
	}

	logger.Info(i18n.T("logger.building_resulting_package"),
		"package", proj.Builder.PKGBUILD.PkgName,
		"version", proj.Builder.PKGBUILD.PkgVer,
		"release", proj.Builder.PKGBUILD.PkgRel)

	err = proj.PackageManager.BuildPackage(mpc.Output, TargetArch)
	if err != nil {
		return err
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

	return -1, fmt.Errorf("package %q not found in projects", pkgName)
}

// getMakeDeps retrieves the make dependencies for the MultipleProject.
//
// It iterates over each child project and collects their make dependencies,
// ensuring no duplicates are included. Returns the collected dependencies.
func (mpc *MultipleProject) getMakeDeps() []string {
	// Use a map to track unique dependencies and prevent duplicates
	uniqueDeps := make(map[string]bool)

	var result []string

	for _, child := range mpc.Projects {
		for _, dep := range child.Builder.PKGBUILD.MakeDepends {
			depName := extractPackageName(dep)
			if !uniqueDeps[depName] {
				uniqueDeps[depName] = true

				result = append(result, dep)
			}
		}
	}

	return result
}

// getRuntimeDeps retrieves the runtime dependencies for the MultipleProject.
// It filters out internal dependencies (packages within the project) and only
// collects external dependencies that need to be installed via package manager.
// Returns the collected external dependencies.
func (mpc *MultipleProject) getRuntimeDeps() []string {
	// Create a set of internal package names for filtering
	internalPackages := make(map[string]bool)
	for _, proj := range mpc.Projects {
		internalPackages[proj.Builder.PKGBUILD.PkgName] = true
	}

	// Use a map to track unique dependencies and prevent duplicates
	uniqueDeps := make(map[string]bool)

	var result []string

	// Collect external runtime dependencies
	for _, child := range mpc.Projects {
		for _, dep := range child.Builder.PKGBUILD.Depends {
			depName := extractPackageName(dep)
			// Only add if it's not an internal package and not already added
			if !internalPackages[depName] && !uniqueDeps[depName] {
				uniqueDeps[depName] = true

				result = append(result, dep)
			}
		}
	}

	if len(result) > 0 {
		logger.Info(i18n.T("logger.external_runtime_dependencies_collected"),
			"count", len(result),
			"dependencies", result)
	}

	return result
}

// populateProjects populates the MultipleProject with projects based on the
// given distro, release, and path.
//
// distro: The distribution of the projects.
// release: The release version of the projects.
// path: The path to the projects.
// error: An error if any occurred during the population process.
func (mpc *MultipleProject) populateProjects(distro, release, path string) error {
	projects := make([]*Project, 0)

	for _, child := range mpc.Projects {
		startDir := filepath.Join(mpc.BuildDir, child.Name)
		home := filepath.Join(path, child.Name)

		pkgbuildFile, err := parser.ParseFile(distro,
			release,
			startDir,
			home)
		if err != nil {
			return err
		}

		// Set target architecture for cross-compilation if specified
		if TargetArch != "" {
			// Keep the native architecture for ArchComputed, set TargetArch for cross-compilation
			pkgbuildFile.ComputeArchitecture() // This sets the native architecture
			pkgbuildFile.TargetArch = TargetArch
		} else {
			pkgbuildFile.ComputeArchitecture()
		}

		if err := pkgbuildFile.ValidateMandatoryItems(); err != nil {
			return err
		}

		if err := pkgbuildFile.ValidateGeneral(); err != nil {
			return err
		}

		packageManager = packer.GetPackageManager(pkgbuildFile, distro)

		proj := &Project{
			Name:           child.Name,
			Builder:        &builder.Builder{PKGBUILD: pkgbuildFile},
			PackageManager: packageManager,
			HasToInstall:   child.HasToInstall,
		}

		projects = append(projects, proj)
	}

	mpc.Projects = projects

	return nil
}

// readProject reads the project file at the specified path
// and populates the MultipleProject struct.
//
// It takes a string parameter `path` which represents the path to the project file.
// It returns an error if there was an issue opening or reading the file, or if the
// JSON data is invalid.
func (mpc *MultipleProject) readProject(path string) error {
	jsonFilePath := filepath.Join(path, "yap.json")
	pkgbuildFilePath := filepath.Join(path, "PKGBUILD")

	var projectFilePath string

	if files.Exists(jsonFilePath) {
		projectFilePath = jsonFilePath
		logger.Info(i18n.T("logger.multi_project_file_found"),
			"path", projectFilePath)
	}

	if files.Exists(pkgbuildFilePath) {
		projectFilePath = pkgbuildFilePath
		logger.Info(i18n.T("logger.single_project_file_found"),
			"path", projectFilePath)

		mpc.setSingleProject(path)
	}

	filePath, err := files.Open(projectFilePath)
	if err != nil || singleProject {
		return err
	}

	defer func() {
		err := filePath.Close()
		if err != nil {
			logger.Warn(i18n.T("logger.failed_to_close_project_file"), "path", projectFilePath, "error", err)
		}
	}()

	prjContent, err := io.ReadAll(filePath)
	if err != nil {
		return err
	}

	//nolint:musttag
	err = json.Unmarshal(prjContent, &mpc)
	if err != nil {
		return err
	}

	err = mpc.validateJSON()
	if err != nil {
		return err
	}

	return err
}

// setSingleProject reads the PKGBUILD file at the given path and updates the
// MultipleProject instance.
func (mpc *MultipleProject) setSingleProject(path string) {
	cleanFilePath := filepath.Clean(path)
	proj := &Project{
		Name:           "",
		PackageManager: packageManager,
		HasToInstall:   false,
	}

	mpc.BuildDir = cleanFilePath
	mpc.Output = cleanFilePath
	mpc.Projects = append(mpc.Projects, proj)
	singleProject = true
}

// ReadProjectOnly reads the project configuration without initializing distro-specific components.
// This is useful for operations like graph generation that only need project structure.
func (mpc *MultipleProject) ReadProjectOnly(path string) error {
	return mpc.readProject(path)
}

// validateJSON validates the JSON of the MultipleProject struct.
//
// It uses the validator package to validate the struct and returns any errors encountered.
// It returns an error if the validation fails.
func (mpc *MultipleProject) validateJSON() error {
	validate := validator.New()

	return validate.Struct(mpc)
}

// displayVerboseDependencyInfo shows detailed dependency information for all projects.
func (mpc *MultipleProject) displayVerboseDependencyInfo() {
	logger.Debug(i18n.T("logger.dependency_analysis_starting"))

	// Create dependency map for internal packages
	packageMap := make(map[string]*Project)
	for _, proj := range mpc.Projects {
		packageMap[proj.Builder.PKGBUILD.PkgName] = proj
	}

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
	if FromPkgName == "" && ToPkgName == "" {
		return mpc.Projects
	}

	var filtered []*Project

	startProcessing := (FromPkgName == "")

	for _, proj := range mpc.Projects {
		pkgName := proj.Builder.PKGBUILD.PkgName

		// Check if this is the FromPkgName - start processing from here
		if FromPkgName != "" && !startProcessing && pkgName == FromPkgName {
			startProcessing = true

			logger.Debug(i18n.T("logger.project.found_from_package"),
				"package", FromPkgName)
		}

		// If we should be processing, include this package
		if startProcessing {
			filtered = append(filtered, proj)

			// Check if this is the ToPkgName - stop after this package
			if ToPkgName != "" && pkgName == ToPkgName {
				logger.Debug(i18n.T("logger.project.found_to_package"),
					"package", ToPkgName)

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

// resolveDependencies builds a dependency graph and returns projects in topologically sorted order.
// Returns slices of project batches that can be built in parallel within each batch.
// Only considers dependencies that exist within the provided project set.
func (mpc *MultipleProject) resolveDependencies(projects []*Project) ([][]*Project, error) {
	logger.Info(i18n.T("logger.analyzing_package_dependencies"))

	// Build dependency graph
	projectMap := make(map[string]*Project)
	dependsOn := make(map[string][]string)
	dependedBy := make(map[string][]string)

	// Index projects by package name - only for projects in range
	for _, proj := range projects {
		pkgName := proj.Builder.PKGBUILD.PkgName
		projectMap[pkgName] = proj
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

// buildRuntimeDependencyMap creates a map of packages that are runtime dependencies
// of other packages.
func (mpc *MultipleProject) buildRuntimeDependencyMap() map[string]bool {
	dependencyMap := make(map[string]bool)
	packageMap := make(map[string]*Project)

	// Index projects by package name
	for _, proj := range mpc.Projects {
		packageMap[proj.Builder.PKGBUILD.PkgName] = proj
	}

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

// calculateDependencyPopularity returns a map of package names to how many other packages
// depend on them.
// This helps identify "fundamental" packages that should be built first.
func (mpc *MultipleProject) calculateDependencyPopularity() map[string]int {
	popularity := make(map[string]int)
	packageMap := make(map[string]*Project)

	// Index projects by package name
	for _, proj := range mpc.Projects {
		packageMap[proj.Builder.PKGBUILD.PkgName] = proj
		popularity[proj.Builder.PKGBUILD.PkgName] = 0 // Initialize
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
	err := mpc.buildAndInstallRuntimeDeps(runtimeDeps, maxWorkers)
	if err != nil {
		return err
	}

	// Phase 2: Build and install regular packages
	// Regular packages may depend on runtime deps from this batch
	return mpc.buildAndInstallRegularPackages(regularPackages, maxWorkers)
}

// buildAndInstallRuntimeDeps handles the building and installation of runtime dependencies.
func (mpc *MultipleProject) buildAndInstallRuntimeDeps(
	runtimeDeps []*Project, maxWorkers int) error {
	if len(runtimeDeps) == 0 {
		return nil
	}

	err := mpc.buildRuntimeDependenciesInOrder(runtimeDeps, maxWorkers)
	if err != nil {
		return err
	}

	return nil
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
	err := mpc.buildProjectsParallel(regularPackages, maxWorkers)
	if err != nil {
		return err
	}

	// Install packages that are marked for installation
	for _, proj := range regularPackages {
		if !NoBuild && proj.HasToInstall {
			err := mpc.installPackage(proj)
			if err != nil {
				return err
			}
		}

		// Stop if the target package has been built
		if ToPkgName != "" && proj.Builder.PKGBUILD.PkgName == ToPkgName {
			logger.Info(i18n.T("logger.stopping_build_at_target_package"),
				"target_package", ToPkgName)

			return nil // Use a sentinel error or other mechanism if specific exit is needed
		}
	}

	return nil
}

// installPackage installs a single package or extracts it for cross-compilation.
func (mpc *MultipleProject) installPackage(proj *Project) error {
	pkgName := proj.Builder.PKGBUILD.PkgName
	logger.Info(i18n.T("logger.installing_package"), "package", pkgName)

	// Use InstallOrExtract to handle both native and cross-compilation builds
	type installOrExtractor interface {
		InstallOrExtract(artifactsPath, buildDir, targetArch string) error
	}

	if installer, ok := proj.PackageManager.(installOrExtractor); ok {
		err := installer.InstallOrExtract(mpc.Output, mpc.BuildDir, TargetArch)
		if err != nil {
			logger.Error(
				i18n.T("logger.project.package_installation_failed"),
				"package", pkgName,
				"error", err)

			return err
		}
	} else {
		// Fallback to regular Install if InstallOrExtract not available
		err := proj.PackageManager.Install(mpc.Output)
		if err != nil {
			logger.Error(
				i18n.T("logger.project.package_installation_failed"),
				"package", pkgName,
				"error", err)

			return err
		}
	}

	logger.Info(i18n.T("logger.package_installed"), "package", pkgName)

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
			logger.Info(
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
			logger.Info(
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
		err := mpc.buildAndInstallProjectsParallel(batch, min(maxWorkers, batchSize), true)
		if err != nil {
			return err
		}
	}

	return nil
}

// cleanZapArtifacts removes build artifacts for a project when Zap is enabled
func (mpc *MultipleProject) cleanZapArtifacts(proj *Project) error {
	// For single projects, StartDir is the actual project directory containing
	// source files and PKGBUILD, so we should NOT remove it. Only remove
	// build artifacts within it.
	if singleProject {
		return mpc.cleanSingleProjectArtifacts(proj)
	}

	// For multi-projects, StartDir is a build directory that can be safely removed
	// Remove StartDir completely (this includes src, pkg, and all build artifacts)
	return os.RemoveAll(proj.Builder.PKGBUILD.StartDir)
}

// cleanSingleProjectArtifacts removes build artifacts for single projects
func (mpc *MultipleProject) cleanSingleProjectArtifacts(proj *Project) error {
	// Remove src directory (contains downloaded and extracted sources)
	srcDir := filepath.Join(proj.Builder.PKGBUILD.StartDir, "src")
	if _, err := os.Stat(srcDir); err == nil {
		if err := os.RemoveAll(srcDir); err != nil {
			return err
		}
	}

	// Remove pkg directory (contains built packages)
	pkgDir := filepath.Join(proj.Builder.PKGBUILD.StartDir, "pkg")
	if _, err := os.Stat(pkgDir); err == nil {
		if err := os.RemoveAll(pkgDir); err != nil {
			return err
		}
	}

	// Remove other common build artifacts but preserve source files
	return mpc.removeBuildArtifacts(proj.Builder.PKGBUILD.StartDir)
}

// removeBuildArtifacts removes common build artifacts from the specified directory
func (mpc *MultipleProject) removeBuildArtifacts(dir string) error {
	buildArtifacts := []string{
		"*.tar.xz", "*.tar.gz", "*.tar.bz2", "*.deb", "*.rpm", "*.pkg.tar.*",
		"*.log", "*.sig",
	}

	for _, pattern := range buildArtifacts {
		matches, err := filepath.Glob(filepath.Join(dir, pattern))
		if err != nil {
			continue // Skip if glob pattern fails
		}

		for _, match := range matches {
			_ = os.Remove(match) // Ignore errors for individual file removal
		}
	}

	return nil
}

// topologicalSortRuntimeDeps performs topological sort specifically for runtime dependencies.
func (mpc *MultipleProject) topologicalSortRuntimeDeps(projectMap map[string]*Project,
	dependsOn map[string][]string, dependedBy map[string][]string,
) ([][]*Project, error) {
	var result [][]*Project

	inDegree := make(map[string]int)

	// Calculate in-degrees for runtime dependencies
	for pkgName := range projectMap {
		inDegree[pkgName] = len(dependsOn[pkgName])
	}

	batchNum := 1

	for len(inDegree) > 0 {
		var currentBatch []*Project

		var candidatePackages []string

		// Find packages with no dependencies
		for pkgName, degree := range inDegree {
			if degree == 0 {
				candidatePackages = append(candidatePackages, pkgName)
			}
		}

		if len(candidatePackages) == 0 {
			var problematicPackages []string
			for pkgName, degree := range inDegree {
				problematicPackages = append(problematicPackages, fmt.Sprintf("%s(%d)", pkgName, degree))
			}

			return nil, fmt.Errorf("%w: %v", ErrCircularRuntimeDependency, problematicPackages)
		}

		// Build current batch
		for _, pkgName := range candidatePackages {
			currentBatch = append(currentBatch, projectMap[pkgName])
		}

		logger.Info(i18n.T("logger.project.runtime_dependency_batch_analysis"),
			"batch_number", batchNum,
			"parallel_packages", len(currentBatch))

		result = append(result, currentBatch)

		// Update in-degrees
		for _, pkgName := range candidatePackages {
			delete(inDegree, pkgName)

			for _, dependent := range dependedBy[pkgName] {
				if _, exists := inDegree[dependent]; exists {
					inDegree[dependent]--
				}
			}
		}

		batchNum++
	}

	return result, nil
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
	}

	return nil
}
