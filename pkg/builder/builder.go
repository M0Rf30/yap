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

	fmt.Printf("%s🖧  :: %sRetrieving sources ...%s\n",
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

	return nil
}

// Package executes the instructions provided by a single project package()
// function. It returns any error if occurred.
func (builder *Builder) Package() error {
	err := RunScript(builder.PKGBUILD.Package)
	if err != nil {
		return err
	}

	return nil
}

// build executes a set of instructions provided by a project build function. If
// there is an error during execution, it returns the error.
func (builder *Builder) build() error {
	err := RunScript(builder.PKGBUILD.Build)
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
