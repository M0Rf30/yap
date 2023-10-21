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
func (builder *Builder) Compile() error {
	err := builder.initDirs()
	if err != nil {
		return err
	}

	fmt.Printf("%s🖧  :: %sGetting sources ...%s\n",
		string(constants.ColorBlue),
		string(constants.ColorYellow),
		string(constants.ColorWhite))

	err = builder.getSources()
	if err != nil {
		return err
	}

	fmt.Printf("%s🏗️  :: %sBuilding ...%s\n",
		string(constants.ColorBlue),
		string(constants.ColorYellow),
		string(constants.ColorWhite))

	err = builder.build()
	if err != nil {
		return err
	}

	fmt.Printf("%s📦 :: %sGenerating package ...%s\n",
		string(constants.ColorBlue),
		string(constants.ColorYellow),
		string(constants.ColorWhite))

	err = builder.Package()
	if err != nil {
		return err
	}

	return err
}

// Package executes the instructions provided by a single project package()
// function. It returns any error if occurred.
func (builder *Builder) Package() error {
	err := RunScript(builder.PKGBUILD.Package)
	if err != nil {
		return err
	}

	return err
}

// build executes the instructions provided by a single project build()
// function. It returns any error if occurred.
func (builder *Builder) build() error {
	err := RunScript(builder.PKGBUILD.Build)
	if err != nil {
		return err
	}

	return err
}

// getSources detects sources provided by a single project source array and
// downloads them if occurred. It returns any error if occurred.
func (builder *Builder) getSources() error {
	var err error

	for index, sourceURI := range builder.PKGBUILD.SourceURI {
		sourceObj := source.Source{
			StartDir:       builder.PKGBUILD.StartDir,
			Hash:           builder.PKGBUILD.HashSums[index],
			SourceItemURI:  sourceURI,
			SrcDir:         builder.PKGBUILD.SourceDir,
			SourceItemPath: "",
		}

		err = sourceObj.Get()

		if err != nil {
			return err
		}
	}

	return err
}

// initDirs creates mandatory fakeroot folders (src, pkg) for a single project.
// It returns any error if occurred.
func (builder *Builder) initDirs() error {
	err := utils.ExistsMakeDir(builder.PKGBUILD.SourceDir)
	if err != nil {
		return err
	}

	err = utils.ExistsMakeDir(builder.PKGBUILD.PackageDir)
	if err != nil {
		return err
	}

	return err
}
