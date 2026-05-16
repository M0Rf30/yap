// Package builder provides build orchestration functionality for YAP packages.
package builder

import (
	"sync"

	"github.com/M0Rf30/yap/v2/pkg/builders/common"
	"github.com/M0Rf30/yap/v2/pkg/constants"
	"github.com/M0Rf30/yap/v2/pkg/errors"
	"github.com/M0Rf30/yap/v2/pkg/files"
	"github.com/M0Rf30/yap/v2/pkg/i18n"
	"github.com/M0Rf30/yap/v2/pkg/logger"
	"github.com/M0Rf30/yap/v2/pkg/pkgbuild"
	"github.com/M0Rf30/yap/v2/pkg/platform"
	"github.com/M0Rf30/yap/v2/pkg/shell"
	"github.com/M0Rf30/yap/v2/pkg/source"
)

// Builder maps PKGBUILD to generic functions aimed at artifacts generation.
type Builder struct {
	PKGBUILD *pkgbuild.PKGBUILD
}

// Compile manages all the instructions that lead to a single project artifact.
// It returns any error if occurred.
func (builder *Builder) Compile(noBuild bool) error {
	pkgName := builder.PKGBUILD.PkgName
	pkgVer := builder.PKGBUILD.PkgVer
	pkgRel := builder.PKGBUILD.PkgRel

	err := builder.initDirs()
	if err != nil {
		return errors.Wrap(err, errors.ErrTypeBuild,
			i18n.T("errors.build.failed_to_initialize_directories")).
			WithContext("package", pkgName).
			WithContext("version", pkgVer).
			WithContext("release", pkgRel).
			WithOperation("initDirs")
	}

	logger.Info(i18n.T("logger.retrieving_sources"),
		"pkgver", builder.PKGBUILD.PkgVer,
		"pkgrel", builder.PKGBUILD.PkgRel)

	err = builder.getSources()
	if err != nil {
		return errors.Wrap(err, errors.ErrTypeBuild, i18n.T("errors.build.failed_to_retrieve_sources")).
			WithContext("package", pkgName).
			WithContext("version", pkgVer).
			WithContext("release", pkgRel).
			WithOperation("getSources")
	}

	// After extraction, recursively restore ownership of the source tree to the
	// original (pre-sudo) user. initDirs only chowned the src/ directory itself;
	// the tarball contents are extracted as root and must be fixed here so the
	// build script can write into them (e.g. Configure creating doc/html/).
	if err := platform.PreserveOwnershipRecursive(builder.PKGBUILD.SourceDir); err != nil {
		logger.Warn(i18n.T("logger.failed_to_preserve_ownership"),
			"path", builder.PKGBUILD.SourceDir, "error", err)
	}

	if !noBuild {
		return builder.runBuildStages()
	}

	return nil
}

// processFunction processes the given pkgbuildFunction with consistent logging and error handling.
//
// It takes three parameters: pkgbuildFunction string, message string, stage string.
// It returns an error if the build stage fails.
func (builder *Builder) processFunction(pkgbuildFunction, message, stage string) error {
	if pkgbuildFunction == "" {
		return nil
	}

	pkgName := builder.PKGBUILD.PkgName
	pkgVer := builder.PKGBUILD.PkgVer
	pkgRel := builder.PKGBUILD.PkgRel

	// Build a per-package environment slice without mutating os.Setenv.
	// This is safe to call concurrently from multiple parallel-build goroutines
	// because it does not touch the global process environment.
	pkgEnv := builder.PKGBUILD.BuildEnvironmentSlice()

	// Use logger for consistent formatting
	logger.Info(i18n.T(message), "pkgver", pkgVer, "pkgrel", pkgRel)

	// Set up ccache for the build stage if ccache is available.
	// SetupCcache / SetupCrossCompilationEnvironment still call os.Setenv, so
	// capture the resulting env *after* they run and before the shell interpreter
	// snapshots it, reducing (but not fully eliminating) the race window for
	// cross-compilation / ccache vars.  A full fix for those helpers is tracked
	// separately.
	if stage == "build" {
		// Create a temporary BaseBuilder to access the SetupCcache and
		// SetupCrossCompilationEnvironment methods
		format := constants.DistroFormat(builder.PKGBUILD.Distro)
		tempBuilder := &common.BaseBuilder{
			PKGBUILD: builder.PKGBUILD,
			Format:   format,
		}

		err := tempBuilder.SetupCcache()
		if err != nil {
			logger.Warn(i18n.T("logger.setupccache.warn.ccache_setup_failed_1"),
				"package", pkgName, "error", err)
		}

		// Set up cross-compilation environment if target architecture is specified
		if builder.PKGBUILD.IsCrossCompilation() {
			err = tempBuilder.SetupCrossCompilationEnvironment(builder.PKGBUILD.TargetArch)
			if err != nil {
				logger.Warn(i18n.T("logger.cross_compilation.cross_compilation_environment_setup_failed"),
					"package", pkgName, "target_arch", builder.PKGBUILD.TargetArch, "error", err)
			}
		}
	}

	// Build preamble: custom scalar variables, custom arrays, and helper function
	// definitions (e.g. _package, _package_systemd, _install_files).  The preamble
	// is prepended to every script so that helper callers in build/package/prepare
	// bodies resolve at runtime instead of failing with "not found in $PATH".
	preamble := builder.PKGBUILD.BuildScriptPreamble()

	// Execute script with package decoration.
	// set -x traces each command to stderr before execution so that when a
	// command fails we can see which one it was (mvdan/sh builtins like cd
	// produce no output on failure — only an exit status).
	// Pass pkgEnv so the interpreter receives per-package dirs/names without
	// relying on the (racy) global os environment.
	err := shell.RunScriptWithPackage("  set -e\n  set -x\n"+preamble+pkgbuildFunction, pkgName, pkgEnv)
	if err != nil {
		return errors.Wrap(err, errors.ErrTypeBuild, i18n.T("errors.build.build_stage_failed")).
			WithContext("package", pkgName).
			WithContext("version", pkgVer).
			WithContext("release", pkgRel).
			WithContext("stage", stage).
			WithOperation(stage)
	}

	return nil
}

// runBuildStages executes prepare → build → package (or split-package) stages.
func (builder *Builder) runBuildStages() error {
	if err := builder.processFunction(builder.PKGBUILD.Prepare, "logger.preparing_sources", "prepare"); err != nil {
		return err
	}

	if err := builder.processFunction(builder.PKGBUILD.Build, "logger.building", "build"); err != nil {
		return err
	}

	if builder.PKGBUILD.IsSplitPackage() {
		return builder.compileSplitPackages()
	}

	return builder.processFunctionInFakeroot(builder.PKGBUILD.Package, "logger.generating_package", "package")
}

// compileSplitPackages runs the package_<name>() function for each sub-package
// defined in a split PKGBUILD (pkgname=('foo' 'bar' ...)). Each sub-package
// gets its own PackageDir so the install trees are kept separate.
func (builder *Builder) compileSplitPackages() error {
	pkgVer := builder.PKGBUILD.PkgVer
	pkgRel := builder.PKGBUILD.PkgRel

	for _, subName := range builder.PKGBUILD.PkgNames {
		// Look up package_<name>() from the dedicated split-package function map.
		funcBody, ok := builder.PKGBUILD.SplitPackageFuncs[subName]

		if !ok {
			// Fall back to the shared package() if no per-package function exists.
			if builder.PKGBUILD.Package == "" {
				logger.Warn("no package function found for split sub-package, skipping",
					"subpackage", subName)

				continue
			}

			funcBody = builder.PKGBUILD.Package
		}

		// Point PackageDir at the sub-package's own directory.
		builder.PKGBUILD.SetPackageDirForSplit(subName)
		builder.PKGBUILD.PkgName = subName

		if err := files.ExistsMakeDir(builder.PKGBUILD.PackageDir); err != nil {
			return errors.Wrap(err, errors.ErrTypeBuild,
				i18n.T("errors.build.failed_to_initialize_directories")).
				WithContext("package", subName).
				WithContext("version", pkgVer).
				WithContext("release", pkgRel).
				WithOperation("compileSplitPackages")
		}

		logger.Info(i18n.T("logger.generating_package"),
			"pkgver", pkgVer, "pkgrel", pkgRel, "subpackage", subName)

		preamble := builder.PKGBUILD.BuildScriptPreamble()
		pkgEnv := builder.PKGBUILD.BuildEnvironmentSlice()

		// Statically parse the function body to extract per-package variable overrides
		// (pkgdesc, depends, conflicts, etc.) before running the script.
		// This uses the same AddItem/parseDirective path as top-level PKGBUILD parsing,
		// giving full __distro and _arch suffix support for free.
		if err := builder.PKGBUILD.ParseSplitOverrides(funcBody); err != nil {
			logger.Warn("failed to parse split-package overrides, using global values",
				"subpackage", subName, "error", err)
		}

		if err := shell.RunScriptInFakeroot(
			"  set -e\n  set -x\n"+preamble+funcBody, subName, pkgEnv,
		); err != nil {
			return errors.Wrap(err, errors.ErrTypeBuild, i18n.T("errors.build.build_stage_failed")).
				WithContext("package", subName).
				WithContext("version", pkgVer).
				WithContext("release", pkgRel).
				WithContext("stage", "package").
				WithOperation("compileSplitPackages")
		}
	}

	return nil
}

// processFunctionInFakeroot is identical to processFunction but executes the script
// inside a Linux user-namespace fakeroot so that ownership operations such as
// `install -o root -g root` succeed without real root privileges.
// Use this for the package() stage.
func (builder *Builder) processFunctionInFakeroot(pkgbuildFunction, message, stage string) error {
	if pkgbuildFunction == "" {
		return nil
	}

	pkgName := builder.PKGBUILD.PkgName
	pkgVer := builder.PKGBUILD.PkgVer
	pkgRel := builder.PKGBUILD.PkgRel

	pkgEnv := builder.PKGBUILD.BuildEnvironmentSlice()

	logger.Info(i18n.T(message), "pkgver", pkgVer, "pkgrel", pkgRel)

	// Propagate cross-compilation environment to the package() stage so that
	// tools like strip/objcopy use the cross-prefixed variants when packaging
	// cross-compiled binaries.
	if builder.PKGBUILD.IsCrossCompilation() {
		format := constants.DistroFormat(builder.PKGBUILD.Distro)
		tempBuilder := &common.BaseBuilder{
			PKGBUILD: builder.PKGBUILD,
			Format:   format,
		}

		if err := tempBuilder.SetupCrossCompilationEnvironment(builder.PKGBUILD.TargetArch); err != nil {
			logger.Warn(i18n.T("logger.cross_compilation.cross_compilation_environment_setup_failed"),
				"package", pkgName, "target_arch", builder.PKGBUILD.TargetArch, "error", err)
		}
	}

	preamble := builder.PKGBUILD.BuildScriptPreamble()

	err := shell.RunScriptInFakeroot("  set -e\n  set -x\n"+preamble+pkgbuildFunction, pkgName, pkgEnv)
	if err != nil {
		return errors.Wrap(err, errors.ErrTypeBuild, i18n.T("errors.build.build_stage_failed")).
			WithContext("package", pkgName).
			WithContext("version", pkgVer).
			WithContext("release", pkgRel).
			WithContext("stage", stage).
			WithOperation(stage)
	}

	return nil
}

// getSources detects sources provided by a single project source array and
// downloads them in parallel with enhanced progress tracking. It returns any error if occurred.
func (builder *Builder) getSources() error {
	if len(builder.PKGBUILD.SourceURI) == 0 {
		return nil
	}

	pkgName := builder.PKGBUILD.PkgName
	pkgVer := builder.PKGBUILD.PkgVer
	pkgRel := builder.PKGBUILD.PkgRel

	// Process all sources using the original method with enhanced downloads
	maxWorkers := min(len(builder.PKGBUILD.SourceURI), 4)

	type sourceTask struct {
		index  int
		source source.Source
	}

	sourceChan := make(chan sourceTask, len(builder.PKGBUILD.SourceURI))
	errorChan := make(chan error, len(builder.PKGBUILD.SourceURI))

	var waitGroup sync.WaitGroup

	// Start workers for source processing (including enhanced downloads)
	for range maxWorkers {
		waitGroup.Go(func() {
			for task := range sourceChan {
				err := task.source.Get()
				if err != nil {
					wrappedErr := errors.Wrap(err, errors.ErrTypeBuild,
						i18n.T("errors.build.failed_to_retrieve_source")).
						WithContext("package", pkgName).
						WithContext("version", pkgVer).
						WithContext("release", pkgRel).
						WithContext("source_index", task.index).
						WithContext("source_uri", task.source.SourceItemURI).
						WithOperation("source_processing")
					errorChan <- wrappedErr

					return
				}
			}
		})
	}

	// Send tasks to workers
	go func() {
		defer close(sourceChan)

		for index, sourceURI := range builder.PKGBUILD.SourceURI {
			sourceObj := source.Source{
				StartDir:       builder.PKGBUILD.StartDir,
				Hash:           builder.PKGBUILD.HashSums[index],
				NoExtract:      builder.PKGBUILD.NoExtract,
				PkgName:        builder.PKGBUILD.PkgName,
				SourceItemURI:  sourceURI,
				SrcDir:         builder.PKGBUILD.SourceDir,
				SourceItemPath: "",
			}

			sourceChan <- sourceTask{
				index:  index,
				source: sourceObj,
			}
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

// initDirs creates mandatory fakeroot folders (src, pkg) for a single project.
// It returns any error if occurred.
func (builder *Builder) initDirs() error {
	err := files.ExistsMakeDir(builder.PKGBUILD.SourceDir)
	if err != nil {
		return err
	}

	return nil
}
