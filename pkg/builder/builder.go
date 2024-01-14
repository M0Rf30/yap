package builder

import (
	"fmt"

	"github.com/M0Rf30/yap/pkg/constants"
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

	fmt.Printf("%s🖧  :: %sRetrieving sources ...%s\n",
		string(constants.ColorBlue),
		string(constants.ColorYellow),
		string(constants.ColorWhite))

	if err := builder.getSources(); err != nil {
		return err
	}

	if !noBuild {
		fmt.Printf("%s🏗️  :: %sBuilding ...%s\n",
			string(constants.ColorBlue),
			string(constants.ColorYellow),
			string(constants.ColorWhite))

		if err := builder.processFunction(builder.PKGBUILD.Build); err != nil {
			return err
		}

		fmt.Printf("%s📦 :: %sGenerating package ...%s\n",
			string(constants.ColorBlue),
			string(constants.ColorYellow),
			string(constants.ColorWhite))

		if err := builder.processFunction(builder.PKGBUILD.Package); err != nil {
			return err
		}
	}

	return nil
}

// processFunction is a function that processes a pkgbuildFunction.
//
// It takes a pkgbuildFunction string as a parameter and runs it as a script.
// It returns an error if the script encounters any issues.
func (builder *Builder) processFunction(pkgbuildFunction string) error {
	err := RunScript("  set -e\n" + pkgbuildFunction)
	if err != nil {
		return err
	}

	return nil
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
	dirs := []string{
		builder.PKGBUILD.SourceDir,
		builder.PKGBUILD.PackageDir}

	for _, dir := range dirs {
		if err := utils.ExistsMakeDir(dir); err != nil {
			return err
		}
	}

	return nil
}
