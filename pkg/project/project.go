// Package project provides multi-package project management and build orchestration.
package project

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/go-playground/validator/v10"

	"github.com/M0Rf30/yap/v2/pkg/aptcache"
	"github.com/M0Rf30/yap/v2/pkg/builder"
	yerrors "github.com/M0Rf30/yap/v2/pkg/errors"
	"github.com/M0Rf30/yap/v2/pkg/files"
	"github.com/M0Rf30/yap/v2/pkg/i18n"
	"github.com/M0Rf30/yap/v2/pkg/logger"
	"github.com/M0Rf30/yap/v2/pkg/options"
	"github.com/M0Rf30/yap/v2/pkg/packer"
	"github.com/M0Rf30/yap/v2/pkg/pkgbuild"
	"github.com/M0Rf30/yap/v2/pkg/platform"
	"github.com/M0Rf30/yap/v2/pkg/repo"
	"github.com/M0Rf30/yap/v2/pkg/signing"
)

var (
	// ErrCircularDependency indicates a circular dependency was detected.
	// Static errors for linting compliance.
	ErrCircularDependency = errors.New(i18n.T("errors.project.circular_dependency_detected"))
	// ErrCircularRuntimeDependency indicates a circular runtime dependency was detected.
	ErrCircularRuntimeDependency = errors.New(
		i18n.T("errors.project.circular_runtime_dependency_detected"))
	// jsonValidator is a package-level singleton to avoid repeated reflection setup.
	jsonValidator = validator.New()
)

// BuildOptions holds all build configuration options.
// Use Apply() to propagate these options to the package-level globals.
type BuildOptions struct {
	Verbose                 bool
	CleanBuild              bool
	NoBuild                 bool
	NoMakeDeps              bool
	SkipSyncDeps            bool
	SkipDeps                []string
	SkipToolchainValidation bool
	Zap                     bool
	Parallel                bool
	SBOM                    bool
	FromPkgName             string
	ToPkgName               string
	TargetArch              string
	OnlyPkgNames            string
	SkipPkgNames            string
	DebugDir                string
	SBOMFormat              string
	ExtraRepos              []string
	// AllowUnverifiedRepos bypasses the OpenPGP signature check on apt
	// `Release` / `InRelease` files. Required when the source declares no
	// `Signed-By` directive and no usable key exists in the default trust
	// paths (`/etc/apt/trusted.gpg.d`, `/usr/share/keyrings`,
	// `/etc/apt/keyrings`, `/etc/apt/trusted.gpg`). A signature that
	// exists and fails to verify is *always* fatal, regardless of this
	// flag — a forged signature is strictly worse than no signature.
	AllowUnverifiedRepos bool
	// SkipHashCheck disables sha256/sha512 integrity verification of
	// downloaded source files. Equivalent to setting every checksum to
	// SKIP in the PKGBUILD. Useful during development when iterating on
	// sources before finalising checksums.
	SkipHashCheck bool
	// NoCheck skips the PKGBUILD check() function, mirroring makepkg's
	// --nocheck. Useful when test suites are slow or require resources
	// unavailable in the build environment.
	NoCheck bool
}

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

	// Split on version operators that may appear without spaces (e.g. "pkg>1.0", "pkg>=2.0").
	name := fields[0]
	if idx := strings.IndexAny(name, "><=!"); idx != -1 {
		name = name[:idx]
	}

	return name
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

// MultipleProject represents a collection of projects.
//
// It contains a slice of Project objects and provides methods to interact
// with multiple projects. The methods include building all the projects,
// finding a package in the projects, and creating packages for each project.
// The MultipleProject struct also contains the output directory for the
// packages and the ToPkgName field, which can be used to stop the build
// process after a specific package.
type MultipleProject struct {
	BuildDir       string          `json:"buildDir"       validate:"required"`
	Description    string          `json:"description"    validate:"required"`
	Name           string          `json:"name"           validate:"required"`
	Output         string          `json:"output"         validate:"required"`
	Projects       []*Project      `json:"projects"       validate:"required,dive,required"`
	CompressionDeb string          `json:"compressionDeb" validate:""`
	CompressionRpm string          `json:"compressionRpm" validate:""`
	Signing        *signing.Config `json:"signing,omitempty"`
	Repos          []repo.Repo     `json:"repos,omitempty"`
	SkipDeps       []string        `json:"skipDeps,omitempty"`
	TargetArch     string          `json:"targetArch,omitempty"`
	DebugDir       string          `json:"debugDir,omitempty"`
	Parallel       bool            `json:"parallel,omitempty"`
	SBOM           bool            `json:"sbom,omitempty"`
	SBOMFormat     string          `json:"sbomFormat,omitempty"`
	// Opts holds build configuration options (not JSON-serialized; set by caller)
	Opts BuildOptions
	// Execution state (not JSON-serialized)
	singleProject  bool
	packageManager packer.Packer
	makeDepends    []string
	runtimeDepends []string
	// allProjects holds the complete project list before any --only/--skip/--from/--to
	// filtering. Used by getRuntimeDeps to correctly identify internal packages so
	// that filtered-out packages are not mistakenly downloaded from apt.
	allProjects []*Project
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
	Signing        *signing.Config
}

// BuildAll builds all projects in the correct order.
// When Parallel is false (default), projects are built sequentially in file order;
// packages with HasToInstall set are installed immediately after being built.
// When Parallel is true, a topological sort determines build order and packages
// are built concurrently using a worker pool.
// The context is used for cancellation (e.g., Ctrl-C / SIGTERM).
func (mpc *MultipleProject) BuildAll(ctx context.Context) error {
	if !mpc.singleProject {
		if err := mpc.checkPkgsRange(mpc.Opts.FromPkgName, mpc.Opts.ToPkgName); err != nil {
			return err
		}
	}

	// Show verbose dependency information at debug level regardless of build mode.
	// In sequential mode this is informational only; resolution happens in the parallel path.
	// Gated on verbose/debug logging: the call allocates a package map plus a runtime
	// dependency map and iterates every project — wasted work when the logs would
	// be discarded anyway.
	if logger.IsVerboseEnabled() {
		mpc.displayVerboseDependencyInfo()
	}

	// Filter projects based on --from and --to flags before building
	projectsToProcess := mpc.getProjectsInRange()

	if !mpc.Opts.Parallel {
		// Default: sequential build in file order.
		// Packages with "install": true are installed immediately after building.
		return mpc.buildProjectsSequential(ctx, projectsToProcess)
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
		if mpc.Opts.CleanBuild {
			err := os.RemoveAll(proj.Builder.PKGBUILD.SourceDir)
			if err != nil {
				return err
			}
		}

		if mpc.Opts.Zap {
			if err := mpc.cleanZapArtifacts(proj); err != nil {
				return err
			}
		}
	}

	return nil
}

// buildSkipSet merges the yap.json skipDeps and --skip-deps CLI values into
// a single lookup set.
func (mpc *MultipleProject) buildSkipSet() map[string]struct{} {
	total := len(mpc.SkipDeps) + len(mpc.Opts.SkipDeps)
	if total == 0 {
		return nil
	}

	set := make(map[string]struct{}, total)

	for _, d := range mpc.SkipDeps {
		set[d] = struct{}{}
	}

	for _, d := range mpc.Opts.SkipDeps {
		set[d] = struct{}{}
	}

	return set
}

// filterSkipDeps removes any package present in the merged skipDeps set from deps.
func (mpc *MultipleProject) filterSkipDeps(deps []string) []string {
	skip := mpc.buildSkipSet()
	if len(skip) == 0 {
		return deps
	}

	filtered := deps[:0:0]

	for _, d := range deps {
		if _, excluded := skip[d]; excluded {
			logger.Info(i18n.T("logger.project.info.skipping_dependency"), "package", d)
		} else {
			filtered = append(filtered, d)
		}
	}

	return filtered
}

// syncDependencies handles dependency synchronization and preparation.
// It updates the package manager, prepares make dependencies, and installs
// external runtime dependencies.
func (mpc *MultipleProject) syncDependencies(ctx context.Context, makeDepends, runtimeDepends []string) error {
	makeDepends = mpc.filterSkipDeps(makeDepends)
	runtimeDepends = mpc.filterSkipDeps(runtimeDepends)

	if !mpc.Opts.SkipSyncDeps {
		err := mpc.packageManager.Update(ctx)
		if err != nil {
			return err
		}

		// Invalidate the aptcache singleton so that packages from newly added
		// repositories (e.g. --repo flags) are visible to subsequent partition
		// decisions in DownloadAndExtractCrossDeps.
		aptcache.Reload()
	}

	if !mpc.Opts.NoMakeDeps {
		err := mpc.packageManager.Prepare(ctx, makeDepends, mpc.Opts.TargetArch)
		if err != nil {
			return err
		}
	}

	// Install external runtime dependencies.
	//
	// During cross-builds, runtime deps are downloaded and extracted directly
	// to the root filesystem (dpkg -x equivalent) instead of apt-installed.
	// This avoids circular dependency conflicts: arch-all meta-packages
	// depend on arch-specific packages for the host arch, which conflict
	// with the target-arch variants needed for cross-compilation linking.
	if err := mpc.installRuntimeDeps(ctx, runtimeDepends); err != nil {
		return err
	}

	return nil
}

// installRuntimeDeps installs external runtime dependencies. During
// cross-builds it downloads and extracts them to avoid arch conflicts;
// otherwise it delegates to the normal package manager install path.
func (mpc *MultipleProject) installRuntimeDeps(ctx context.Context, runtimeDepends []string) error {
	if mpc.Opts.SkipSyncDeps || len(runtimeDepends) == 0 {
		return nil
	}

	logger.Debug(i18n.T("logger.installing_external_runtime_dependencies"),
		"count", len(runtimeDepends))

	isCrossBuild := mpc.Opts.TargetArch != "" && mpc.Opts.TargetArch != runtime.GOARCH
	if !isCrossBuild {
		return mpc.packageManager.Prepare(ctx, runtimeDepends, mpc.Opts.TargetArch)
	}

	extractor, ok := mpc.packageManager.(packer.CrossDepsExtractor)
	if !ok {
		return mpc.packageManager.Prepare(ctx, runtimeDepends, mpc.Opts.TargetArch)
	}

	return extractor.DownloadAndExtractCrossDeps(ctx, runtimeDepends, mpc.Opts.TargetArch)
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

	if err := mpc.applyJSONDefaults(); err != nil {
		return err
	}

	err = files.ExistsMakeDir(mpc.BuildDir)
	if err != nil {
		return err
	}

	mpc.packageManager, err = packer.GetPackageManager(&pkgbuild.PKGBUILD{}, distro, "", "")
	if err != nil {
		return err
	}

	if err := mpc.setupExtraRepos(distro, release); err != nil {
		return err
	}

	err = mpc.populateProjects(distro, release, path)
	if err != nil {
		return err
	}

	if mpc.Opts.CleanBuild || mpc.Opts.Zap {
		if err := mpc.Clean(); err != nil {
			return err
		}
	}

	err = mpc.copyProjects()
	if err != nil {
		return err
	}

	mpc.makeDepends = mpc.getMakeDeps()
	mpc.runtimeDepends = mpc.getRuntimeDeps()

	ctx := context.Background()

	err = mpc.syncDependencies(ctx, mpc.makeDepends, mpc.runtimeDepends)
	if err != nil {
		return err
	}

	// Resolve output path to absolute once, before any parallel work
	// This prevents data races in createPackage when called from parallel workers
	return mpc.resolveOutputPath()
}

// doInstallOrExtract performs the actual package extraction.
// All packers implement InstallOrExtractor and always extract to sysroot.
func (mpc *MultipleProject) doInstallOrExtract(proj *Project) error {
	installer, ok := proj.PackageManager.(packer.InstallOrExtractor)
	if !ok {
		// This branch is an invariant violation: all BaseBuilder-backed packers implement
		// InstallOrExtractor. If this error fires, a new packer was added without embedding
		// BaseBuilder or implementing InstallOrExtract.
		return yerrors.New(yerrors.ErrTypeInternal, "package manager does not implement InstallOrExtractor").
			WithOperation("doInstallOrExtract")
	}

	return installer.InstallOrExtract(mpc.Output, mpc.BuildDir, mpc.Opts.TargetArch)
}

// installPackageForWorker handles package extraction for a worker.
// It calls doInstallOrExtract, which always extracts to the root filesystem — extraction is
// goroutine-safe.
func (mpc *MultipleProject) installPackageForWorker(proj *Project, pkgName, workerID string) error {
	logger.Info(i18n.T("logger.installing_package"),
		"package", pkgName,
		"worker_id", workerID)

	if err := mpc.doInstallOrExtract(proj); err != nil {
		return err
	}

	logger.Info(i18n.T("logger.package_installed"),
		"package", pkgName,
		"worker_id", workerID)

	return nil
}

// createPackage creates packages for the MultipleProject.
//
// It takes a pointer to a MultipleProject as a receiver and a pointer to a Project as a parameter.
// It returns an error.
// Note: mpc.Output is expected to be an absolute path, resolved once in MultiProject()
// before any parallel workers are launched.
func (mpc *MultipleProject) createPackage(proj *Project) error {
	if err := files.ExistsMakeDir(mpc.Output); err != nil {
		return err
	}

	// Preserve ownership of the output directory so consumers running as the
	// invoking user (e.g. CI agents) can traverse it after a sudo build.
	if err := platform.PreserveOwnership(mpc.Output); err != nil {
		logger.Warn(i18n.T("logger.common.warn.failed_to_get_original"),
			"path", mpc.Output,
			"error", err)
	}

	// Configure debug symbol separation before stripping occurs in PrepareFakeroot.
	if mpc.Opts.DebugDir != "" {
		absDebugDir, absErr := filepath.Abs(mpc.Opts.DebugDir)
		if absErr != nil {
			return absErr
		}

		if mkErr := files.ExistsMakeDir(absDebugDir); mkErr != nil {
			return mkErr
		}

		if err := platform.PreserveOwnership(absDebugDir); err != nil {
			logger.Warn(i18n.T("logger.common.warn.failed_to_get_original"),
				"path", absDebugDir,
				"error", err)
		}

		options.SetDebugDir(absDebugDir)
	}

	if proj.Builder.PKGBUILD.IsSplitPackage() {
		return mpc.createSplitPackages(proj)
	}

	return mpc.createSinglePackage(proj)
}

// createSinglePackage handles the PrepareFakeroot → BuildPackage → post-build
// pipeline for a non-split (single) package.
// Note: This method is called from createPackage which is called from the build
// pipeline. The context is created at the top level in BuildAll and passed through.
// For now, we use context.Background() as a fallback, but ideally this would be
// threaded through the call chain.
func (mpc *MultipleProject) createSinglePackage(proj *Project) error {
	ctx := context.Background()

	defer func() {
		if err := os.RemoveAll(proj.Builder.PKGBUILD.PackageDir); err != nil {
			logger.Warn(i18n.T("logger.failed_to_remove_package_directory"),
				"path", proj.Builder.PKGBUILD.PackageDir,
				"error", err)
		}
	}()

	if err := proj.PackageManager.PrepareFakeroot(ctx, mpc.Output, mpc.Opts.TargetArch); err != nil {
		return err
	}

	logger.Info(i18n.T("logger.building_resulting_package"),
		"package", proj.Builder.PKGBUILD.PkgName,
		"version", proj.Builder.PKGBUILD.PkgVer,
		"release", proj.Builder.PKGBUILD.PkgRel)

	artifactPath, err := proj.PackageManager.BuildPackage(ctx, mpc.Output, mpc.Opts.TargetArch)
	if err != nil {
		return err
	}

	return mpc.runPostBuildHooks(proj, artifactPath)
}

// createSplitPackages iterates over each sub-package produced by a split PKGBUILD
// and runs PrepareFakeroot → BuildPackage → post-build hooks for each one.
func (mpc *MultipleProject) createSplitPackages(proj *Project) error {
	ctx := context.Background()

	for _, subName := range proj.Builder.PKGBUILD.PkgNames {
		// Point the PKGBUILD at this sub-package's install tree.
		proj.Builder.PKGBUILD.PkgName = subName
		proj.Builder.PKGBUILD.SetPackageDirForSplit(subName)

		pkgDir := proj.Builder.PKGBUILD.PackageDir

		// Restore top-level values, then re-apply this sub-package's overrides
		// so PrepareFakeroot sees the correct metadata (pkgdesc, depends, etc.).
		proj.Builder.PKGBUILD.RestoreTopLevelOverrides()

		if funcBody, ok := proj.Builder.PKGBUILD.SplitPackageFuncs[subName]; ok {
			if err := proj.Builder.PKGBUILD.ParseSplitOverrides(funcBody); err != nil {
				logger.Warn(i18n.T("logger.project.warn.failed_parse_split_package"), "subpackage", subName, "error", err)
			}
		}

		if err := proj.PackageManager.PrepareFakeroot(ctx, mpc.Output, mpc.Opts.TargetArch); err != nil {
			return err
		}

		logger.Info(i18n.T("logger.building_resulting_package"),
			"package", subName,
			"version", proj.Builder.PKGBUILD.PkgVer,
			"release", proj.Builder.PKGBUILD.PkgRel)

		artifactPath, err := proj.PackageManager.BuildPackage(ctx, mpc.Output, mpc.Opts.TargetArch)
		if err != nil {
			return err
		}

		if err := mpc.runPostBuildHooks(proj, artifactPath); err != nil {
			return err
		}

		// Clean up this sub-package's install tree.
		if err := os.RemoveAll(pkgDir); err != nil {
			logger.Warn(i18n.T("logger.failed_to_remove_package_directory"),
				"path", pkgDir, "error", err)
		}
	}

	return nil
}

// installPackage installs a single package or extracts it for cross-compilation.
func (mpc *MultipleProject) installPackage(proj *Project) error {
	pkgName := proj.Builder.PKGBUILD.PkgName
	logger.Info(i18n.T("logger.installing_package"), "package", pkgName)

	if err := mpc.doInstallOrExtract(proj); err != nil {
		return err
	}

	logger.Info(i18n.T("logger.package_installed"), "package", pkgName)

	return nil
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
//
// Internal classification (hybrid rule):
//
//   - Packages currently in the build set (mpc.Projects) are always internal.
//   - Packages that were filtered out via --from/--to/--only/--skip are
//     internal only when a built artifact for them is already present in
//     mpc.Output (i.e. a previous run produced it locally). Otherwise they
//     are treated as external so the package manager can fetch them.
//
// This avoids the previous bug where --from <pkg> caused later yap.json
// siblings to be silently dropped from apt downloads even though no local
// artifact for them existed.
func (mpc *MultipleProject) getRuntimeDeps() []string {
	source := mpc.allProjects
	if len(source) == 0 {
		source = mpc.Projects // fallback for single-project or test paths
	}

	// Packages being built in this invocation (post-filter).
	buildingNow := make(map[string]bool)
	for _, proj := range mpc.Projects {
		buildingNow[proj.Builder.PKGBUILD.PkgName] = true
		for _, name := range proj.Builder.PKGBUILD.PkgNames {
			buildingNow[name] = true
		}
	}

	internalPackages := make(map[string]bool)
	for _, proj := range source {
		names := make([]string, 0, 1+len(proj.Builder.PKGBUILD.PkgNames))
		names = append(names, proj.Builder.PKGBUILD.PkgName)
		names = append(names, proj.Builder.PKGBUILD.PkgNames...)

		for _, name := range names {
			switch {
			case buildingNow[name]:
				internalPackages[name] = true
			case mpc.localArtifactExists(name):
				internalPackages[name] = true
				logger.Debug(i18n.T("logger.project.debug.treating_filtered_package_as"), "package", name, "output", mpc.Output)
			default:
				logger.Debug(i18n.T("logger.project.debug.treating_filtered_package_as_external"),
					"package", name, "output", mpc.Output)
			}
		}
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

// localArtifactExists reports whether a built package artifact for pkgName
// exists in mpc.Output. It uses a digit-anchored glob (`name-[0-9]*` and
// `name_[0-9]*`) so e.g. `foo-curl` does not falsely match
// `foo-curl-dev-1.0.0...`. Recognised extensions cover deb, rpm, apk
// and pacman (zst/xz/gz).
func (mpc *MultipleProject) localArtifactExists(pkgName string) bool {
	if mpc.Output == "" || pkgName == "" {
		return false
	}

	patterns := []string{
		pkgName + "-[0-9]*",
		pkgName + "_[0-9]*",
	}

	for _, pat := range patterns {
		matches, err := filepath.Glob(filepath.Join(mpc.Output, pat))
		if err != nil {
			continue
		}

		for _, m := range matches {
			if _, ok := signingFormatForArtifact(m); ok {
				return true
			}
		}
	}

	return false
}

// cleanZapArtifacts removes build artifacts for a project when Zap is enabled
func (mpc *MultipleProject) cleanZapArtifacts(proj *Project) error {
	// For single projects, StartDir is the actual project directory containing
	// source files and PKGBUILD, so we should NOT remove it. Only remove
	// build artifacts within it.
	if mpc.singleProject {
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
