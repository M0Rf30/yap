package builder

import (
	"github.com/M0Rf30/yap/pkg/pkgbuild"
	"github.com/M0Rf30/yap/pkg/source"
	"github.com/M0Rf30/yap/pkg/utils"
)

// Builder maps PKGBUILD to generic functions aimed at artifacts generation.
type Builder struct {
	PKGBUILD *pkgbuild.PKGBUILD
}

// Compile manages all the instructions that lead to a single project artifact.
// It returns any error if occurred.
func (builder *Builder) Compile(noBuild bool) error {
	if err := builder.initDirs(); err != nil {
		return err
	}

	utils.Logger.Info("retrieving sources")

	if err := builder.getSources(); err != nil {
		return err
	}

	if !noBuild {
		if err := processFunction(builder.PKGBUILD.Prepare,
			"preparing sources"); err != nil {
			return err
		}

		if err := processFunction(builder.PKGBUILD.Build,
			"building"); err != nil {
			return err
		}

		if err := processFunction(builder.PKGBUILD.Package,
			"generating package"); err != nil {
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

	utils.Logger.Info(message)

	return utils.RunScript("  set -e\n" + pkgbuildFunction)
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

		if err := sourceObj.Get(); err != nil {
			return err
		}
	}

	return nil
}

// initDirs creates mandatory fakeroot folders (src, pkg) for a single project.
// It returns any error if occurred.
func (builder *Builder) initDirs() error {
	return utils.ExistsMakeDir(builder.PKGBUILD.SourceDir)
}
