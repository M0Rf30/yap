package builder

import (
	"github.com/M0Rf30/yap/pkg/osutils"
	"github.com/M0Rf30/yap/pkg/pkgbuild"
	"github.com/M0Rf30/yap/pkg/source"
)

// Builder maps PKGBUILD to generic functions aimed at artifacts generation.
type Builder struct {
	PKGBUILD *pkgbuild.PKGBUILD
}

// Compile manages all the instructions that lead to a single project artifact.
// It returns any error if occurred.
func (builder *Builder) Compile(noBuild bool) error {
	// Inject helper function definitions into scriptlet bodies so they are
	// available when the package is installed on the target system.
	builder.PKGBUILD.PrepareScriptlets()
	builder.PKGBUILD.FormatScriptlets()

	err := builder.initDirs()
	if err != nil {
		return err
	}

	osutils.Logger.Info("retrieving sources")

	err = builder.getSources()
	if err != nil {
		return err
	}

	if !noBuild {
		preamble := builder.PKGBUILD.BuildScriptPreamble()

		err := builder.processFunction(preamble, builder.PKGBUILD.Prepare,
			"preparing sources")
		if err != nil {
			return err
		}

		err = builder.processFunction(preamble, builder.PKGBUILD.Build,
			"building")
		if err != nil {
			return err
		}

		err = builder.processFunction(preamble, builder.PKGBUILD.Package,
			"generating package")
		if err != nil {
			return err
		}
	}

	return nil
}

// processFunction processes the given pkgbuildFunction and message.
//
// It takes three parameters: preamble string, pkgbuildFunction string, message string.
// The preamble contains custom array declarations and helper function definitions.
// It resets environment variables before each execution to ensure correct
// per-project values in multi-project builds.
// It returns an error.
func (builder *Builder) processFunction(preamble, pkgbuildFunction, message string) error {
	if pkgbuildFunction == "" {
		return nil
	}

	err := builder.PKGBUILD.SetEnvironmentVariables()
	if err != nil {
		return err
	}

	osutils.Logger.Info(message)

	return osutils.RunScript("  set -e\n" + preamble + pkgbuildFunction)
}

// getSources detects sources provided by a single project source array and
// downloads them if occurred. It returns any error if occurred.
func (builder *Builder) getSources() error {
	for index, sourceURI := range builder.PKGBUILD.SourceURI {
		sourceObj := source.Source{
			StartDir:       builder.PKGBUILD.StartDir,
			Hash:           builder.PKGBUILD.HashSums[index],
			SourceItemURI:  sourceURI,
			SrcDir:         builder.PKGBUILD.SourceDir,
			SourceItemPath: "",
		}

		err := sourceObj.Get()
		if err != nil {
			return err
		}
	}

	return nil
}

// initDirs creates mandatory fakeroot folders (src, pkg) for a single project.
// It returns any error if occurred.
func (builder *Builder) initDirs() error {
	return osutils.ExistsMakeDir(builder.PKGBUILD.SourceDir)
}
