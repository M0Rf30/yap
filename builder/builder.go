package builder

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/packagefoundation/yap/constants"
	"github.com/packagefoundation/yap/pack"
	"github.com/packagefoundation/yap/source"
	"github.com/packagefoundation/yap/utils"
)

const IDLenght = 12

type Builder struct {
	id   string
	Pack *pack.Pack
}

func (b *Builder) initDirs() error {
	err := utils.ExistsMakeDir(b.Pack.SourceDir)
	if err != nil {
		return err
	}

	err = utils.ExistsMakeDir(b.Pack.PackageDir)
	if err != nil {
		return err
	}

	return err
}

func (b *Builder) getSources() error {
	var err error

	for index, path := range b.Pack.Sources {
		source := source.Source{
			Root:   b.Pack.Root,
			Hash:   b.Pack.HashSums[index],
			Source: path,
			Output: b.Pack.SourceDir,
			Path:   "",
		}
		err = source.Get()

		if err != nil {
			return err
		}
	}

	return err
}

func (b *Builder) build() error {
	path := filepath.Join(string(os.PathSeparator), "tmp",
		fmt.Sprintf("yap_%s_build", b.id))
	defer os.Remove(path)

	err := createScript(path, b.Pack.Build)
	if err != nil {
		return err
	}

	err = runScript(path, b.Pack.SourceDir)
	if err != nil {
		return err
	}

	return err
}

func (b *Builder) Package() error {
	path := filepath.Join(string(os.PathSeparator), "tmp",
		fmt.Sprintf("yap_%s_package", b.id))
	defer os.Remove(path)

	err := createScript(path, b.Pack.Package)
	if err != nil {
		return err
	}

	err = runScript(path, b.Pack.SourceDir)
	if err != nil {
		return err
	}

	return err
}

func (b *Builder) Build() error {
	b.id = utils.RandStr(IDLenght)

	err := b.initDirs()
	if err != nil {
		return err
	}

	fmt.Printf("\t%s🖧  :: %sgetting sources ...%s\n",
		string(constants.ColorBlue),
		string(constants.ColorYellow),
		string(constants.ColorWhite))

	err = b.getSources()
	if err != nil {
		return err
	}

	fmt.Printf("\t%s🏗️  :: %sbuilding ...%s\n",
		string(constants.ColorBlue),
		string(constants.ColorYellow),
		string(constants.ColorWhite))

	err = b.build()

	if err != nil {
		return err
	}

	fmt.Printf("\t%s📦 :: %sgenerating package ...%s\n",
		string(constants.ColorBlue),
		string(constants.ColorYellow),
		string(constants.ColorWhite))

	err = b.Package()
	if err != nil {
		return err
	}

	return err
}
