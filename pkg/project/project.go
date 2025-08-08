// Package project provides multi-package project management and build orchestration.
package project

import (
	"encoding/json"
	"errors"
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

	"github.com/M0Rf30/yap/v2/pkg/builder"
	"github.com/M0Rf30/yap/v2/pkg/osutils"
	"github.com/M0Rf30/yap/v2/pkg/packer"
	"github.com/M0Rf30/yap/v2/pkg/parser"
	"github.com/M0Rf30/yap/v2/pkg/pkgbuild"
)

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
	// Zap controls whether to use zap functionality.
	Zap bool
	// FromPkgName specifies the source package name for transformation.
	FromPkgName string
	// ToPkgName specifies the target package name for transformation.
	ToPkgName string

	// Global state variables
	singleProject  bool
	packageManager packer.Packer
	makeDepends    []string
	runtimeDepends []string
)

var (
	// ErrCircularDependency indicates a circular dependency was detected.
	// Static errors for linting compliance.
	ErrCircularDependency = errors.New("circular dependency detected")
	// ErrCircularRuntimeDependency indicates a circular runtime dependency was detected.
	ErrCircularRuntimeDependency = errors.New("circular dependency in runtime dependencies")

	// PrefixMiddle is the prefix for middle dependency items in display.
	PrefixMiddle = "  │  ├─"
	// PrefixLast is the prefix for last dependency items in display.
	PrefixLast = "  │  └─"
)

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

// BuildAll builds all the projects in the MultipleProject struct with optimizations.
//
// It compiles each project's package and creates the necessary packages.
// Uses proper dependency-aware parallel processing to improve performance.
// If the project has to be installed, it installs the package.
// If ToPkgName is not empty, it stops building after the specified package.
// It returns an error if any error occurs during the build process.
func (mpc *MultipleProject) BuildAll() error {
	if !singleProject {
		mpc.checkPkgsRange(FromPkgName, ToPkgName)
	}

	// Show verbose dependency information automatically via debug logging
	mpc.displayVerboseDependencyInfo()

	// Build dependency graph and get topologically sorted build order
	buildOrder, err := mpc.resolveDependencies()
	if err != nil {
		return err
	}

	// Performance optimization: determine optimal parallelism
	maxWorkers := min(runtime.NumCPU(), len(mpc.Projects))

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

		if Zap && !singleProject {
			err := os.RemoveAll(proj.Builder.PKGBUILD.StartDir)
			if err != nil {
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

	err = osutils.ExistsMakeDir(mpc.BuildDir)
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
			osutils.Logger.Fatal("fatal error",
				osutils.Logger.Args("error", err))
		}
	}

	err = mpc.copyProjects()
	if err != nil {
		return err
	}

	if !NoMakeDeps {
		mpc.getMakeDeps()

		err := packageManager.Prepare(makeDepends)
		if err != nil {
			return err
		}
	}

	// Install external runtime dependencies
	if !SkipSyncDeps {
		mpc.getRuntimeDeps()

		if len(runtimeDepends) > 0 {
			osutils.Logger.Info("installing external runtime dependencies",
				osutils.Logger.Args("count", len(runtimeDepends)))

			err := packageManager.Prepare(runtimeDepends)
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
	for range maxWorkers {
		waitGroup.Add(1)

		go func() {
			defer waitGroup.Done()

			for proj := range projectChan {
				pkgLogger := osutils.WithComponent(proj.Builder.PKGBUILD.PkgName)
				pkgLogger.Info("making package", pkgLogger.Args("pkgver", proj.Builder.PKGBUILD.PkgVer,
					"pkgrel", proj.Builder.PKGBUILD.PkgRel))

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
		}()
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
func (mpc *MultipleProject) checkPkgsRange(fromPkgName, toPkgName string) {
	var firstIndex, lastIndex int

	if fromPkgName != "" {
		firstIndex = mpc.findPackageInProjects(fromPkgName)
	}

	if toPkgName != "" {
		lastIndex = mpc.findPackageInProjects(toPkgName)
	}

	if fromPkgName != "" && toPkgName != "" && firstIndex > lastIndex {
		osutils.Logger.Fatal("invalid package order: %s should be built before %s",
			osutils.Logger.Args(fromPkgName, toPkgName))
	}
}

// copyProjects copies PKGBUILD directories for all projects, creating the
// target directory if it doesn't exist.
// It skips files with extensions: .apk, .deb, .pkg.tar.zst, and .rpm,
// as well as symlinks.
// Returns an error if any operation fails; otherwise, returns nil.
func (mpc *MultipleProject) copyProjects() error {
	copyOpt := copy.Options{
		OnSymlink: func(_ string) copy.SymlinkAction {
			return copy.Skip
		},
		Skip: func(_ os.FileInfo, src, _ string) (bool, error) {
			// Define a slice of file extensions to skip
			skipExtensions := []string{".apk", ".deb", ".pkg.tar.zst", ".rpm"}
			for _, ext := range skipExtensions {
				if strings.HasSuffix(src, ext) {
					return true, nil
				}
			}

			return false, nil
		},
	}

	for _, proj := range mpc.Projects {
		// Ensure the target directory exists
		err := osutils.ExistsMakeDir(proj.Builder.PKGBUILD.StartDir)
		if err != nil {
			return err
		}

		// Ensure the pkgdir directory exists
		err = osutils.ExistsMakeDir(proj.Builder.PKGBUILD.PackageDir)
		if err != nil {
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

	return nil // Return nil if all operations succeed
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
			osutils.Logger.Warn("failed to remove package directory",
				osutils.Logger.Args("path", proj.Builder.PKGBUILD.PackageDir, "error", err))
		}
	}()

	err := osutils.ExistsMakeDir(mpc.Output)
	if err != nil {
		return err
	}

	err = proj.PackageManager.PrepareFakeroot(mpc.Output)
	if err != nil {
		return err
	}

	pkgLogger := osutils.WithComponent(proj.Builder.PKGBUILD.PkgName)
	pkgLogger.Info("building resulting package", pkgLogger.Args("pkgver", proj.Builder.PKGBUILD.PkgVer,
		"pkgrel", proj.Builder.PKGBUILD.PkgRel))

	err = proj.PackageManager.BuildPackage(mpc.Output)
	if err != nil {
		return err
	}

	return nil
}

// findPackageInProjects finds a package in the MultipleProject struct.
//
// pkgName: the name of the package to find.
// int: the index of the package if found, else -1.
func (mpc *MultipleProject) findPackageInProjects(pkgName string) int {
	var matchFound bool

	var index int

	for i, proj := range mpc.Projects {
		if pkgName == proj.Builder.PKGBUILD.PkgName {
			matchFound = true
			index = i
		}
	}

	if !matchFound {
		osutils.Logger.Fatal("package not found",
			osutils.Logger.Args("pkgname", pkgName))
	}

	return index
}

// getMakeDeps retrieves the make dependencies for the MultipleProject.
//
// It iterates over each child project and appends their make dependencies
// to the makeDepends slice. The makeDepends slice is then assigned to the
// makeDepends field of the MultipleProject.
func (mpc *MultipleProject) getMakeDeps() {
	for _, child := range mpc.Projects {
		makeDepends = append(makeDepends, child.Builder.PKGBUILD.MakeDepends...)
	}
}

// getRuntimeDeps retrieves the runtime dependencies for the MultipleProject.
// It filters out internal dependencies (packages within the project) and only
// collects external dependencies that need to be installed via package manager.
func (mpc *MultipleProject) getRuntimeDeps() {
	// Create a set of internal package names for filtering
	internalPackages := make(map[string]bool)
	for _, proj := range mpc.Projects {
		internalPackages[proj.Builder.PKGBUILD.PkgName] = true
	}

	// Collect external runtime dependencies
	for _, child := range mpc.Projects {
		for _, dep := range child.Builder.PKGBUILD.Depends {
			depName := strings.Fields(dep)[0] // Extract package name (ignore version constraints)
			// Only add if it's not an internal package
			if !internalPackages[depName] {
				runtimeDepends = append(runtimeDepends, dep)
			}
		}
	}

	if len(runtimeDepends) > 0 {
		osutils.Logger.Info("external runtime dependencies collected",
			osutils.Logger.Args("count", len(runtimeDepends), "dependencies", runtimeDepends))
	}
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

		pkgbuildFile.ComputeArchitecture()
		pkgbuildFile.ValidateMandatoryItems()
		pkgbuildFile.ValidateGeneral()

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

	if osutils.Exists(jsonFilePath) {
		projectFilePath = jsonFilePath
		osutils.Logger.Info("multi-project file found",
			osutils.Logger.Args("path", projectFilePath))
	}

	if osutils.Exists(pkgbuildFilePath) {
		projectFilePath = pkgbuildFilePath
		osutils.Logger.Info("single-project file found",
			osutils.Logger.Args("path", projectFilePath))

		mpc.setSingleProject(path)
	}

	filePath, err := osutils.Open(projectFilePath)
	if err != nil || singleProject {
		return err
	}

	defer func() {
		err := filePath.Close()
		if err != nil {
			osutils.Logger.Warn("failed to close project file",
				osutils.Logger.Args("path", projectFilePath, "error", err))
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
	osutils.Logger.Debug("dependency analysis starting")

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

	osutils.Logger.Debug("dependency analysis complete")
}

// displayPackageDependencyInfo shows dependency information for a specific package.
func (mpc *MultipleProject) displayPackageDependencyInfo(
	proj *Project, packageMap map[string]*Project, runtimeDependencyMap map[string]bool,
) {
	pkgName := proj.Builder.PKGBUILD.PkgName
	pkgVer := proj.Builder.PKGBUILD.PkgVer
	pkgRel := proj.Builder.PKGBUILD.PkgRel

	osutils.Logger.Debug(fmt.Sprintf("📦 %s-%s-%s", pkgName, pkgVer, pkgRel))

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
		var reason string
		if proj.HasToInstall {
			reason = "Will be installed after build (explicitly marked)"
		} else {
			reason = "Will be installed after build (runtime dependency)"
		}

		osutils.Logger.Debug("  └─ " + reason)
	} else {
		osutils.Logger.Debug("  └─ Build only (no installation)")
	}

	osutils.Logger.Debug("")
}

// displayDependencies is a helper function to display dependency information.
func (mpc *MultipleProject) displayDependencies(
	title string, deps []string, packageMap map[string]*Project) {
	osutils.Logger.Debug(fmt.Sprintf("  ├─ %s:", title))

	internalDeps := make([]string, 0)
	externalDeps := make([]string, 0)

	for _, dep := range deps {
		depName := strings.Fields(dep)[0] // Extract package name (ignore version constraints)
		if _, exists := packageMap[depName]; exists {
			internalDeps = append(internalDeps, dep+" (internal)")
		} else {
			externalDeps = append(externalDeps, dep+" (external)")
		}
	}

	// Display internal dependencies
	for i, dep := range internalDeps {
		prefix := PrefixMiddle
		if i == len(internalDeps)-1 && len(externalDeps) == 0 {
			prefix = PrefixLast
		}

		osutils.Logger.Debug(fmt.Sprintf("%s %s", prefix, dep))
	}

	// Display external dependencies
	for i, dep := range externalDeps {
		prefix := PrefixMiddle
		if i == len(externalDeps)-1 {
			prefix = PrefixLast
		}

		osutils.Logger.Debug(fmt.Sprintf("%s %s", prefix, dep))
	}
}

// resolveDependencies builds a dependency graph and returns projects in topologically sorted order.
// Returns slices of project batches that can be built in parallel within each batch.
func (mpc *MultipleProject) resolveDependencies() ([][]*Project, error) {
	osutils.Logger.Info("analyzing package dependencies")

	// Build dependency graph
	projectMap := make(map[string]*Project)
	dependsOn := make(map[string][]string)
	dependedBy := make(map[string][]string)

	// Index projects by package name
	for _, proj := range mpc.Projects {
		pkgName := proj.Builder.PKGBUILD.PkgName
		projectMap[pkgName] = proj
		dependsOn[pkgName] = make([]string, 0)
		dependedBy[pkgName] = make([]string, 0)
	}

	osutils.Logger.Info(
		"building dependency graph",
		osutils.Logger.Args("total_packages", len(projectMap)))

	// Build dependency relationships
	totalDeps := 0

	for _, proj := range mpc.Projects {
		pkgName := proj.Builder.PKGBUILD.PkgName
		packageDeps := make([]string, 0)

		// Debug: show what dependencies each package declares
		allDeps := make(
			[]string,
			0,
			len(proj.Builder.PKGBUILD.Depends)+len(proj.Builder.PKGBUILD.MakeDepends))
		allDeps = append(allDeps, proj.Builder.PKGBUILD.Depends...)
		allDeps = append(allDeps, proj.Builder.PKGBUILD.MakeDepends...)

		if len(allDeps) > 0 {
			osutils.Logger.Info("package declares dependencies",
				osutils.Logger.Args("package", pkgName, "all_dependencies", allDeps))
		}

		// Check runtime dependencies
		for _, dep := range proj.Builder.PKGBUILD.Depends {
			depName := strings.Fields(dep)[0] // Extract package name (ignore version constraints)
			if _, exists := projectMap[depName]; exists {
				dependsOn[pkgName] = append(dependsOn[pkgName], depName)
				dependedBy[depName] = append(dependedBy[depName], pkgName)
				packageDeps = append(packageDeps, depName+" (runtime)")
				totalDeps++
			}
		}

		// Check make dependencies (build-time dependencies)
		for _, dep := range proj.Builder.PKGBUILD.MakeDepends {
			depName := strings.Fields(dep)[0] // Extract package name (ignore version constraints)
			if _, exists := projectMap[depName]; exists {
				dependsOn[pkgName] = append(dependsOn[pkgName], depName)
				dependedBy[depName] = append(dependedBy[depName], pkgName)
				packageDeps = append(packageDeps, depName+" (make)")
				totalDeps++
			}
		}

		if len(packageDeps) > 0 {
			osutils.Logger.Info("package dependencies found",
				osutils.Logger.Args("package", pkgName, "depends_on", packageDeps))
		}
	}

	osutils.Logger.Info("dependency analysis complete",
		osutils.Logger.Args("total_internal_dependencies", totalDeps))

	// Perform topological sort using Kahn's algorithm
	return mpc.topologicalSort(projectMap, dependsOn, dependedBy)
}

// topologicalSort performs Kahn's algorithm to sort projects by dependencies.
// Returns batches of projects that can be built in parallel within each batch.
// Fundamental packages (those depended on by many others) are prioritized within each batch.
func (mpc *MultipleProject) topologicalSort(projectMap map[string]*Project,
	dependsOn map[string][]string, dependedBy map[string][]string,
) ([][]*Project, error) {
	osutils.Logger.Info("performing topological sort for build order")

	// Calculate dependency popularity to identify fundamental packages
	popularity := mpc.calculateDependencyPopularity()

	osutils.Logger.Debug("dependency popularity analysis:")

	for pkgName, count := range popularity {
		if count > 0 {
			osutils.Logger.Debug(fmt.Sprintf("  %s: depended on by %d packages", pkgName, count))
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

			osutils.Logger.Error("circular dependency detected",
				osutils.Logger.Args("remaining_packages", problematicPackages))

			return nil, fmt.Errorf("%w: %v", ErrCircularDependency, problematicPackages)
		}

		osutils.Logger.Debug("build batch determined",
			osutils.Logger.Args("batch_number", batchNum,
				"batch_size", len(currentBatch),
				"packages", batchPackages,
				"parallel_workers", min(runtime.NumCPU(), len(currentBatch))))

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

	osutils.Logger.Info("build order determined",
		osutils.Logger.Args("total_batches", len(result), "total_packages", len(projectMap)))

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
			for _, dep := range proj.Builder.PKGBUILD.Depends {
				fields := strings.Fields(dep)
				if len(fields) == 0 {
					continue
				}

				depName := fields[0] // Extract package name (ignore version constraints)
				if _, exists := packageMap[depName]; exists {
					dependencyMap[depName] = true
				}
			}
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
		for _, dep := range proj.Builder.PKGBUILD.Depends {
			depName := strings.Fields(dep)[0]
			if _, exists := packageMap[depName]; exists {
				popularity[depName]++
			}
		}
		// Count make dependencies
		for _, dep := range proj.Builder.PKGBUILD.MakeDepends {
			depName := strings.Fields(dep)[0]
			if _, exists := packageMap[depName]; exists {
				popularity[depName]++
			}
		}
	}

	return popularity
}

// buildBatchWithDependencyInstall builds a batch of projects with immediate installation
// of runtime dependencies.
// Runtime dependencies are built in parallel when they don't depend on each other.
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
	osutils.ServiceLogger().Info("batch build strategy",
		osutils.Logger.Args("runtime_dependencies", len(runtimeDeps),
			"regular_packages", len(regularPackages),
			"batch_number", batchNumber))

	// Phase 1: Build and install runtime dependencies first
	err := mpc.buildAndInstallRuntimeDeps(runtimeDeps, maxWorkers)
	if err != nil {
		return err
	}

	// Phase 2: Build and install regular packages
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
func (mpc *MultipleProject) buildAndInstallRegularPackages(
	regularPackages []*Project, maxWorkers int) error {
	if len(regularPackages) == 0 {
		return nil
	}

	osutils.Logger.Info(
		"building regular packages",
		osutils.Logger.Args("count", len(regularPackages)))

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
			osutils.Logger.Info("stopping build at target package",
				osutils.Logger.Args("target_package", ToPkgName))

			return nil // Use a sentinel error or other mechanism if specific exit is needed
		}
	}

	return nil
}

// installPackage installs a single package.
func (mpc *MultipleProject) installPackage(proj *Project) error {
	pkgName := proj.Builder.PKGBUILD.PkgName
	osutils.Logger.Info("installing package", osutils.Logger.Args("package", pkgName))

	err := proj.PackageManager.Install(mpc.Output)
	if err != nil {
		osutils.Logger.Error(
			"package installation failed",
			osutils.Logger.Args("package", pkgName, "error", err))

		return err
	}

	osutils.Logger.Info("package installed", osutils.Logger.Args("package", pkgName))

	return nil
}

// buildRuntimeDependenciesInOrder builds runtime dependencies in dependency-aware parallel batches.
// Independent runtime dependencies can build in parallel, but dependent ones wait for their
// dependencies.
func (mpc *MultipleProject) buildRuntimeDependenciesInOrder(
	runtimeDeps []*Project, maxWorkers int) error {
	serviceLogger := osutils.ServiceLogger()
	serviceLogger.Info(
		"runtime dependencies build optimization",
		osutils.Logger.Args("count", len(runtimeDeps)))

	// Build dependency graph for runtime dependencies only
	runtimeProjectMap, runtimeDependsOn, runtimeDependedBy :=
		mpc.buildRuntimeDependencyGraph(runtimeDeps)

	// Perform topological sort on runtime dependencies
	runtimeBatches, err :=
		mpc.topologicalSortRuntimeDeps(runtimeProjectMap, runtimeDependsOn, runtimeDependedBy)
	if err != nil {
		return err
	}

	osutils.Logger.Info(
		"runtime dependencies batching complete",
		osutils.Logger.Args("batches", len(runtimeBatches)))

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
		runtimeDependsOn[pkgName] = make([]string, 0)
		runtimeDependedBy[pkgName] = make([]string, 0)
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
		depName := strings.Fields(dep)[0]
		if _, exists := runtimeProjectMap[depName]; exists {
			runtimeDependsOn[pkgName] = append(runtimeDependsOn[pkgName], depName)
			runtimeDependedBy[depName] = append(runtimeDependedBy[depName], pkgName)
			osutils.Logger.Info(
				"runtime dependency found",
				osutils.Logger.Args("dependent", pkgName, "dependency", depName))
		}
	}

	// Check make dependencies
	for _, dep := range proj.Builder.PKGBUILD.MakeDepends {
		depName := strings.Fields(dep)[0]
		if _, exists := runtimeProjectMap[depName]; exists {
			runtimeDependsOn[pkgName] = append(runtimeDependsOn[pkgName], depName)
			runtimeDependedBy[depName] = append(runtimeDependedBy[depName], pkgName)
			osutils.Logger.Info(
				"make dependency found",
				osutils.Logger.Args("dependent", pkgName, "dependency", depName))
		}
	}
}

// buildAndInstallRuntimeBatches builds and installs runtime dependency batches.
func (mpc *MultipleProject) buildAndInstallRuntimeBatches(
	runtimeBatches [][]*Project, maxWorkers int) error {
	serviceLogger := osutils.ServiceLogger()

	// Build and install each batch of runtime dependencies
	for batchIndex, batch := range runtimeBatches {
		batchSize := len(batch)

		var batchPackages []string
		for _, proj := range batch {
			batchPackages = append(batchPackages, proj.Builder.PKGBUILD.PkgName)
		}

		serviceLogger.Info("building runtime dependency batch",
			osutils.Logger.Args("batch", batchIndex+1,
				"parallel_packages", batchSize,
				"packages", batchPackages))

		// Build this batch in parallel
		err := mpc.buildProjectsParallel(batch, min(maxWorkers, batchSize))
		if err != nil {
			return err
		}

		// Install all packages in this batch immediately after building
		err = mpc.installRuntimeBatch(batch)
		if err != nil {
			return err
		}
	}

	return nil
}

// installRuntimeBatch installs all projects in a runtime dependency batch.
func (mpc *MultipleProject) installRuntimeBatch(batch []*Project) error {
	for _, proj := range batch {
		pkgName := proj.Builder.PKGBUILD.PkgName

		if !NoBuild {
			osutils.Logger.Info("installing runtime dependency", osutils.Logger.Args("package", pkgName))

			err := proj.PackageManager.Install(mpc.Output)
			if err != nil {
				osutils.Logger.Error("runtime dependency installation failed",
					osutils.Logger.Args("package", pkgName, "error", err))

				return err
			}

			osutils.Logger.Info("runtime dependency installed", osutils.Logger.Args("package", pkgName))
		}

		if ToPkgName != "" && pkgName == ToPkgName {
			osutils.Logger.Info("stopping build at target package",
				osutils.Logger.Args("target_package", ToPkgName))

			return nil
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

		osutils.Logger.Info(fmt.Sprintf("runtime dependency batch %d: %d packages can build in parallel",
			batchNum, len(currentBatch)))

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

	osutils.Logger.Info("dependency-aware build process starting")
	osutils.Logger.Info("runtime dependency map:")

	for pkgName, isRuntimeDep := range runtimeDependencyMap {
		if isRuntimeDep {
			osutils.Logger.Info(fmt.Sprintf("  %s -> WILL BE INSTALLED (runtime dependency)", pkgName))
		}
	}

	osutils.Logger.Info("starting dependency-aware build process",
		osutils.Logger.Args("total_batches", len(buildOrder),
			"total_packages", totalPackages,
			"max_parallel_workers", maxWorkers))

	processedPackages := 0

	for batchIndex, batch := range buildOrder {
		// Filter batch based on FromPkgName and ToPkgName
		filteredBatch := mpc.filterBatch(batch)
		if len(filteredBatch) == 0 {
			osutils.Logger.Debug("skipping batch (filtered out)",
				osutils.Logger.Args("batch_number", batchIndex+1))

			continue
		}

		batchWorkers := min(maxWorkers, len(filteredBatch))

		var batchPackages []string
		for _, proj := range filteredBatch {
			batchPackages = append(batchPackages, proj.Builder.PKGBUILD.PkgName)
		}

		osutils.Logger.Debug("processing build batch",
			osutils.Logger.Args("batch_number", batchIndex+1,
				"batch_size", len(filteredBatch),
				"packages", batchPackages,
				"parallel_workers", batchWorkers,
				"progress", fmt.Sprintf("%d/%d packages", processedPackages, totalPackages)))

		// Build current batch in parallel - but handle runtime dependencies specially
		err := mpc.buildBatchWithDependencyInstall(
			filteredBatch,
			batchWorkers,
			runtimeDependencyMap,
			batchIndex+1)
		if err != nil {
			return err
		}
	}

	return nil
}

// filterBatch filters a batch of projects based on FromPkgName criteria.
func (mpc *MultipleProject) filterBatch(batch []*Project) []*Project {
	if FromPkgName == "" {
		return batch
	}

	var filtered []*Project

	startProcessing := false

	for _, proj := range batch {
		if proj.Builder.PKGBUILD.PkgName == FromPkgName {
			startProcessing = true
		}

		if startProcessing {
			filtered = append(filtered, proj)
		}
	}

	return filtered
}
