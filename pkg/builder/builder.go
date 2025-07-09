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
		err := processFunction(builder.PKGBUILD.Prepare,
			"preparing sources")
		if err != nil {
			return err
		}

		err = processFunction(builder.PKGBUILD.Build,
			"building")
		if err != nil {
			return err
		}

		err = processFunction(builder.PKGBUILD.Package,
			"generating package")
		if err != nil {
			return err
		}
	}

	return nil
}

// processFunction processes the given pkgbuildFunction and message.
//
// It takes two parameters: pkgbuildFunction string, message string.
// It returns an error.
func processFunction(pkgbuildFunction, message string) error {
	if pkgbuildFunction == "" {
		return nil
	}

	osutils.Logger.Info(message)

	return osutils.RunScript("  set -e\n" + pkgbuildFunction)
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
