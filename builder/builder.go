package builder

import (
	"fmt"

	"github.com/M0Rf30/yap/constants"
	"github.com/M0Rf30/yap/pkgbuild"
	"github.com/M0Rf30/yap/source"
	"github.com/M0Rf30/yap/utils"
)

type Builder struct {
	PKGBUILD *pkgbuild.PKGBUILD
}

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

func (builder *Builder) Package() error {
	err := RunScript(builder.PKGBUILD.Package)
	if err != nil {
		return err
	}

	return err
}

func (builder *Builder) build() error {
	err := RunScript(builder.PKGBUILD.Build)
	if err != nil {
		return err
	}

	return err
}

func (builder *Builder) getSources() error {
	var err error

	for index, sourceURI := range builder.PKGBUILD.SourceURI {
		source := source.Source{
			StartDir:       builder.PKGBUILD.StartDir,
			Hash:           builder.PKGBUILD.HashSums[index],
			SourceItemURI:  sourceURI,
			SrcDir:         builder.PKGBUILD.SourceDir,
			SourceItemPath: "",
		}
		err = source.Get()

		if err != nil {
			return err
		}
	}

	return err
}

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
