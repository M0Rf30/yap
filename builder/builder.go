package builder

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/M0Rf30/yap/constants"
	"github.com/M0Rf30/yap/pack"
	"github.com/M0Rf30/yap/source"
	"github.com/M0Rf30/yap/utils"
)

const IDLength = 12

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

	err := runScript(b.Pack.Build)
	if err != nil {
		return err
	}

	return err
}

func (b *Builder) Package() error {
	path := filepath.Join(string(os.PathSeparator), "tmp",
		fmt.Sprintf("yap_%s_package", b.id))
	defer os.Remove(path)

	err := runScript(b.Pack.Package)
	if err != nil {
		return err
	}

	return err
}

func (b *Builder) Build() error {
	b.id = utils.RandStr(IDLength)

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
