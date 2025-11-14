// Package builder provides build orchestration functionality for YAP packages.
package builder

import (
	"sync"

	"github.com/M0Rf30/yap/v2/pkg/builders/common"
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

	if !noBuild {
		err := builder.processFunction(builder.PKGBUILD.Prepare, "logger.preparing_sources", "prepare")
		if err != nil {
			return err
		}

		err = builder.processFunction(builder.PKGBUILD.Build, "logger.building", "build")
		if err != nil {
			return err
		}

		err = builder.processFunction(builder.PKGBUILD.Package, "logger.generating_package", "package")
		if err != nil {
			return err
		}
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

	// Set environment variables for this package before executing any function
	// This ensures each package uses its own pkgdir, srcdir, and startdir
	err := builder.PKGBUILD.SetEnvironmentVariables()
	if err != nil {
		errMsg := i18n.T("errors.pkgbuild.failed_to_set_environment_variables")

		return errors.Wrap(err, errors.ErrTypeBuild, errMsg).
			WithContext("package", pkgName).
			WithContext("stage", stage).
			WithOperation("SetEnvironmentVariables")
	}

	// Use logger for consistent formatting
	logger.Info(i18n.T(message), "pkgver", pkgVer, "pkgrel", pkgRel)

	// Set up ccache for the build stage if ccache is available
	if stage == "build" {
		// Create a temporary BaseBuilder to access the SetupCcache and
		// SetupCrossCompilationEnvironment methods
		format := determineFormatFromContext()
		tempBuilder := &common.BaseBuilder{
			PKGBUILD: builder.PKGBUILD,
			Format:   format,
		}

		err = tempBuilder.SetupCcache()
		if err != nil {
			logger.Warn(i18n.T("logger.setupccache.warn.ccache_setup_failed_1"),
				"package", pkgName, "error", err)
		}

		// Set up cross-compilation environment if target architecture is specified
		// Check if cross-compilation is needed
		if builder.PKGBUILD.TargetArch != "" &&
			builder.PKGBUILD.TargetArch != builder.PKGBUILD.ArchComputed {
			err = tempBuilder.SetupCrossCompilationEnvironment(builder.PKGBUILD.TargetArch)
			if err != nil {
				logger.Warn(i18n.T("logger.cross_compilation.cross_compilation_environment_setup_failed"),
					"package", pkgName, "target_arch", builder.PKGBUILD.TargetArch, "error", err)
			}
		}
	}

	// Execute script with package decoration
	err = shell.RunScriptWithPackage("  set -e\n"+pkgbuildFunction, pkgName)
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

// determineFormatFromContext determines the package format based on context
func determineFormatFromContext() string {
	// The format information should ideally be passed through the call chain
	// from the packer where it's known, but as a fallback we can try to infer it
	// This is a temporary workaround - the proper solution is to pass format information

	// Since we can't determine the format easily, return empty
	// and let the SetupCrossCompilationEnvironment handle the fallback logic
	return "" // Let the cross-compilation logic handle fallbacks
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
		waitGroup.Add(1)

		go func() {
			defer waitGroup.Done()

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
		}()
	}

	// Send tasks to workers
	go func() {
		defer close(sourceChan)

		for index, sourceURI := range builder.PKGBUILD.SourceURI {
			sourceObj := source.Source{
				StartDir:       builder.PKGBUILD.StartDir,
				Hash:           builder.PKGBUILD.HashSums[index],
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

	if err := platform.PreserveOwnership(builder.PKGBUILD.SourceDir); err != nil {
		logger.Warn(i18n.T("logger.failed_to_preserve_ownership"),
			"path", builder.PKGBUILD.SourceDir, "error", err)
	}

	return nil
}
