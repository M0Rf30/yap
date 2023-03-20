package builder

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/M0Rf30/yap/constants"
	"github.com/M0Rf30/yap/pkgbuild"
	"github.com/M0Rf30/yap/source"
	"github.com/M0Rf30/yap/utils"
)

const IDLength = 12

type Builder struct {
	id       string
	PKGBUILD *pkgbuild.PKGBUILD
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

func (builder *Builder) getSources() error {
	var err error

	for index, path := range builder.PKGBUILD.Source {
		source := source.Source{
			Root:   builder.PKGBUILD.Root,
			Hash:   builder.PKGBUILD.HashSums[index],
			Source: path,
			Output: builder.PKGBUILD.SourceDir,
			Path:   "",
		}
		err = source.Get()

		if err != nil {
			return err
		}
	}

	return err
}

func (builder *Builder) build() error {
	path := filepath.Join(string(os.PathSeparator), "tmp",
		fmt.Sprintf("yap_%s_build", builder.id))
	defer os.Remove(path)

	err := runScript(builder.PKGBUILD.Build)
	if err != nil {
		return err
	}

	return err
}

func (builder *Builder) Package() error {
	path := filepath.Join(string(os.PathSeparator), "tmp",
		fmt.Sprintf("yap_%s_package", builder.id))
	defer os.Remove(path)

	err := runScript(builder.PKGBUILD.Package)
	if err != nil {
		return err
	}

	return err
}

func (builder *Builder) Build() error {
	builder.id = utils.GenerateRandomString(IDLength)

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

	fmt.Printf("%s🏗️ :: %sBuilding ...%s\n",
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
