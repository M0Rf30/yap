// Package builder provides build orchestration functionality for YAP packages.
package builder

import (
	"context"
	"runtime"

	"golang.org/x/sync/errgroup"

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

// scriptPrologue mirrors makepkg's run_function(): every PKGBUILD function
// (prepare/build/check/package) is executed with errexit and xtrace enabled
// and an implicit `cd "${srcdir}"`, so function bodies can assume they start
// inside the source directory exactly like under makepkg.
const scriptPrologue = "  set -e\n  set -x\n  cd \"${srcdir}\"\n"

// Builder maps PKGBUILD to generic functions aimed at artifacts generation.
type Builder struct {
	PKGBUILD      *pkgbuild.PKGBUILD
	SkipHashCheck bool
}

// Compile manages all the instructions that lead to a single project artifact.
// It returns any error if occurred.
func (builder *Builder) Compile(ctx context.Context, noBuild bool) error {
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

	err = builder.getSources(ctx)
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
		return builder.runBuildStages(ctx)
	}

	return nil
}

// processFunction processes the given pkgbuildFunction with consistent logging and error handling.
//
// It takes three parameters: pkgbuildFunction string, message string, stage string.
// It returns an error if the build stage fails.
func (builder *Builder) processFunction(ctx context.Context, pkgbuildFunction, message, stage string) error {
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

	// Set up ccache and cross-compilation environment for the build stage.
	// Use the new slice-based methods (BuildCcacheEnvSlice, BuildCrossEnvSlice)
	// which do NOT call os.Setenv, making them safe for parallel builds.
	if stage == "build" { //nolint:nestif
		// Create a temporary BaseBuilder to access the environment slice methods
		format := constants.DistroFormat(builder.PKGBUILD.Distro)
		tempBuilder := &common.BaseBuilder{
			PKGBUILD: builder.PKGBUILD,
			Format:   format,
		}

		// Collect ccache env vars without mutating os.Setenv
		if ccacheEnv := tempBuilder.BuildCcacheEnvSlice(); len(ccacheEnv) > 0 {
			pkgEnv = append(pkgEnv, ccacheEnv...)

			logger.Info(i18n.T("logger.builder.info.ccache_enabled_for_build"),
				"package", pkgName)
		}

		// Collect cross-compilation env vars without mutating os.Setenv
		if builder.PKGBUILD.IsCrossCompilation() {
			crossEnv, err := tempBuilder.BuildCrossEnvSlice(builder.PKGBUILD.TargetArch)
			if err != nil {
				logger.Warn(i18n.T("logger.cross_compilation.cross_compilation_environment_setup_failed"),
					"package", pkgName, "target_arch", builder.PKGBUILD.TargetArch, "error", err)
			} else if len(crossEnv) > 0 {
				pkgEnv = append(pkgEnv, crossEnv...)
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
	err := shell.RunScriptWithPackage(ctx, scriptPrologue+preamble+pkgbuildFunction, pkgName, pkgEnv)
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
func (builder *Builder) runBuildStages(ctx context.Context) error {
	if err := builder.processFunction(ctx, builder.PKGBUILD.Prepare, "logger.preparing_sources", "prepare"); err != nil {
		return err
	}

	if err := builder.processFunction(ctx, builder.PKGBUILD.Build, "logger.building", "build"); err != nil {
		return err
	}

	if builder.PKGBUILD.IsSplitPackage() {
		return builder.compileSplitPackages(ctx)
	}

	return builder.processFunctionInFakeroot(ctx, builder.PKGBUILD.Package, "logger.generating_package", "package")
}

// compileSplitPackages runs the package_<name>() function for each sub-package
// defined in a split PKGBUILD (pkgname=('foo' 'bar' ...)). Each sub-package
// gets its own PackageDir so the install trees are kept separate.
func (builder *Builder) compileSplitPackages(ctx context.Context) error {
	pkgVer := builder.PKGBUILD.PkgVer
	pkgRel := builder.PKGBUILD.PkgRel

	for _, subName := range builder.PKGBUILD.PkgNames {
		// Look up package_<name>() from the dedicated split-package function map.
		funcBody, ok := builder.PKGBUILD.SplitPackageFuncs[subName]

		if !ok {
			// Fall back to the shared package() if no per-package function exists.
			if builder.PKGBUILD.Package == "" {
				logger.Warn(i18n.T("logger.builder.warn.no_package_function_found"), "subpackage", subName)

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

		// Reset to top-level values before parsing this sub-package's overrides,
		// preventing fields set by a previous sub-package from bleeding through.
		builder.PKGBUILD.RestoreTopLevelOverrides()

		// Statically parse the function body to extract per-package variable overrides
		// (pkgdesc, depends, conflicts, etc.) before running the script.
		// This uses the same AddItem/parseDirective path as top-level PKGBUILD parsing,
		// giving full __distro and _arch suffix support for free.
		if err := builder.PKGBUILD.ParseSplitOverrides(funcBody); err != nil {
			logger.Warn(i18n.T("logger.builder.warn.failed_parse_split_package"), "subpackage", subName, "error", err)
		}

		if err := shell.RunScriptInFakeroot(
			ctx, scriptPrologue+preamble+funcBody, subName, pkgEnv,
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
func (builder *Builder) processFunctionInFakeroot(ctx context.Context, pkgbuildFunction, message, stage string) error {
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
	// cross-compiled binaries. Use the slice-based method which does NOT call os.Setenv.
	if builder.PKGBUILD.IsCrossCompilation() {
		format := constants.DistroFormat(builder.PKGBUILD.Distro)
		tempBuilder := &common.BaseBuilder{
			PKGBUILD: builder.PKGBUILD,
			Format:   format,
		}

		crossEnv, err := tempBuilder.BuildCrossEnvSlice(builder.PKGBUILD.TargetArch)
		if err != nil {
			logger.Warn(i18n.T("logger.cross_compilation.cross_compilation_environment_setup_failed"),
				"package", pkgName, "target_arch", builder.PKGBUILD.TargetArch, "error", err)
		} else if len(crossEnv) > 0 {
			pkgEnv = append(pkgEnv, crossEnv...)
		}
	}

	preamble := builder.PKGBUILD.BuildScriptPreamble()

	err := shell.RunScriptInFakeroot(ctx, scriptPrologue+preamble+pkgbuildFunction, pkgName, pkgEnv)
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
func (builder *Builder) getSources(ctx context.Context) error {
	if len(builder.PKGBUILD.SourceURI) == 0 {
		return nil
	}

	pkgName := builder.PKGBUILD.PkgName
	pkgVer := builder.PKGBUILD.PkgVer
	pkgRel := builder.PKGBUILD.PkgRel

	// Concurrency: source fetch is network-bound; cap at NumCPU but never above 8.
	// First error cancels in-flight workers.
	maxWorkers := min(len(builder.PKGBUILD.SourceURI), max(2, min(runtime.NumCPU(), 8)))

	g, gctx := errgroup.WithContext(ctx)
	g.SetLimit(maxWorkers)

	for index, sourceURI := range builder.PKGBUILD.SourceURI {
		sourceObj := source.Source{
			StartDir:       builder.PKGBUILD.StartDir,
			Hash:           builder.PKGBUILD.HashSums[index],
			NoExtract:      builder.PKGBUILD.NoExtract,
			PkgName:        builder.PKGBUILD.PkgName,
			SourceItemURI:  sourceURI,
			SrcDir:         builder.PKGBUILD.SourceDir,
			SourceItemPath: "",
			SkipHashCheck:  builder.SkipHashCheck,
		}

		g.Go(func() error {
			if err := gctx.Err(); err != nil {
				return err
			}

			if err := sourceObj.Get(); err != nil {
				return errors.Wrap(err, errors.ErrTypeBuild,
					i18n.T("errors.build.failed_to_retrieve_source")).
					WithContext("package", pkgName).
					WithContext("version", pkgVer).
					WithContext("release", pkgRel).
					WithContext("source_index", index).
					WithContext("source_uri", sourceObj.SourceItemURI).
					WithOperation("source_processing")
			}

			return nil
		})
	}

	return g.Wait()
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
